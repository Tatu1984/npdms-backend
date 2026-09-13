-- Down migration for hierarchy tables

-- Drop trigger
DROP TRIGGER IF EXISTS trg_update_district_station_count ON police_stations;
DROP FUNCTION IF EXISTS update_district_station_count();

-- Drop indexes
DROP INDEX IF EXISTS idx_resource_allocations_type;
DROP INDEX IF EXISTS idx_resource_allocations_status;
DROP INDEX IF EXISTS idx_resource_allocations_from_station;
DROP INDEX IF EXISTS idx_resource_allocations_to_station;
DROP INDEX IF EXISTS idx_resource_allocations_district;

DROP INDEX IF EXISTS idx_district_meetings_chairperson;
DROP INDEX IF EXISTS idx_district_meetings_status;
DROP INDEX IF EXISTS idx_district_meetings_date;
DROP INDEX IF EXISTS idx_district_meetings_district;

DROP INDEX IF EXISTS idx_cross_station_requests_requested_by;
DROP INDEX IF EXISTS idx_cross_station_requests_priority;
DROP INDEX IF EXISTS idx_cross_station_requests_status;
DROP INDEX IF EXISTS idx_cross_station_requests_target;
DROP INDEX IF EXISTS idx_cross_station_requests_requesting;

DROP INDEX IF EXISTS idx_police_stations_location;
DROP INDEX IF EXISTS idx_police_stations_type;
DROP INDEX IF EXISTS idx_police_stations_sho;
DROP INDEX IF EXISTS idx_police_stations_division;
DROP INDEX IF EXISTS idx_police_stations_district;
DROP INDEX IF EXISTS idx_divisions_district;
DROP INDEX IF EXISTS idx_districts_range;
DROP INDEX IF EXISTS idx_ranges_zone;
DROP INDEX IF EXISTS idx_zones_state;

-- Drop tables in reverse order
DROP TABLE IF EXISTS resource_allocations;
DROP TABLE IF EXISTS district_coordination_meetings;
DROP TABLE IF EXISTS cross_station_requests;
DROP TABLE IF EXISTS police_stations;
DROP TABLE IF EXISTS divisions;
DROP TABLE IF EXISTS districts;
DROP TABLE IF EXISTS ranges;
DROP TABLE IF EXISTS zones;
DROP TABLE IF EXISTS states;
