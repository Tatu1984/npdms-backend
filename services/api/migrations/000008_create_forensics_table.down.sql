-- Drop forensics table
DROP TRIGGER IF EXISTS update_forensics_updated_at ON forensics;
DROP INDEX IF EXISTS idx_forensics_expected_date;
DROP INDEX IF EXISTS idx_forensics_submitted_date;
DROP INDEX IF EXISTS idx_forensics_evidence_id;
DROP INDEX IF EXISTS idx_forensics_case_id;
DROP INDEX IF EXISTS idx_forensics_priority;
DROP INDEX IF EXISTS idx_forensics_type;
DROP INDEX IF EXISTS idx_forensics_status;
DROP TABLE IF EXISTS forensics;
