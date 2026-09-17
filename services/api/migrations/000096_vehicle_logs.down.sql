-- Remove the fleet logs. The vehicles table keeps whatever odometer reading
-- the logs last moved it to; it is not rolled back, because a reading that was
-- true is not made untrue by deleting the record that explained it.

DROP TRIGGER IF EXISTS trg_fill_moves_the_odometer ON vehicle_fuel_logs;
DROP TRIGGER IF EXISTS trg_trip_moves_the_odometer ON vehicle_trips;
DROP FUNCTION IF EXISTS vehicle_odometer_from_trip();
DROP FUNCTION IF EXISTS vehicle_odometer_from_reading();

DROP TABLE IF EXISTS vehicle_maintenance;
DROP TABLE IF EXISTS vehicle_fuel_logs;
DROP TABLE IF EXISTS vehicle_trips;
DROP TYPE IF EXISTS vehicle_maintenance_kind;
