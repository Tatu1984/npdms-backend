-- Down migration for state-level tables

-- Drop functions
DROP FUNCTION IF EXISTS check_and_expire_alerts();
DROP FUNCTION IF EXISTS expire_state_alerts();

-- Drop indexes
DROP INDEX IF EXISTS idx_state_resource_requests_type;
DROP INDEX IF EXISTS idx_state_resource_requests_status;
DROP INDEX IF EXISTS idx_state_resource_requests_district;
DROP INDEX IF EXISTS idx_state_resource_requests_state;

DROP INDEX IF EXISTS idx_state_performance_type;
DROP INDEX IF EXISTS idx_state_performance_date;
DROP INDEX IF EXISTS idx_state_performance_state;

DROP INDEX IF EXISTS idx_state_crime_patterns_status;
DROP INDEX IF EXISTS idx_state_crime_patterns_category;
DROP INDEX IF EXISTS idx_state_crime_patterns_state;

DROP INDEX IF EXISTS idx_state_meetings_chairperson;
DROP INDEX IF EXISTS idx_state_meetings_status;
DROP INDEX IF EXISTS idx_state_meetings_date;
DROP INDEX IF EXISTS idx_state_meetings_state;

DROP INDEX IF EXISTS idx_state_alerts_expires;
DROP INDEX IF EXISTS idx_state_alerts_status;
DROP INDEX IF EXISTS idx_state_alerts_severity;
DROP INDEX IF EXISTS idx_state_alerts_type;
DROP INDEX IF EXISTS idx_state_alerts_state;

DROP INDEX IF EXISTS idx_inter_district_requests_created;
DROP INDEX IF EXISTS idx_inter_district_requests_priority;
DROP INDEX IF EXISTS idx_inter_district_requests_status;
DROP INDEX IF EXISTS idx_inter_district_requests_target;
DROP INDEX IF EXISTS idx_inter_district_requests_requesting;
DROP INDEX IF EXISTS idx_inter_district_requests_state;

-- Drop tables
DROP TABLE IF EXISTS state_resource_requests;
DROP TABLE IF EXISTS state_performance_snapshots;
DROP TABLE IF EXISTS state_crime_patterns;
DROP TABLE IF EXISTS state_coordination_meetings;
DROP TABLE IF EXISTS state_alerts;
DROP TABLE IF EXISTS inter_district_requests;
