-- Remove the federated change-capture triggers.
--
-- Migration 000017 attached a capture_changes_* trigger to ten core tables.
-- Each one writes a row to entity_change_log carrying a jurisdiction_id, for a
-- federated deployment that would ship those changes to a parent node.
--
-- Two things are wrong with that here:
--
--   1. The deployment is a single central edge server. There is no parent node
--      and nothing consumes entity_change_log.
--   2. The trigger's jurisdiction_id has a foreign key to jurisdictions, and
--      nothing populates that table. Every INSERT into firs, cases, evidence,
--      warrants, bail, alerts, personnel, vehicles, cyber_crimes and
--      citizen_complaints therefore failed with a foreign key violation.
--
-- The second point is why the core tables were empty: the API returned 500 on
-- every create and the cause was swallowed by the handlers.
--
-- The tables are left in place — they hold no rows and dropping them is not
-- required — but nothing writes to them any more.

DROP TRIGGER IF EXISTS capture_changes_alerts ON alerts;
DROP TRIGGER IF EXISTS capture_changes_bail ON bail;
DROP TRIGGER IF EXISTS capture_changes_cases ON cases;
DROP TRIGGER IF EXISTS capture_changes_citizen_complaints ON citizen_complaints;
DROP TRIGGER IF EXISTS capture_changes_cyber_crimes ON cyber_crimes;
DROP TRIGGER IF EXISTS capture_changes_evidence ON evidence;
DROP TRIGGER IF EXISTS capture_changes_firs ON firs;
DROP TRIGGER IF EXISTS capture_changes_personnel ON personnel;
DROP TRIGGER IF EXISTS capture_changes_vehicles ON vehicles;
DROP TRIGGER IF EXISTS capture_changes_warrants ON warrants;

DROP FUNCTION IF EXISTS capture_entity_change() CASCADE;

COMMENT ON TABLE entity_change_log IS
    'Federated change capture. Not used on a single-server deployment; the triggers that wrote to it were removed by migration 000029.';
