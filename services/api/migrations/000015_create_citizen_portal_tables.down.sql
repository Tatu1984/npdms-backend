-- Drop Citizen Portal Tables

DROP TRIGGER IF EXISTS update_fir_requests_updated_at ON public_fir_requests;
DROP TRIGGER IF EXISTS update_missing_reports_updated_at ON missing_person_reports;
DROP TRIGGER IF EXISTS update_grievances_updated_at ON grievances;
DROP TRIGGER IF EXISTS update_complaints_updated_at ON citizen_complaints;

DROP INDEX IF EXISTS idx_fir_requests_status;
DROP INDEX IF EXISTS idx_fir_requests_fir;
DROP INDEX IF EXISTS idx_fir_requests_number;
DROP INDEX IF EXISTS idx_missing_reports_station;
DROP INDEX IF EXISTS idx_missing_reports_status;
DROP INDEX IF EXISTS idx_missing_reports_number;
DROP INDEX IF EXISTS idx_grievances_officer;
DROP INDEX IF EXISTS idx_grievances_status;
DROP INDEX IF EXISTS idx_grievances_number;
DROP INDEX IF EXISTS idx_complaint_updates_public;
DROP INDEX IF EXISTS idx_complaint_updates_complaint;
DROP INDEX IF EXISTS idx_complaints_submitted;
DROP INDEX IF EXISTS idx_complaints_station;
DROP INDEX IF EXISTS idx_complaints_category;
DROP INDEX IF EXISTS idx_complaints_status;
DROP INDEX IF EXISTS idx_complaints_tracking;

DROP TABLE IF EXISTS public_fir_requests;
DROP TABLE IF EXISTS missing_person_reports;
DROP TABLE IF EXISTS grievances;
DROP TABLE IF EXISTS complaint_updates;
DROP TABLE IF EXISTS citizen_complaints;

DROP TYPE IF EXISTS complaint_category;
DROP TYPE IF EXISTS complaint_status;
