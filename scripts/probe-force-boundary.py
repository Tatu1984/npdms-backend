#!/usr/bin/env python3
"""Check that every scoped register really stops at the force boundary.

Signs in as a Kolkata Police station officer and a CID officer and reads every
register through the API as each of them. For a scoped register the two must
not see the same records; for the four state-wide registers they must.

The failure this catches is the one the platform actually had: a register
listed on screen, advertised as bounded, and returning identical counts to both
departments — the boundary enforced on five registers and nowhere else.

Two things are checked per register:

  * no record id appears in both departments' listings — the direct proof;
  * the two totals are not equal and non-zero — the symptom that was visible
    on screen, kept because a register can leak without the id lists being
    fetchable (a count endpoint, a page size cap).

    python3 probe-force-boundary.py [base_url]
"""

import json
import sys
import time
import urllib.error
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18085/api/v1"
PASSWORD = "Demo@123"
WINDOW = 62  # the API's rate-limit window, plus a second

ok, fail, skipped = [], [], []


def check(name, condition, detail=""):
    (ok if condition else fail).append(name)
    print(("  ok   " if condition else "  FAIL ") + name + ("" if condition else " — " + str(detail)))


def call(method, path, token=None, body=None):
    # The API allows a hundred requests a minute from one address, and this
    # probe reads sixty registers. A 429 is the server working as intended, so
    # the probe waits the window out rather than reporting a register it never
    # managed to read.
    for attempt in range(4):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if token:
            req.add_header("Authorization", "Bearer " + token)
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                raw = r.read().decode()
                return r.status, (json.loads(raw) if raw else {})
        except urllib.error.HTTPError as e:
            raw = e.read().decode()
            try:
                parsed = json.loads(raw)
            except json.JSONDecodeError:
                parsed = {"raw": raw[:300]}
            if e.code == 429 and attempt < 3:
                print(f"       (rate limited; waiting {WINDOW}s before retrying {path})")
                time.sleep(WINDOW)
                continue
            return e.code, parsed
        except urllib.error.URLError as e:
            return 0, {"raw": str(e)}


def login(username):
    status, body = call("POST", "/auth/login", body={"username": username, "password": PASSWORD})
    if status != 200:
        print(f"cannot sign in as {username}: {status} {body}")
        sys.exit(1)
    force = (body["user"].get("force") or {}).get("shortName", "?")
    print(f"  signed in: {username:<10} {body['user']['name']:<26} {force}")
    return body["accessToken"]


def rows(payload):
    """Pull the record list out of whatever shape an endpoint answers with."""
    if isinstance(payload, list):
        return payload
    for key in ("data", "items", "results", "beats", "cameras", "devices", "locations"):
        value = payload.get(key)
        if isinstance(value, list):
            return value
    return []


def read(path, token):
    """Return (ids, total, detail) for one register as one officer."""
    status, body = call("GET", path, token)
    if status != 200:
        return None, None, f"HTTP {status} {str(body)[:120]}"
    listed = rows(body)
    ids = {r.get("id") for r in listed if isinstance(r, dict) and r.get("id")}
    total = body.get("total") if isinstance(body, dict) else None
    if total is None:
        total = len(listed)
    return ids, total, ""


# Every register this probe knows about, and whether it is bounded by force or
# state-wide. The state-wide four are read from shared_registers by the server;
# they are here as a control, because a probe that only ever expects separation
# would pass just as happily against a platform that showed nobody anything.
SCOPED = [
    ("FIRs", "/firs?pageSize=500"),
    ("cases", "/cases?pageSize=500"),
    ("complaints", "/complaints?pageSize=500"),
    ("cyber-crime register", "/cyber-crime?pageSize=500"),
    ("evidence", "/evidence?pageSize=500"),
    ("evidence (custody register)", "/custody?pageSize=500"),
    ("warrants", "/warrants?pageSize=500"),
    ("bail applications", "/bail?pageSize=500"),
    ("forensic requests", "/forensics?pageSize=500"),
    ("personnel", "/personnel?pageSize=500"),
    ("fleet", "/vehicles?pageSize=500"),
    ("armoury", "/armoury/weapons?pageSize=500"),
    ("armoury issue ledger", "/armoury/issuances?pageSize=500"),
    ("malkhana items", "/malkhana/items?pageSize=500"),
    ("malkhana storage locations", "/malkhana/locations"),
    ("court hearings", "/court/hearings?pageSize=500"),
    ("court orders", "/court/orders?pageSize=500"),
    ("investigation workspaces", "/investigation?pageSize=500"),
    ("court-readiness files", "/case-files?pageSize=500"),
    ("dispatch incidents", "/dispatch/incidents?pageSize=500"),
    ("traffic incidents", "/traffic-incidents?pageSize=500"),
    ("traffic challans", "/traffic/challans?pageSize=500"),
    ("CCTV cameras", "/video/cameras?pageSize=500"),
    ("body-worn cameras", "/bodycam/devices?pageSize=500"),
    ("AI review queue", "/ai-review/queue?pageSize=500"),
]

STATE_WIDE = [
    ("missing persons", "/missing-persons?pageSize=500"),
    ("lookouts", "/lookouts?pageSize=500"),
    ("alerts", "/alerts?pageSize=500"),
    ("ANPR watchlist", "/anpr/watchlist?pageSize=500"),
]


def main():
    print(f"Probing {BASE}\n")
    print("Who is who")
    kp = login("sho")       # Kolkata Police, a station officer
    cid = login("sp.cid")   # CID, a wing of West Bengal Police

    # A record that has been referred and accepted is meant to be in both
    # registers: that is the whole point of a referral. Those ids are collected
    # first so that an overlap the referral explains is not reported as a leak.
    referred = set()
    for token in (kp, cid):
        status, body = call("GET", "/referrals?pageSize=500", token)
        for r in rows(body) if status == 200 else []:
            if r.get("status") == "ACCEPTED" and r.get("recordId"):
                referred.add(r["recordId"])
    if referred:
        print(f"\n  {len(referred)} record(s) have been referred and accepted; "
              "those are meant to be in both registers")

    print(f"\n{'register':<32}{'KP':>6}{'CID':>7}   verdict")
    print("-" * 72)

    for name, path in SCOPED:
        kp_ids, kp_total, kp_err = read(path, kp)
        cid_ids, cid_total, cid_err = read(path, cid)
        if kp_ids is None or cid_ids is None:
            skipped.append(name)
            print(f"{name:<32}{'-':>6}{'-':>7}   not read: {kp_err or cid_err}")
            continue

        overlap = (kp_ids & cid_ids) - referred
        # Equal totals are only suspicious when neither side is empty and the
        # referrals cannot account for the difference.
        same_count = kp_total == cid_total and kp_total > 0
        verdict = "separate"
        if overlap:
            verdict = f"LEAK: {len(overlap)} shared record(s)"
        elif same_count:
            verdict = "LEAK: identical non-zero counts"
        print(f"{name:<32}{kp_total:>6}{cid_total:>7}   {verdict}")

        check(f"{name}: no record is in both departments' registers", not overlap,
              f"{len(overlap)} shared: {sorted(overlap)[:3]}")
        check(f"{name}: the two departments do not report the same total", not same_count,
              f"both report {kp_total}")

    print("\nThe state-wide registers, which every force is meant to see")
    print("-" * 72)
    for name, path in STATE_WIDE:
        kp_ids, kp_total, kp_err = read(path, kp)
        cid_ids, cid_total, cid_err = read(path, cid)
        if kp_ids is None or cid_ids is None:
            skipped.append(name)
            print(f"{name:<32}{'-':>6}{'-':>7}   not read: {kp_err or cid_err}")
            continue
        print(f"{name:<32}{kp_total:>6}{cid_total:>7}   shared")
        if kp_total == 0 and cid_total == 0:
            skipped.append(name + " (empty on this database)")
            continue
        check(f"{name}: both departments see the same register", kp_ids == cid_ids,
              f"KP {kp_total}, CID {cid_total}")

    print("\nA detail read of another department's record says whose it is")
    print("-" * 72)
    _, kp_firs, _ = None, None, None
    status, body = call("GET", "/firs?pageSize=1", kp)
    fir = (rows(body) or [None])[0]
    if fir:
        status, body = call("GET", "/firs/" + fir["id"], cid)
        check("CID is refused a Kolkata Police FIR by name",
              status == 403 and body.get("error") == "other_force",
              f"{status} {body.get('message')}")
    status, body = call("GET", "/evidence?pageSize=1", kp)
    exhibit = (rows(body) or [None])[0]
    if exhibit:
        status, body = call("GET", "/evidence/" + exhibit["id"], cid)
        check("CID is refused a Kolkata Police exhibit by name",
              status == 403 and body.get("error") == "other_force",
              f"{status} {body.get('message')}")
    status, body = call("GET", "/armoury/weapons?pageSize=1", kp)
    weapon = (rows(body) or [None])[0]
    if weapon:
        status, body = call("GET", "/armoury/weapons/" + weapon["id"], cid)
        check("CID is refused a Kolkata Police weapon by name",
              status == 403 and body.get("error") == "other_force",
              f"{status} {body.get('message')}")

    # Said plainly rather than left for the reader to work out: on the demo
    # database every record belongs to Kolkata Police, so what this probe shows
    # is that CID has stopped seeing them. That one department's register is
    # empty is not proof the other direction holds — the repository tests in
    # internal/repository/force_boundary_registers_test.go write a row on each
    # side of the boundary and check both ways round.
    print("\nNote: on a database where one department holds every record, this probe")
    print("shows only that the other department has stopped seeing them. Both")
    print("directions are covered by TestEveryScopedRegisterStopsAtTheForceBoundary.")

    print(f"\n{len(ok)} passed, {len(fail)} failed, {len(skipped)} not exercised")
    for name in skipped:
        print("  not exercised: " + name)
    for name in fail:
        print("  FAILED " + name)
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
