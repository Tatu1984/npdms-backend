#!/usr/bin/env python3
"""Call every GET endpoint the API registers and report what it returns.

The platform has a recurring defect: a query naming a column or a table that
does not exist. It compiles, it routes, and it returns 500 on every call —
but nothing ever called it, so nobody noticed. This sweep calls all of them.

It reads the route list from docs/api/routes.txt (generated from the running
server), signs in once as a single administrator, and walks every GET route.
Routes with a path parameter get a real identifier: the sweep first calls the
collection the detail route hangs off, and takes the first id out of the
response. A detail endpoint that fails on a real id is exactly what this is
looking for, so a made-up UUID would be worse than useless.

    python3 probe-endpoints.py [base_url] [routes_file]

Default base url http://localhost:18080/api/v1. Exits non-zero if any
endpoint returns a 5xx.
"""

import collections
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18080/api/v1").rstrip("/")
ROUTES_FILE = sys.argv[2] if len(sys.argv) > 2 else os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "..", "docs", "api", "routes.txt"
)
USERNAME = os.environ.get("PROBE_USER", "admin")
PASSWORD = os.environ.get("PROBE_PASSWORD", "Demo@123")
TIMEOUT = 30
# The API allows 100 requests a minute per address. Stay under it, or the
# sweep reports the rate limiter's refusals as if they were the endpoints'.
BUDGET = int(os.environ.get("PROBE_BUDGET", "90"))
WINDOW = 60.0

# Path parameters that are not record identifiers. Each is a value the live
# demo database is expected to hold, or a shape the endpoint parses.
LITERALS = {
    "type": "daily_crime_summary",
    "format": "json",
    "ip": "8.8.8.8",
    "version": "1",
    "reportNumber": "/UNKNOWN-REPORT",
}

# Collections that do not sit at the parent path of their detail route, or
# whose detail route parameter is not the collection's own id.
COLLECTION_FOR = {
    "/ai-review/decisions/:id": "/ai-review/queue",
    "/ai-review/models/:modelName": "/ai-review/models",
    "/biometric/verifications/:subjectId/history": "/personnel",
    "/case-files/by-workspace/:workspaceId": "/investigation",
    "/knowledge/runs/:id": None,
    "/malkhana/items/by-number/:number": "/malkhana/items",
    "/state/code/:code": "/state",
    "/traffic/challans/number/:number": "/traffic/challans",
    "/traffic/vehicle/:vehicleNumber": "/traffic/challans",
}

# Field to read off a collection record when the parameter is not an id.
FIELD_FOR = {
    "modelName": ("modelName", "model_name", "name"),
    "number": ("number", "propertyNumber", "itemNumber", "challanNumber", "trackingNumber"),
    "code": ("code", "stateCode", "state_code"),
    "vehicleNumber": ("vehicleNumber", "vehicle_number", "registrationNumber", "registration_number"),
    "subjectId": ("id", "userId", "user_id"),
    "workspaceId": ("id",),
}

# Query parameters an endpoint refuses to work without. A plain string is
# sent as is; a (route, field) pair is looked up on the first record the
# route returns, so the sweep asks about a district that exists. Without
# these the endpoint answers 400 and its own query is never run.
QUERY_FOR = {
    "/district/coordination/requests": {"districtId": ("/district", "id")},
    "/district/meetings": {"districtId": ("/district", "id")},
    "/district/resources": {"districtId": ("/district", "id")},
    "/district/stations": {"districtId": ("/district", "id")},
    "/gazetteer/nearest": {"lat": "22.5726", "lng": "88.3639"},
    "/legal/correspondence": {"ipc": "302"},
    "/risk/firs": {"stationId": ("/stations", "id")},
    "/search": {"q": "Kolkata"},
    "/state/alerts": {"stateId": ("/state", "id")},
    "/state/coordination/requests": {"stateId": ("/state", "id")},
    "/state/ranges": {"zoneId": ("/state/zones", "id")},
    "/state/zones": {"stateId": ("/state", "id")},
}

results = []          # (route, status, note)
cache = {}            # concrete path -> (status, parsed body)
resolved = {}         # route template -> (path, reason)
token = None


sent = collections.deque()


def throttle():
    now = time.monotonic()
    while sent and now - sent[0] > WINDOW:
        sent.popleft()
    if len(sent) >= BUDGET:
        time.sleep(max(0.0, WINDOW - (now - sent[0])) + 0.2)
        throttle()
        return
    sent.append(time.monotonic())


def call(method, path, body=None, attempt=0):
    """Call the API. Returns (status, parsed body)."""
    throttle()
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            payload = resp.read()
            status = resp.status
    except urllib.error.HTTPError as e:
        payload = e.read()
        status = e.code
    except Exception as e:                       # timeout, connection reset
        return 0, {"error": str(e)}
    if status == 429 and attempt < 3:
        # The limiter, not the endpoint. Let the window roll over and retry.
        sent.clear()
        time.sleep(WINDOW / 2)
        return call(method, path, body, attempt + 1)
    text = payload.decode("utf-8", "replace")
    try:
        return status, json.loads(text) if text else {}
    except json.JSONDecodeError:
        return status, {"raw": text[:400]}


def login():
    status, body = call("POST", "/auth/login", body={"username": USERNAME, "password": PASSWORD})
    if status != 200 or "accessToken" not in body:
        print(f"cannot sign in as {USERNAME}: {status} {body}")
        print("login is rate limited to 5 a minute per address — wait a minute and retry")
        sys.exit(2)
    user = body["user"]
    force = (user.get("force") or {}).get("shortName", "-")
    print(f"signed in as {USERNAME}: {user.get('name')} ({user.get('role')}, {force})\n")
    return body["accessToken"]


def read_routes():
    routes = []
    with open(ROUTES_FILE) as fh:
        for line in fh:
            line = line.strip()
            if not line.startswith("GET "):
                continue
            path = line[4:].strip()
            if path.startswith("/api/v1"):
                routes.append(path[len("/api/v1"):])
            else:
                routes.append(None if path in ("/health", "/ready", "/openapi.yaml") else path)
    return [r for r in routes if r]


def params_of(path):
    return re.findall(r"[:*](\w+)", path)


def first_record(body):
    """Pull the first record out of a list response, whatever it is wrapped in."""
    if isinstance(body, list):
        return body[0] if body and isinstance(body[0], dict) else None
    if not isinstance(body, dict):
        return None
    # Prefer the conventional wrappers, then any list of objects.
    keys = [k for k in ("data", "items", "results", "records") if k in body]
    keys += [k for k in body if k not in keys]
    for key in keys:
        value = body[key]
        if isinstance(value, list) and value and isinstance(value[0], dict):
            return value[0]
        if isinstance(value, dict):
            found = first_record(value)
            if found is not None:
                return found
    return None


def field(record, names):
    for name in names:
        if record.get(name) not in (None, ""):
            return str(record[name])
    return None


def collection_get(path):
    """GET a collection, caching the body, so a list is fetched at most once."""
    if path not in cache:
        cache[path] = call("GET", path)
    return cache[path]


def value_from(source_route, name, seen):
    """Read one field off the first record the given route returns."""
    source, reason = resolve(source_route, seen)
    if source is None:
        return None, f"{source_route}: {reason}"
    status, body = collection_get(source)
    if status != 200:
        return None, f"{source_route} answered {status or 'nothing'}"
    record = first_record(body)
    if record is None:
        return None, f"{source_route} holds no records"
    value = field(record, FIELD_FOR.get(name, ("id", "ID", "uuid")))
    if value is None:
        return None, f"no {name} on a record from {source_route}"
    return value, None


def resolve(route, seen=()):
    """Turn a route template into a callable path, or return (None, reason)."""
    if route in resolved:
        return resolved[route]
    if route in seen:
        return None, "its own identifier comes from itself"
    seen = seen + (route,)

    path = route
    for name in params_of(route):
        placeholder = ":" + name if ":" + name in path else "*" + name
        if name in LITERALS:
            path = path.replace(placeholder, LITERALS[name], 1)
            continue
        # The collection this parameter identifies is everything before it,
        # with any earlier parameters already substituted.
        if route in COLLECTION_FOR:
            collection = COLLECTION_FOR[route]
            if collection is None:
                resolved[route] = (None, "no list endpoint to take an id from")
                return resolved[route]
        else:
            collection = path[: path.index(placeholder)].rstrip("/")
            # A parent still holding a placeholder is a template; substituted
            # ones are already concrete and must not be resolved again.
            if params_of(collection):
                collection = route[: route.index(placeholder)].rstrip("/")
        value, reason = value_from(collection, name, seen)
        if value is None:
            resolved[route] = (None, reason)
            return resolved[route]
        path = path.replace(placeholder, urllib.parse.quote(value, safe=""), 1)

    query = {}
    for name, source in QUERY_FOR.get(route, {}).items():
        if isinstance(source, str):
            query[name] = source
            continue
        value, reason = value_from(source[0], source[1], seen)
        if value is None:
            resolved[route] = (None, reason)
            return resolved[route]
        query[name] = value
    if query:
        path += "?" + urllib.parse.urlencode(query)

    resolved[route] = (path, None)
    return resolved[route]


def probe(route):
    path, reason = resolve(route)
    if path is None:
        results.append((route, None, reason))
        print(f"  skip  {route:<62} {reason}")
        return
    status, body = collection_get(path)
    note = ""
    if status >= 500 or status == 0:
        note = json.dumps(body)[:200] if isinstance(body, (dict, list)) else str(body)[:200]
    results.append((route, status, note))
    if 200 <= status < 300:
        mark = "ok  "
    elif status == 503:
        mark = "down"
    elif status >= 500 or status == 0:
        mark = "FAIL"
    else:
        mark = "----"
    shown = route if path == route else f"{route}  [{path}]"
    print(f"  {mark}  {shown:<62} {status if status else 'no answer'} {note}")


def main():
    global token
    routes = read_routes()
    print(f"Probing {BASE} — {len(routes)} GET routes from {os.path.normpath(ROUTES_FILE)}\n")
    token = login()

    # Collections first: their bodies supply the identifiers for the rest.
    plain = [r for r in routes if not params_of(r)]
    parametrised = [r for r in routes if params_of(r)]

    print("Routes with no path parameter")
    for route in sorted(plain):
        probe(route)

    print("\nRoutes with a path parameter")
    for route in sorted(parametrised):
        probe(route)

    # A 503 is an endpoint saying a service it depends on is not answering.
    # That is the endpoint working, so it is counted apart from the crashes.
    unavailable = [r for r in results if r[1] == 503]
    broken = [r for r in results if r[1] is not None and r[1] != 503 and (r[1] >= 500 or r[1] == 0)]
    refused = [r for r in results if r[1] is not None and 400 <= r[1] < 500]
    skipped = [r for r in results if r[1] is None]
    served = [r for r in results if r[1] is not None and 200 <= r[1] < 400]

    print(f"\n{len(results)} routes called: {len(served)} answered, "
          f"{len(refused)} refused (4xx), {len(unavailable)} dependency down (503), "
          f"{len(skipped)} skipped, {len(broken)} broken (5xx)")
    if refused:
        print("\nRefused — a 4xx is the endpoint working, but check the ones you did not expect:")
        for route, status, _ in sorted(refused):
            print(f"  {status}  {route}")
    if unavailable:
        print("\nDependency down — the endpoint answered, the service behind it did not:")
        for route, _, note in sorted(unavailable):
            print(f"  {route:<62} {note}")
    if skipped:
        print("\nSkipped — no identifier to call them with:")
        for route, _, reason in sorted(skipped):
            print(f"  {route:<62} {reason}")
    if broken:
        print("\nBroken:")
        for route, status, note in sorted(broken):
            print(f"  {status if status else 'no answer'}  {route}  {note}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
