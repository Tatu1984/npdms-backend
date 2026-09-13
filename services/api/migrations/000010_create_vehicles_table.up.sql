-- Create vehicles table
CREATE TABLE IF NOT EXISTS vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    registration_number VARCHAR(50) UNIQUE NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('Patrol', 'Gypsy', 'PCR', 'Bus')),
    make VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('ON_DUTY', 'AVAILABLE', 'MAINTENANCE', 'RESERVED')),
    current_driver UUID REFERENCES users(id) ON DELETE SET NULL,
    fuel_level INTEGER CHECK (fuel_level >= 0 AND fuel_level <= 100),
    odometer_reading BIGINT NOT NULL DEFAULT 0,
    last_service DATE NOT NULL,
    gps_latitude DOUBLE PRECISION,
    gps_longitude DOUBLE PRECISION,
    current_duty TEXT,
    maintenance_note TEXT,
    reserved_for TEXT,
    station_id UUID NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for vehicles
CREATE INDEX idx_vehicles_status ON vehicles(status);
CREATE INDEX idx_vehicles_type ON vehicles(type);
CREATE INDEX idx_vehicles_station_id ON vehicles(station_id);
CREATE INDEX idx_vehicles_current_driver ON vehicles(current_driver);
CREATE INDEX idx_vehicles_registration_number ON vehicles(registration_number);
CREATE INDEX idx_vehicles_gps ON vehicles(gps_latitude, gps_longitude);

-- Create updated_at trigger for vehicles
CREATE TRIGGER update_vehicles_updated_at
    BEFORE UPDATE ON vehicles
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
