-- The four departments the platform is meant to connect.
--
-- Until now the platform held one force's records and had no column saying so:
-- every station was a Kolkata Police station, every officer was posted to one,
-- and the four departments existed only as a dropdown in the interface that
-- switched nothing. This gives them somewhere to live.
--
-- The shape follows how the forces are actually organised, because a model
-- that flatters the organisation chart will not survive contact with it:
--
--   Kolkata Police       a force, policing the city
--     Traffic            a wing of it — Kolkata Traffic Police
--   West Bengal Police   a force, policing the rest of the state
--     CID                a wing of it — the Criminal Investigation Department
--
-- Two forces, each with a wing. That is what makes "inter-department" mean
-- something: a case referred from Kolkata Police to CID genuinely crosses a
-- boundary, and can be made to require a reason and leave a record.

CREATE TABLE IF NOT EXISTS forces (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code          VARCHAR(12) NOT NULL UNIQUE,
    name          VARCHAR(120) NOT NULL,
    name_bn       VARCHAR(160),
    short_name    VARCHAR(20) NOT NULL,

    -- A FORCE stands on its own; a WING belongs to one.
    kind          VARCHAR(10) NOT NULL CHECK (kind IN ('FORCE', 'WING')),
    parent_id     UUID REFERENCES forces(id),

    headquarters  VARCHAR(160),
    -- What this department is for, in one line, shown on screen so an officer
    -- of another force knows what they are looking at.
    remit         TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- A wing belongs to a force; a force belongs to nothing.
    CONSTRAINT forces_wing_has_a_parent CHECK (
        (kind = 'WING' AND parent_id IS NOT NULL) OR
        (kind = 'FORCE' AND parent_id IS NULL))
);

INSERT INTO forces (code, name, name_bn, short_name, kind, headquarters, remit) VALUES
    ('KP',  'Kolkata Police', 'কলকাতা পুলিশ', 'KP',
     'FORCE', 'Lalbazar, Kolkata',
     'Policing the city of Kolkata.'),
    ('WBP', 'West Bengal Police', 'পশ্চিমবঙ্গ পুলিশ', 'WBP',
     'FORCE', 'Bhabani Bhavan, Alipore',
     'Policing West Bengal outside the Kolkata Police area.')
ON CONFLICT (code) DO NOTHING;

INSERT INTO forces (code, name, name_bn, short_name, kind, parent_id, headquarters, remit)
SELECT 'TRAFFIC', 'Kolkata Traffic Police', 'কলকাতা ট্রাফিক পুলিশ', 'KTP',
       'WING', f.id, 'Traffic Headquarters, Lalbazar',
       'Traffic regulation, accidents and prosecutions in the Kolkata Police area.'
  FROM forces f WHERE f.code = 'KP'
ON CONFLICT (code) DO NOTHING;

INSERT INTO forces (code, name, name_bn, short_name, kind, parent_id, headquarters, remit)
SELECT 'CID', 'Criminal Investigation Department', 'অপরাধ তদন্ত বিভাগ', 'CID',
       'WING', f.id, 'Bhabani Bhavan, Alipore',
       'Specialised investigation of cases referred to it from across West Bengal.'
  FROM forces f WHERE f.code = 'WBP'
ON CONFLICT (code) DO NOTHING;

-- --------------------------------------------------- who belongs where ------

ALTER TABLE stations  ADD COLUMN IF NOT EXISTS force_id UUID REFERENCES forces(id);
ALTER TABLE districts ADD COLUMN IF NOT EXISTS force_id UUID REFERENCES forces(id);
ALTER TABLE users     ADD COLUMN IF NOT EXISTS force_id UUID REFERENCES forces(id);

-- Everything that exists today is Kolkata Police, and saying so is a fact
-- about the data rather than an assumption: every station is a Kolkata
-- station and every officer is posted to one.
UPDATE stations  SET force_id = (SELECT id FROM forces WHERE code = 'KP') WHERE force_id IS NULL;
UPDATE districts SET force_id = (SELECT id FROM forces WHERE code = 'KP') WHERE force_id IS NULL;
UPDATE users u
   SET force_id = COALESCE(
        (SELECT s.force_id FROM stations s WHERE s.id = u.station_id),
        (SELECT id FROM forces WHERE code = 'KP'))
 WHERE u.force_id IS NULL;

ALTER TABLE stations  ALTER COLUMN force_id SET NOT NULL;
ALTER TABLE users     ALTER COLUMN force_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_stations_force ON stations (force_id);
CREATE INDEX IF NOT EXISTS idx_users_force    ON users (force_id);

-- An officer posted to a station belongs to that station's force. Getting this
-- wrong is how a Kolkata sub-inspector ends up reading CID files, so the
-- database holds it rather than trusting every caller to remember.
--
-- A wing's officer may be posted to a station of its parent force — a traffic
-- sergeant works out of a Kolkata station — so the check allows that.
CREATE OR REPLACE FUNCTION user_force_matches_posting() RETURNS TRIGGER AS $$
DECLARE
    v_station_force UUID;
    v_parent        UUID;
BEGIN
    IF NEW.station_id IS NULL OR NEW.force_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT force_id INTO v_station_force FROM stations WHERE id = NEW.station_id;
    IF v_station_force IS NULL OR v_station_force = NEW.force_id THEN
        RETURN NEW;
    END IF;

    SELECT parent_id INTO v_parent FROM forces WHERE id = NEW.force_id;
    IF v_parent IS NOT NULL AND v_parent = v_station_force THEN
        RETURN NEW;  -- a wing's officer at a station of its own force
    END IF;

    RAISE EXCEPTION 'an officer of % cannot be posted to a station of another force',
        (SELECT short_name FROM forces WHERE id = NEW.force_id)
        USING ERRCODE = 'check_violation', CONSTRAINT = 'user_force_matches_posting';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_user_force_matches_posting ON users;
CREATE TRIGGER trg_user_force_matches_posting
    BEFORE INSERT OR UPDATE OF force_id, station_id ON users
    FOR EACH ROW EXECUTE FUNCTION user_force_matches_posting();

-- ------------------------------------------------ what everyone can see -----

-- The registers that only work if every force can see them. A missing child
-- does not stop being missing at a jurisdiction boundary, and a stolen car is
-- driven across one within the hour.
--
-- Everything not listed here is the recording force's own, and reaches another
-- force only by an explicit, audited act — a referral or an assistance
-- request. Kept as a table so the list can be changed by an order rather than
-- by a deployment.
CREATE TABLE IF NOT EXISTS shared_registers (
    register     VARCHAR(40) PRIMARY KEY,
    description  TEXT NOT NULL,
    shared       BOOLEAN NOT NULL DEFAULT TRUE,
    decided_by   UUID REFERENCES users(id),
    decided_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO shared_registers (register, description) VALUES
    ('MISSING_PERSONS', 'Open missing-person reports, so every force can check a sighting.'),
    ('VEHICLES',        'Stolen and wanted vehicles, which cross a boundary within the hour.'),
    ('LOOKOUTS',        'Lookout notices and their sightings.'),
    ('ALERTS',          'Alerts raised for other forces to act on.')
ON CONFLICT (register) DO NOTHING;

COMMENT ON TABLE forces IS
    'The departments the platform connects: Kolkata Police and West Bengal Police, each with a wing (Traffic, CID).';
COMMENT ON TABLE shared_registers IS
    'The registers every force sees. Everything else belongs to the force that recorded it and crosses a boundary only by referral or an assistance request.';
