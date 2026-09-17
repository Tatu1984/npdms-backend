-- A court order can be complied with, and a recorded one can be corrected.
--
-- Two defects, one cause: court_orders held what a court directed and nothing
-- about what happened next.
--
-- The dashboard reported "pending orders", and the query behind it counted
-- every order ever recorded, because there was no pending state to count. A
-- figure that never falls is not a figure an officer can act on; it told a
-- station with six compliance directions outstanding the same number as one
-- with none.
--
-- And an order, once recorded, could not be touched. A hearing date typed
-- wrongly, a summary transcribed from the wrong paragraph — nothing could
-- correct either, so the register's only remedy was to record a second order
-- contradicting the first.
--
-- Deletion is deliberately not added. A court order is a record of what a
-- court directed; the platform does not offer to make one disappear, for the
-- same reason an officer's account is closed rather than removed. Correction
-- is amendment, and amendment is audited.

-- ------------------------------------------------- compliance --------------

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'court_order_compliance') THEN
        CREATE TYPE court_order_compliance AS ENUM (
            'PENDING',       -- directed, nothing recorded against it yet
            'COMPLIED',      -- done, with who recorded it and when
            'NOT_COMPLIED',  -- the date passed and it was not done
            'NOT_REQUIRED'   -- the order directs nothing of this force
        );
    END IF;
END$$;

ALTER TABLE court_orders
    ADD COLUMN IF NOT EXISTS compliance_status court_order_compliance NOT NULL DEFAULT 'PENDING';

-- When the order must be complied with. Null where the court set no date.
ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS comply_by DATE;

ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS complied_at   TIMESTAMPTZ;
ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS complied_by   UUID REFERENCES users(id);
ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS compliance_note TEXT;

COMMENT ON COLUMN court_orders.compliance_status IS
    'What happened after the direction. PENDING is the state the dashboard counts.';

-- A settled order names who settled it and when. Recording an outcome without
-- an officer behind it is how a register becomes unanswerable at inspection.
ALTER TABLE court_orders DROP CONSTRAINT IF EXISTS court_orders_compliance_is_attributed;
ALTER TABLE court_orders ADD CONSTRAINT court_orders_compliance_is_attributed
    CHECK (
        compliance_status = 'PENDING'
        OR (complied_at IS NOT NULL AND complied_by IS NOT NULL)
    );

CREATE INDEX IF NOT EXISTS idx_court_orders_compliance
    ON court_orders(compliance_status, comply_by);

-- Orders recorded before this migration are pending by default, which is true:
-- nobody has said otherwise about any of them.

-- ------------------------------------------------- amendment ---------------

ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS amended_at TIMESTAMPTZ;
ALTER TABLE court_orders ADD COLUMN IF NOT EXISTS amended_by UUID REFERENCES users(id);

COMMENT ON COLUMN court_orders.amended_by IS
    'The officer who last corrected this record. The order itself is never deleted.';

-- The platform does not delete a court order. Enforced here and not only in
-- the service, because a rule that lives in one caller lasts until somebody
-- writes a second.
CREATE OR REPLACE FUNCTION court_orders_are_never_deleted() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'A court order is a record of what a court directed and is not deleted. '
                    'Correct it by amendment, or record its compliance.'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_court_orders_are_never_deleted ON court_orders;
CREATE TRIGGER trg_court_orders_are_never_deleted
    BEFORE DELETE ON court_orders
    FOR EACH ROW EXECUTE FUNCTION court_orders_are_never_deleted();

-- ------------------------------------------------- hearings tidy up --------

-- court_hearings carries two names for three things, from the base schema and
-- from migration 000012 on top of it: court_name/court, hearing_type/type,
-- documents_required/required_documents. The code reads the second of each.
-- Consolidating the data is safe; dropping the columns is not, until every
-- reader is known, so this fills the surviving column from the abandoned one
-- where the survivor is empty and leaves both in place. 000032 did the same
-- for warrants before dropping anything.
UPDATE court_hearings SET court = court_name
    WHERE (court IS NULL OR court = '') AND court_name IS NOT NULL;
UPDATE court_hearings SET type = hearing_type
    WHERE (type IS NULL OR type = '') AND hearing_type IS NOT NULL;
UPDATE court_hearings SET required_documents = documents_required
    WHERE (required_documents IS NULL OR cardinality(required_documents) = 0)
      AND documents_required IS NOT NULL;

COMMENT ON COLUMN court_hearings.court_name IS
    'Superseded by court. Kept until every reader is known; see migration 000092.';
COMMENT ON COLUMN court_hearings.hearing_type IS
    'Superseded by type. Kept until every reader is known; see migration 000092.';
COMMENT ON COLUMN court_hearings.documents_required IS
    'Superseded by required_documents. Kept until every reader is known; see migration 000092.';
