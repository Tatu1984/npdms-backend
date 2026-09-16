-- Where one department's records end.
--
-- The rule the platform now works to: an officer sees their own force's
-- records. A short list of registers is state-wide and seen by everyone. Any
-- other record reaches another force only by an explicit, audited act — a
-- referral or an assistance request.
--
-- The boundary is drawn around the FORCE, not the wing. A CID officer is West
-- Bengal Police; a traffic sergeant is Kolkata Police. Drawing it around the
-- wing would mean a traffic sergeant could not see the station's own record of
-- the accident they attended, which is not how either force works. What stops
-- a traffic sergeant reading a murder file is rank and module, which the
-- platform already enforces — not the force boundary.
--
-- So there are two boundaries here, not four: the Kolkata Police family
-- (Kolkata Police and its Traffic wing) and the West Bengal Police family
-- (West Bengal Police and CID). Referral between them is the inter-department
-- act worth recording.

-- The forces whose records an officer of this force may see.
CREATE OR REPLACE FUNCTION force_family(p_force UUID) RETURNS UUID[] AS $$
    WITH root AS (
        SELECT COALESCE(f.parent_id, f.id) AS id FROM forces f WHERE f.id = p_force
    )
    SELECT COALESCE(array_agg(f.id), ARRAY[]::UUID[])
      FROM forces f, root
     WHERE f.id = root.id OR f.parent_id = root.id
$$ LANGUAGE sql STABLE;

-- The stations of that family, which is how a record is placed: nearly every
-- register carries a station, so the station carries the force.
CREATE OR REPLACE FUNCTION force_family_stations(p_force UUID) RETURNS UUID[] AS $$
    SELECT COALESCE(array_agg(s.id), ARRAY[]::UUID[])
      FROM stations s
     WHERE s.force_id = ANY(force_family(p_force))
$$ LANGUAGE sql STABLE;

-- Is this register one everybody sees? Reads the table an order can change,
-- rather than a list compiled into the software.
CREATE OR REPLACE FUNCTION register_is_shared(p_register TEXT) RETURNS BOOLEAN AS $$
    SELECT COALESCE((SELECT shared FROM shared_registers WHERE register = p_register), FALSE)
$$ LANGUAGE sql STABLE;

-- ------------------------------------------------------------ referrals ----

-- A case handed from one force to another — Kolkata Police to CID being the
-- ordinary example. The referral is what grants the receiving force sight of
-- the record, and it is a record in its own right: who asked, why, who
-- accepted, and when.
CREATE TABLE IF NOT EXISTS case_referrals (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    referral_number VARCHAR(40) UNIQUE,

    -- What is being referred. Kept general so the same workflow serves a case,
    -- an FIR or a complaint without a table each.
    record_type     VARCHAR(30) NOT NULL CHECK (record_type IN ('CASE', 'FIR', 'COMPLAINT')),
    record_id       UUID NOT NULL,
    record_reference VARCHAR(60),

    from_force_id   UUID NOT NULL REFERENCES forces(id),
    to_force_id     UUID NOT NULL REFERENCES forces(id),

    -- Why. A referral without a reason is not a referral, it is a leak.
    reason          TEXT NOT NULL CHECK (btrim(reason) <> ''),
    -- The order or authority behind it, where there is one.
    authority       VARCHAR(160),

    status          VARCHAR(20) NOT NULL DEFAULT 'PROPOSED'
                    CHECK (status IN ('PROPOSED', 'ACCEPTED', 'DECLINED', 'WITHDRAWN', 'RETURNED')),

    referred_by     UUID NOT NULL REFERENCES users(id),
    referred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Decided by the receiving force: a force cannot hand its work to another
    -- by announcing it.
    decided_by      UUID REFERENCES users(id),
    decided_at      TIMESTAMPTZ,
    decision_note   TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT referral_crosses_a_boundary CHECK (from_force_id <> to_force_id)
);

CREATE INDEX IF NOT EXISTS idx_referrals_record ON case_referrals (record_type, record_id);
CREATE INDEX IF NOT EXISTS idx_referrals_to     ON case_referrals (to_force_id, status);
CREATE INDEX IF NOT EXISTS idx_referrals_from   ON case_referrals (from_force_id, status);

-- Only one referral of a record may be live at a time: a record cannot be
-- handed to two forces at once, and the confusion if it could would land on
-- whoever had to explain it in court.
CREATE UNIQUE INDEX IF NOT EXISTS idx_referrals_one_live
    ON case_referrals (record_type, record_id)
    WHERE status IN ('PROPOSED', 'ACCEPTED');

-- The decision belongs to the receiving force, and it is made once.
CREATE OR REPLACE FUNCTION referral_decision_rules() RETURNS TRIGGER AS $$
DECLARE
    v_decider_force UUID;
BEGIN
    IF NEW.status = OLD.status THEN
        NEW.updated_at := NOW();
        RETURN NEW;
    END IF;

    IF OLD.status <> 'PROPOSED' THEN
        RAISE EXCEPTION 'this referral has already been %', lower(OLD.status)
            USING ERRCODE = 'check_violation', CONSTRAINT = 'referral_decided_once';
    END IF;

    IF NEW.status IN ('ACCEPTED', 'DECLINED') THEN
        IF NEW.decided_by IS NULL THEN
            RAISE EXCEPTION 'a referral is accepted or declined by a named officer'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'referral_needs_a_decider';
        END IF;
        SELECT force_id INTO v_decider_force FROM users WHERE id = NEW.decided_by;
        IF NOT (NEW.to_force_id = ANY(force_family(v_decider_force))) THEN
            RAISE EXCEPTION 'a referral is decided by the force it was sent to'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'referral_decided_by_recipient';
        END IF;
        NEW.decided_at := COALESCE(NEW.decided_at, NOW());
    END IF;

    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_referral_decision_rules ON case_referrals;
CREATE TRIGGER trg_referral_decision_rules
    BEFORE UPDATE ON case_referrals
    FOR EACH ROW EXECUTE FUNCTION referral_decision_rules();

-- Has this record been referred to a force, and accepted by it? This is what
-- lets a CID officer see a Kolkata Police case without seeing Kolkata Police.
CREATE OR REPLACE FUNCTION record_referred_to_force(p_type TEXT, p_record UUID, p_force UUID)
RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM case_referrals r
         WHERE r.record_type = p_type
           AND r.record_id = p_record
           AND r.status = 'ACCEPTED'
           AND r.to_force_id = ANY(force_family(p_force))
    )
$$ LANGUAGE sql STABLE;

COMMENT ON TABLE case_referrals IS
    'A record handed from one force to another. The referral is what grants the receiving force sight of it, and it records who asked, why, and who accepted.';
