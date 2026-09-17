#!/usr/bin/env python3
"""Operational records for the three departments that had none.

scripts/seed-departments-demo.py gave West Bengal Police, the CID and the
traffic wing their stations and their officers. It gave them no work: every
FIR, case and exhibit in the platform belongs to Kolkata Police, so signing in
as any of the other three opened an empty register and a dashboard of zeroes.
Three of the four departments the platform claims to serve had nothing in them.

Every record here is created THROUGH THE API as the officer who would record
it, exactly as scripts/seed-kolkata-demo.py does, so numbering, workflow rules,
the force boundary and the audit trail are all genuine rather than rows pushed
past them with SQL. Each department gets the work its remit actually covers —
which is also what the module rules say it may see:

  West Bengal Police   district policing: FIRs at Barasat, Titagarh and
                       Howrah, taken through to cases.
  Traffic              challans and a road accident. No investigation: the
                       wing hands a case to the station rather than taking
                       it up, which is why the module is hidden from it.
  CID                  cases referred to it, because that is how a matter
                       reaches the CID — on a referral, not a walk-in.

Every person is fictional. The places are real.

Idempotent through the same `demo_seed_ledger` the other scripts use, so a
second run creates nothing and says so.

Usage:
    DEMO_SEED_CONFIRM=yes \\
    DATABASE_URL=postgresql://sudipto@localhost:5432/npdms \\
    API_URL=http://localhost:18080/api/v1 \\
    python3 scripts/seed-departments-records.py

The API must be running against the same DATABASE_URL.
"""
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

PASSWORD = "Demo@123"
API = os.environ.get("API_URL", "http://localhost:18080/api/v1")
DATABASE_URL = os.environ.get("DATABASE_URL")

if os.environ.get("DEMO_SEED_CONFIRM") != "yes":
    sys.exit("Refusing to run: this loads FICTIONAL demo records. Set DEMO_SEED_CONFIRM=yes.")
if not DATABASE_URL:
    sys.exit("DATABASE_URL is required.")

tokens = {}
created = {}
skipped = 0


def psql(sql):
    r = subprocess.run(["psql", DATABASE_URL, "-v", "ON_ERROR_STOP=1", "-qAt", "-c", sql],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"{sql[:120]}\n{r.stderr.strip()}")
    return r.stdout.strip()


def q(v):
    return "NULL" if v is None else "'" + str(v).replace("'", "''") + "'"


def ledger_get(key):
    return psql(f"SELECT entity_id FROM demo_seed_ledger WHERE key = {q(key)}") or None


def ledger_put(key, entity_id, entity):
    psql(f"INSERT INTO demo_seed_ledger (key, entity, entity_id) "
         f"VALUES ({q(key)}, {q(entity)}, {q(entity_id)}) ON CONFLICT (key) DO NOTHING")


def iso(dt):
    return dt.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def days_ago(d, hour=11, minute=0):
    at = datetime.now(timezone.utc) - timedelta(days=d)
    return at.replace(hour=hour, minute=minute, second=0, microsecond=0)


def token(user):
    """Sign in as an officer, naming them if that fails.

    The failure this saves: a login refused inside request() reported the
    caller's default user rather than the officer whose credentials were
    actually rejected, so a username that did not exist looked like the
    administrator's password being wrong.
    """
    if user not in tokens:
        try:
            tokens[user] = request("POST", "/auth/login",
                                   {"username": user, "password": PASSWORD}, auth=False)["accessToken"]
        except RuntimeError as e:
            raise RuntimeError(f"could not sign in as {user}: {e}") from None
    return tokens[user]


def request(method, path, body=None, user="admin", auth=True, ok=(200, 201, 204)):
    """Call the API, backing off on rate limiting. Raises on any other failure."""
    for attempt in range(8):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(API + path, method=method, data=data)
        req.add_header("Content-Type", "application/json")
        req.add_header("User-Agent", "npdms-departments-seed/1.0")
        if auth:
            req.add_header("Authorization", f"Bearer {token(user)}")
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                payload = r.read()
                return json.loads(payload) if payload else {}
        except urllib.error.HTTPError as e:
            text = e.read().decode(errors="replace")
            if e.code == 429:
                wait = 15 * (attempt + 1)
                print(f"    rate limited on {method} {path}; waiting {wait}s")
                time.sleep(wait)
                continue
            if e.code == 401 and auth and attempt == 0:
                tokens.pop(user, None)
                continue
            if e.code in ok:
                return {}
            raise RuntimeError(f"{method} {path} as {user} -> {e.code}: {text[:400]}")
    raise RuntimeError(f"{method} {path}: still rate limited after retries")


def once(key, entity, fn):
    global skipped
    existing = ledger_get(key)
    if existing:
        skipped += 1
        return existing
    result = fn()
    rid = result if isinstance(result, str) else (result.get("id") or result.get("data", {}).get("id"))
    if not rid:
        raise RuntimeError(f"{key}: API returned no id: {json.dumps(result)[:300]}")
    ledger_put(key, rid, entity)
    created[entity] = created.get(entity, 0) + 1
    return rid


def station_id(code):
    sid = psql(f"SELECT id FROM stations WHERE code = {q(code)}")
    if not sid:
        raise RuntimeError(f"no station with code {code}. Run scripts/seed-departments-demo.py first.")
    return sid


def officer_id(username):
    uid = psql(f"SELECT id FROM users WHERE username = {q(username)}")
    if not uid:
        raise RuntimeError(f"no officer {username}. Run scripts/seed-departments-demo.py first.")
    return uid


psql("""CREATE TABLE IF NOT EXISTS demo_seed_ledger (
          key TEXT PRIMARY KEY, entity TEXT NOT NULL, entity_id TEXT NOT NULL,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())""")

# ---------------------------------------------------------------- WBP -------
# Ordinary district policing at three thanas of two ranges. The offences are
# the everyday work of a district: theft, a road-side assault, a house
# break-in, a dowry complaint, a missing cattle head in a rural thana.
print("› West Bengal Police — FIRs and cases")

WBP_FIRS = [
    ("BRS", "oc.brs", 26, "IPC 379",
     "Theft of a motorcycle from outside Barasat station bazaar",
     "Ranjit Halder", "9832100011",
     "Barasat station bazaar, opposite the ticket counter, North 24 Parganas"),
    ("BRS", "oc.brs", 19, "IPC 323, 504",
     "Assault and abuse following a dispute over a shared boundary wall",
     "Mamata Sardar", "9832100012",
     "Nabapally, Ward 12, Barasat Municipality"),
    ("TTG", "si.ttg", 14, "IPC 457, 380",
     "House break-in at night; gold ornaments and cash reported missing",
     "Dibyendu Saha", "9832100013",
     "Talpukur More, Titagarh, North 24 Parganas"),
    ("TTG", "si.ttg", 9, "IPC 279, 337",
     "Rash driving on the Barrackpore Trunk Road causing injury to a pedestrian",
     "Sabina Khatun", "9832100014",
     "BT Road near Titagarh jute mill gate"),
    ("HWR", "dsp.hwc", 6, "IPC 420, 406",
     "Cheating and breach of trust over an advance paid for building material",
     "Prakash Dutta", "9832100015",
     "Golabari, Howrah"),
    ("BRS", "sp.n24", 3, "IPC 363",
     "Report of a minor missing from a wedding gathering, since traced and restored",
     "Anjali Biswas", "9832100016",
     "Madhyamgram Chowmatha, North 24 Parganas"),
]

wbp_firs = []
for i, (code, officer, ago, sections, description, complainant, phone, place) in enumerate(WBP_FIRS, start=1):
    key = f"wbp:fir:{i}"

    def make(code=code, officer=officer, ago=ago, sections=sections,
             description=description, complainant=complainant, phone=phone, place=place):
        return request("POST", "/firs", {
            "stationId": station_id(code),
            "complainantName": complainant,
            "complainantPhone": phone,
            "complainantAddress": place,
            "incidentDate": iso(days_ago(ago, hour=9 + (ago % 8))),
            "incidentLocation": place,
            "incidentDescription": description,
            "ipcSections": [x.strip() for x in sections.split(",")],
            "priority": "MEDIUM" if ago % 3 else "HIGH",
        }, user=officer)

    wbp_firs.append((once(key, "wbp_firs", make), officer))

# Two of them taken up as cases, which is what a district does with the ones
# that do not close at the thana.
for i, (fir_id, officer) in enumerate(wbp_firs[:3], start=1):
    key = f"wbp:case:{i}"

    def make(fir_id=fir_id, officer=officer):
        return request("POST", "/cases", {
            "firId": fir_id,
            "title": "Investigation arising from the FIR",
            "description": "Taken up for investigation by the thana.",
            "status": "UNDER_INVESTIGATION",
            "priority": "MEDIUM",
        }, user=officer)

    once(key, "wbp_cases", make)

# ------------------------------------------------------------ TRAFFIC ------
# Challans and one road accident. No investigation work: the wing hands a case
# to the station, which is why the investigation module is hidden from it.
print("› Kolkata Traffic Police — challans and an accident")

# The challan body uses snake_case and names a violation type from the
# reference table rather than free text, so the fine follows the schedule
# instead of whatever a script typed.
CHALLANS = [
    ("SAF001", "WB02AF4471", "TWO_WHEELER", "Park Street crossing, Kolkata", "sgt.pks", 12),
    ("DNG002", "WB06C7782", "FOUR_WHEELER", "Camac Street, opposite the fire station", "oc.tg.pks", 10),
    ("SIG001", "WB20B1194", "FOUR_WHEELER", "Jadavpur 8B bus stand crossing", "sgt.tg.jdp", 8),
    ("PRK001", "WB04E5560", "FOUR_WHEELER", "Park Street, outside the Asiatic Society", "sgt.pks", 6),
    ("SAF002", "WB01A2231", "FOUR_WHEELER", "Howrah Bridge approach, Kolkata end", "sgt.tg.hwb", 5),
    ("SAF001", "WB19F8803", "TWO_WHEELER", "Jadavpur Police Station crossing", "sgt.tg.jdp", 3),
]


def violation_type(code):
    vid = psql(f"SELECT id FROM traffic_violation_types WHERE code = {q(code)}")
    if not vid:
        raise RuntimeError(f"no violation type {code}")
    return vid


for i, (code, vehicle, vtype, place, officer, ago) in enumerate(CHALLANS, start=1):
    key = f"traffic:challan:{i}"

    def make(code=code, vehicle=vehicle, vtype=vtype, place=place, officer=officer, ago=ago):
        return request("POST", "/traffic/challans", {
            "violation_type_id": violation_type(code),
            "violation_date": iso(days_ago(ago, hour=8 + (ago % 10))),
            "violation_location": place,
            "vehicle_number": vehicle,
            "vehicle_type": vtype,
        }, user=officer)

    once(key, "traffic_challans", make)

# ----------------------------------------------------------------- CID -----
# A matter reaches the CID on a referral, not at a counter. Two are sent from
# Kolkata Police and accepted, which is the mechanism the referral module
# exists for and the only way a case crosses the force boundary.
print("› CID — cases referred to it")

kp_firs = psql("""SELECT f.id FROM firs f
                  JOIN stations s ON s.id = f.station_id
                  JOIN forces fo ON fo.id = s.force_id
                  WHERE fo.code = 'KP'
                  ORDER BY f.created_at DESC LIMIT 2""").split("\n")
kp_firs = [x for x in kp_firs if x]

for i, fir_id in enumerate(kp_firs, start=1):
    key = f"cid:referral:{i}"

    def make(fir_id=fir_id, i=i):
        return request("POST", "/referrals", {
            "recordType": "FIR",
            "recordId": fir_id,
            "toForceCode": "CID",
            "reason": "Referred to the CID for specialised investigation: the matter "
                      "crosses commissionerate limits and involves a suspected organised group.",
        }, user="admin")

    referral = once(key, "cid_referrals", make)

    # Accepted by the CID, which is what makes the record readable to them.
    accept_key = f"cid:referral:{i}:accepted"
    if not ledger_get(accept_key):
        try:
            request("POST", f"/referrals/{referral}/decision",
                    {"accept": True,
                     "note": "Accepted by the Homicide Squad for investigation."},
                    user="sp.cid")
            ledger_put(accept_key, referral, "cid_referrals_accepted")
            created["cid_referrals_accepted"] = created.get("cid_referrals_accepted", 0) + 1
        except RuntimeError as e:
            print(f"    referral {i} could not be accepted: {str(e)[:160]}")

# ------------------------------------------------------------------ done ---
print()
if created:
    for entity, n in sorted(created.items()):
        print(f"  created {n:3d}  {entity}")
else:
    print("  nothing to create")
if skipped:
    print(f"  skipped {skipped} already recorded in demo_seed_ledger")
