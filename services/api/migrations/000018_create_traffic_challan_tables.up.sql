-- Traffic Challan Management System
-- Implements: Traffic violations, e-challans, payment tracking

-- Traffic violations master table
CREATE TABLE traffic_violation_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    section VARCHAR(100) NOT NULL, -- MV Act section
    fine_amount BIGINT NOT NULL, -- In paise (lowest denomination)
    compoundable BOOLEAN DEFAULT TRUE,
    points_deducted INTEGER DEFAULT 0,
    license_suspension_days INTEGER DEFAULT 0,
    vehicle_seizure BOOLEAN DEFAULT FALSE,
    category VARCHAR(50) NOT NULL CHECK (category IN (
        'SPEEDING', 'SIGNAL_VIOLATION', 'DOCUMENT_VIOLATION',
        'SAFETY_VIOLATION', 'PARKING_VIOLATION', 'DRUNK_DRIVING',
        'DANGEROUS_DRIVING', 'EMISSION_VIOLATION', 'OVERLOADING',
        'OTHER'
    )),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('MINOR', 'MODERATE', 'MAJOR', 'SEVERE')),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Traffic challans table
CREATE TABLE traffic_challans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challan_number VARCHAR(50) UNIQUE NOT NULL,

    -- Violation details
    violation_type_id UUID NOT NULL REFERENCES traffic_violation_types(id),
    violation_date TIMESTAMPTZ NOT NULL,
    violation_location TEXT NOT NULL,
    violation_latitude DOUBLE PRECISION,
    violation_longitude DOUBLE PRECISION,
    violation_description TEXT,

    -- Vehicle details
    vehicle_number VARCHAR(20) NOT NULL,
    vehicle_type VARCHAR(50) NOT NULL CHECK (vehicle_type IN (
        'TWO_WHEELER', 'THREE_WHEELER', 'FOUR_WHEELER', 'COMMERCIAL',
        'HEAVY_VEHICLE', 'TRANSPORT', 'OTHER'
    )),
    vehicle_make VARCHAR(100),
    vehicle_model VARCHAR(100),
    vehicle_color VARCHAR(50),
    chassis_number VARCHAR(50),
    engine_number VARCHAR(50),

    -- Owner/Driver details
    owner_name VARCHAR(255),
    owner_address TEXT,
    owner_phone VARCHAR(20),
    driver_name VARCHAR(255),
    driver_license_number VARCHAR(50),
    driver_license_state VARCHAR(50),
    driver_license_valid_until DATE,
    driver_phone VARCHAR(20),
    driver_address TEXT,

    -- Issuing officer
    issuing_officer_id UUID REFERENCES users(id),
    issuing_station_id UUID NOT NULL,
    issuing_officer_name VARCHAR(255),
    issuing_officer_badge VARCHAR(50),

    -- Documents verified
    rc_verified BOOLEAN DEFAULT FALSE,
    license_verified BOOLEAN DEFAULT FALSE,
    insurance_verified BOOLEAN DEFAULT FALSE,
    puc_verified BOOLEAN DEFAULT FALSE,

    -- Fine details
    original_fine_amount BIGINT NOT NULL,
    discount_amount BIGINT DEFAULT 0,
    penalty_amount BIGINT DEFAULT 0, -- Late payment penalty
    final_amount BIGINT NOT NULL,
    payment_due_date DATE NOT NULL,

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'ISSUED' CHECK (status IN (
        'ISSUED', 'PENDING', 'PAID', 'DISPUTED', 'CANCELLED',
        'COURT_REFERRED', 'COMPOUNDED', 'DEFAULTED'
    )),

    -- Payment details
    payment_date TIMESTAMPTZ,
    payment_method VARCHAR(50),
    payment_reference VARCHAR(100),
    payment_gateway VARCHAR(50),

    -- Court details (if referred)
    court_date DATE,
    court_name VARCHAR(255),
    court_order_number VARCHAR(100),
    court_decision TEXT,

    -- Evidence
    evidence_photos TEXT[], -- URLs to photos
    evidence_videos TEXT[], -- URLs to videos
    speed_reading DECIMAL(10, 2), -- For speeding violations
    breath_analyzer_reading DECIMAL(5, 2), -- For drunk driving

    -- Additional actions
    license_seized BOOLEAN DEFAULT FALSE,
    vehicle_seized BOOLEAN DEFAULT FALSE,
    towing_required BOOLEAN DEFAULT FALSE,
    towing_reference VARCHAR(100),

    -- Disputes
    dispute_filed BOOLEAN DEFAULT FALSE,
    dispute_date TIMESTAMPTZ,
    dispute_reason TEXT,
    dispute_status VARCHAR(20),
    dispute_resolution TEXT,

    -- Sync metadata
    sync_status VARCHAR(20) DEFAULT 'LOCAL',
    synced_to_central BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Payment transactions for challans
CREATE TABLE challan_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challan_id UUID NOT NULL REFERENCES traffic_challans(id) ON DELETE CASCADE,

    -- Transaction details
    transaction_id VARCHAR(100) UNIQUE NOT NULL,
    payment_gateway VARCHAR(50) NOT NULL,
    payment_method VARCHAR(50) NOT NULL CHECK (payment_method IN (
        'CASH', 'UPI', 'DEBIT_CARD', 'CREDIT_CARD', 'NET_BANKING',
        'WALLET', 'CHEQUE', 'DEMAND_DRAFT'
    )),

    amount BIGINT NOT NULL,
    gateway_fee BIGINT DEFAULT 0,
    total_amount BIGINT NOT NULL,

    -- Status
    status VARCHAR(20) NOT NULL CHECK (status IN (
        'INITIATED', 'PENDING', 'SUCCESS', 'FAILED', 'REFUNDED'
    )),

    -- Gateway response
    gateway_response JSONB,
    gateway_transaction_id VARCHAR(255),

    -- Timestamps
    initiated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    -- Receipt
    receipt_number VARCHAR(50),
    receipt_generated BOOLEAN DEFAULT FALSE
);

-- Defaulter tracking
CREATE TABLE challan_defaulters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_number VARCHAR(20) NOT NULL,
    owner_name VARCHAR(255),
    owner_phone VARCHAR(20),

    total_challans INTEGER NOT NULL DEFAULT 0,
    total_pending_amount BIGINT NOT NULL DEFAULT 0,
    oldest_pending_date DATE,

    -- Status
    blacklisted BOOLEAN DEFAULT FALSE,
    blacklist_date DATE,
    blacklist_reason TEXT,

    -- Actions taken
    notice_sent BOOLEAN DEFAULT FALSE,
    notice_date DATE,
    court_notice_sent BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(vehicle_number)
);

-- Challan statistics by location
CREATE TABLE challan_hotspots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_name VARCHAR(255) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    radius_meters INTEGER DEFAULT 100,

    -- Statistics (updated periodically)
    total_challans INTEGER DEFAULT 0,
    challans_last_30_days INTEGER DEFAULT 0,
    most_common_violation VARCHAR(50),
    peak_hours TEXT[], -- Array of peak hours

    -- Recommendations
    requires_signal BOOLEAN DEFAULT FALSE,
    requires_speed_breaker BOOLEAN DEFAULT FALSE,
    requires_patrol BOOLEAN DEFAULT FALSE,
    recommendation_notes TEXT,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert common violation types (Motor Vehicles Act 2019)
INSERT INTO traffic_violation_types (code, name, section, fine_amount, category, severity, points_deducted) VALUES
    ('SPD001', 'Speeding - Light Motor Vehicle', 'Section 183', 100000, 'SPEEDING', 'MODERATE', 3),
    ('SPD002', 'Speeding - Two Wheeler', 'Section 183', 200000, 'SPEEDING', 'MODERATE', 3),
    ('SPD003', 'Racing/Dangerous Speed', 'Section 189', 500000, 'DANGEROUS_DRIVING', 'SEVERE', 6),
    ('SIG001', 'Jumping Red Light', 'Section 184', 100000, 'SIGNAL_VIOLATION', 'MAJOR', 4),
    ('SIG002', 'Ignoring Traffic Signs', 'Section 184', 50000, 'SIGNAL_VIOLATION', 'MINOR', 2),
    ('DOC001', 'Driving Without License', 'Section 181', 500000, 'DOCUMENT_VIOLATION', 'MAJOR', 0),
    ('DOC002', 'Driving Without Registration', 'Section 39', 500000, 'DOCUMENT_VIOLATION', 'MAJOR', 0),
    ('DOC003', 'Driving Without Insurance', 'Section 196', 200000, 'DOCUMENT_VIOLATION', 'MODERATE', 0),
    ('DOC004', 'Driving Without PUC', 'Section 190(2)', 1000000, 'EMISSION_VIOLATION', 'MODERATE', 0),
    ('SAF001', 'Not Wearing Helmet', 'Section 194D', 100000, 'SAFETY_VIOLATION', 'MINOR', 0),
    ('SAF002', 'Not Wearing Seatbelt', 'Section 194B', 100000, 'SAFETY_VIOLATION', 'MINOR', 0),
    ('SAF003', 'Using Mobile While Driving', 'Section 184', 500000, 'DANGEROUS_DRIVING', 'MAJOR', 4),
    ('SAF004', 'Triple Riding on Two Wheeler', 'Section 128', 100000, 'SAFETY_VIOLATION', 'MINOR', 0),
    ('DRK001', 'Drunk Driving - First Offense', 'Section 185', 1000000, 'DRUNK_DRIVING', 'SEVERE', 6),
    ('DRK002', 'Drunk Driving - Repeat Offense', 'Section 185', 1500000, 'DRUNK_DRIVING', 'SEVERE', 12),
    ('DNG001', 'Rash/Negligent Driving', 'Section 184', 500000, 'DANGEROUS_DRIVING', 'MAJOR', 4),
    ('DNG002', 'Wrong Side Driving', 'Section 184', 500000, 'DANGEROUS_DRIVING', 'MAJOR', 4),
    ('PRK001', 'No Parking Zone', 'Section 177', 50000, 'PARKING_VIOLATION', 'MINOR', 0),
    ('PRK002', 'Blocking Traffic', 'Section 177', 100000, 'PARKING_VIOLATION', 'MODERATE', 0),
    ('OVL001', 'Overloading - Passengers', 'Section 194', 200000, 'OVERLOADING', 'MODERATE', 0),
    ('OVL002', 'Overloading - Goods', 'Section 194', 2000000, 'OVERLOADING', 'MAJOR', 0)
ON CONFLICT (code) DO NOTHING;

-- Create indexes
CREATE INDEX idx_challans_number ON traffic_challans(challan_number);
CREATE INDEX idx_challans_vehicle ON traffic_challans(vehicle_number);
CREATE INDEX idx_challans_status ON traffic_challans(status);
CREATE INDEX idx_challans_date ON traffic_challans(violation_date);
CREATE INDEX idx_challans_station ON traffic_challans(issuing_station_id);
CREATE INDEX idx_challans_officer ON traffic_challans(issuing_officer_id);
CREATE INDEX idx_challans_payment_due ON traffic_challans(payment_due_date) WHERE status IN ('ISSUED', 'PENDING');
CREATE INDEX idx_challans_location ON traffic_challans(violation_latitude, violation_longitude);

CREATE INDEX idx_payments_challan ON challan_payments(challan_id);
CREATE INDEX idx_payments_status ON challan_payments(status);
CREATE INDEX idx_payments_transaction ON challan_payments(transaction_id);

CREATE INDEX idx_defaulters_vehicle ON challan_defaulters(vehicle_number);
CREATE INDEX idx_defaulters_blacklist ON challan_defaulters(blacklisted) WHERE blacklisted = TRUE;

CREATE INDEX idx_hotspots_location ON challan_hotspots USING GIST (
    ST_SetSRID(ST_MakePoint(longitude, latitude), 4326)
);

-- Create updated_at triggers
CREATE TRIGGER update_traffic_challans_updated_at
    BEFORE UPDATE ON traffic_challans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_challan_defaulters_updated_at
    BEFORE UPDATE ON challan_defaulters
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to generate challan number
CREATE OR REPLACE FUNCTION generate_challan_number(station_code VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    seq_num INTEGER;
    challan_num VARCHAR;
BEGIN
    -- Get next sequence for today
    SELECT COALESCE(MAX(
        CAST(SUBSTRING(challan_number FROM '[0-9]+$') AS INTEGER)
    ), 0) + 1 INTO seq_num
    FROM traffic_challans
    WHERE DATE(created_at) = CURRENT_DATE
    AND challan_number LIKE station_code || '/' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '/%';

    challan_num := station_code || '/' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '/' || LPAD(seq_num::TEXT, 5, '0');

    RETURN challan_num;
END;
$$ LANGUAGE plpgsql;

-- Function to update defaulter record
CREATE OR REPLACE FUNCTION update_defaulter_record()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('ISSUED', 'PENDING', 'DEFAULTED') THEN
        INSERT INTO challan_defaulters (vehicle_number, owner_name, owner_phone, total_challans, total_pending_amount, oldest_pending_date)
        VALUES (NEW.vehicle_number, NEW.owner_name, NEW.owner_phone, 1, NEW.final_amount, NEW.violation_date::DATE)
        ON CONFLICT (vehicle_number) DO UPDATE SET
            total_challans = challan_defaulters.total_challans + 1,
            total_pending_amount = challan_defaulters.total_pending_amount + NEW.final_amount,
            oldest_pending_date = LEAST(challan_defaulters.oldest_pending_date, NEW.violation_date::DATE),
            updated_at = NOW();
    ELSIF NEW.status = 'PAID' AND OLD.status != 'PAID' THEN
        UPDATE challan_defaulters SET
            total_challans = GREATEST(0, total_challans - 1),
            total_pending_amount = GREATEST(0, total_pending_amount - NEW.final_amount),
            updated_at = NOW()
        WHERE vehicle_number = NEW.vehicle_number;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_defaulter_on_challan
    AFTER INSERT OR UPDATE OF status ON traffic_challans
    FOR EACH ROW EXECUTE FUNCTION update_defaulter_record();

COMMENT ON TABLE traffic_challans IS 'Traffic violation challans with payment tracking';
COMMENT ON TABLE challan_defaulters IS 'Vehicles with pending challans for enforcement';
