-- Drop vehicles table
DROP TRIGGER IF EXISTS update_vehicles_updated_at ON vehicles;
DROP INDEX IF EXISTS idx_vehicles_gps;
DROP INDEX IF EXISTS idx_vehicles_registration_number;
DROP INDEX IF EXISTS idx_vehicles_current_driver;
DROP INDEX IF EXISTS idx_vehicles_station_id;
DROP INDEX IF EXISTS idx_vehicles_type;
DROP INDEX IF EXISTS idx_vehicles_status;
DROP TABLE IF EXISTS vehicles;
