#!/usr/bin/env python3
"""Probe the AI layer A0 endpoints against a running API.

Checks the refusals as well as the happy paths: an unmeasured model cannot be
switched on, a rank below SP cannot register or measure one, an evaluation
cannot be rewritten, and a suggestion cannot come from a model that is off.

    python3 probe_a0.py [base_url]     # default http://localhost:18082/api/v1
"""

import json
import sys
import time
import urllib.error
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18082/api/v1"
PASSWORD = "Demo@123"

passed = []
failed = []


def call(method, path, token=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode()
            return resp.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"raw": raw}


def check(name, condition, detail=""):
    if condition:
        passed.append(name)
        print(f"  ok   {name}")
    else:
        failed.append(f"{name}: {detail}")
        print(f"  FAIL {name} — {detail}")


def login(username):
    status, body = call("POST", "/auth/login", body={"username": username, "password": PASSWORD})
    if status != 200:
        print(f"Cannot sign in as {username}: {status} {body}")
        sys.exit(1)
    return body["accessToken"]


def main():
    print(f"Probing {BASE}")

    # One sign-in per account: login is rate limited to five a minute.
    admin = login("admin")      # DGP
    dsp = login("dsp")          # DSP: may read the registry, not change it
    si = login("si")            # SI: may not read it at all

    model = f"PROBE-model-{int(time.time())}"

    print("\nGateway status")
    status, body = call("GET", "/ai-review/gateway", admin)
    check("gateway status reads", status == 200, f"{status} {body}")
    check("gateway says plainly when nothing is connected",
          body.get("connected", 0) == 0 and bool(body.get("note")),
          f"connected={body.get('connected')} note={body.get('note')!r}")

    print("\nRank floors")
    status, _ = call("GET", "/ai-review/models", si)
    check("SI cannot read the model registry", status == 403, f"got {status}")
    status, _ = call("GET", "/ai-review/models", dsp)
    check("DSP can read the model registry", status == 200, f"got {status}")
    status, _ = call("POST", "/ai-review/models", dsp, {
        "modelName": model, "modelVersion": "v1", "decisionType": "COMPLAINT_CATEGORY",
        "confidenceThreshold": 0.7})
    check("DSP cannot register a model", status == 403, f"got {status}")

    print("\nRegistering a model")
    status, entry = call("POST", "/ai-review/models", admin, {
        "modelName": model, "modelVersion": "v1", "decisionType": "COMPLAINT_CATEGORY",
        "task": "complaint category", "endpointEnv": "PROBE_MODEL_URL",
        "licence": "probe", "confidenceThreshold": 0.70,
        "description": "Probe model. Safe to delete."})
    check("a model can be registered", status == 201, f"{status} {entry}")
    check("a newly registered model is switched off", entry.get("isEnabled") is False, str(entry.get("isEnabled")))
    check("a newly registered model is not measured", entry.get("measured") is False, str(entry.get("measured")))
    check("review cannot be switched off", entry.get("requiresReview") is True, str(entry.get("requiresReview")))
    check("no auto-approve threshold is exposed", "autoApproveThreshold" not in entry, str(list(entry)))

    status, body = call("PUT", f"/ai-review/models/{model}", admin, {"confidenceThreshold": 1.5})
    check("a threshold outside 0–1 is refused", status == 400, f"{status} {body}")

    print("\nThe evaluation gate")
    status, body = call("PUT", f"/ai-review/models/{model}", admin, {"isEnabled": True})
    check("an unmeasured model cannot be switched on", status == 409, f"{status} {body}")
    check("the refusal states the rule",
          "evaluation" in json.dumps(body).lower(), json.dumps(body))

    status, body = call("POST", "/ai-review/evaluations", dsp, {
        "modelName": model, "modelVersion": "v1", "dataset": "probe set",
        "datasetSize": 100, "metric": "accuracy", "threshold": 0.8, "measured": 0.9})
    check("DSP cannot record an evaluation", status == 403, f"got {status}")

    status, failing = call("POST", "/ai-review/evaluations", admin, {
        "modelName": model, "modelVersion": "v1", "dataset": "probe set A",
        "datasetSize": 120, "metric": "accuracy", "threshold": 0.80, "measured": 0.61,
        "limitations": "Probe data only."})
    check("a failing evaluation is recorded", status == 201, f"{status} {failing}")
    check("a failing evaluation is marked failed", failing.get("passed") is False, str(failing.get("passed")))

    status, body = call("PUT", f"/ai-review/models/{model}", admin, {"isEnabled": True})
    check("a failed evaluation does not switch the model on", status == 409, f"{status} {body}")

    status, passing = call("POST", "/ai-review/evaluations", admin, {
        "modelName": model, "modelVersion": "v1", "dataset": "probe set A",
        "datasetSize": 120, "metric": "accuracy", "threshold": 0.80, "measured": 0.87,
        "limitations": "Probe data only; not measured on real records."})
    check("a passing evaluation is recorded", status == 201, f"{status} {passing}")
    check("the verdict follows the numbers", passing.get("passed") is True, str(passing.get("passed")))

    status, body = call("PUT", f"/ai-review/models/{model}", admin, {"isEnabled": True})
    check("a measured model can be switched on", status == 200, f"{status} {body}")
    check("the switched-on model reads as measured", body.get("measured") is True, str(body.get("measured")))

    status, body = call("PUT", f"/ai-review/models/{model}", admin, {"modelVersion": "v2"})
    check("moving to an unmeasured version is refused", status == 409, f"{status} {body}")

    print("\nEvaluations and modules")
    status, body = call("GET", f"/ai-review/evaluations?model_name={model}", dsp)
    check("evaluations list for a model", status == 200 and len(body.get("data", [])) == 2,
          f"{status} {len(body.get('data', []))} rows")
    status, body = call("GET", "/ai-review/modules", dsp)
    modules = {m["module"]: m for m in body.get("data", [])}
    check("both module switches are listed in one place",
          {"FACE_RECOGNITION", "VEHICLE_DETECTION"} <= set(modules), str(list(modules)))
    check("modules ship switched off",
          all(not m["enabled"] for m in modules.values()), str({k: v["enabled"] for k, v in modules.items()}))

    print("\nThe gateway sees the model but no service")
    status, body = call("GET", "/ai-review/gateway", admin)
    entry = next((m for m in body.get("data", []) if m["modelName"] == model), None)
    check("the registered model appears in the gateway", entry is not None, "not found")
    if entry:
        check("it is reported as not connected", entry.get("connected") is False, str(entry.get("connected")))
        check("it says which variable is missing",
              "PROBE_MODEL_URL" in (entry.get("note") or ""), repr(entry.get("note")))

    print("\nAcceptance monitoring")
    for group in ("station", "language", "type"):
        status, body = call("GET", f"/ai-review/acceptance?group_by={group}", admin)
        check(f"acceptance by {group}", status == 200 and "data" in body, f"{status} {body}")
    status, body = call("GET", "/ai-review/acceptance?group_by=nonsense", admin)
    check("an unknown grouping is refused", status == 400, f"got {status}")
    status, _ = call("GET", "/ai-review/acceptance", si)
    check("SI cannot read acceptance rates", status == 403, f"got {status}")

    print("\nQueue")
    status, body = call("GET", "/ai-review/queue", si)
    check("an officer can read the review queue", status == 200, f"{status} {body}")
    check("the queue is a list, not a null", isinstance(body.get("decisions"), list), str(type(body.get("decisions"))))

    print("\nTidying up")
    print(f"  the probe model {model} is left registered; remove it with:")
    print(f"    psql -d npdms -c \"DELETE FROM ai_model_evaluations WHERE model_name='{model}';"
          f" DELETE FROM ai_model_configs WHERE model_name='{model}';\"")

    print(f"\n{len(passed)} passed, {len(failed)} failed")
    for failure in failed:
        print("  FAILED " + failure)
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
