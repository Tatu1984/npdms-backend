-- Drop warrants table
DROP TRIGGER IF EXISTS update_warrants_updated_at ON warrants;
DROP INDEX IF EXISTS idx_warrants_valid_until;
DROP INDEX IF EXISTS idx_warrants_issued_date;
DROP INDEX IF EXISTS idx_warrants_fir_id;
DROP INDEX IF EXISTS idx_warrants_case_id;
DROP INDEX IF EXISTS idx_warrants_priority;
DROP INDEX IF EXISTS idx_warrants_type;
DROP INDEX IF EXISTS idx_warrants_status;
DROP TABLE IF EXISTS warrants;
