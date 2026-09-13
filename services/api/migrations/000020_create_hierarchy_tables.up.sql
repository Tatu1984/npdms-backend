-- Hierarchy Tables for Police Administrative Structure

-- States
CREATE TABLE IF NOT EXISTS states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    code VARCHAR(5) NOT NULL UNIQUE,
    dgp_name VARCHAR(200),
    dgp_phone VARCHAR(20),
    dgp_email VARCHAR(255),
    population BIGINT,
    area DECIMAL(15,2),
    total_officers INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Zones
CREATE TABLE IF NOT EXISTS zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(10) NOT NULL,
    ig_name VARCHAR(200),
    ig_phone VARCHAR(20),
    ig_email VARCHAR(255),
    headquarters VARCHAR(200),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(state_id, code)
);

-- Ranges
CREATE TABLE IF NOT EXISTS ranges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id UUID NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(10) NOT NULL,
    dig_name VARCHAR(200),
    dig_phone VARCHAR(20),
    dig_email VARCHAR(255),
    headquarters VARCHAR(200),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(zone_id, code)
);

-- Districts
CREATE TABLE IF NOT EXISTS districts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    range_id UUID NOT NULL REFERENCES ranges(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(10) NOT NULL,
    sp_name VARCHAR(200),
    sp_phone VARCHAR(20),
    sp_email VARCHAR(255),
    headquarters VARCHAR(200),
    population BIGINT,
    area DECIMAL(15,2),
    total_stations INTEGER DEFAULT 0,
    total_officers INTEGER DEFAULT 0,
    control_room_num VARCHAR(20),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(range_id, code)
);

-- Divisions (Sub-Divisions within Districts)
CREATE TABLE IF NOT EXISTS divisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    district_id UUID NOT NULL REFERENCES districts(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(10) NOT NULL,
    dsp_name VARCHAR(200),
    dsp_phone VARCHAR(20),
    dsp_email VARCHAR(255),
    headquarters VARCHAR(200),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(district_id, code)
);

-- Police Stations
CREATE TABLE IF NOT EXISTS police_stations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    division_id UUID REFERENCES divisions(id) ON DELETE SET NULL,
    district_id UUID NOT NULL REFERENCES districts(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(15) NOT NULL,
    type VARCHAR(20) DEFAULT 'URBAN' CHECK (type IN ('URBAN', 'RURAL', 'WOMEN', 'CYBER', 'TRAFFIC', 'RAILWAY', 'AIRPORT')),
    sho_name VARCHAR(200),
    sho_id UUID REFERENCES users(id),
    phone VARCHAR(20),
    emergency_phone VARCHAR(20),
    email VARCHAR(255),
    address TEXT,
    latitude DECIMAL(10,8),
    longitude DECIMAL(11,8),
    jurisdiction_area DECIMAL(15,2),
    population BIGINT,
    total_officers INTEGER DEFAULT 0,
    sanctioned INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(district_id, code)
);

-- Cross-Station Coordination Requests
CREATE TABLE IF NOT EXISTS cross_station_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_type VARCHAR(20) NOT NULL CHECK (request_type IN ('TRANSFER', 'ASSISTANCE', 'INFORMATION', 'JOINT_OPERATION')),
    requesting_station UUID NOT NULL REFERENCES police_stations(id),
    target_station UUID NOT NULL REFERENCES police_stations(id),
    related_fir UUID REFERENCES firs(id),
    related_case UUID REFERENCES cases(id),
    priority VARCHAR(10) DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'URGENT')),
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    subject VARCHAR(500) NOT NULL,
    description TEXT,
    requested_by UUID NOT NULL REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    approved_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- District Coordination Meetings
CREATE TABLE IF NOT EXISTS district_coordination_meetings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    district_id UUID NOT NULL REFERENCES districts(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    agenda TEXT,
    meeting_date TIMESTAMP WITH TIME ZONE NOT NULL,
    venue VARCHAR(200),
    chairperson_id UUID NOT NULL REFERENCES users(id),
    participants UUID[] DEFAULT '{}',
    minutes TEXT,
    decisions TEXT,
    status VARCHAR(20) DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'POSTPONED')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Resource Allocation
CREATE TABLE IF NOT EXISTS resource_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    district_id UUID NOT NULL REFERENCES districts(id) ON DELETE CASCADE,
    resource_type VARCHAR(30) NOT NULL CHECK (resource_type IN ('VEHICLE', 'WEAPON', 'EQUIPMENT', 'PERSONNEL', 'COMMUNICATION')),
    resource_id UUID NOT NULL,
    from_station UUID REFERENCES police_stations(id),
    to_station UUID NOT NULL REFERENCES police_stations(id),
    quantity INTEGER DEFAULT 1,
    purpose TEXT NOT NULL,
    duration VARCHAR(20) DEFAULT 'TEMPORARY' CHECK (duration IN ('TEMPORARY', 'PERMANENT')),
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'ALLOCATED', 'RETURNED', 'CANCELLED')),
    approved_by UUID REFERENCES users(id),
    requested_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_zones_state ON zones(state_id);
CREATE INDEX idx_ranges_zone ON ranges(zone_id);
CREATE INDEX idx_districts_range ON districts(range_id);
CREATE INDEX idx_divisions_district ON divisions(district_id);
CREATE INDEX idx_police_stations_district ON police_stations(district_id);
CREATE INDEX idx_police_stations_division ON police_stations(division_id);
CREATE INDEX idx_police_stations_sho ON police_stations(sho_id);
CREATE INDEX idx_police_stations_type ON police_stations(type);
CREATE INDEX idx_police_stations_location ON police_stations(latitude, longitude);

CREATE INDEX idx_cross_station_requests_requesting ON cross_station_requests(requesting_station);
CREATE INDEX idx_cross_station_requests_target ON cross_station_requests(target_station);
CREATE INDEX idx_cross_station_requests_status ON cross_station_requests(status);
CREATE INDEX idx_cross_station_requests_priority ON cross_station_requests(priority);
CREATE INDEX idx_cross_station_requests_requested_by ON cross_station_requests(requested_by);

CREATE INDEX idx_district_meetings_district ON district_coordination_meetings(district_id);
CREATE INDEX idx_district_meetings_date ON district_coordination_meetings(meeting_date);
CREATE INDEX idx_district_meetings_status ON district_coordination_meetings(status);
CREATE INDEX idx_district_meetings_chairperson ON district_coordination_meetings(chairperson_id);

CREATE INDEX idx_resource_allocations_district ON resource_allocations(district_id);
CREATE INDEX idx_resource_allocations_to_station ON resource_allocations(to_station);
CREATE INDEX idx_resource_allocations_from_station ON resource_allocations(from_station);
CREATE INDEX idx_resource_allocations_status ON resource_allocations(status);
CREATE INDEX idx_resource_allocations_type ON resource_allocations(resource_type);

-- Trigger to update station count in district
CREATE OR REPLACE FUNCTION update_district_station_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE districts SET total_stations = total_stations + 1 WHERE id = NEW.district_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE districts SET total_stations = total_stations - 1 WHERE id = OLD.district_id;
    ELSIF TG_OP = 'UPDATE' AND OLD.district_id != NEW.district_id THEN
        UPDATE districts SET total_stations = total_stations - 1 WHERE id = OLD.district_id;
        UPDATE districts SET total_stations = total_stations + 1 WHERE id = NEW.district_id;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_district_station_count
AFTER INSERT OR UPDATE OR DELETE ON police_stations
FOR EACH ROW EXECUTE FUNCTION update_district_station_count();

-- Insert sample data for Indian police hierarchy
INSERT INTO states (name, code, dgp_name) VALUES
    ('Maharashtra', 'MH', 'DGP Maharashtra'),
    ('Delhi', 'DL', 'Commissioner of Police'),
    ('Karnataka', 'KA', 'DGP Karnataka'),
    ('Tamil Nadu', 'TN', 'DGP Tamil Nadu'),
    ('Uttar Pradesh', 'UP', 'DGP Uttar Pradesh')
ON CONFLICT (code) DO NOTHING;

-- Insert zones for Maharashtra
INSERT INTO zones (state_id, name, code, headquarters)
SELECT s.id, 'Mumbai Zone', 'MH-Z1', 'Mumbai'
FROM states s WHERE s.code = 'MH'
ON CONFLICT (state_id, code) DO NOTHING;

INSERT INTO zones (state_id, name, code, headquarters)
SELECT s.id, 'Pune Zone', 'MH-Z2', 'Pune'
FROM states s WHERE s.code = 'MH'
ON CONFLICT (state_id, code) DO NOTHING;

-- Insert ranges
INSERT INTO ranges (zone_id, name, code, headquarters)
SELECT z.id, 'Mumbai City Range', 'MH-Z1-R1', 'Mumbai'
FROM zones z WHERE z.code = 'MH-Z1'
ON CONFLICT (zone_id, code) DO NOTHING;

INSERT INTO ranges (zone_id, name, code, headquarters)
SELECT z.id, 'Thane Range', 'MH-Z1-R2', 'Thane'
FROM zones z WHERE z.code = 'MH-Z1'
ON CONFLICT (zone_id, code) DO NOTHING;

-- Insert districts
INSERT INTO districts (range_id, name, code, headquarters, control_room_num)
SELECT r.id, 'Mumbai City', 'MH-MUM', 'Mumbai', '100'
FROM ranges r WHERE r.code = 'MH-Z1-R1'
ON CONFLICT (range_id, code) DO NOTHING;

INSERT INTO districts (range_id, name, code, headquarters, control_room_num)
SELECT r.id, 'Thane', 'MH-THN', 'Thane', '100'
FROM ranges r WHERE r.code = 'MH-Z1-R2'
ON CONFLICT (range_id, code) DO NOTHING;
