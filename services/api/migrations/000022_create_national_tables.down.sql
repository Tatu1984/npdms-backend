-- Down migration for national tables

-- Drop functions
DROP FUNCTION IF EXISTS create_national_snapshot();
DROP FUNCTION IF EXISTS expire_national_alerts();

-- Drop indexes
DROP INDEX IF EXISTS idx_national_most_wanted_state;
DROP INDEX IF EXISTS idx_national_most_wanted_danger;
DROP INDEX IF EXISTS idx_national_most_wanted_status;

DROP INDEX IF EXISTS idx_national_meetings_chairperson;
DROP INDEX IF EXISTS idx_national_meetings_status;
DROP INDEX IF EXISTS idx_national_meetings_date;

DROP INDEX IF EXISTS idx_national_performance_type;
DROP INDEX IF EXISTS idx_national_performance_date;

DROP INDEX IF EXISTS idx_national_crime_patterns_severity;
DROP INDEX IF EXISTS idx_national_crime_patterns_status;
DROP INDEX IF EXISTS idx_national_crime_patterns_category;
DROP INDEX IF EXISTS idx_national_crime_patterns_type;

DROP INDEX IF EXISTS idx_inter_state_requests_created;
DROP INDEX IF EXISTS idx_inter_state_requests_priority;
DROP INDEX IF EXISTS idx_inter_state_requests_status;
DROP INDEX IF EXISTS idx_inter_state_requests_target;
DROP INDEX IF EXISTS idx_inter_state_requests_requesting;

DROP INDEX IF EXISTS idx_national_alerts_created;
DROP INDEX IF EXISTS idx_national_alerts_expires;
DROP INDEX IF EXISTS idx_national_alerts_status;
DROP INDEX IF EXISTS idx_national_alerts_severity;
DROP INDEX IF EXISTS idx_national_alerts_type;

-- Drop tables
DROP TABLE IF EXISTS national_most_wanted;
DROP TABLE IF EXISTS national_coordination_meetings;
DROP TABLE IF EXISTS national_performance_snapshots;
DROP TABLE IF EXISTS national_crime_patterns;
DROP TABLE IF EXISTS inter_state_requests;
DROP TABLE IF EXISTS national_alerts;
