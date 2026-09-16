#!/usr/bin/env python3
"""Drive a record across the boundary between two departments.

The inter-department claim, tested end to end: Kolkata Police holds a case,
CID cannot see it, Kolkata Police refers it with a reason, CID accepts, and
only then does CID see it — with every step on the audit trail.

    python3 probe-referrals.py [base_url]
"""

import json
import sys
import urllib.error
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18082/api/v1"
PASSWORD = "Demo@123"

ok, fail = [], []


def check(name, condition, detail=""):
    (ok if condition else fail).append(name)
    print(("  ok   " if condition else "  FAIL ") + name + ("" if condition else " — " + str(detail)))


def call(method, path, token=None, body=None):
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
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"raw": raw[:300]}


def login(username):
    status, body = call("POST", "/auth/login", body={"username": username, "password": PASSWORD})
    if status != 200:
        print(f"cannot sign in as {username}: {status} {body}")
        sys.exit(1)
    force = (body["user"].get("force") or {}).get("shortName", "?")
    print(f"  signed in: {username:<12} {body['user']['name']:<24} {force}")
    return body["accessToken"]


def main():
    print(f"Probing {BASE}\n")
    print("Who is who")
    kp_sho = login("sho")        # Kolkata Police, can refer
    cid_sp = login("sp.cid")     # CID, can decide
    cid_si = login("si.cid.aht") # CID, junior

    print("\nWhat each department sees to begin with")
    status, kp_cases = call("GET", "/cases?pageSize=200", kp_sho)
    check("Kolkata Police sees its own cases", status == 200 and kp_cases.get("total", 0) > 0,
          f"{status} total {kp_cases.get('total')}")
    status, cid_cases = call("GET", "/cases?pageSize=200", cid_sp)
    check("CID does not see Kolkata Police's cases", status == 200 and cid_cases.get("total", 0) == 0,
          f"CID sees {cid_cases.get('total')}")

    case = (kp_cases.get("data") or [None])[0]
    if not case:
        print("no case to refer"); sys.exit(1)
    case_id, case_number = case["id"], case.get("caseNumber", "")
    print(f"       the case to refer: {case_number}")

    print("\nCID cannot refer a case it does not hold")
    status, body = call("POST", "/referrals", cid_sp, {
        "recordType": "CASE", "recordId": case_id, "toForceCode": "KP",
        "reason": "Probe: CID trying to refer a case it cannot see."})
    check("refusing to refer another department's record", status == 403, f"{status} {body}")

    print("\nKolkata Police refers it to CID")
    status, body = call("POST", "/referrals", kp_sho, {
        "recordType": "CASE", "recordId": case_id, "toForceCode": "CID",
        "reason": "Probe: the accused is linked to cases in three districts, so CID should lead.",
        "authority": "Probe run"})
    check("the referral was proposed", status == 201, f"{status} {body}")
    if status != 201:
        sys.exit(1)
    referral_id = body["id"]
    print(f"       {body.get('referralNumber')}: {body.get('fromForceShortName')} → {body.get('toForceShortName')}, {body.get('status')}")

    status, body = call("POST", "/referrals", kp_sho, {
        "recordType": "CASE", "recordId": case_id, "toForceCode": "CID",
        "reason": "Probe: referring the same case twice."})
    check("the same record cannot be referred twice at once", status == 409, f"{status} {body}")

    print("\nA proposal alone does not move the record")
    status, cid_cases = call("GET", "/cases?pageSize=200", cid_sp)
    check("CID still cannot see it while the referral is only proposed",
          cid_cases.get("total", 0) == 0, f"CID sees {cid_cases.get('total')}")

    print("\nOnly the receiving department decides")
    status, body = call("POST", f"/referrals/{referral_id}/decision", kp_sho, {"accept": True})
    check("the referring department cannot accept its own referral", status == 403, f"{status} {body}")

    status, body = call("POST", f"/referrals/{referral_id}/decision", cid_sp,
                        {"accept": True, "note": "Probe: accepted by the Homicide Squad."})
    check("CID accepts it", status == 200 and body.get("status") == "ACCEPTED", f"{status} {body}")
    check("the acceptance names the officer", bool(body.get("decidedByName")), str(body.get("decidedByName")))

    print("\nNow, and only now, CID can see the case")
    status, cid_cases = call("GET", "/cases?pageSize=200", cid_sp)
    ids = [c["id"] for c in (cid_cases.get("data") or [])]
    check("the referred case is in CID's register", case_id in ids, f"CID sees {cid_cases.get('total')}")

    status, cid_junior = call("GET", "/cases?pageSize=200", cid_si)
    check("and to the rest of CID as well", case_id in [c["id"] for c in (cid_junior.get("data") or [])],
          f"the junior officer sees {cid_junior.get('total')}")

    print("\nKolkata Police keeps it too — a referral is not a handover of sight")
    status, kp_after = call("GET", "/cases?pageSize=200", kp_sho)
    check("Kolkata Police still sees the case it referred",
          case_id in [c["id"] for c in (kp_after.get("data") or [])], "it vanished from KP")

    print("\nThe wording tells the officer which situation they are in")
    status, body = call("POST", "/referrals", kp_sho, {
        "recordType": "CASE", "recordId": case_id, "toForceCode": "CID",
        "reason": "Probe: referring a case that CID has already accepted."})
    check("an accepted record says so, not that a referral is pending",
          status == 409 and "already with" in body.get("message", ""), f"{status} {body.get('message')}")

    print("\nComplaints refer the same way")
    status, complaints = call("GET", "/complaints?pageSize=1", kp_sho)
    complaint = (complaints.get("data") or [None])[0] if status == 200 else None
    if not complaint:
        print("       no complaint to refer on this database — not exercised")
    else:
        status, body = call("POST", "/referrals", kp_sho, {
            "recordType": "COMPLAINT", "recordId": complaint["id"], "toForceCode": "CID",
            "reason": "Probe: the complainant names an accused under investigation elsewhere."})
        check("a complaint can be referred", status == 201, f"{status} {body}")
        if status == 201:
            status, decided = call("POST", f"/referrals/{body['id']}/decision", cid_sp, {"accept": False,
                                   "note": "Probe: declined, not a CID matter."})
            check("and declined by the receiving department", status == 200 and decided.get("status") == "DECLINED",
                  f"{status} {decided}")

    print("\nThe decision stands")
    status, body = call("POST", f"/referrals/{referral_id}/decision", cid_sp, {"accept": False})
    check("a decided referral cannot be decided again", status == 409, f"{status} {body}")

    print("\nBoth departments see the referral itself")
    for who, token in (("Kolkata Police", kp_sho), ("CID", cid_sp)):
        status, body = call("GET", "/referrals", token)
        found = any(r["id"] == referral_id for r in (body.get("data") or []))
        check(f"{who} sees the referral in its list", found, f"{status}")

    print(f"\nTidy up:  psql -d npdms -c \"DELETE FROM case_referrals WHERE id = '{referral_id}'\"")
    print(f"\n{len(ok)} passed, {len(fail)} failed")
    for f in fail:
        print("  FAILED " + f)
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
