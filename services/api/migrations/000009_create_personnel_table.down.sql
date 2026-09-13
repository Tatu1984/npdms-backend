-- Drop personnel table
DROP TRIGGER IF EXISTS update_personnel_updated_at ON personnel;
DROP INDEX IF EXISTS idx_personnel_badge_number;
DROP INDEX IF EXISTS idx_personnel_user_id;
DROP INDEX IF EXISTS idx_personnel_station_id;
DROP INDEX IF EXISTS idx_personnel_rank;
DROP INDEX IF EXISTS idx_personnel_status;
DROP TABLE IF EXISTS personnel;
