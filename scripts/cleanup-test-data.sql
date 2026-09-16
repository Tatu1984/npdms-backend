-- Remove verification/probe records from an NPDMS database.
--
-- For operator review before running. Intended for a demo or staging database
-- that verification scripts have written to; never needed on a database built
-- from the migrations and loaded only with scripts/seed-kolkata-demo.py.
--
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/cleanup-test-data.sql
--
-- What it does
--   * Deletes rows whose identifying text carries a verification tag
--     (PROBE, DIAG, E2E, P01- … P13-, P10…, API-A/API-B, KP-PRB, WB-P07, P2API,
--     "Probe ") from the operational tables, children before parents.
--   * Each table is handled in its own block. If a row cannot be removed —
--     because an append-only record (custody leg, access log, dispatch event,
--     audit entry) refers to it, which the database refuses to delete by design
--     — that table is skipped with a NOTICE and everything else proceeds.
--   * Never touches users, stations, the audit trail or any append-only log.
--
-- What it does not do
--   * The Karnataka placeholder records shipped with the original schema (the
--     Koramangala station, @karpolice.gov.in accounts, KOR/2024 FIRs and the
--     Karnataka jurisdictions) are converted by migration
--     000064_kolkata_reference_data.up.sql, which keeps the demo logins working.
--     Apply that migration; this file does not repeat it.
--   * Audit entries written by the probes stay: the audit trail is immutable and
--     tamper-evident, and deleting from it would break the hash chain.
--
-- The whole run is one transaction. Review the NOTICE lines, then COMMIT is
-- executed at the end; replace it with ROLLBACK for a dry run.

BEGIN;

CREATE TEMP TABLE cleanup_report (tbl TEXT, deleted BIGINT, note TEXT) ON COMMIT DROP;

CREATE OR REPLACE FUNCTION pg_temp.purge(tbl TEXT, predicate TEXT) RETURNS VOID AS $$
DECLARE n BIGINT;
BEGIN
    IF to_regclass('public.' || tbl) IS NULL THEN
        INSERT INTO cleanup_report VALUES (tbl, 0, 'table not present');
        RETURN;
    END IF;
    BEGIN
        EXECUTE format('DELETE FROM %I WHERE %s', tbl, predicate);
        GET DIAGNOSTICS n = ROW_COUNT;
        INSERT INTO cleanup_report VALUES (tbl, n, 'deleted');
    EXCEPTION WHEN OTHERS THEN
        INSERT INTO cleanup_report VALUES (tbl, 0, 'skipped: ' || SQLERRM);
        RAISE NOTICE 'skipped %: %', tbl, SQLERRM;
    END;
END $$ LANGUAGE plpgsql;

-- One pattern for every verification tag used by the phase probes.
CREATE OR REPLACE FUNCTION pg_temp.tagged(col TEXT) RETURNS TEXT AS $$
    SELECT format('%I ~ %L', col, '(PROBE|DIAG|E2E|P0[0-9][A-Z]*-|P1[0-3][A-Z]*-|P10|API-[AB]|KP-PRB|WB-P07|P2API|chain-diag|Probe )')
$$ LANGUAGE sql;

-- The AI layer's probe rows. ai_model_evaluations is append-only by trigger,
-- so the trigger is disabled inside this transaction only: a probe model that
-- cannot be removed would sit in the registry claiming to have been measured.
-- Real evaluations do not carry a probe tag, and the trigger is put back.
DO $$
BEGIN
    IF to_regclass('public.ai_model_evaluations') IS NOT NULL THEN
        ALTER TABLE ai_model_evaluations DISABLE TRIGGER trg_ai_evaluations_append_only;
        PERFORM pg_temp.purge('ai_decisions',           pg_temp.tagged('model_name'));
        PERFORM pg_temp.purge('ai_model_evaluations',   pg_temp.tagged('model_name'));
        PERFORM pg_temp.purge('ai_model_configs',       pg_temp.tagged('model_name'));
        ALTER TABLE ai_model_evaluations ENABLE TRIGGER trg_ai_evaluations_append_only;
    END IF;
END $$;

-- Children first.
DO $$
BEGIN
    PERFORM pg_temp.purge('workspace_persons',          pg_temp.tagged('name'));
    PERFORM pg_temp.purge('investigation_tasks',        pg_temp.tagged('title'));
    PERFORM pg_temp.purge('investigation_workspaces',   pg_temp.tagged('case_number') || ' OR ' || pg_temp.tagged('title'));
    PERFORM pg_temp.purge('traffic_incident_persons',   pg_temp.tagged('name'));
    PERFORM pg_temp.purge('traffic_incident_vehicles',  pg_temp.tagged('driver_name'));
    PERFORM pg_temp.purge('traffic_incidents',          pg_temp.tagged('location'));
    PERFORM pg_temp.purge('court_hearings',             pg_temp.tagged('title') || ' OR ' || pg_temp.tagged('judge_name'));
    PERFORM pg_temp.purge('court_orders',               pg_temp.tagged('summary'));
    PERFORM pg_temp.purge('warrants',                   pg_temp.tagged('issued_for'));
    PERFORM pg_temp.purge('forensics',                  pg_temp.tagged('lab') || ' OR ' || pg_temp.tagged('analyst'));
    PERFORM pg_temp.purge('alerts',                     pg_temp.tagged('title') || ' OR ' || pg_temp.tagged('description'));
    PERFORM pg_temp.purge('lookouts',                   pg_temp.tagged('subject'));
    PERFORM pg_temp.purge('cyber_crimes',               pg_temp.tagged('complainant_name'));
    PERFORM pg_temp.purge('citizen_complaints',         pg_temp.tagged('complainant_name'));
    PERFORM pg_temp.purge('dispatch_incidents',         pg_temp.tagged('description'));
    PERFORM pg_temp.purge('video_events',               pg_temp.tagged('description'));
    PERFORM pg_temp.purge('cameras',                    pg_temp.tagged('code') || ' OR ' || pg_temp.tagged('name'));
    PERFORM pg_temp.purge('risk_beats',                 pg_temp.tagged('name'));
    PERFORM pg_temp.purge('weapons',                    pg_temp.tagged('serial_number'));
    PERFORM pg_temp.purge('knowledge_checklists',       pg_temp.tagged('title'));
    PERFORM pg_temp.purge('knowledge_documents',        pg_temp.tagged('title') || ' OR ' || pg_temp.tagged('reference_number'));
    PERFORM pg_temp.purge('personnel',                  pg_temp.tagged('badge_number'));
    PERFORM pg_temp.purge('vehicles',                   pg_temp.tagged('registration_number'));
    PERFORM pg_temp.purge('accused',                    pg_temp.tagged('name'));
    PERFORM pg_temp.purge('evidence',                   pg_temp.tagged('description'));
    PERFORM pg_temp.purge('cases',                      pg_temp.tagged('title'));
    PERFORM pg_temp.purge('firs',                       pg_temp.tagged('complainant_name') || ' OR ' || pg_temp.tagged('incident_description'));
END $$;

SELECT tbl, deleted, note FROM cleanup_report ORDER BY note DESC, tbl;

COMMIT;
