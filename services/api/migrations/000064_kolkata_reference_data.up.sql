-- Replace the Karnataka placeholder records shipped with the original schema.
--
-- The base schema used to insert a Koramangala (Bangalore) station, officer
-- accounts with @karpolice.gov.in addresses and KAR- badges, two sample FIRs
-- numbered KOR/2024/…, and migrations 000017 and 000020 seeded Karnataka and
-- Maharashtra jurisdictions. Fresh databases no longer receive any of that;
-- this migration converts databases that were built before the change.
--
-- Safe to run on a fresh database (every statement is conditional) and to run
-- twice, which scripts/bootstrap-db.sh does.

ALTER TABLE stations ALTER COLUMN state SET DEFAULT 'West Bengal';

-- ---------------------------------------------------------------- station ---
-- The placeholder station keeps its id, so existing references stay valid.
UPDATE stations
SET name = 'Bhowanipore Police Station', code = 'BHW',
    address = 'Harish Mukherjee Road, Bhowanipore, Kolkata 700025',
    district = 'South Division', state = 'West Bengal', phone = '033-2223-5210',
    latitude = 22.5301, longitude = 88.3421
WHERE code = 'KOR'
  AND NOT EXISTS (SELECT 1 FROM stations WHERE code = 'BHW');

UPDATE stations SET state = 'West Bengal' WHERE state = 'Karnataka';

-- ------------------------------------------------------- demo accounts ---
-- Only the fixed demo usernames are touched, and only while they still carry
-- the Karnataka placeholder identity.
UPDATE users u
SET email = u.username || '@kolkatapolice.gov.in',
    name = v.name,
    badge_number = v.badge
FROM (VALUES
    ('constable', 'Ramesh Kumar Das',   'KP-PC-1001'),
    ('hc',        'Tapan Kumar Sarkar', 'KP-HC-3320'),
    ('asi',       'Rituparna Ghosh',    'KP-ASI-7734'),
    ('si',        'Sutapa Mukherjee',   'KP-SI-5182'),
    ('inspector', 'Arindam Chatterjee', 'KP-INS-2417'),
    ('sho',       'Debashis Roy',       'KP-OC-1109'),
    ('dsp',       'Subhankar Pal',      'KP-AC-0412'),
    ('sp',        'Paromita Banerjee',  'KP-DC-0187')
) AS v(username, name, badge)
WHERE u.username = v.username
  AND (u.email LIKE '%@karpolice.gov.in' OR u.badge_number LIKE 'KAR-%')
  AND NOT EXISTS (SELECT 1 FROM users o WHERE o.badge_number = v.badge AND o.id <> u.id);

UPDATE users SET email = 'admin@kolkatapolice.gov.in', badge_number = 'KP-ADMIN-001'
WHERE username = 'admin' AND email = 'admin@npdms.gov.in'
  AND NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@kolkatapolice.gov.in');

-- ---------------------------------------------------------- sample FIRs ---
-- Removed where nothing refers to them; otherwise renamed so no Karnataka
-- text remains.
DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN SELECT id, fir_number FROM firs WHERE fir_number LIKE 'KOR/%' LOOP
        BEGIN
            DELETE FROM firs WHERE id = r.id;
        EXCEPTION WHEN foreign_key_violation THEN
            UPDATE firs
            SET fir_number = replace(r.fir_number, 'KOR/', 'BHW/'),
                complainant_address = 'Bhowanipore, Kolkata 700025',
                incident_location = 'Bhowanipore, Kolkata'
            WHERE id = r.id;
        END;
    END LOOP;
END $$;

-- --------------------------------------------------------- jurisdictions ---
UPDATE jurisdictions SET code = 'STATE_WB', name = 'West Bengal State'
WHERE code = 'STATE_KA' AND NOT EXISTS (SELECT 1 FROM jurisdictions WHERE code = 'STATE_WB');
UPDATE jurisdictions SET code = 'DIST_KOL', name = 'Kolkata Police Commissionerate'
WHERE code = 'DIST_BLR' AND NOT EXISTS (SELECT 1 FROM jurisdictions WHERE code = 'DIST_KOL');
UPDATE jurisdictions SET code = 'STN_LBZ', name = 'Lalbazar (Kolkata Police HQ)'
WHERE code = 'STN_KOR' AND NOT EXISTS (SELECT 1 FROM jurisdictions WHERE code = 'STN_LBZ');

-- ------------------------------------------------------- state hierarchy ---
-- Drop the sample hierarchy for other states where nothing is attached to it.
DO $$
BEGIN
    DELETE FROM districts d USING ranges r, zones z, states s
    WHERE d.range_id = r.id AND r.zone_id = z.id AND z.state_id = s.id
      AND s.code IN ('MH', 'DL', 'KA', 'TN', 'UP')
      AND NOT EXISTS (SELECT 1 FROM police_stations p WHERE p.district_id = d.id);
    DELETE FROM ranges r USING zones z, states s
    WHERE r.zone_id = z.id AND z.state_id = s.id AND s.code IN ('MH', 'DL', 'KA', 'TN', 'UP')
      AND NOT EXISTS (SELECT 1 FROM districts d WHERE d.range_id = r.id);
    DELETE FROM zones z USING states s
    WHERE z.state_id = s.id AND s.code IN ('MH', 'DL', 'KA', 'TN', 'UP')
      AND NOT EXISTS (SELECT 1 FROM ranges r WHERE r.zone_id = z.id);
    DELETE FROM states s
    WHERE s.code IN ('MH', 'DL', 'KA', 'TN', 'UP')
      AND NOT EXISTS (SELECT 1 FROM zones z WHERE z.state_id = s.id);
EXCEPTION WHEN foreign_key_violation THEN
    RAISE NOTICE 'sample hierarchy still referenced; left in place';
END $$;

INSERT INTO states (name, code, dgp_name) VALUES ('West Bengal', 'WB', 'DGP & IGP, West Bengal')
ON CONFLICT (code) DO NOTHING;

INSERT INTO zones (state_id, name, code, headquarters)
SELECT s.id, 'Kolkata Police Commissionerate', 'WB-KP', 'Lalbazar, Kolkata'
FROM states s WHERE s.code = 'WB'
ON CONFLICT (state_id, code) DO NOTHING;

INSERT INTO ranges (zone_id, name, code, headquarters)
SELECT z.id, 'Kolkata Police Divisions', 'WB-KP-DIV', 'Lalbazar, Kolkata'
FROM zones z WHERE z.code = 'WB-KP'
ON CONFLICT (zone_id, code) DO NOTHING;

INSERT INTO districts (range_id, name, code, headquarters, control_room_num)
SELECT r.id, d.name, d.code, d.hq, '100'
FROM ranges r
CROSS JOIN (VALUES
    ('Central Division',          'KP-CEN', 'Lalbazar'),
    ('North Division',            'KP-NTH', 'Shyampukur'),
    ('South Division',            'KP-STH', 'Bhowanipore'),
    ('South East Division',       'KP-SE',  'Park Street'),
    ('South West Division',       'KP-SW',  'Behala'),
    ('East Division',             'KP-EST', 'Entally'),
    ('Port Division',             'KP-PRT', 'Garden Reach'),
    ('Eastern Suburban Division', 'KP-ESD', 'Jadavpur')
) AS d(name, code, hq)
WHERE r.code = 'WB-KP-DIV'
ON CONFLICT (range_id, code) DO NOTHING;
