-- Rollback federated sync tables

-- Drop triggers from main tables
DO $$
DECLARE
    tbl TEXT;
    tables TEXT[] := ARRAY['firs', 'cases', 'evidence', 'warrants', 'bail',
                           'personnel', 'vehicles', 'alerts', 'cyber_crimes',
                           'citizen_complaints'];
BEGIN
    FOREACH tbl IN ARRAY tables
    LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS capture_changes_%I ON %I', tbl, tbl);
    END LOOP;
END $$;

-- Drop functions
DROP FUNCTION IF EXISTS capture_entity_change();
DROP FUNCTION IF EXISTS merge_vector_clocks(JSONB, JSONB);
DROP FUNCTION IF EXISTS compare_vector_clocks(JSONB, JSONB);

-- Drop tables
DROP TABLE IF EXISTS entity_change_log;
DROP TABLE IF EXISTS sync_checkpoints;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS sync_conflicts;
DROP TABLE IF EXISTS sync_inbox;
DROP TABLE IF EXISTS sync_outbox;
DROP TABLE IF EXISTS vector_clocks;
DROP TABLE IF EXISTS jurisdictions;
