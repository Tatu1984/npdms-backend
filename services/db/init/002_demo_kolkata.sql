-- Kolkata Police reference stations and DEMO sign-in accounts.
--
-- Loaded by `scripts/bootstrap-db.sh --with-demo-data` and by
-- scripts/seed-kolkata-demo.py. Safe to re-run: every row is keyed on its code
-- or username and updated in place.
--
-- Station names, divisions and approximate locations are real Kolkata Police
-- reference data. Every officer below is FICTIONAL, and every account shares
-- the demo password Demo@123. Remove these accounts, or rotate every password,
-- before the platform holds real case data.

BEGIN;

-- ----------------------------------------------------------------- stations --
-- `district` carries the Kolkata Police division, which the workload rollups
-- group by. `force_id` is Kolkata Police for every station here, and has to be
-- stated: migration 000082 made the column NOT NULL, which stopped this file
-- loading on a fresh database until it said which force these belong to. The
-- other three forces are loaded by scripts/seed-departments-demo.py.
INSERT INTO stations (id, name, code, address, district, state, phone, latitude, longitude, force_id)
SELECT COALESCE(s.id, v.id::uuid), v.name, v.code, v.address, v.division, 'West Bengal', v.phone, v.lat, v.lng,
       (SELECT id FROM forces WHERE code = 'KP')
FROM (VALUES
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a01', 'Lalbazar (Kolkata Police Headquarters)', 'LBZ', '18 Lalbazar Street, Kolkata 700001',               'Central Division',          '033-2214-5000', 22.5697, 88.3506),
  ('550e8400-e29b-41d4-a716-446655440001', 'Bhowanipore Police Station',             'BHW', 'Harish Mukherjee Road, Bhowanipore, Kolkata 700025', 'South Division',            '033-2223-5210', 22.5301, 88.3421),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a03', 'Park Street Police Station',             'PKS', 'Park Street, Kolkata 700016',                      'South East Division',       '033-2229-7432', 22.5525, 88.3529),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a04', 'Jadavpur Police Station',                'JDP', 'Raja S C Mallick Road, Jadavpur, Kolkata 700032',  'Eastern Suburban Division', '033-2413-1000', 22.4996, 88.3712),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a05', 'Kasba Police Station',                   'KSB', 'Rajdanga Main Road, Kasba, Kolkata 700107',        'South East Division',       '033-2441-8100', 22.5147, 88.4017),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a06', 'Tollygunge Police Station',              'TLG', 'Deshapran Sasmal Road, Tollygunge, Kolkata 700033', 'South Division',           '033-2424-1010', 22.4988, 88.3453),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a07', 'Gariahat Police Station',                'GRH', 'Gariahat Road, Kolkata 700019',                    'South East Division',       '033-2440-5500', 22.5183, 88.3660),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a08', 'Shyampukur Police Station',              'SHY', 'Bidhan Sarani, Shyampukur, Kolkata 700004',        'North Division',            '033-2555-1010', 22.5958, 88.3697),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a09', 'Burrabazar Police Station',              'BBZ', 'Rabindra Sarani, Burrabazar, Kolkata 700007',      'Central Division',          '033-2268-3100', 22.5780, 88.3560),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a10', 'Hare Street Police Station',             'HST', 'Hare Street, Kolkata 700001',                      'Central Division',          '033-2248-2111', 22.5700, 88.3480),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a11', 'New Market Police Station',              'NMK', 'Lindsay Street, New Market, Kolkata 700087',       'Central Division',          '033-2286-1200', 22.5590, 88.3510),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a12', 'Entally Police Station',                 'ENT', 'AJC Bose Road, Entally, Kolkata 700014',           'East Division',             '033-2244-3900', 22.5555, 88.3800),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a13', 'Garden Reach Police Station',            'GRE', 'Garden Reach Road, Kolkata 700024',                'Port Division',             '033-2469-1500', 22.5390, 88.2930),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a14', 'Maidan Police Station',                  'MDN', 'Casuarina Avenue, Maidan, Kolkata 700021',         'South Division',            '033-2223-0181', 22.5535, 88.3450),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a15', 'Watgunge Police Station',                'WTG', 'Watgunge Street, Kolkata 700023',                  'Port Division',             '033-2439-2000', 22.5400, 88.3150),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a16', 'Behala Police Station',                  'BEH', 'Diamond Harbour Road, Behala, Kolkata 700034',     'South West Division',       '033-2491-2020', 22.4989, 88.3103),
  ('3f1b2c84-6d0e-4b8a-9a31-0b5e7c1d2a17', 'Ballygunge Police Station',              'BLG', 'Gurusaday Road, Ballygunge, Kolkata 700019',       'South East Division',       '033-2440-7700', 22.5265, 88.3654)
) AS v(id, name, code, address, division, phone, lat, lng)
LEFT JOIN stations s ON s.code = v.code
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name, address = EXCLUDED.address, district = EXCLUDED.district,
    state = EXCLUDED.state, phone = EXCLUDED.phone,
    latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude;

-- -------------------------------------------------------------------- users --
-- Password for every account below: Demo@123 (bcrypt cost 10).
-- The nine short usernames are the documented demo roles; the dotted ones are
-- additional FICTIONAL officers so other stations have staff.
INSERT INTO users (id, username, email, password_hash, name, role, badge_number, station_id, phone, is_active, force_id)
SELECT v.id::uuid, v.username, v.username || '@kolkatapolice.gov.in',
       '$2a$10$V/BKpV1pqxSJGl2RsrTMnO9uhyPjQA9ZMEFBllxhbbnWrbmNq.pTu',
       v.name, v.role::user_role, v.badge, st.id, v.phone, true,
       (SELECT id FROM forces WHERE code = 'KP')
FROM (VALUES
  ('550e8400-e29b-41d4-a716-446655440009', 'admin',     'System Administrator',  'DGP',            'KP-ADMIN-001', 'LBZ', '9830000001'),
  ('550e8400-e29b-41d4-a716-446655440017', 'sp',        'Paromita Banerjee',     'SP',             'KP-DC-0187',   'LBZ', '9830000002'),
  ('550e8400-e29b-41d4-a716-446655440016', 'dsp',       'Subhankar Pal',         'DSP',            'KP-AC-0412',   'LBZ', '9830000003'),
  ('550e8400-e29b-41d4-a716-446655440015', 'sho',       'Debashis Roy',          'SHO',            'KP-OC-1109',   'BHW', '9830000004'),
  ('550e8400-e29b-41d4-a716-446655440014', 'inspector', 'Arindam Chatterjee',    'INSPECTOR',      'KP-INS-2417',  'BHW', '9830000005'),
  ('550e8400-e29b-41d4-a716-446655440013', 'si',        'Sutapa Mukherjee',      'SI',             'KP-SI-5182',   'BHW', '9830000006'),
  ('550e8400-e29b-41d4-a716-446655440012', 'asi',       'Rituparna Ghosh',       'ASI',            'KP-ASI-7734',  'BHW', '9830000007'),
  ('550e8400-e29b-41d4-a716-446655440011', 'hc',        'Tapan Kumar Sarkar',    'HEAD_CONSTABLE', 'KP-HC-3320',   'BHW', '9830000008'),
  ('550e8400-e29b-41d4-a716-446655440010', 'constable', 'Ramesh Kumar Das',      'CONSTABLE',      'KP-PC-1001',   'BHW', '9830000009'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a01', 'oc.pks',    'Anirban Das',           'SHO',            'KP-OC-1214',   'PKS', '9830000010'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a02', 'si.pks',    'Moumita Sen',           'SI',             'KP-SI-6421',   'PKS', '9830000011'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a03', 'asi.pks',   'Imran Hossain',         'ASI',            'KP-ASI-7810',  'PKS', '9830000012'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a04', 'oc.jdp',    'Kaushik Bhattacharya',  'SHO',            'KP-OC-1320',   'JDP', '9830000013'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a05', 'si.jdp',    'Soma Chakraborty',      'SI',             'KP-SI-6512',   'JDP', '9830000014'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a06', 'oc.ksb',    'Partha Pratim Ghosal',  'SHO',            'KP-OC-1402',   'KSB', '9830000015'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a07', 'si.ksb',    'Nazia Parveen',         'SI',             'KP-SI-6630',   'KSB', '9830000016'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a08', 'oc.tlg',    'Sujoy Majumdar',        'SHO',            'KP-OC-1511',   'TLG', '9830000017'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a09', 'si.tlg',    'Rakesh Yadav',          'SI',             'KP-SI-6744',   'TLG', '9830000018'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a10', 'oc.shy',    'Amitava Dutta',         'SHO',            'KP-OC-1603',   'SHY', '9830000019'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a11', 'si.shy',    'Priyanka Saha',         'SI',             'KP-SI-6858',   'SHY', '9830000020'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a12', 'oc.bbz',    'Vikash Agarwal',        'SHO',            'KP-OC-1705',   'BBZ', '9830000021'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a13', 'si.bbz',    'Shubhajit Nandi',       'SI',             'KP-SI-6962',   'BBZ', '9830000022'),
  ('7a2e5d10-1c3b-4f6a-8e9d-2b4c6d8e0a14', 'si.grh',    'Tanushree Basu',        'SI',             'KP-SI-7076',   'GRH', '9830000023')
) AS v(id, username, name, role, badge, station_code, phone)
JOIN stations st ON st.code = v.station_code
ON CONFLICT (username) DO UPDATE
SET email = EXCLUDED.email, password_hash = EXCLUDED.password_hash, name = EXCLUDED.name,
    role = EXCLUDED.role, badge_number = EXCLUDED.badge_number,
    station_id = EXCLUDED.station_id, phone = EXCLUDED.phone, is_active = true;

COMMIT;

SELECT 'stations' AS entity, COUNT(*) AS rows FROM stations
UNION ALL SELECT 'users', COUNT(*) FROM users;
