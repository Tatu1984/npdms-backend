-- Drop Cyber Crime Tables

DROP TRIGGER IF EXISTS update_cyber_crimes_updated_at ON cyber_crimes;

DROP INDEX IF EXISTS idx_financial_trails_crime;
DROP INDEX IF EXISTS idx_osint_crime;
DROP INDEX IF EXISTS idx_digital_evidence_crime;
DROP INDEX IF EXISTS idx_cyber_crimes_reported;
DROP INDEX IF EXISTS idx_cyber_crimes_io;
DROP INDEX IF EXISTS idx_cyber_crimes_fir;
DROP INDEX IF EXISTS idx_cyber_crimes_status;
DROP INDEX IF EXISTS idx_cyber_crimes_type;

DROP TABLE IF EXISTS financial_trails;
DROP TABLE IF EXISTS osint_reports;
DROP TABLE IF EXISTS digital_evidence;
DROP TABLE IF EXISTS cyber_crimes;

DROP TYPE IF EXISTS platform_type;
DROP TYPE IF EXISTS cyber_crime_status;
DROP TYPE IF EXISTS cyber_crime_type;
