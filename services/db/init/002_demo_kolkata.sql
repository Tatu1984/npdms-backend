-- Demo reference data for a Kolkata Police edge deployment.
--
-- Loaded by `scripts/bootstrap-db.sh --with-demo-data`. Safe to re-run.
--
-- This replaces the Karnataka placeholders the base schema ships with, and sets
-- a working password hash for the demo accounts — the hash in 001_schema.sql
-- does not correspond to any known password, so sign-in fails against a fresh
-- database without this.
--
-- The accounts below are for demonstration and local development. Remove them,
-- or rotate every password, before the server carries real case data.

BEGIN;

-- ----------------------------------------------------------------- stations --
UPDATE stations
SET name      = 'Bhowanipore Police Station',
    address   = 'Harish Mukherjee Road, Bhowanipore, Kolkata',
    district  = 'Kolkata South',
    state     = 'West Bengal',
    phone     = '033-2223-5210',
    latitude  = 22.5301,
    longitude = 88.3421
WHERE code = 'KOR';

UPDATE stations SET code = 'BHW' WHERE code = 'KOR';

INSERT INTO stations (name, code, address, district, state, phone, latitude, longitude) VALUES
  ('Park Street Police Station',  'PKS', 'Park Street, Kolkata',                          'Kolkata South',      'West Bengal', '033-2229-7432', 22.5525, 88.3529),
  ('Jadavpur Police Station',     'JDP', 'Raja S C Mallick Road, Jadavpur, Kolkata',      'Kolkata South',      'West Bengal', '033-2413-1000', 22.4996, 88.3712),
  ('Kasba Police Station',        'KSB', 'Rajdanga Main Road, Kasba, Kolkata',            'Kolkata South East', 'West Bengal', '033-2441-8100', 22.5147, 88.4017),
  ('Behala Police Station',       'BEH', 'Diamond Harbour Road, Behala, Kolkata',         'Kolkata South West', 'West Bengal', '033-2491-2020', 22.4989, 88.3103),
  ('Burrabazar Police Station',   'BBZ', 'Rabindra Sarani, Burrabazar, Kolkata',          'Kolkata Central',    'West Bengal', '033-2268-3100', 22.5780, 88.3560),
  ('Shyampukur Police Station',   'SHY', 'Bidhan Sarani, Shyampukur, Kolkata',            'Kolkata North',      'West Bengal', '033-2555-1010', 22.5958, 88.3697),
  ('Ballygunge Police Station',   'BLG', 'Gariahat Road, Ballygunge, Kolkata',            'Kolkata South East', 'West Bengal', '033-2440-5500', 22.5265, 88.3654)
ON CONFLICT (code) DO NOTHING;

-- -------------------------------------------------------------------- users --
-- Password for every demo account below: Demo@123
-- bcrypt cost 10. Replace before any real deployment.
UPDATE users
SET password_hash = '$2a$10$V/BKpV1pqxSJGl2RsrTMnO9uhyPjQA9ZMEFBllxhbbnWrbmNq.pTu'
WHERE username IN ('admin', 'constable', 'hc', 'asi', 'si', 'inspector', 'sho', 'dsp');

-- Re-badge the demo officers for Kolkata Police and give them Bengali names.
UPDATE users SET name = 'Debashis Roy',        badge_number = 'KP-OC-1109'  WHERE username = 'sho';
UPDATE users SET name = 'Arindam Chatterjee',  badge_number = 'KP-INS-2417' WHERE username = 'inspector';
UPDATE users SET name = 'Sutapa Mukherjee',    badge_number = 'KP-SI-5182'  WHERE username = 'si';
UPDATE users SET name = 'Rituparna Ghosh',     badge_number = 'KP-ASI-7734' WHERE username = 'asi';
UPDATE users SET name = 'Suresh Patil',        badge_number = 'KP-HC-3320'  WHERE username = 'hc';
UPDATE users SET name = 'Ramesh Kumar Das',    badge_number = 'KP-PC-1001'  WHERE username = 'constable';
UPDATE users SET name = 'Subhankar Pal',       badge_number = 'KP-AC-0412'  WHERE username = 'dsp';
UPDATE users SET name = 'System Administrator', badge_number = 'KP-ADMIN-001' WHERE username = 'admin';

-- Point every demo officer at Bhowanipore so the station scoping has effect.
UPDATE users
SET station_id = (SELECT id FROM stations WHERE code = 'BHW')
WHERE station_id IS NULL
   OR station_id NOT IN (SELECT id FROM stations);

COMMIT;

-- A short report so the operator can see what is in place.
SELECT 'stations' AS entity, COUNT(*) AS rows FROM stations
UNION ALL SELECT 'users', COUNT(*) FROM users
UNION ALL SELECT 'firs', COUNT(*) FROM firs
UNION ALL SELECT 'investigation workspaces', COUNT(*) FROM investigation_workspaces;
