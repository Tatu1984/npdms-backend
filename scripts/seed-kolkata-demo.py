#!/usr/bin/env python3
"""Load the Kolkata Police DEMO dataset into an NPDMS database.

Every person in this dataset is fictional. Station names, divisions, courts
and localities are real Kolkata / West Bengal reference data so the demo reads
as the place it is built for.

Reference data (stations and demo accounts) is applied with psql from
services/db/init/002_demo_kolkata.sql. Every operational record — FIRs, cases,
evidence, warrants and the rest — is created THROUGH THE API as the officer who
would record it, so numbering, workflow rules, signatures and the audit trail
are all real.

Idempotent: each seeded item is recorded under a stable key in
`demo_seed_ledger` (created in the target database by this script). Re-running
skips everything already created.

Usage:
    DEMO_SEED_CONFIRM=yes \\
    DATABASE_URL=postgres://localhost:5432/npdms_demo?sslmode=disable \\
    API_URL=http://localhost:8080/api/v1 \\
    python3 scripts/seed-kolkata-demo.py

The API must be running against the same DATABASE_URL.
"""
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

ROOT = pathlib.Path(__file__).resolve().parent.parent
REFERENCE_SQL = ROOT / "services" / "db" / "init" / "002_demo_kolkata.sql"
PASSWORD = "Demo@123"

if os.environ.get("DEMO_SEED_CONFIRM") != "yes":
    sys.exit("Refusing to run: this loads FICTIONAL demo records. Set DEMO_SEED_CONFIRM=yes to proceed.")
DATABASE_URL = os.environ.get("DATABASE_URL")
API = os.environ.get("API_URL", "http://localhost:8080/api/v1").rstrip("/")
if not DATABASE_URL:
    sys.exit("DATABASE_URL is required (the database the API at API_URL is using).")

NOW = datetime.now(timezone.utc).replace(microsecond=0)
IST = timezone(timedelta(hours=5, minutes=30))
created = {}


# ------------------------------------------------------------------ helpers --
def psql(sql, *, file=None):
    cmd = ["psql", DATABASE_URL, "-v", "ON_ERROR_STOP=1", "-qAt"]
    cmd += ["-f", str(file)] if file else ["-c", sql]
    return subprocess.run(cmd, capture_output=True, text=True, check=True).stdout.strip()


def iso(dt):
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def days_ago(d, hour=11, minute=0):
    base = (NOW - timedelta(days=d)).astimezone(IST).replace(hour=hour, minute=minute, second=0)
    return base


def ledger_get(key):
    return psql(f"SELECT entity_id FROM demo_seed_ledger WHERE key = '{key}'") or None


def ledger_put(key, entity_id, entity):
    psql(f"INSERT INTO demo_seed_ledger (key, entity, entity_id) VALUES ('{key}', '{entity}', '{entity_id}') "
         "ON CONFLICT (key) DO NOTHING")


tokens = {}


def token(user):
    if user not in tokens:
        tokens[user] = request("POST", "/auth/login", {"username": user, "password": PASSWORD}, auth=False)["accessToken"]
    return tokens[user]


def request(method, path, body=None, user="admin", auth=True, raw=None, ctype="application/json", ok=(200, 201, 204)):
    """Call the API, backing off on rate limiting. Raises on any other failure."""
    for attempt in range(8):
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        req = urllib.request.Request(API + path, method=method, data=data)
        req.add_header("Content-Type", ctype)
        req.add_header("User-Agent", "npdms-demo-seed/1.0")
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
    """Create an item unless the ledger already has it. Returns its id."""
    existing = ledger_get(key)
    if existing:
        return existing
    result = fn()
    entity_id = result if isinstance(result, str) else (
        result.get("id") or result.get("entity", {}).get("id") or (result.get("complaint") or {}).get("id")
        or result.get("trackingNumber"))
    if not entity_id:
        raise RuntimeError(f"{key}: API returned no id: {json.dumps(result)[:300]}")
    ledger_put(key, entity_id, entity)
    created[entity] = created.get(entity, 0) + 1
    return entity_id


def step(key, fn):
    """Run a follow-up action once (a transition, an acknowledgement)."""
    if ledger_get(key):
        return
    fn()
    ledger_put(key, "00000000-0000-0000-0000-000000000000", "action")
    created["actions"] = created.get("actions", 0) + 1


def sql_id(query):
    value = psql(query)
    if not value:
        raise RuntimeError(f"reference lookup returned nothing: {query}")
    return value


# ------------------------------------------------------------ reference data --
print("› Reference data: Kolkata Police stations and demo accounts")
psql("", file=REFERENCE_SQL)
psql("""CREATE TABLE IF NOT EXISTS demo_seed_ledger (
          key TEXT PRIMARY KEY, entity TEXT NOT NULL, entity_id TEXT NOT NULL,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())""")

STATION = {code: sql_id(f"SELECT id FROM stations WHERE code = '{code}'")
           for code in ["LBZ", "BHW", "PKS", "JDP", "KSB", "TLG", "GRH", "SHY", "BBZ"]}
USER = {u: sql_id(f"SELECT id FROM users WHERE username = '{u}'")
        for u in ["admin", "sp", "dsp", "sho", "inspector", "si", "asi", "hc", "constable",
                  "oc.pks", "si.pks", "asi.pks", "oc.jdp", "si.jdp", "oc.ksb", "si.ksb", "oc.tlg", "si.tlg",
                  "oc.shy", "si.shy", "oc.bbz", "si.bbz", "si.grh"]}
# The station each station-level officer works from.
STATION_OFFICERS = {
    "BHW": ("sho", "si", "asi"), "PKS": ("oc.pks", "si.pks", "asi.pks"), "JDP": ("oc.jdp", "si.jdp", "asi"),
    "KSB": ("oc.ksb", "si.ksb", "asi"), "TLG": ("oc.tlg", "si.tlg", "asi"), "SHY": ("oc.shy", "si.shy", "asi"),
    "BBZ": ("oc.bbz", "si.bbz", "asi"), "GRH": ("sho", "si.grh", "asi"),
}

# ---------------------------------------------------------------- personnel --
print("› Personnel roster")
PERSONNEL = [
    ("sho", "SHO", "BHW", "Station Duty", "Day (0600-1400)"), ("inspector", "INSPECTOR", "BHW", "Investigation", "Day (0600-1400)"),
    ("si", "SI", "BHW", "Investigation", "Day (0600-1400)"), ("asi", "ASI", "BHW", "Beat Patrol", "Evening (1400-2200)"),
    ("hc", "HEAD_CONSTABLE", "BHW", "Beat Patrol", "Evening (1400-2200)"), ("constable", "CONSTABLE", "BHW", "PCR Mobile", "Night (2200-0600)"),
    ("oc.pks", "SHO", "PKS", "Station Duty", "Day (0600-1400)"), ("si.pks", "SI", "PKS", "Investigation", "Day (0600-1400)"),
    ("asi.pks", "ASI", "PKS", "Traffic Control", "Evening (1400-2200)"), ("oc.jdp", "SHO", "JDP", "Station Duty", "Day (0600-1400)"),
    ("si.jdp", "SI", "JDP", "Investigation", "Day (0600-1400)"), ("oc.ksb", "SHO", "KSB", "Station Duty", "Day (0600-1400)"),
    ("si.ksb", "SI", "KSB", "Investigation", "Evening (1400-2200)"), ("oc.tlg", "SHO", "TLG", "Station Duty", "Day (0600-1400)"),
    ("si.tlg", "SI", "TLG", "Court Escort", "Day (0600-1400)"), ("oc.shy", "SHO", "SHY", "Station Duty", "Day (0600-1400)"),
    ("si.shy", "SI", "SHY", "Investigation", "Day (0600-1400)"), ("oc.bbz", "SHO", "BBZ", "Station Duty", "Day (0600-1400)"),
    ("si.bbz", "SI", "BBZ", "Investigation", "Day (0600-1400)"), ("si.grh", "SI", "GRH", "Investigation", "Day (0600-1400)"),
]
for i, (u, rank, stn, duty, shift) in enumerate(PERSONNEL):
    badge = psql(f"SELECT badge_number FROM users WHERE username = '{u}'")
    pid = once(f"personnel:{u}", "personnel", lambda u=u, rank=rank, stn=stn, badge=badge, i=i: request("POST", "/personnel", {
        "userId": USER[u], "badgeNumber": badge, "rank": rank, "status": "ON_DUTY", "stationId": STATION[stn],
        "joiningDate": iso(days_ago(365 * (4 + i % 12))), "currentDuty": duty, "shift": shift}))

# ------------------------------------------------------------------ vehicles --
print("› Fleet")
VEHICLES = [
    ("WB-01-PC-4102", "PCR", "Mahindra Bolero", "BHW", 22.5290, 88.3440, "constable", "Hazra Road beat"),
    ("WB-01-PC-4117", "PCR", "Mahindra Bolero", "PKS", 22.5530, 88.3540, None, None),
    ("WB-02-GY-2231", "Gypsy", "Maruti Gypsy", "JDP", None, None, None, None),
    ("WB-06-PT-0934", "Patrol", "Tata Sumo Gold", "KSB", 22.5150, 88.3990, None, None),
    ("WB-01-PC-4188", "PCR", "Mahindra Scorpio", "TLG", None, None, None, None),
    ("WB-02-BS-0071", "Bus", "Tata Starbus (prison van)", "LBZ", None, None, None, None),
    ("WB-01-PT-5520", "Patrol", "Maruti Ertiga", "SHY", 22.5960, 88.3700, None, None),
    ("WB-06-PC-3312", "PCR", "Mahindra Bolero", "BBZ", None, None, None, None),
]
VEH = {}
for reg, vtype, make, stn, lat, lng, driver, duty in VEHICLES:
    body = {"registrationNumber": reg, "type": vtype, "make": make, "status": "AVAILABLE", "fuelLevel": 70,
            "odometerReading": 18000 + hash(reg) % 40000, "lastService": iso(days_ago(40)), "stationId": STATION[stn]}
    if lat:
        body.update({"gpsLatitude": lat, "gpsLongitude": lng})
    VEH[reg] = once(f"vehicle:{reg}", "vehicles", lambda body=body: request("POST", "/vehicles", body))
    if driver:
        step(f"vehicle:{reg}:allocate", lambda reg=reg, driver=driver, duty=duty: request(
            "POST", f"/vehicles/{VEH[reg]}/allocate", {"driverId": USER[driver], "duty": duty}))

# ---------------------------------------------------------------------- FIRs --
print("› FIRs")
COMPLAINANTS = ["Bikash Haldar", "Sharmila Dutta", "Tanmoy Bhattacharya", "Nabanita Saha", "Pradip Kundu",
                "Ishita Chowdhury", "Sanjoy Mondal", "Farhana Khatun", "Rajib Mitra", "Sudeshna Pal",
                "Mohammed Aslam", "Anjana Biswas", "Debjit Sarkar", "Payel Ghosh", "Gautam Jana",
                "Rupa Adhikari", "Arjun Prasad", "Mitali Roy"]
FIRS = [
    # (key, station, days ago, hour, BNS sections, priority, location, description)
    ("bhw-01", "BHW", 170, 21, ["BNS 304"], "HIGH", "Hazra Road near Jatin Das Park, Kolkata 700026", "Gold chain snatched from complainant by two men on a scooter while she walked home."),
    ("bhw-02", "BHW", 150, 14, ["BNS 305"], "MEDIUM", "Chakraberia Road (South), Bhowanipore, Kolkata 700025", "Flat broken into while the family was away; jewellery and cash taken from a steel almirah."),
    ("bhw-03", "BHW", 120, 23, ["BNS 115", "BNS 352"], "MEDIUM", "Paddapukur Road, Bhowanipore", "Assault following an argument over parking outside a residential building."),
    ("bhw-04", "BHW", 64, 19, ["BNS 318", "IT Act 66D"], "HIGH", "Elgin Road, Kolkata 700020", "Complainant cheated of savings by a caller posing as a bank official, OTP shared under pressure."),
    ("bhw-05", "BHW", 21, 22, ["BNS 309"], "CRITICAL", "Bhowanipore Metro gate 2, S.P. Mukherjee Road", "Robbery at knife-point of a courier carrying cash; mobile phone and bag taken."),
    ("pks-01", "PKS", 160, 1, ["BNS 115", "BNS 351"], "MEDIUM", "Park Street near Mullick Bazar crossing", "Brawl outside a restaurant; bouncer and patron injured, threats issued."),
    ("pks-02", "PKS", 110, 16, ["BNS 303"], "LOW", "Park Street Metro concourse", "Mobile phone stolen from complainant's bag in the evening rush."),
    ("pks-03", "PKS", 75, 11, ["BNS 316"], "HIGH", "Camac Street office complex, Kolkata 700016", "Accounts assistant misappropriated company funds over eight months."),
    ("pks-04", "PKS", 30, 20, ["BNS 74"], "HIGH", "Free School Street, Kolkata 700016", "Woman assaulted with intent to outrage modesty near a bus stop."),
    ("pks-05", "PKS", 9, 18, ["BNS 318", "IT Act 66C"], "MEDIUM", "Shakespeare Sarani, Kolkata 700017", "Online shopping fraud using a cloned seller page; payment via UPI."),
    ("jdp-01", "JDP", 140, 13, ["BNS 303"], "LOW", "8B Bus Stand, Jadavpur, Kolkata 700032", "Laptop bag stolen from a parked two-wheeler."),
    ("jdp-02", "JDP", 95, 9, ["BNS 137"], "CRITICAL", "Raja S C Mallick Road near Jadavpur University gate 4", "Minor girl did not return from tuition; family suspects kidnapping."),
    ("jdp-03", "JDP", 50, 15, ["BNS 324"], "LOW", "Sulekha More, Jadavpur", "Shop signboard and shutter damaged during a dispute."),
    ("jdp-04", "JDP", 12, 10, ["BNS 318"], "MEDIUM", "Baghajatin Station Road", "Job racket: advance taken for promised railway employment."),
    ("ksb-01", "KSB", 175, 2, ["BNS 305", "BNS 331"], "HIGH", "Rajdanga Main Road, Kasba, Kolkata 700107", "House-breaking at night; electronics and gold ornaments taken."),
    ("ksb-02", "KSB", 130, 17, ["BNS 106"], "HIGH", "EM Bypass near Ruby crossing", "Pedestrian died after being hit by a speeding vehicle that fled."),
    ("ksb-03", "KSB", 80, 12, ["BNS 85"], "HIGH", "Kasba New Market area", "Complaint of cruelty and dowry demand by husband and in-laws."),
    ("ksb-04", "KSB", 40, 21, ["BNS 303"], "MEDIUM", "Acropolis Mall parking, Kasba", "Two-wheeler stolen from the mall parking level B1."),
    ("ksb-05", "KSB", 5, 11, ["IT Act 66D", "BNS 318"], "HIGH", "Kasba Industrial Estate", "Investment app fraud; complainant transferred money to multiple UPI IDs."),
    ("tlg-01", "TLG", 155, 20, ["BNS 304"], "MEDIUM", "Tollygunge Metro, N.S.C. Bose Road", "Mobile phone snatched from complainant at the station entrance."),
    ("tlg-02", "TLG", 100, 15, ["BNS 316"], "MEDIUM", "Prince Anwar Shah Road, Kolkata 700033", "Contractor took advance for renovation and absconded."),
    ("tlg-03", "TLG", 45, 22, ["BNS 115"], "LOW", "Charu Market, Tollygunge", "Hawkers' quarrel led to minor injuries."),
    ("tlg-04", "TLG", 16, 8, ["BNS 305"], "MEDIUM", "Regent Park, Kolkata 700040", "Burglary at a ground-floor flat; cash and a tablet taken."),
    ("shy-01", "SHY", 165, 18, ["BNS 303"], "LOW", "Shyambazar five-point crossing", "Wallet stolen from a passenger boarding a bus."),
    ("shy-02", "SHY", 125, 10, ["BNS 318"], "MEDIUM", "Bidhan Sarani, Shyampukur", "Elderly complainant cheated in a fake pension revision scheme."),
    ("shy-03", "SHY", 70, 23, ["BNS 309"], "HIGH", "Girish Park Metro area", "Late-night robbery of a delivery rider; motorcycle keys and phone taken."),
    ("shy-04", "SHY", 25, 13, ["BNS 351"], "LOW", "Kumartuli Street", "Threats issued over a property boundary dispute."),
    ("bbz-01", "BBZ", 145, 12, ["BNS 316", "BNS 318"], "HIGH", "Rabindra Sarani, Burrabazar, Kolkata 700007", "Trader alleges breach of trust by a commission agent in a textile consignment."),
    ("bbz-02", "BBZ", 90, 15, ["BNS 303"], "MEDIUM", "Posta Bazar, Burrabazar", "Cash bag stolen from a wholesale shop counter."),
    ("bbz-03", "BBZ", 35, 11, ["BNS 318", "IT Act 66C"], "HIGH", "Kalakar Street, Burrabazar", "GST refund fraud using the complainant's forged credentials."),
    ("bbz-04", "BBZ", 7, 19, ["BNS 115"], "LOW", "Cotton Street, Burrabazar", "Porters' scuffle over loading work; one injured."),
    ("grh-01", "GRH", 135, 19, ["BNS 304"], "MEDIUM", "Gariahat crossing, Kolkata 700019", "Bag snatched from a shopper outside a saree store."),
    ("grh-02", "GRH", 60, 14, ["BNS 303"], "LOW", "Gariahat Market", "Mobile phone pickpocketed during festive shopping."),
    ("grh-03", "GRH", 18, 21, ["BNS 324", "BNS 351"], "MEDIUM", "Hindustan Park, Kolkata 700029", "Car windscreen smashed and owner threatened after a road-rage incident."),
    ("grh-04", "GRH", 3, 17, ["BNS 318"], "MEDIUM", "Rashbehari Avenue", "Advance paid for a rented flat to a person who was not the owner."),
    ("bhw-06", "BHW", 2, 9, ["BNS 303"], "LOW", "Jadu Bhattacharya Lane, Bhowanipore", "Bicycle stolen from a building courtyard."),
]
FIR = {}
for i, (key, stn, ago, hour, sections, prio, loc, desc) in enumerate(FIRS):
    officers = STATION_OFFICERS[stn]
    when = days_ago(ago, hour, (i * 7) % 60)
    body = {"stationId": STATION[stn], "complainantName": COMPLAINANTS[i % len(COMPLAINANTS)],
            "complainantPhone": f"98300{(100 + i):05d}", "complainantAddress": loc,
            "incidentDate": iso(when.replace(hour=0, minute=0)), "incidentTime": when.strftime("%H:%M"),
            "incidentLocation": loc, "incidentDescription": desc, "ipcSections": sections, "priority": prio}
    FIR[key] = once(f"fir:{key}", "firs", lambda body=body, officers=officers: request("POST", "/firs", body, user=officers[1]))

# Status progression on older FIRs.
for key, status in [("bhw-01", "UNDER_INVESTIGATION"), ("bhw-02", "UNDER_INVESTIGATION"), ("bhw-03", "CHARGESHEET_FILED"),
                    ("bhw-05", "UNDER_INVESTIGATION"), ("pks-01", "CHARGESHEET_FILED"), ("pks-03", "UNDER_INVESTIGATION"),
                    ("pks-04", "UNDER_INVESTIGATION"), ("jdp-02", "UNDER_INVESTIGATION"), ("ksb-01", "CHARGESHEET_FILED"),
                    ("ksb-02", "UNDER_INVESTIGATION"), ("ksb-03", "UNDER_INVESTIGATION"), ("ksb-05", "UNDER_INVESTIGATION"),
                    ("tlg-02", "UNDER_INVESTIGATION"), ("shy-03", "UNDER_INVESTIGATION"), ("bbz-01", "UNDER_INVESTIGATION"),
                    ("bbz-03", "UNDER_INVESTIGATION"), ("grh-01", "CLOSED"), ("shy-01", "CLOSED")]:
    step(f"fir:{key}:status:{status}", lambda key=key, status=status: request("PATCH", f"/firs/{FIR[key]}/status", {"status": status}))

# --------------------------------------------------------------------- cases --
print("› Cases, accused and witnesses")
CASES = [
    ("bhw-01", "Hazra Road chain snatching", "inspector", "Alipore CJM Court"),
    ("bhw-02", "Chakraberia Road house burglary", "si", "Alipore CJM Court"),
    ("bhw-03", "Paddapukur Road assault", "si", "Alipore CJM Court"),
    ("bhw-05", "Bhowanipore Metro courier robbery", "inspector", "City Sessions Court"),
    ("pks-01", "Park Street restaurant brawl", "si.pks", "Bankshall Court"),
    ("pks-03", "Camac Street funds misappropriation", "si.pks", "Bankshall Court"),
    ("pks-04", "Free School Street assault on a woman", "si.pks", "City Sessions Court"),
    ("jdp-02", "Jadavpur minor kidnapping", "si.jdp", "Special POCSO Court"),
    ("ksb-01", "Rajdanga house-breaking", "si.ksb", "Alipore CJM Court"),
    ("ksb-02", "EM Bypass fatal hit-and-run", "si.ksb", "Alipore Judges Court"),
    ("ksb-03", "Kasba dowry cruelty complaint", "si.ksb", "Alipore CJM Court"),
    ("ksb-05", "Kasba investment app fraud", "si.ksb", "Special Court (Cyber)"),
    ("tlg-02", "Prince Anwar Shah Road contractor fraud", "si.tlg", "Alipore CJM Court"),
    ("shy-03", "Girish Park delivery rider robbery", "si.shy", "City Sessions Court"),
    ("bbz-01", "Burrabazar textile consignment breach of trust", "si.bbz", "Bankshall Court"),
    ("bbz-03", "Kalakar Street GST refund fraud", "si.bbz", "Bankshall Court"),
]
CASE = {}
for key, title, io, court in CASES:
    sections = next(f[4] for f in FIRS if f[0] == key)
    prio = next(f[5] for f in FIRS if f[0] == key)
    CASE[key] = once(f"case:{key}", "cases", lambda key=key, title=title, io=io, court=court, sections=sections, prio=prio: request(
        "POST", "/cases", {"firId": FIR[key], "title": title, "priority": prio, "status": "UNDER_INVESTIGATION",
                           "ipcSections": sections, "investigatingOfficer": USER[io], "courtName": court}, user=io))

ACCUSED = [
    ("bhw-01", "Sk. Rafique", "Bapi", 27, "MALE", "Kalighat, Kolkata 700026", "ARRESTED", 150),
    ("bhw-01", "Tapas Naskar", None, 24, "MALE", "Chetla, Kolkata 700027", "ABSCONDING", None),
    ("bhw-03", "Rohit Sharma", None, 35, "MALE", "Paddapukur Road, Kolkata 700020", "ON_BAIL", 118),
    ("bhw-05", "Sunny Mallick", "Kalu", 29, "MALE", "Tiljala, Kolkata 700039", "ARRESTED", 18),
    ("pks-01", "Aftab Alam", None, 31, "MALE", "Ripon Street, Kolkata 700016", "ON_BAIL", 158),
    ("pks-03", "Subrata Dey", None, 42, "MALE", "Dum Dum, Kolkata 700028", "ARRESTED", 40),
    ("pks-04", "Unknown male (identified via CCTV)", None, None, "MALE", "Not known", "ABSCONDING", None),
    ("jdp-02", "Kartik Halder", None, 33, "MALE", "Sonarpur, South 24 Parganas", "ARRESTED", 92),
    ("ksb-01", "Joydeb Sardar", "Jojo", 38, "MALE", "Canning, South 24 Parganas", "ARRESTED", 170),
    ("ksb-03", "Amit Kumar Paul", None, 34, "MALE", "Kasba, Kolkata 700042", "ON_BAIL", 70),
    ("ksb-05", "Rakesh Verma", None, 26, "MALE", "Howrah Maidan, Howrah", "ARRESTED", 3),
    ("shy-03", "Pintu Das", None, 22, "MALE", "Sovabazar, Kolkata 700005", "ARRESTED", 66),
    ("bbz-01", "Manish Jalan", None, 47, "MALE", "Salt Lake Sector II", "ABSCONDING", None),
]
ACC = {}
for i, (ckey, name, alias, age, gender, addr, status, arrested_ago) in enumerate(ACCUSED):
    body = {"name": name, "gender": gender, "address": addr, "status": status, "firId": FIR[ckey]}
    if alias:
        body["alias"] = alias
    if age:
        body["age"] = age
    if arrested_ago is not None:
        body["arrestDate"] = iso(days_ago(arrested_ago, 14))
    ACC[f"{ckey}:{i}"] = once(f"accused:{ckey}:{i}", "accused", lambda ckey=ckey, body=body: request(
        "POST", f"/cases/{CASE[ckey]}/accused", body, user="inspector"))

WITNESSES = [
    ("bhw-01", "Mampi Sen", "EYE_WITNESS"), ("bhw-01", "Gopal Shaw (tea stall owner)", "EYE_WITNESS"),
    ("bhw-05", "Arnab Kar (courier supervisor)", "OTHER"), ("pks-01", "Joseph D'Souza (restaurant manager)", "EYE_WITNESS"),
    ("pks-03", "Anita Agarwal (company director)", "COMPLAINANT"), ("jdp-02", "Tuition teacher, Smt. Rina Paul", "OTHER"),
    ("ksb-01", "Neighbour, Sri Bhola Ghosh", "EYE_WITNESS"), ("ksb-02", "Auto driver, Sri Ratan Mondal", "EYE_WITNESS"),
    ("shy-03", "Sanjib Dhar (shopkeeper)", "EYE_WITNESS"), ("bbz-01", "Rakhi Jain (accountant)", "OTHER"),
]
for i, (ckey, name, wtype) in enumerate(WITNESSES):
    once(f"witness:{ckey}:{i}", "witnesses", lambda ckey=ckey, name=name, wtype=wtype, i=i: request(
        "POST", f"/cases/{CASE[ckey]}/witnesses",
        {"name": name, "witnessType": wtype, "phone": f"98300{(600 + i):05d}", "firId": FIR[ckey],
         "statementRecorded": i % 3 != 2, "statementDate": iso(days_ago(20 + i, 12)) if i % 3 != 2 else None},
        user="inspector"))

# ------------------------------------------------------ evidence and custody --
print("› Evidence register, files and signed custody")
EVIDENCE = [
    ("bhw-01", "PHYSICAL", "Recovered gold chain (18 g) with broken clasp", "Recovered from accused at Kalighat", "Malkhana rack B-3", "BHW/SL/2026/041"),
    ("bhw-01", "DIGITAL", "CCTV footage, Hazra Road junction camera, 21:00–21:30", "KMC camera export", "Digital evidence locker 2", "BHW/SL/2026/042"),
    ("bhw-02", "TRACE", "Fingerprint lifts from almirah handle (4 lifts)", "Scene of burglary", "Malkhana rack B-5", "BHW/SL/2026/055"),
    ("bhw-05", "PHYSICAL", "Folding knife seized from accused", "Seized at Tiljala on arrest", "Malkhana rack A-1", "BHW/SL/2026/118"),
    ("bhw-05", "DIGITAL", "Metro station CCTV extract, gate 2", "Metro Railway Kolkata control room", "Digital evidence locker 2", "BHW/SL/2026/119"),
    ("pks-01", "DOCUMENTARY", "Injury report, Calcutta National Medical College", "Hospital records", "Case file cabinet 4", "PKS/SL/2026/021"),
    ("pks-03", "DOCUMENTARY", "Bank statements of company current account (8 months)", "Obtained from bank under BNSS s.94", "Case file cabinet 2", "PKS/SL/2026/033"),
    ("pks-03", "DIGITAL", "Accounting software export and email extract", "Company server, seized with hash", "Digital evidence locker 1", "PKS/SL/2026/034"),
    ("jdp-02", "DIGITAL", "Call detail records of the accused's mobile number", "Service provider under BNSS s.94", "Digital evidence locker 3", "JDP/SL/2026/015"),
    ("ksb-01", "PHYSICAL", "Iron rod used to break the window grill", "Scene of house-breaking", "Malkhana rack C-2", "KSB/SL/2026/009"),
    ("ksb-02", "PHYSICAL", "Broken side-mirror fragments of the offending vehicle", "EM Bypass scene", "Malkhana rack C-4", "KSB/SL/2026/027"),
    ("ksb-02", "BIOLOGICAL", "Blood-stained soil sample from impact point", "EM Bypass scene", "Cold storage unit 1", "KSB/SL/2026/028"),
    ("ksb-05", "DIGITAL", "Screenshots and UPI transaction receipts from complainant's phone", "Complainant's handset, forensic image", "Digital evidence locker 3", "KSB/SL/2026/061"),
    ("shy-03", "PHYSICAL", "Motorcycle key bunch and mobile phone recovered", "Recovered from accused at Sovabazar", "Malkhana rack D-1", "SHY/SL/2026/044"),
    ("bbz-01", "DOCUMENTARY", "Consignment invoices and lorry receipts", "Complainant's firm", "Case file cabinet 1", "BBZ/SL/2026/019"),
    ("bbz-03", "DIGITAL", "GST portal login logs and forged digital signature file", "Obtained from GSTN", "Digital evidence locker 1", "BBZ/SL/2026/020"),
]
EVID = {}
boundary = "----npdmsDemoSeed"


def multipart(filename, content):
    return (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\n"
            "Content-Type: text/plain\r\n\r\n").encode() + content + f"\r\n--{boundary}--\r\n".encode()


for i, (ckey, etype, desc, collected, storage, seal) in enumerate(EVIDENCE):
    io = next(c[2] for c in CASES if c[0] == ckey)
    stn = next(f[1] for f in FIRS if f[0] == ckey)
    key = f"evidence:{ckey}:{i}"
    eid = once(key, "evidence", lambda ckey=ckey, etype=etype, desc=desc, collected=collected, storage=storage, seal=seal, io=io: request(
        "POST", "/custody", {"caseId": CASE[ckey], "evidenceType": etype, "description": desc,
                             "collectionLocation": collected, "collectionDate": iso(days_ago(next(f[2] for f in FIRS if f[0] == ckey) - 1, 13)),
                             "storageLocation": storage, "sealNumber": seal}, user=io))
    EVID[f"{ckey}:{i}"] = eid
    content = (f"DEMO EVIDENCE RECORD — fictional\nItem: {desc}\nSeal: {seal}\nCase: {ckey.upper()}\n").encode()
    step(f"{key}:file", lambda eid=eid, content=content, i=i, io=io: request(
        "POST", f"/custody/{eid}/file", raw=multipart(f"evidence-{i + 1:02d}.txt", content),
        ctype=f"multipart/form-data; boundary={boundary}", user=io))
    step(f"{key}:verify", lambda eid=eid, io=io: request("POST", f"/custody/{eid}/verify",
                                                          {"purpose": "Integrity check after registration"}, user=io))
    if i % 2 == 0:
        step(f"{key}:transfer", lambda eid=eid, io=io, storage=storage: request(
            "POST", f"/custody/{eid}/transfer",
            {"toUserId": USER["hc"], "toLocation": storage, "purpose": "Deposit in malkhana after seizure",
             "sealIntact": True, "conditionNote": "Sealed packet received intact"}, user=io))
    if etype in ("BIOLOGICAL", "TRACE") or "CCTV" in desc:
        step(f"{key}:transfer-lab", lambda eid=eid: request(
            "POST", f"/custody/{eid}/transfer",
            {"toLocation": "CFSL Kolkata, 30 Gorachand Road", "purpose": "Sent for forensic examination",
             "sealIntact": True, "notes": "Forwarded under memo with specimen seal"}, user="inspector"))

# ---------------------------------------------------------------- forensics --
print("› Forensic requests")
FORENSICS = [
    ("bhw-01:1", "bhw-01", "DIGITAL", "CFSL Kolkata", 140, True),
    ("bhw-02:2", "bhw-02", "FINGERPRINT", "State Finger Print Bureau, West Bengal", 130, True),
    ("pks-03:7", "pks-03", "DIGITAL", "CFSL Kolkata", 60, False),
    ("ksb-02:11", "ksb-02", "DNA", "CFSL Kolkata", 125, False),
    ("ksb-05:12", "ksb-05", "DIGITAL", "CFSL Kolkata", 4, False),
    ("bbz-03:15", "bbz-03", "DIGITAL", "FSL West Bengal, Belgachia", 30, False),
]
for ekey, ckey, ftype, lab, ago, complete in FORENSICS:
    fid = once(f"forensic:{ekey}", "forensics", lambda ekey=ekey, ckey=ckey, ftype=ftype, lab=lab, ago=ago: request(
        "POST", "/forensics", {"evidenceId": EVID[ekey], "caseId": CASE[ckey], "type": ftype, "priority": "HIGH",
                               "submittedDate": iso(days_ago(ago)), "expectedDate": iso(days_ago(ago - 45)),
                               "lab": lab, "status": "PENDING"}, user="inspector"))
    if complete:
        step(f"forensic:{ekey}:complete", lambda fid=fid: request(
            "POST", f"/forensics/{fid}/complete",
            {"findings": "Examination completed; results consistent with the seizure memo.",
             "summary": "Report received and placed in the case diary."}, user="inspector"))

# ------------------------------------------------------ warrants, bail, court --
print("› Warrants, bail and court diary")
WARRANTS = [
    ("bhw-01", "ARREST", "Tapas Naskar", "Alipore CJM Court", "Smt. Madhumita Basu", 140, "Chetla Hat Road", None),
    ("pks-04", "ARREST", "Unknown male identified through CCTV", "City Sessions Court", "Sri Sabyasachi Ghosh", 25, "Ripon Street area", None),
    ("bbz-01", "NBW", "Manish Jalan", "Bankshall Court", "Sri Arijit Dasgupta", 100, "Salt Lake Sector II", None),
    ("ksb-01", "SEARCH", "Premises of Joydeb Sardar, Canning", "Alipore CJM Court", "Smt. Madhumita Basu", 168, None, "EXECUTED"),
    ("pks-03", "SEARCH", "Residence of Subrata Dey, Dum Dum", "Bankshall Court", "Sri Arijit Dasgupta", 45, None, "EXECUTED"),
    ("jdp-02", "SUMMONS", "Smt. Rina Paul (witness)", "Special POCSO Court", "Smt. Chandrima Mitra", 30, None, None),
]
for i, (ckey, wtype, target, court, judge, ago, loc, status) in enumerate(WARRANTS):
    wid = once(f"warrant:{ckey}:{i}", "warrants", lambda ckey=ckey, wtype=wtype, target=target, court=court, judge=judge, ago=ago, loc=loc: request(
        "POST", "/warrants", {"type": wtype, "issuedFor": target, "issuedBy": court, "judgeName": judge,
                              "issuedDate": iso(days_ago(ago)), "validUntil": iso(days_ago(ago - 90)),
                              "charges": next(f[4] for f in FIRS if f[0] == ckey), "priority": "HIGH",
                              "caseId": CASE[ckey], "firId": FIR[ckey], "lastKnownLocation": loc}, user="inspector"))
    if status:
        step(f"warrant:{ckey}:{i}:executed", lambda wid=wid: request(
            "PATCH", f"/warrants/{wid}/status", {"status": "EXECUTED", "executedBy": USER["si"]}, user="inspector"))

BAIL = [
    ("bhw-03:2", "bhw-03", "REGULAR", "Alipore CJM Court", 115, "APPROVED", "Adv. Sumit Chakraborty"),
    ("pks-01:4", "pks-01", "REGULAR", "Bankshall Court", 150, "APPROVED", "Adv. Nilanjana Roy"),
    ("ksb-03:9", "ksb-03", "ANTICIPATORY", "Calcutta High Court", 72, "APPROVED", "Adv. Debangshu Basak"),
    ("bhw-05:3", "bhw-05", "REGULAR", "City Sessions Court", 10, "PENDING", "Adv. Farid Ahmed"),
    ("ksb-01:8", "ksb-01", "REGULAR", "Alipore Judges Court", 120, "REJECTED", "Adv. Partha Sen"),
]
for akey, ckey, btype, court, ago, status, lawyer in BAIL:
    idx = int(akey.split(":")[1])
    bid = once(f"bail:{akey}", "bail", lambda ckey=ckey, btype=btype, court=court, ago=ago, lawyer=lawyer, akey=akey: request(
        "POST", "/bail", {"accusedId": ACC[f"{ckey}:{akey.split(':')[1]}"], "caseId": CASE[ckey], "firId": FIR[ckey],
                          "charges": next(f[4] for f in FIRS if f[0] == ckey), "bailType": btype,
                          "applicationDate": iso(days_ago(ago)), "court": court, "lawyer": lawyer,
                          "proposedBailAmount": 2500000, "status": "PENDING"}, user="inspector"))
    if status != "PENDING":
        body = {"status": status}
        if status == "REJECTED":
            body["reason"] = "Offence grave; accused likely to tamper with witnesses."
        step(f"bail:{akey}:{status}", lambda bid=bid, body=body: request("PATCH", f"/bail/{bid}/status", body, user="inspector"))

# Last numeric field: days in the past (negative = upcoming).
HEARINGS = [
    ("bhw-03", "Framing of charge", "Alipore CJM Court", "12", "Smt. Madhumita Basu", -6, "11:00", "ARGUMENTS", "MEDIUM"),
    ("pks-01", "Evidence of prosecution witness 2", "Bankshall Court", "7", "Sri Arijit Dasgupta", -2, "12:30", "EVIDENCE", "MEDIUM"),
    ("bhw-05", "Bail hearing", "City Sessions Court", "4", "Sri Sabyasachi Ghosh", 0, "11:30", "BAIL_HEARING", "HIGH"),
    ("ksb-01", "Charge-sheet submission", "Alipore CJM Court", "12", "Smt. Madhumita Basu", -12, "10:30", "CHARGESHEET", "HIGH"),
    ("jdp-02", "Remand extension of accused", "Special POCSO Court", "2", "Smt. Chandrima Mitra", -1, "14:00", "REMAND_EXTENSION", "CRITICAL"),
    ("pks-03", "Remand extension", "Bankshall Court", "7", "Sri Arijit Dasgupta", -9, "11:00", "REMAND_EXTENSION", "HIGH"),
    ("ksb-05", "Police custody remand", "Special Court (Cyber)", "1", "Sri Indranil Chatterjee", 3, "12:00", "REMAND_EXTENSION", "HIGH"),
    ("shy-03", "Hearing on bail plea", "City Sessions Court", "4", "Sri Sabyasachi Ghosh", -20, "13:00", "BAIL_HEARING", "MEDIUM"),
    ("bbz-01", "Arguments on NBW recall", "Bankshall Court", "9", "Sri Arijit Dasgupta", -4, "11:30", "ARGUMENTS", "MEDIUM"),
    ("ksb-02", "Evidence of investigating officer", "Alipore Judges Court", "3", "Sri Pradyot Sinha", -15, "12:00", "EVIDENCE", "HIGH"),
]
for i, (ckey, title, court, room, judge, ahead, tm, htype, prio) in enumerate(HEARINGS):
    # Negative values are days ahead (upcoming hearings), positive days past.
    date = (NOW - timedelta(days=ahead)).astimezone(IST)
    once(f"hearing:{ckey}:{i}", "court_hearings", lambda ckey=ckey, title=title, court=court, room=room, judge=judge, date=date, tm=tm, htype=htype, prio=prio: request(
        "POST", "/court/hearings", {"caseId": CASE[ckey], "title": title, "court": court, "courtRoom": room,
                                    "judge": judge, "date": iso(date.replace(hour=0, minute=0)), "time": tm, "type": htype,
                                    "priority": prio, "charges": next(f[4] for f in FIRS if f[0] == ckey),
                                    "ioId": USER[next(c[2] for c in CASES if c[0] == ckey)]}, user="inspector"))

ORDERS = [
    ("bhw-03", 110, "BAIL_GRANTED", "Accused enlarged on bail of ₹25,000 with two sureties; to appear on every date.", "Alipore CJM Court", "Smt. Madhumita Basu"),
    ("ksb-01", 118, "BAIL_REJECTED", "Bail prayer rejected considering the gravity of the offence and recovery pending.", "Alipore Judges Court", "Sri Pradyot Sinha"),
    ("jdp-02", 90, "REMAND", "Accused remanded to police custody for seven days.", "Special POCSO Court", "Smt. Chandrima Mitra"),
    ("pks-03", 38, "DIRECTIONS", "Investigating officer directed to obtain the bank's certificate under BSA s.63.", "Bankshall Court", "Sri Arijit Dasgupta"),
    ("ksb-03", 70, "BAIL_GRANTED", "Anticipatory bail granted on condition of cooperating with the investigation.", "Calcutta High Court", "Justice (fictional) R. K. Sen"),
]
for i, (ckey, ago, otype, summary, court, judge) in enumerate(ORDERS):
    once(f"order:{ckey}:{i}", "court_orders", lambda ckey=ckey, ago=ago, otype=otype, summary=summary, court=court, judge=judge: request(
        "POST", "/court/orders", {"caseId": CASE[ckey], "orderDate": iso(days_ago(ago)), "orderType": otype,
                                  "summary": summary, "court": court, "judgeName": judge}, user="inspector"))

# ------------------------------------------------------ investigation (Phase 01) --
print("› Investigation workspaces")
WORKSPACES = [
    ("bhw-05", "Bhowanipore Metro courier robbery", "ভবানীপুর মেট্রোতে কুরিয়ার ডাকাতি", "Robbery (BNS 309)", "inspector", "sho"),
    ("ksb-05", "Kasba investment app fraud", "কসবা বিনিয়োগ অ্যাপ প্রতারণা", "Cheating by personation (IT Act 66D, BNS 318)", "si.ksb", "oc.ksb"),
    ("jdp-02", "Jadavpur minor kidnapping", "যাদবপুরে নাবালিকা অপহরণ", "Kidnapping (BNS 137)", "si.jdp", "oc.jdp"),
    ("ksb-02", "EM Bypass fatal hit-and-run", "ইএম বাইপাসে প্রাণঘাতী দুর্ঘটনা", "Causing death by negligence (BNS 106)", "si.ksb", "oc.ksb"),
]
WS = {}
for ckey, title, title_bn, offence, io, sup in WORKSPACES:
    stn = next(f[1] for f in FIRS if f[0] == ckey)
    case_number = psql(f"SELECT case_number FROM cases WHERE id = '{CASE[ckey]}'")
    WS[ckey] = once(f"workspace:{ckey}", "investigation_workspaces", lambda ckey=ckey, title=title, title_bn=title_bn, offence=offence, io=io, sup=sup, stn=stn, case_number=case_number: request(
        "POST", "/investigation", {"caseNumber": case_number, "firId": FIR[ckey], "caseId": CASE[ckey], "title": title,
                                   "titleBn": title_bn, "offence": offence, "sections": next(f[4] for f in FIRS if f[0] == ckey),
                                   "stationId": STATION[stn], "ioId": USER[io], "supervisorId": USER[sup],
                                   "priority": "high"}, user=io))

ws = WS["bhw-05"]
for i, (name, role, age, phone, vehicles) in enumerate([
        ("Sunny Mallick", "accused", 29, "9830000701", ["WB-02-AK-7719"]),
        ("Arnab Kar", "witness", 41, "9830000702", []),
        ("Courier victim, Sri Deepak Singh", "victim", 26, "9830000703", []),
        ("Imtiaz Ansari", "suspect", 31, None, ["WB-02-AK-7719"])]):
    once(f"ws:bhw-05:person:{i}", "investigation_persons", lambda name=name, role=role, age=age, phone=phone, vehicles=vehicles: request(
        "POST", f"/investigation/{ws}/persons", {"name": name, "role": role, "age": age, "phone": phone, "vehicles": vehicles}, user="inspector"))
fir_time = days_ago(21, 22, 35)
for i, (offset, title, loc, kind) in enumerate([
        (-40, "Courier leaves Esplanade office with cash bag", "Esplanade, Kolkata", "movement"),
        (0, "Robbery at knife-point near Metro gate 2", "Bhowanipore Metro gate 2", "incident"),
        (6, "Call to 112 by passer-by", "Bhowanipore", "report"),
        (95, "Scooter seen on Tiljala Road CCTV", "Tiljala Road", "detection"),
        (3000, "Accused arrested at Tiljala", "Tiljala, Kolkata 700039", "report")]):
    once(f"ws:bhw-05:timeline:{i}", "investigation_timeline", lambda offset=offset, title=title, loc=loc, kind=kind: request(
        "POST", f"/investigation/{ws}/timeline",
        {"occurredAt": (fir_time + timedelta(minutes=offset)).strftime("%Y-%m-%dT%H:%M"), "title": title, "location": loc,
         "kind": kind}, user="inspector"))
for i, (title, assignee, due, prio) in enumerate([
        ("Collect Metro Railway CCTV for gate 2 and 3", "si", 3, "high"),
        ("Record statement of courier supervisor", "asi", 7, "medium"),
        ("Trace the scooter WB-02-AK-7719 through RTO", "si", -2, "high")]):
    once(f"ws:bhw-05:task:{i}", "investigation_tasks", lambda title=title, assignee=assignee, due=due, prio=prio: request(
        "POST", f"/investigation/{ws}/tasks",
        {"title": title, "assigneeId": USER[assignee], "dueDate": (NOW + timedelta(days=due)).astimezone(IST).strftime("%Y-%m-%d"),
         "priority": prio}, user="inspector"))
for ekey in ["bhw-05:3", "bhw-05:4"]:
    step(f"ws:bhw-05:evidence:{ekey}", lambda ekey=ekey: request(
        "POST", f"/investigation/{ws}/evidence", {"evidenceId": EVID[ekey], "note": "Linked from the evidence register"},
        user="inspector", ok=(200, 201, 204, 409)))

ws2 = WS["ksb-05"]
for i, (name, role) in enumerate([("Rakesh Verma", "accused"), ("Complainant, Smt. Sulagna Das", "victim")]):
    once(f"ws:ksb-05:person:{i}", "investigation_persons", lambda name=name, role=role: request(
        "POST", f"/investigation/{ws2}/persons", {"name": name, "role": role}, user="si.ksb"))
step("ws:ksb-05:evidence", lambda: request(
    "POST", f"/investigation/{ws2}/evidence", {"evidenceId": EVID["ksb-05:12"]}, user="si.ksb", ok=(200, 201, 204, 409)))

# ------------------------------------------------------------------ alerts --
print("› Alerts")
ALERTS = [
    ("BOLO", "STATION", "BOLO: grey scooter WB-02-AK-7719", "Scooter used in the Bhowanipore Metro courier robbery. Stop and inform Bhowanipore PS.", 1, 30, "BHW", "sho"),
    ("URGENT", "DISTRICT", "Chain snatching pattern, South Division", "Three chain snatchings by two men on a scooter between 20:00 and 22:00 near Hazra and Kalighat.", 2, 14, "BHW", "dsp"),
    ("NOTICE", "STATION", "Durga Puja crowd management briefing", "All beat staff to attend the pandal traffic briefing at Park Street PS at 16:00.", 3, 10, "PKS", "oc.pks"),
    ("FLASH", "STATE", "Missing minor girl, Jadavpur", "Minor girl, 14, last seen near Jadavpur University gate 4 in navy school uniform. See lookout register.", 1, 20, "JDP", "oc.jdp"),
    ("NOTICE", "DISTRICT", "Cyber fraud advisory: fake investment apps", "Rise in complaints about investment apps promising daily returns. Register on NCRP and freeze beneficiary accounts promptly.", 3, 60, "KSB", "dsp"),
]
for i, (atype, scope, title, desc, prio, days_valid, stn, user) in enumerate(ALERTS):
    aid = once(f"alert:{i}", "alerts", lambda atype=atype, scope=scope, title=title, desc=desc, prio=prio, days_valid=days_valid, stn=stn, user=user: request(
        "POST", "/alerts", {"type": atype, "scope": scope, "title": title, "description": desc, "priority": prio,
                            "expiresAt": iso(NOW + timedelta(days=days_valid)), "stationId": STATION[stn]}, user=user))
    if i in (2, 4):
        step(f"alert:{i}:ack", lambda aid=aid: request("POST", f"/alerts/{aid}/acknowledge", {}, user="si"))

# ----------------------------------------------------------------- lookouts --
print("› Lookout notices and sightings")
LOOKOUTS = [
    ("WANTED", "Tapas Naskar", "Absconding accused in the Hazra Road chain snatching. Lean build, about 5'6\".", {"Age": "24", "Last seen": "Chetla Hat Road", "Warrant": "Alipore CJM Court"}, "HIGH", "bhw-01"),
    ("STOLEN_VEHICLE", "Honda Activa WB-06-AX-3304", "Two-wheeler stolen from Acropolis Mall parking.", {"Colour": "Matte grey", "Registration": "WB-06-AX-3304"}, "NORMAL", "ksb-04"),
    ("SUSPECT", "Unknown male in the Free School Street assault", "Male, 30–35, black T-shirt, identified on CCTV near the bus stop.", {"Height": "about 5'8\"", "Clothing": "Black T-shirt, jeans"}, "HIGH", "pks-04"),
    ("WANTED", "Manish Jalan", "Accused in the Burrabazar textile consignment breach of trust; NBW issued.", {"Age": "47", "Last known address": "Salt Lake Sector II"}, "NORMAL", "bbz-01"),
]
LO = {}
for i, (ltype, subject, desc, details, prio, fkey) in enumerate(LOOKOUTS):
    LO[i] = once(f"lookout:{i}", "lookouts", lambda ltype=ltype, subject=subject, desc=desc, details=details, prio=prio, fkey=fkey: request(
        "POST", "/lookouts", {"type": ltype, "subject": subject, "description": desc, "details": details,
                              "priority": prio, "firId": FIR[fkey]}, user="inspector"))
s1 = once("lookout:0:sighting:0", "lookout_sightings", lambda: request(
    "POST", f"/lookouts/{LO[0]}/sightings", {"location": "Chetla Bridge", "latitude": 22.5195, "longitude": 88.3353,
                                            "sightedAt": iso(NOW - timedelta(days=4, hours=3)), "details": "Seen near the bridge in a red jacket."}, user="constable"))
step("lookout:0:sighting:0:verify", lambda: request("POST", f"/lookouts/{LO[0]}/sightings/{s1}/verify", {}, user="asi"))
once("lookout:1:sighting:0", "lookout_sightings", lambda: request(
    "POST", f"/lookouts/{LO[1]}/sightings", {"location": "Ruby crossing service road", "sightedAt": iso(NOW - timedelta(days=2)),
                                            "details": "Matching scooter parked; plate partly covered."}, user="asi"))

# ----------------------------------------------------------------- armoury --
print("› Armoury")
WEAPONS = [("9mm Pistol", "Glock 17", "KP-AR-G17-0412"), ("9mm Pistol", "Glock 17", "KP-AR-G17-0418"),
           ("Revolver .38", "Indian Ordnance .38", "KP-AR-R38-1103"), ("5.56mm Rifle", "INSAS", "KP-AR-INS-2201"),
           ("5.56mm Rifle", "INSAS", "KP-AR-INS-2207"), ("7.62mm Rifle", "SLR", "KP-AR-SLR-3304"),
           ("9mm Carbine", "MP5", "KP-AR-MP5-4410"), ("Tear gas gun", "1.5 inch riot gun", "KP-AR-TGG-5102"),
           ("9mm Pistol", "Glock 17", "KP-AR-G17-0431"), ("Revolver .38", "Indian Ordnance .38", "KP-AR-R38-1117")]
WPN = {}
for wtype, make, serial in WEAPONS:
    WPN[serial] = once(f"weapon:{serial}", "weapons", lambda wtype=wtype, make=make, serial=serial: request(
        "POST", "/armoury/weapons", {"type": wtype, "make": make, "serialNumber": serial, "stationId": STATION["BHW"]}, user="sho"))
for serial, officer, purpose, rounds, back, returned_rounds, condition in [
        ("KP-AR-G17-0412", "si", "Night patrol, Hazra–Kalighat sector", 10, True, 10, "SERVICEABLE"),
        ("KP-AR-INS-2201", "asi", "Durga Puja pandal security duty", 20, True, 18, "SERVICEABLE"),
        ("KP-AR-R38-1103", "inspector", "Raid at Tiljala (courier robbery)", 6, True, 6, "UNDER_REPAIR"),
        ("KP-AR-G17-0418", "hc", "VIP route bandobast, AJC Bose Road", 10, False, None, None),
        ("KP-AR-MP5-4410", "si", "Escort of accused to City Sessions Court", 15, False, None, None)]:
    step(f"weapon:{serial}:issue", lambda serial=serial, officer=officer, purpose=purpose, rounds=rounds, back=back: request(
        "POST", f"/armoury/weapons/{WPN[serial]}/issue",
        {"issuedTo": USER[officer], "purpose": purpose, "roundsIssued": rounds,
         "expectedReturn": iso(NOW + timedelta(hours=-6 if back else 8))}, user="asi"))
    if back:
        body = {"roundsReturned": returned_rounds, "condition": condition}
        if condition != "SERVICEABLE":
            body["note"] = "Trigger mechanism stiff; sent for armourer's inspection."
        elif returned_rounds < rounds:
            body["note"] = "Two rounds fired in the air during crowd control; incident report filed."
        step(f"weapon:{serial}:return", lambda serial=serial, body=body: request(
            "POST", f"/armoury/weapons/{WPN[serial]}/return", body, user="asi"))

# ---------------------------------------------------------- missing persons --
print("› Missing and vulnerable persons")
MISSING = [
    ("Riya Halder", 14, "FEMALE", "Raja S C Mallick Road near Jadavpur University gate 4", 95, "Mother", "Navy school uniform, red backpack", "Mole on left cheek", "si.jdp"),
    ("Haripada Mondal", 78, "MALE", "Sealdah station platform 9", 12, "Son", "White dhoti and brown shawl", "Walks with a stick; hearing impaired", "si"),
    ("Shabnam Khatun", 19, "FEMALE", "Park Circus Seven Point crossing", 6, "Elder brother", "Green salwar kameez", "Scar on right wrist", "si.pks"),
]
for i, (name, age, gender, loc, ago, relation, wearing, marks, officer) in enumerate(MISSING):
    seen = days_ago(ago, 17, 30)
    body = {"personName": name, "age": age, "gender": gender, "lastSeenLocation": loc, "lastSeenAt": iso(seen),
            "reporterName": f"{relation} of {name.split()[0]}", "reporterPhone": f"98300{(800 + i):05d}",
            "reporterRelation": relation, "lastSeenWearing": wearing, "identifyingMarks": marks}
    if age >= 75:
        body["vulnerabilities"] = ["ELDERLY", "DISABILITY"]
    if i == 2:
        body["vulnerabilities"] = ["TRAFFICKING_RISK"]
    mid = once(f"missing:{i}", "missing_person_reports", lambda body=body, officer=officer: request("POST", "/missing-persons", body, user=officer))
    step(f"missing:{i}:start", lambda mid=mid, officer=officer: request("POST", f"/missing-persons/{mid}/start-search", {}, user=officer, ok=(200, 201, 204, 409)))
    # A child's report is restricted to SI and above and the officers working it,
    # so its follow-up is recorded by the registering SI and verified by the OC.
    recorder, verifier = (officer, "oc.jdp") if age < 18 else ("constable", "asi")
    step(f"missing:{i}:checklist", lambda mid=mid, verifier=verifier: request(
        "POST", f"/missing-persons/{mid}/checklist/REGISTER_ENTRY/complete", {"note": "Entered in the missing persons register"}, user=verifier))
    sid = once(f"missing:{i}:sighting", "missing_sightings", lambda mid=mid, seen=seen, i=i, recorder=recorder: request(
        "POST", f"/missing-persons/{mid}/sightings",
        {"source": "CCTV_REVIEW" if i != 1 else "PUBLIC_TIP", "location": ["Garia station approach", "Howrah station subway", "Rajabazar crossing"][i],
         "sightedAt": iso(seen + timedelta(hours=3)), "details": "Reported by control room after footage review" if i != 1 else "Tea stall owner called 112"}, user=recorder))
    step(f"missing:{i}:sighting:verify", lambda mid=mid, sid=sid, verifier=verifier: request(
        "POST", f"/missing-persons/{mid}/sightings/{sid}/verify", {}, user=verifier))
    once(f"missing:{i}:family", "missing_family_contacts", lambda mid=mid, relation=relation, recorder=recorder: request(
        "POST", f"/missing-persons/{mid}/family-contacts",
        {"direction": "OUTBOUND", "channel": "PHONE", "contactName": relation, "summary": "Updated family on the verified sighting and next steps.",
         "contactedAt": iso(NOW - timedelta(days=1))}, user=recorder))
    if i == 1:
        step("missing:1:close", lambda mid=mid: request(
            "POST", f"/missing-persons/{mid}/close", {"outcome": "TRACED", "note": "Found at Howrah station subway and handed over to his son."}, user="si"))

# -------------------------------------------------------------- cyber fraud --
print("› Cyber fraud complaints")
CYBER = [
    ("Sulagna Das", "ONLINE_FRAUD", "BANKING", "Fake investment app 'KolkataGrowth Pro' promised daily returns; paid through UPI in six instalments.", 43000000, "ksb-05"),
    ("Abhijit Guha", "ONLINE_FRAUD", "MESSAGING", "WhatsApp group posing as a stock tips channel took ₹1.2 lakh as 'membership fee'.", 12000000, None),
    ("Tanima Sarkar", "IDENTITY_THEFT", "SOCIAL_MEDIA", "Impersonation account used her photos to seek money from relatives.", 1500000, None),
    ("Rafiqul Islam", "ONLINE_FRAUD", "ECOMMERCE", "Paid for a second-hand motorcycle on a classifieds site; seller vanished.", 5500000, None),
]
CY = {}
for i, (name, ctype, platform, desc, loss, fkey) in enumerate(CYBER):
    body = {"type": ctype, "priority": "HIGH" if loss > 10000000 else "MEDIUM", "incidentDate": iso(days_ago(8 + i * 9)),
            "platform": platform, "incidentDescription": desc, "complainantName": name,
            "complainantPhone": f"+91 98300 {(900 + i):05d}", "ncrpReference": f"31220260{(40000 + i):05d}",
            "helplineReference": f"1930-KOL-{(5200 + i)}", "reportedLossPaise": loss}
    CY[i] = once(f"cyber:{i}", "cyber_crimes", lambda body=body: request("POST", "/cyber-crime", body, user="si.ksb"))

cid = CY[0]
ent = {}
for key, body in [
        ("victim", {"type": "BANK_ACCOUNT", "value": "30112004567", "ifsc": "SBIN0001234", "provider": "State Bank of India, Kasba branch", "role": "VICTIM_OWN"}),
        ("upi", {"type": "UPI", "value": "kolkatagrowth.pay@okaxis", "role": "BENEFICIARY"}),
        ("mule", {"type": "BANK_ACCOUNT", "value": "50200098761234", "ifsc": "HDFC0002077", "provider": "HDFC Bank, Howrah branch", "role": "BENEFICIARY"}),
        ("phone", {"type": "PHONE", "value": "9830077123", "role": "SUSPECT_CONTACT"}),
        ("url", {"type": "URL", "value": "https://kolkatagrowth-pro.example", "role": "SUSPECT_CONTACT"})]:
    ent[key] = once(f"cyber:0:entity:{key}", "fraud_entities", lambda body=body: request("POST", f"/cyber-crime/{cid}/entities", body, user="si.ksb"))
# The same beneficiary UPI appears in the second complaint, linking them in the network.
once("cyber:1:entity:upi", "fraud_entities", lambda: request(
    "POST", f"/cyber-crime/{CY[1]}/entities", {"type": "UPI", "value": "kolkatagrowth.pay@okaxis", "role": "BENEFICIARY"}, user="si.ksb"))


def entity_id(key):
    return psql(f"SELECT entity_id FROM complaint_entities WHERE id = '{ent[key]}'") or ent[key]


for i, (src, dst, amount, ago_h) in enumerate([("victim", "upi", 25000000, 200), ("victim", "upi", 18000000, 150), ("upi", "mule", 42000000, 140)]):
    once(f"cyber:0:txn:{i}", "fraud_transactions", lambda src=src, dst=dst, amount=amount, ago_h=ago_h, i=i: request(
        "POST", f"/cyber-crime/{cid}/transactions",
        {"fromEntityId": entity_id(src), "toEntityId": entity_id(dst), "amountPaise": amount,
         "reference": f"UTR4263{(8100 + i)}", "occurredAt": iso(NOW - timedelta(hours=ago_h))}, user="si.ksb"))
fz = once("cyber:0:freeze", "freeze_requests", lambda: request(
    "POST", f"/cyber-crime/{cid}/freeze-requests",
    {"entityId": entity_id("mule"), "addressee": "Nodal Officer, HDFC Bank (Cyber Cell), Kolkata",
     "amountRequestedPaise": 42000000, "grounds": "Account received proceeds of investment fraud (UTR42638102)."}, user="oc.ksb"))
step("cyber:0:freeze:send", lambda: request("POST", f"/cyber-crime/{cid}/freeze-requests/{fz}/transition",
                                            {"action": "send", "sentVia": "Email to bank nodal officer"}, user="oc.ksb"))
step("cyber:0:freeze:ack", lambda: request("POST", f"/cyber-crime/{cid}/freeze-requests/{fz}/transition",
                                           {"action": "acknowledge", "acknowledgementRef": "HDFC/CYB/KOL/2026/1187"}, user="oc.ksb"))
step("cyber:0:freeze:frozen", lambda: request("POST", f"/cyber-crime/{cid}/freeze-requests/{fz}/transition",
                                              {"action": "frozen", "amountFrozenPaise": 31000000}, user="oc.ksb"))
once("cyber:0:recovery", "fraud_recoveries", lambda: request(
    "POST", f"/cyber-crime/{cid}/recoveries",
    {"amountPaise": 12000000, "recoveredOn": iso(NOW - timedelta(days=1)), "freezeRequestId": fz,
     "reference": "Refund under order of the Special Court (Cyber)"}, user="oc.ksb"))

# ---------------------------------------------------------- traffic incidents --
print("› Traffic incidents")
T0 = days_ago(130, 17, 42)
ti = once("traffic:0", "traffic_incidents", lambda: request(
    "POST", "/traffic-incidents",
    {"occurredAt": iso(T0), "location": "EM Bypass, Ruby crossing, Kolkata 700107", "latitude": 22.5147, "longitude": 88.4017,
     "firId": FIR["ksb-02"], "collisionType": "PEDESTRIAN", "roadCondition": "DRY", "weather": "CLEAR", "lighting": "DARK_LIT",
     "description": "Pedestrian crossing the service road was hit by a car that did not stop."}, user="asi"))
va = once("traffic:0:vehicle", "traffic_vehicles", lambda: request(
    "POST", f"/traffic-incidents/{ti}/vehicles", {"registrationNumber": "WB-06-BC-2210", "vehicleType": "CAR"}, user="asi"))
once("traffic:0:person", "traffic_persons", lambda: request(
    "POST", f"/traffic-incidents/{ti}/persons", {"name": "Pedestrian (identified later)", "role": "PEDESTRIAN", "injurySeverity": "FATAL", "hospital": "Calcutta Medical College and Hospital"}, user="asi"))
once("traffic:0:camera", "traffic_cameras", lambda: request(
    "POST", f"/traffic-incidents/{ti}/cameras",
    {"cameraRef": "KTP-EMB-044", "cameraName": "EM Bypass, Ruby crossing (Kolkata Traffic Police)", "distanceM": 40,
     "footageFrom": iso(T0 - timedelta(minutes=10)), "footageTo": iso(T0 + timedelta(minutes=10))}, user="asi"))
once("traffic:0:plate", "traffic_plate_reads", lambda: request(
    "POST", f"/traffic-incidents/{ti}/plate-reads",
    {"registrationNumber": "WB 06 BC 2210", "readAt": iso(T0 - timedelta(seconds=35)), "location": "Ruby crossing northbound",
     "cameraRef": "KTP-EMB-044", "source": "ANPR_SYSTEM", "sourceDetail": "Kolkata Traffic Police ANPR export"}, user="asi"))
once("traffic:0:fact:measured", "traffic_facts", lambda: request(
    "POST", f"/traffic-incidents/{ti}/facts",
    {"occurredAt": iso(T0), "description": "Impact recorded on camera", "provenance": "MEASURED", "source": "KTP-EMB-044 frame timestamp", "vehicleId": va}, user="asi"))
once("traffic:0:fact:estimated", "traffic_facts", lambda: request(
    "POST", f"/traffic-incidents/{ti}/facts",
    {"occurredAt": iso(T0), "description": "Speed of the car before impact", "provenance": "ESTIMATED", "source": "KTP-EMB-044",
     "method": "Distance between lane markings over frame count at 25 fps", "vehicleId": va, "quantity": "SPEED", "valueLow": 58, "valueHigh": 66}, user="asi"))
once("traffic:0:fact:observed", "traffic_facts", lambda: request(
    "POST", f"/traffic-incidents/{ti}/facts",
    {"occurredAt": iso(T0 - timedelta(seconds=4)), "description": "Car changed lanes without slowing at the crossing", "provenance": "OBSERVED",
     "source": "Statement of auto driver Sri Ratan Mondal"}, user="asi"))
rep = once("traffic:0:report", "traffic_reports", lambda: request("POST", f"/traffic-incidents/{ti}/reports", {"findings": ""}, user="asi"))
step("traffic:0:report:findings", lambda: request(
    "PUT", f"/traffic-incidents/{ti}/reports/{rep}",
    {"findings": "The car WB06BC2210 crossed Ruby junction without slowing; estimated speed 58–66 km/h from camera calibration (estimate, not measured)."}, user="asi"))
step("traffic:0:report:submit", lambda: request("POST", f"/traffic-incidents/{ti}/reports/{rep}/submit", {}, user="asi"))
step("traffic:0:report:approve", lambda: request("POST", f"/traffic-incidents/{ti}/reports/{rep}/approve", {}, user="si.ksb"))
once("traffic:1", "traffic_incidents", lambda: request(
    "POST", "/traffic-incidents",
    {"occurredAt": iso(days_ago(11, 8, 50)), "location": "Park Circus Seven Point crossing", "latitude": 22.5405, "longitude": 88.3662,
     "collisionType": "SIDE_IMPACT", "roadCondition": "WET", "weather": "RAIN", "lighting": "DAYLIGHT",
     "description": "Bus and taxi collided at the crossing during rain; two passengers injured."}, user="asi.pks"))

# ------------------------------------------------------------------ dispatch --
print("› Dispatch incidents")
D1 = once("dispatch:0", "dispatch_incidents", lambda: request(
    "POST", "/dispatch/incidents",
    {"source": "PHONE_112", "callerName": "Rina Das", "callerPhone": "98300 00950",
     "description": "Two men snatched a chain and fled towards Kalighat on a scooter", "locationText": "Hazra Road & Sarat Bose Road",
     "latitude": 22.5250, "longitude": 88.3500, "receivedAt": iso(NOW - timedelta(hours=5))}, user="asi"))
step("dispatch:0:classify", lambda: request("POST", f"/dispatch/incidents/{D1}/classify", {"incidentType": "Chain snatching", "severity": "HIGH"}, user="asi"))
A1 = once("dispatch:0:assign", "dispatch_assignments", lambda: request(
    "POST", f"/dispatch/incidents/{D1}/assign", {"kind": "VEHICLE", "id": VEH["WB-01-PC-4102"]}, user="asi"))
step("dispatch:0:ack", lambda: request("POST", f"/dispatch/assignments/{A1}/acknowledge", {}, user="asi"))
step("dispatch:0:scene", lambda: request("POST", f"/dispatch/assignments/{A1}/on-scene", {}, user="asi"))
step("dispatch:0:clear", lambda: request("POST", f"/dispatch/assignments/{A1}/clear", {}, user="asi"))
step("dispatch:0:close", lambda: request("POST", f"/dispatch/incidents/{D1}/close",
                                         {"outcome": "RESOLVED_ON_SCENE", "note": "Victim assisted; FIR registered at Bhowanipore PS."}, user="asi"))
D2 = once("dispatch:1", "dispatch_incidents", lambda: request(
    "POST", "/dispatch/incidents",
    {"source": "PHONE_100", "callerName": "Gariahat market trader", "callerPhone": "98300 00951",
     "description": "Crowd gathered after a scuffle between shoppers and a hawker", "locationText": "Gariahat Market, Kolkata 700019",
     "latitude": 22.5183, "longitude": 88.3660, "receivedAt": iso(NOW - timedelta(minutes=25))}, user="asi"))
step("dispatch:1:classify", lambda: request("POST", f"/dispatch/incidents/{D2}/classify", {"incidentType": "Affray", "severity": "MEDIUM"}, user="asi"))
once("dispatch:2", "dispatch_incidents", lambda: request(
    "POST", "/dispatch/incidents",
    {"source": "CONTROL_ROOM", "description": "Unattended bag reported on platform 2", "locationText": "Esplanade Metro, platform 2",
     "latitude": 22.5646, "longitude": 88.3512, "receivedAt": iso(NOW - timedelta(minutes=8))}, user="asi"))
once("dispatch:3", "dispatch_incidents", lambda: request(
    "POST", "/dispatch/incidents",
    {"source": "WALK_IN", "description": "Elderly man reports his wallet lost on the bus", "locationText": "Bhowanipore PS counter"}, user="constable"))

# --------------------------------------------------------- citizen complaints --
print("› Citizen complaints")
pub = once("complaint:public:bn", "citizen_complaints", lambda: request(
    "POST", "/public/complaints",
    {"category": "TRAFFIC", "subject": "উল্টাডাঙ্গা মোড়ে রাতে লরি রাস্তা আটকে রাখে",
     "description": "প্রতি রাতে উল্টাডাঙ্গা মোড়ের কাছে লরিগুলি রাস্তা আটকে দাঁড়িয়ে থাকে, অ্যাম্বুলেন্স যেতে পারে না। অনুগ্রহ করে ব্যবস্থা নিন।",
     "complainantName": "সুমনা ভট্টাচার্য", "complainantPhone": "+91 98300 00961", "incidentLocation": "Ultadanga crossing"}, auth=False))
once("complaint:public:en", "citizen_complaints", lambda: request(
    "POST", "/public/complaints",
    {"category": "NOISE_COMPLAINT", "subject": "Loudspeakers past midnight near Deshapriya Park",
     "description": "Loudspeakers are played well past midnight near Deshapriya Park on weekends, disturbing elderly residents.",
     "complainantName": "Ananya Bose", "complainantPhone": "+91 98300 00962", "incidentLocation": "Deshapriya Park, Kolkata 700029"}, auth=False))
cc = once("complaint:counter", "citizen_complaints", lambda: request(
    "POST", "/complaints",
    {"channel": "COUNTER", "category": "THEFT", "subject": "Bicycle stolen outside Park Street Metro gate 2",
     "description": "My bicycle was stolen from outside Park Street Metro gate 2 between 9 am and 6 pm.",
     "complainantName": "Soumik Dey", "complainantPhone": "9830000963"}, user="constable"))
wa = once("complaint:whatsapp", "citizen_complaints", lambda: request(
    "POST", "/complaints",
    {"channel": "WHATSAPP", "sourceReference": "WA-KP-2026-0412", "category": "OTHER", "subject": "Street lights not working, Lake Gardens",
     "description": "Street lights on Lake Gardens Road have not worked for two weeks; women feel unsafe walking at night.",
     "complainantName": "Piyali Chakraborty", "complainantPhone": "9830000964"}, user="asi"))
step("complaint:counter:categorise", lambda: request("POST", f"/complaints/{cc}/categorise", {"category": "THEFT", "priority": "NORMAL"}, user="asi"))
step("complaint:counter:route", lambda: request("POST", f"/complaints/{cc}/route",
                                                {"stationId": STATION["PKS"], "unit": "Detective Department liaison", "reason": "Location falls under Park Street PS"}, user="asi"))
step("complaint:counter:progress", lambda: request("POST", f"/complaints/{cc}/status", {"status": "IN_PROGRESS", "note": "CCTV at the Metro gate requested"}, user="asi"))
resp = once("complaint:counter:response", "complaint_responses", lambda: request(
    "POST", f"/complaints/{cc}/responses",
    {"body": "Your complaint has been registered at Park Street PS. CCTV footage has been requested and you will be informed of progress."}, user="asi"))
step("complaint:counter:approve", lambda: request("POST", f"/complaints/{cc}/responses/{resp}/review", {"approve": True}, user="si"))
step("complaint:whatsapp:categorise", lambda: request("POST", f"/complaints/{wa}/categorise", {"category": "OTHER", "priority": "HIGH"}, user="asi"))

# -------------------------------------------------------------------- CCTV --
print("› CCTV register and events")
CAMERAS = [
    ("KP-CAM-PKC-01", "Park Circus Seven Point crossing", 22.5405, 88.3662, "KP"),
    ("KP-CAM-GRH-01", "Gariahat crossing (Rashbehari Avenue)", 22.5183, 88.3660, "KP"),
    ("KP-CAM-ESP-01", "Esplanade crossing (Lenin Sarani)", 22.5646, 88.3512, "KP"),
    ("KP-CAM-HZR-01", "Hazra crossing", 22.5245, 88.3490, "KP"),
    ("KP-CAM-RBY-01", "EM Bypass, Ruby crossing", 22.5147, 88.4017, "KP"),
    ("KP-CAM-SHB-01", "Shyambazar five-point crossing", 22.6012, 88.3730, "KP"),
    ("KMC-CAM-NMK-03", "New Market, Lindsay Street", 22.5590, 88.3510, "KMC"),
    ("PVT-CAM-ACR-02", "Acropolis Mall parking entry", 22.5140, 88.3930, "PRIVATE"),
]
CAM = {}
for code, name, lat, lng, owner in CAMERAS:
    CAM[code] = once(f"camera:{code}", "cameras", lambda code=code, name=name, lat=lat, lng=lng, owner=owner: request(
        "POST", "/video/cameras", {"code": code, "name": name, "location": f"{name}, Kolkata", "latitude": lat, "longitude": lng,
                                   "ownerAgency": owner, "retentionClass": "STANDARD" if owner != "PRIVATE" else "SHORT"}, user="sho"))
ev = once("camera:event:0", "video_events", lambda: request(
    "POST", "/video/events", {"cameraId": CAM["KP-CAM-HZR-01"], "eventType": "SUSPICIOUS_ACTIVITY", "severity": "high",
                              "occurredAt": iso(NOW - timedelta(days=3, hours=2)),
                              "description": "Two men on a grey scooter circling the crossing repeatedly before 21:00."}, user="constable"))
step("camera:event:0:confirm", lambda: request("POST", f"/video/events/{ev}/triage", {"decision": "CONFIRMED", "note": "Matches the chain snatching pattern alert"}, user="si"))
once("camera:event:1", "video_events", lambda: request(
    "POST", "/video/events", {"cameraId": CAM["KP-CAM-ESP-01"], "eventType": "ABANDONED_OBJECT", "severity": "critical",
                              "occurredAt": iso(NOW - timedelta(hours=1)), "description": "Unattended black bag near the tram depot kiosk."}, user="asi"))

# -------------------------------------------------------------------- risk --
print("› Risk beats")
BEATS = [("BHW", "Hazra–Kalighat beat", 22.5233, 88.3496, 800), ("PKS", "Park Street–Mullick Bazar beat", 22.5510, 88.3620, 700),
         ("KSB", "Ruby–Kasba beat", 22.5150, 88.3990, 900), ("GRH", "Gariahat market beat", 22.5183, 88.3660, 600)]
BEAT = {}
for stn, name, lat, lng, radius in BEATS:
    officer = "admin"
    BEAT[stn] = once(f"beat:{stn}", "risk_beats", lambda stn=stn, name=name, lat=lat, lng=lng, radius=radius, officer=officer: request(
        "POST", "/risk/beats", {"stationId": STATION[stn], "name": name, "latitude": lat, "longitude": lng, "radiusMeters": radius}, user=officer))
for stn, keys in [("BHW", ["bhw-01", "bhw-05", "bhw-03"]), ("PKS", ["pks-01", "pks-04"]), ("KSB", ["ksb-02", "ksb-04"]), ("GRH", ["grh-01", "grh-03"])]:
    for k in keys:
        step(f"placement:{k}", lambda k=k, stn=stn: request(
            "POST", "/risk/placements", {"firId": FIR[k], "beatId": BEAT[stn], "note": "Placed by station officer from the incident location"},
            user="admin", ok=(200, 201, 204, 409)))

# ------------------------------------------------------------------ report --
print()
if created:
    print("Created this run:")
    for entity, count in sorted(created.items()):
        print(f"  {entity:28s} {count}")
else:
    print("Nothing new — every demo record was already present.")
print("\nDemo accounts (password Demo@123): admin, sp, dsp, sho, inspector, si, asi, hc, constable,")
print("  oc.pks, si.pks, asi.pks, oc.jdp, si.jdp, oc.ksb, si.ksb, oc.tlg, si.tlg, oc.shy, si.shy, oc.bbz, si.bbz, si.grh")
