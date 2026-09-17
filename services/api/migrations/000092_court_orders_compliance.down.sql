-- Remove order compliance and amendment. The hearings consolidation is not
-- undone: it only filled empty columns from abandoned ones, and putting the
-- emptiness back would lose data rather than restore a state.

DROP TRIGGER IF EXISTS trg_court_orders_are_never_deleted ON court_orders;
DROP FUNCTION IF EXISTS court_orders_are_never_deleted();

ALTER TABLE court_orders DROP CONSTRAINT IF EXISTS court_orders_compliance_is_attributed;
DROP INDEX IF EXISTS idx_court_orders_compliance;

ALTER TABLE court_orders
    DROP COLUMN IF EXISTS compliance_status,
    DROP COLUMN IF EXISTS comply_by,
    DROP COLUMN IF EXISTS complied_at,
    DROP COLUMN IF EXISTS complied_by,
    DROP COLUMN IF EXISTS compliance_note,
    DROP COLUMN IF EXISTS amended_at,
    DROP COLUMN IF EXISTS amended_by;

DROP TYPE IF EXISTS court_order_compliance;
