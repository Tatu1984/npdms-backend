#!/usr/bin/env python3
"""Give the other three departments enough reference data to be demonstrated.

Migration 000082 created the four forces — Kolkata Police, West Bengal Police,
Kolkata Traffic Police and the CID — and backfilled every existing station,
division and officer to Kolkata Police, because that is all the platform has
ever held. This script adds the other three: West Bengal Police districts and
thanas, CID units, traffic guards, and FICTIONAL officers for each so all four
can actually be signed into.

What is real and what is not, stated plainly:

  Real     district, thana, unit and traffic-guard names; the ranges and zones
           they sit under; headquarters towns.
  Illustrative  station telephone numbers and approximate coordinates.
  Fictional     every officer — name, badge number, email address, telephone
                number and password. So is every district's named SP.

Every account here shares the demo password Demo@123. Remove these accounts, or
rotate every password, before the platform holds real case data.

Written as SQL rather than API calls, unlike scripts/seed-kolkata-demo.py. That
script creates operational records — FIRs, cases, evidence — through the API so
that numbering, workflow rules and the audit trail are genuine. Stations and
officers are not operational records and the API has no endpoint that creates
them: POST /district/stations writes to `police_stations`, the district
module's own table, and there is no endpoint that creates a user at all.
Reference data has always been loaded with psql here (services/db/init/
002_demo_kolkata.sql does the same for Kolkata Police), so that is what this
does.

Idempotent: each row is recorded under a stable key in `demo_seed_ledger`, the
same table scripts/seed-kolkata-demo.py uses. Every insert is additionally
guarded with ON CONFLICT DO NOTHING, so a second run creates nothing and says
so, and a database seeded by an earlier version gains only what was missing.
Nothing existing is updated or deleted.

Usage:
    DEMO_SEED_CONFIRM=yes \\
    DATABASE_URL=postgresql://sudipto@localhost:5432/npdms \\
    python3 scripts/seed-departments-demo.py

The API need not be running.
"""
import os
import subprocess
import sys

PASSWORD = "Demo@123"
# bcrypt cost 10 of Demo@123 — the same hash every other demo account carries.
PASSWORD_HASH = "$2a$10$V/BKpV1pqxSJGl2RsrTMnO9uhyPjQA9ZMEFBllxhbbnWrbmNq.pTu"

if os.environ.get("DEMO_SEED_CONFIRM") != "yes":
    sys.exit("Refusing to run: this loads FICTIONAL demo accounts. Set DEMO_SEED_CONFIRM=yes to proceed.")
DATABASE_URL = os.environ.get("DATABASE_URL")
if not DATABASE_URL:
    sys.exit("DATABASE_URL is required.")

created = {}
skipped = {}


# ------------------------------------------------------------------ helpers --
def psql(sql):
    result = subprocess.run(["psql", DATABASE_URL, "-v", "ON_ERROR_STOP=1", "-qAt", "-c", sql],
                            capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"{sql[:160]}\n{result.stderr.strip()}")
    return result.stdout.strip()


def q(value):
    """A SQL string literal, or NULL."""
    if value is None:
        return "NULL"
    return "'" + str(value).replace("'", "''") + "'"


def ledger_get(key):
    return psql(f"SELECT entity_id FROM demo_seed_ledger WHERE key = {q(key)}") or None


def once(key, entity, insert_sql, lookup_sql):
    """Insert a row unless it is already there. Returns its id.

    Three cases, all reported honestly: the ledger already has it; the row
    exists but predates the ledger, so it is adopted rather than duplicated;
    or it is genuinely new.
    """
    existing = ledger_get(key)
    if existing:
        skipped[entity] = skipped.get(entity, 0) + 1
        return existing

    entity_id = psql(insert_sql).splitlines()
    if entity_id:
        created[entity] = created.get(entity, 0) + 1
        entity_id = entity_id[0]
    else:
        entity_id = psql(lookup_sql)
        if not entity_id:
            raise RuntimeError(f"{key}: insert created nothing and the row cannot be found")
        skipped[entity] = skipped.get(entity, 0) + 1
    psql(f"INSERT INTO demo_seed_ledger (key, entity, entity_id) "
         f"VALUES ({q(key)}, {q(entity)}, {q(entity_id)}) ON CONFLICT (key) DO NOTHING")
    return entity_id


# ------------------------------------------------------------ prerequisites --
if psql("SELECT to_regclass('public.forces') IS NOT NULL") != "t":
    sys.exit("The forces table does not exist. Apply migration 000082_forces.up.sql first.")
if psql("SELECT COUNT(*) FROM forces WHERE code IN ('KP','WBP','TRAFFIC','CID')") != "4":
    sys.exit("All four forces (KP, WBP, TRAFFIC, CID) must exist before this runs.")
if psql("SELECT COUNT(*) FROM states WHERE code = 'WB'") != "1":
    sys.exit("The West Bengal state row is missing. Run scripts/bootstrap-db.sh first.")

psql("""CREATE TABLE IF NOT EXISTS demo_seed_ledger (
          key TEXT PRIMARY KEY, entity TEXT NOT NULL, entity_id TEXT NOT NULL,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())""")

# ------------------------------------------------- West Bengal Police: where --
# The hierarchy is state › zone › range › district, and West Bengal Police fills
# all four levels where Kolkata Police only ever needed a stub. The zone and the
# two ranges below are real — an ADG commands the zone, a DIG or IG each range —
# and only the ones the districts here sit under are created, so nothing in the
# hierarchy is a level with nothing beneath it.
print("› West Bengal Police: zones and ranges")
ZONES = [
    ("WB-SB", "South Bengal", "Bhabani Bhavan, Alipore"),
]
for code, name, hq in ZONES:
    once(f"zone:{code}", "zones",
         f"""INSERT INTO zones (state_id, name, code, headquarters)
             SELECT s.id, {q(name)}, {q(code)}, {q(hq)} FROM states s WHERE s.code = 'WB'
             ON CONFLICT (state_id, code) DO NOTHING RETURNING id""",
         f"SELECT id FROM zones WHERE code = {q(code)}")

RANGES = [
    ("WB-PRES", "Presidency Range", "WB-SB", "Barasat"),
    ("WB-BDWN", "Burdwan Range", "WB-SB", "Burdwan"),
]
for code, name, zone, hq in RANGES:
    once(f"range:{code}", "ranges",
         f"""INSERT INTO ranges (zone_id, name, code, headquarters)
             SELECT z.id, {q(name)}, {q(code)}, {q(hq)} FROM zones z WHERE z.code = {q(zone)}
             ON CONFLICT (zone_id, code) DO NOTHING RETURNING id""",
         f"SELECT id FROM ranges WHERE code = {q(code)}")

# `districts` already holds the eight Kolkata Police divisions, which are not
# districts at all — they are what a commissionerate has instead. These are the
# real thing, and the force_id column is what tells them apart: KP divisions
# carry KP, these carry WBP. Their codes say so too (KP-CEN against WBP-N24).
#
# A commissionerate answers to the zone directly rather than to a range. The
# schema has no level for that, so each is filed under the range covering the
# same ground; the alternative was to invent a table.
print("› West Bengal Police: districts and commissionerates")
DISTRICTS = [
    ("WBP-N24", "North 24 Parganas", "WB-PRES", "Barasat", "Bhaskar Roychoudhury", "9832000101"),
    ("WBP-BPC", "Barrackpore Police Commissionerate", "WB-PRES", "Barrackpore", "Sohini Talukdar", "9832000102"),
    ("WBP-HWC", "Howrah Police Commissionerate", "WB-PRES", "Howrah", "Nilanjan Bhaduri", "9832000103"),
    ("WBP-HGR", "Hooghly Rural", "WB-BDWN", "Chinsurah", "Ritwika Mazumdar", "9832000104"),
]
DISTRICT = {}
for code, name, rng, hq, sp_name, sp_phone in DISTRICTS:
    DISTRICT[code] = once(f"district:{code}", "districts",
        f"""INSERT INTO districts (range_id, name, code, headquarters, sp_name, sp_phone,
                                   control_room_num, force_id)
            SELECT r.id, {q(name)}, {q(code)}, {q(hq)}, {q(sp_name)}, {q(sp_phone)}, '100', f.id
              FROM ranges r, forces f WHERE r.code = {q(rng)} AND f.code = 'WBP'
            ON CONFLICT (range_id, code) DO NOTHING RETURNING id""",
        f"""SELECT d.id FROM districts d JOIN ranges r ON r.id = d.range_id
             WHERE d.code = {q(code)} AND r.code = {q(rng)}""")

# Real thanas of those districts. Telephone numbers and coordinates are
# illustrative — near enough to put a pin on a map, not a directory.
print("› West Bengal Police: thanas")
WBP_STATIONS = [
    ("BRS", "Barasat Police Station",     "WBP-N24", "Barasat, North 24 Parganas 700124",   "033-2552-1100", 22.7220, 88.4810),
    ("MDG", "Madhyamgram Police Station", "WBP-N24", "Jessore Road, Madhyamgram 700129",    "033-2538-2200", 22.6960, 88.4560),
    ("TTG", "Titagarh Police Station",    "WBP-BPC", "B.T. Road, Titagarh 700119",          "033-2541-3300", 22.7410, 88.3720),
    ("NPR", "Noapara Police Station",     "WBP-BPC", "Noapara, Barrackpore 700125",         "033-2593-4400", 22.7360, 88.3820),
    ("HWR", "Howrah Police Station",      "WBP-HWC", "Panchanantala Road, Howrah 711101",   "033-2641-5500", 22.5890, 88.3100),
    ("SBP", "Shibpur Police Station",     "WBP-HWC", "Andul Road, Shibpur, Howrah 711102",  "033-2668-6600", 22.5770, 88.3110),
    ("SNG", "Singur Police Station",      "WBP-HGR", "Singur, Hooghly 712409",              "033-2685-7700", 22.8120, 88.2320),
    ("TKR", "Tarakeswar Police Station",  "WBP-HGR", "Tarakeswar, Hooghly 712410",          "033-2686-8800", 22.8870, 88.0250),
]
for code, name, district_code, address, phone, lat, lng in WBP_STATIONS:
    district_name = next(d[1] for d in DISTRICTS if d[0] == district_code)
    once(f"station:{code}", "stations",
         f"""INSERT INTO stations (name, code, address, district, district_id, state, phone,
                                   latitude, longitude, force_id)
             SELECT {q(name)}, {q(code)}, {q(address)}, {q(district_name)}, {q(DISTRICT[district_code])}::uuid,
                    'West Bengal', {q(phone)}, {lat}, {lng}, f.id
               FROM forces f WHERE f.code = 'WBP'
             ON CONFLICT (code) DO NOTHING RETURNING id""",
         f"SELECT id FROM stations WHERE code = {q(code)}")

# -------------------------------------------------------------- CID and KTP --
# Neither wing has thanas in the ordinary sense. The CID has specialised units
# working cases referred to it from across the state; the Traffic Police has
# traffic guards, each covering a stretch of road rather than a jurisdiction.
#
# Both are recorded as `stations` rows against their own force, because a
# station row is what everything else in the platform — postings, FIRs, the
# malkhana, the duty roster — hangs off, and a second table for places that are
# almost stations would have to be taught to every one of them.
#
# What keeps them from being mistaken for thanas is the name, which says Unit
# or Traffic Guard, and the code, which is prefixed rather than the bare
# three-letter thana code (CID-HOM, TG-PKS against BHW, BRS). Their `district`
# is the wing itself, so they do not land in a division's workload rollup, and
# `district_id` is left null: no row in `districts` describes them honestly.
#
# One unit really is a police station — the CID Cyber Crime Police Station at
# Bhabani Bhavan registers its own FIRs — and its name says so.
print("› CID units")
CID_UNITS = [
    ("CID-HOM", "CID Homicide Squad",                  "Bhabani Bhavan, Alipore, Kolkata 700027", "033-2479-0101", 22.5230, 88.3320),
    ("CID-CYB", "CID Cyber Crime Police Station",      "Bhabani Bhavan, Alipore, Kolkata 700027", "033-2479-0102", 22.5230, 88.3320),
    ("CID-AHT", "CID Anti-Human Trafficking Unit",     "Bhabani Bhavan, Alipore, Kolkata 700027", "033-2479-0103", 22.5230, 88.3320),
    ("CID-EOW", "CID Economic Offences Wing",          "Bhabani Bhavan, Alipore, Kolkata 700027", "033-2479-0104", 22.5230, 88.3320),
]
for code, name, address, phone, lat, lng in CID_UNITS:
    once(f"station:{code}", "stations",
         f"""INSERT INTO stations (name, code, address, district, state, phone, latitude, longitude, force_id)
             SELECT {q(name)}, {q(code)}, {q(address)}, 'Criminal Investigation Department',
                    'West Bengal', {q(phone)}, {lat}, {lng}, f.id
               FROM forces f WHERE f.code = 'CID'
             ON CONFLICT (code) DO NOTHING RETURNING id""",
         f"SELECT id FROM stations WHERE code = {q(code)}")

print("› Traffic guards")
TRAFFIC_GUARDS = [
    ("TG-PKS", "Park Street Traffic Guard",        "Park Street, Kolkata 700016",                  "033-2215-0201", 22.5525, 88.3529),
    ("TG-JDP", "Jadavpur Traffic Guard",           "Raja S C Mallick Road, Jadavpur, Kolkata 700032", "033-2215-0202", 22.4996, 88.3712),
    ("TG-HWB", "Howrah Bridge Traffic Guard",      "Strand Road, Kolkata 700001",                  "033-2215-0203", 22.5850, 88.3470),
    ("TG-SHK", "Shakespeare Sarani Traffic Guard", "Shakespeare Sarani, Kolkata 700017",           "033-2215-0204", 22.5410, 88.3560),
]
for code, name, address, phone, lat, lng in TRAFFIC_GUARDS:
    once(f"station:{code}", "stations",
         f"""INSERT INTO stations (name, code, address, district, state, phone, latitude, longitude, force_id)
             SELECT {q(name)}, {q(code)}, {q(address)}, 'Kolkata Traffic Police',
                    'West Bengal', {q(phone)}, {lat}, {lng}, f.id
               FROM forces f WHERE f.code = 'TRAFFIC'
             ON CONFLICT (code) DO NOTHING RETURNING id""",
         f"SELECT id FROM stations WHERE code = {q(code)}")

# ------------------------------------------------------------------ officers --
# Every officer below is FICTIONAL. Ranks come from the `user_role` enum, which
# has twelve values and none of the ones two of these forces actually use:
#
#   Sergeant       a Kolkata Traffic Police rank, roughly an SI. Recorded as SI,
#                  with the badge number saying SGT.
#   DC / AC        Kolkata and the commissionerates use Deputy and Assistant
#   ASP            Commissioner; the districts use Additional SP. Both are
#                  recorded as SP and DSP, which is what the existing demo
#                  already does — `sp` carries badge KP-DC-0187.
#   OC             an officer-in-charge, of a thana or of a traffic guard, is
#                  the SHO role.
#
# Inventing enum values the database would reject, or bending a rank into
# something it is not, would both be worse than saying this here.
#
# The last entry is deliberate: a traffic sergeant of the Traffic wing posted to
# Park Street, a Kolkata Police thana. The trigger migration 000082 installs
# allows exactly that — a wing's officer at a station of its parent force — and
# it is how traffic sergeants actually work. If the rule ever breaks, this row
# is what fails first.
print("› Officers")
OFFICERS = [
    # username, name, role, badge, force, station, phone, email domain
    ("sp.n24",      "Debarati Sengupta",     "SP",        "WBP-SP-0214",  "WBP",     "BRS",     "9832000201", "wbpolice.gov.in"),
    ("dsp.hwc",     "Ashis Adhikari",        "DSP",       "WBP-ASP-1180", "WBP",     "HWR",     "9832000202", "wbpolice.gov.in"),
    ("oc.brs",      "Sougata Bhowmick",      "SHO",       "WBP-OC-3310",  "WBP",     "BRS",     "9832000203", "wbpolice.gov.in"),
    ("si.ttg",      "Ruma Karmakar",         "SI",        "WBP-SI-4477",  "WBP",     "TTG",     "9832000204", "wbpolice.gov.in"),

    ("sp.cid",      "Ananya Raychaudhuri",   "SP",        "CID-SP-0106",  "CID",     "CID-HOM", "9832000301", "cidwestbengal.gov.in"),
    ("dsp.cid",     "Biswajit Mondal",       "DSP",       "CID-DSP-0421", "CID",     "CID-HOM", "9832000302", "cidwestbengal.gov.in"),
    ("insp.cid.cyb", "Rupam Chakraborty",    "INSPECTOR", "CID-INS-2208", "CID",     "CID-CYB", "9832000303", "cidwestbengal.gov.in"),
    ("si.cid.aht",  "Sanchari Dutta",        "SI",        "CID-SI-5507",  "CID",     "CID-AHT", "9832000304", "cidwestbengal.gov.in"),

    ("dc.traffic",  "Sohini Gangopadhyay",   "SP",        "KTP-DC-0231",  "TRAFFIC", "TG-PKS",  "9832000401", "kolkatatrafficpolice.gov.in"),
    ("oc.tg.pks",   "Pradipta Kar",          "SHO",       "KTP-OC-1142",  "TRAFFIC", "TG-PKS",  "9832000402", "kolkatatrafficpolice.gov.in"),
    ("sgt.tg.jdp",  "Arka Mukhopadhyay",     "SI",        "KTP-SGT-3096", "TRAFFIC", "TG-JDP",  "9832000403", "kolkatatrafficpolice.gov.in"),
    ("sgt.tg.hwb",  "Firoz Ahmed",           "SI",        "KTP-SGT-3104", "TRAFFIC", "TG-HWB",  "9832000404", "kolkatatrafficpolice.gov.in"),
    ("sgt.pks",     "Shibaji Halder",        "SI",        "KTP-SGT-3117", "TRAFFIC", "PKS",     "9832000405", "kolkatatrafficpolice.gov.in"),
]
postings = "', '".join(sorted({o[5] for o in OFFICERS}))
missing = psql(f"""SELECT string_agg(c, ', ') FROM unnest(ARRAY['{postings}']) c
                    WHERE c NOT IN (SELECT code FROM stations)""")
if missing:
    sys.exit(f"No station with code {missing}. The Kolkata Police stations come from "
             "services/db/init/002_demo_kolkata.sql — load that first.")

for username, name, role, badge, force, station, phone, domain in OFFICERS:
    once(f"officer:{username}", "officers",
         f"""INSERT INTO users (username, email, password_hash, name, role, badge_number,
                                station_id, phone, is_active, force_id)
             SELECT {q(username)}, {q(username + '@' + domain)}, {q(PASSWORD_HASH)}, {q(name)},
                    {q(role)}::user_role, {q(badge)}, s.id, {q(phone)}, true, f.id
               FROM stations s, forces f WHERE s.code = {q(station)} AND f.code = {q(force)}
             ON CONFLICT (username) DO NOTHING RETURNING id""",
         f"SELECT id FROM users WHERE username = {q(username)}")

# The district station counts are now wrong by however many thanas were added.
psql("""UPDATE districts d
           SET total_stations = (SELECT COUNT(*) FROM stations s WHERE s.district_id = d.id),
               total_officers = (SELECT COUNT(*) FROM users u JOIN stations s ON s.id = u.station_id
                                  WHERE s.district_id = d.id)
         WHERE d.code LIKE 'WBP-%'""")

# ------------------------------------------------------------------- report --
print()
if created:
    print("Created this run:")
    for entity, count in sorted(created.items()):
        print(f"  {entity:12s} {count}")
else:
    print("Nothing new — every department row was already present.")
if skipped:
    print("Already present:")
    for entity, count in sorted(skipped.items()):
        print(f"  {entity:12s} {count}")

print("\nStations and officers by force:")
for line in psql("""SELECT rpad(f.short_name, 6) || ' ' || rpad(f.kind, 6)
                           || lpad(COUNT(DISTINCT s.id)::text, 4) || ' stations '
                           || lpad(COUNT(DISTINCT u.id)::text, 4) || ' officers'
                      FROM forces f
                      LEFT JOIN stations s ON s.force_id = f.id
                      LEFT JOIN users u    ON u.force_id = f.id
                     GROUP BY f.short_name, f.kind, f.code ORDER BY f.code""").splitlines():
    print(f"  {line}")

mismatched = psql("""SELECT COUNT(*) FROM users u
                       JOIN stations s ON s.id = u.station_id
                       JOIN forces uf  ON uf.id = u.force_id
                      WHERE s.force_id <> u.force_id
                        AND uf.parent_id IS DISTINCT FROM s.force_id""")
print(f"\nOfficers posted outside their force: {mismatched} (the trigger refuses any others)")

print(f"\nDemo accounts (password {PASSWORD}):")
for force, label in [("WBP", "West Bengal Police"), ("CID", "CID"), ("TRAFFIC", "Kolkata Traffic Police")]:
    names = ", ".join(o[0] for o in OFFICERS if o[4] == force)
    print(f"  {label:24s} {names}")
print("  Kolkata Police           admin, sp, dsp, sho, inspector, si, asi, hc, constable (seeded elsewhere)")
