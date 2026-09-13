-- Drop tables in reverse order to handle foreign key constraints
DROP TABLE IF EXISTS leave_balances;
DROP TABLE IF EXISTS leave_requests;
DROP TABLE IF EXISTS shifts;
DROP TABLE IF EXISTS daily_attendance;
DROP TABLE IF EXISTS attendance_records;
DROP TABLE IF EXISTS weapon_access_biometrics;
DROP TABLE IF EXISTS evidence_custody_biometrics;
DROP TABLE IF EXISTS suspect_identifications;
DROP TABLE IF EXISTS biometric_verifications;
DROP TABLE IF EXISTS biometric_templates;
DROP TABLE IF EXISTS biometric_devices;
