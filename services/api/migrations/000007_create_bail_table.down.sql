-- Drop bail table
DROP TRIGGER IF EXISTS update_bail_updated_at ON bail;
DROP INDEX IF EXISTS idx_bail_hearing_date;
DROP INDEX IF EXISTS idx_bail_application_date;
DROP INDEX IF EXISTS idx_bail_accused_id;
DROP INDEX IF EXISTS idx_bail_fir_id;
DROP INDEX IF EXISTS idx_bail_case_id;
DROP INDEX IF EXISTS idx_bail_type;
DROP INDEX IF EXISTS idx_bail_status;
DROP TABLE IF EXISTS bail;
