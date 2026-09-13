-- Drop alerts table
DROP TRIGGER IF EXISTS update_alerts_updated_at ON alerts;
DROP INDEX IF EXISTS idx_alerts_unacknowledged;
DROP INDEX IF EXISTS idx_alerts_active;
DROP INDEX IF EXISTS idx_alerts_priority;
DROP INDEX IF EXISTS idx_alerts_station_id;
DROP INDEX IF EXISTS idx_alerts_expires_at;
DROP INDEX IF EXISTS idx_alerts_issued_at;
DROP INDEX IF EXISTS idx_alerts_acknowledged;
DROP INDEX IF EXISTS idx_alerts_scope;
DROP INDEX IF EXISTS idx_alerts_type;
DROP TABLE IF EXISTS alerts;
