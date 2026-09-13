-- Create biometric devices table
CREATE TABLE IF NOT EXISTS biometric_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('FINGERPRINT', 'FACE', 'IRIS', 'CARD')),
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'MAINTENANCE', 'OFFLINE')),
    station_id UUID NOT NULL,
    location VARCHAR(200),
    ip_address VARCHAR(50),
    port INTEGER,
    serial_number VARCHAR(100),
    manufacturer VARCHAR(100),
    model VARCHAR(100),
    firmware_version VARCHAR(50),
    last_sync TIMESTAMP WITH TIME ZONE,
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create biometric templates table
CREATE TABLE IF NOT EXISTS biometric_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL,
    person_type VARCHAR(50) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('FINGERPRINT', 'FACE', 'IRIS', 'AADHAAR')),
    template_data BYTEA,
    template_hash VARCHAR(128) NOT NULL,
    quality DOUBLE PRECISION,
    captured_by UUID NOT NULL,
    captured_at TIMESTAMP WITH TIME ZONE NOT NULL,
    device_id UUID REFERENCES biometric_devices(id),
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create biometric verifications table
CREATE TABLE IF NOT EXISTS biometric_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(20) NOT NULL CHECK (type IN ('FINGERPRINT', 'FACE', 'IRIS', 'AADHAAR')),
    purpose VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'VERIFIED', 'FAILED', 'EXPIRED')),
    initiated_by UUID NOT NULL,
    subject_id UUID,
    subject_type VARCHAR(50),
    resource_id UUID,
    resource_type VARCHAR(50),
    matched_template_id UUID REFERENCES biometric_templates(id),
    match_score DOUBLE PRECISION,
    confidence DOUBLE PRECISION,
    captured_data BYTEA,
    device_id UUID REFERENCES biometric_devices(id),
    ip_address VARCHAR(50),
    location VARCHAR(200),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    error_message VARCHAR(500),
    aadhaar_last_digits VARCHAR(4),
    aadhaar_txn_id VARCHAR(100),
    initiated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create suspect identifications table
CREATE TABLE IF NOT EXISTS suspect_identifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID,
    fir_id UUID,
    identified_by UUID NOT NULL,
    identification_type VARCHAR(20) NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    source_reference VARCHAR(200),
    captured_image TEXT,
    matched_person_id UUID,
    matched_person_type VARCHAR(50),
    match_score DOUBLE PRECISION NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    is_confirmed BOOLEAN NOT NULL DEFAULT false,
    confirmed_by UUID,
    confirmed_at TIMESTAMP WITH TIME ZONE,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create evidence custody biometrics table
CREATE TABLE IF NOT EXISTS evidence_custody_biometrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id UUID NOT NULL,
    verification_id UUID NOT NULL REFERENCES biometric_verifications(id),
    action VARCHAR(50) NOT NULL,
    from_custodian UUID,
    to_custodian UUID,
    reason VARCHAR(500),
    verified BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create weapon access biometrics table
CREATE TABLE IF NOT EXISTS weapon_access_biometrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    weapon_id UUID NOT NULL,
    personnel_id UUID NOT NULL,
    verification_id UUID NOT NULL REFERENCES biometric_verifications(id),
    action VARCHAR(50) NOT NULL,
    purpose VARCHAR(500),
    approved_by UUID,
    verified BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMP WITH TIME ZONE,
    returned_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create attendance records table
CREATE TABLE IF NOT EXISTS attendance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    personnel_id UUID NOT NULL,
    station_id UUID NOT NULL,
    device_id UUID REFERENCES biometric_devices(id),
    date DATE NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('CHECK_IN', 'CHECK_OUT', 'BREAK_IN', 'BREAK_OUT')),
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    biometric_type VARCHAR(20),
    confidence DOUBLE PRECISION,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    address VARCHAR(500),
    is_manual BOOLEAN NOT NULL DEFAULT false,
    manual_reason VARCHAR(500),
    approved_by UUID,
    photo_url VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create daily attendance table
CREATE TABLE IF NOT EXISTS daily_attendance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    personnel_id UUID NOT NULL,
    station_id UUID NOT NULL,
    date DATE NOT NULL,
    status VARCHAR(20) NOT NULL,
    check_in_time TIMESTAMP WITH TIME ZONE,
    check_out_time TIMESTAMP WITH TIME ZONE,
    total_hours DOUBLE PRECISION DEFAULT 0,
    break_minutes INTEGER DEFAULT 0,
    overtime_hours DOUBLE PRECISION DEFAULT 0,
    late_by_minutes INTEGER DEFAULT 0,
    early_by_minutes INTEGER DEFAULT 0,
    shift_id UUID,
    leave_id UUID,
    remarks VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(personnel_id, date)
);

-- Create shifts table
CREATE TABLE IF NOT EXISTS shifts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    start_time VARCHAR(5) NOT NULL,
    end_time VARCHAR(5) NOT NULL,
    grace_period INTEGER DEFAULT 15,
    break_duration INTEGER DEFAULT 60,
    is_night_shift BOOLEAN DEFAULT false,
    station_id UUID NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create leave requests table
CREATE TABLE IF NOT EXISTS leave_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    personnel_id UUID NOT NULL,
    station_id UUID NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('CASUAL', 'SICK', 'EARNED', 'MATERNITY', 'PATERNITY', 'SPECIAL')),
    from_date DATE NOT NULL,
    to_date DATE NOT NULL,
    days INTEGER NOT NULL,
    reason TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED')),
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL,
    approved_by UUID,
    approved_at TIMESTAMP WITH TIME ZONE,
    rejected_reason VARCHAR(500),
    document_url VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create leave balance table
CREATE TABLE IF NOT EXISTS leave_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    personnel_id UUID NOT NULL,
    year INTEGER NOT NULL,
    casual_total INTEGER DEFAULT 12,
    casual_used INTEGER DEFAULT 0,
    sick_total INTEGER DEFAULT 12,
    sick_used INTEGER DEFAULT 0,
    earned_total INTEGER DEFAULT 30,
    earned_used INTEGER DEFAULT 0,
    special_total INTEGER DEFAULT 5,
    special_used INTEGER DEFAULT 0,
    carried_forward INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(personnel_id, year)
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_biometric_devices_station ON biometric_devices(station_id);
CREATE INDEX IF NOT EXISTS idx_biometric_devices_status ON biometric_devices(status);
CREATE INDEX IF NOT EXISTS idx_biometric_templates_person ON biometric_templates(person_id);
CREATE INDEX IF NOT EXISTS idx_biometric_templates_type ON biometric_templates(type);
CREATE INDEX IF NOT EXISTS idx_biometric_templates_active ON biometric_templates(is_active);
CREATE INDEX IF NOT EXISTS idx_biometric_verifications_initiated ON biometric_verifications(initiated_by);
CREATE INDEX IF NOT EXISTS idx_biometric_verifications_subject ON biometric_verifications(subject_id);
CREATE INDEX IF NOT EXISTS idx_biometric_verifications_status ON biometric_verifications(status);
CREATE INDEX IF NOT EXISTS idx_biometric_verifications_initiated_at ON biometric_verifications(initiated_at);
CREATE INDEX IF NOT EXISTS idx_suspect_identifications_case ON suspect_identifications(case_id);
CREATE INDEX IF NOT EXISTS idx_suspect_identifications_fir ON suspect_identifications(fir_id);
CREATE INDEX IF NOT EXISTS idx_suspect_identifications_confirmed ON suspect_identifications(is_confirmed);
CREATE INDEX IF NOT EXISTS idx_evidence_custody_evidence ON evidence_custody_biometrics(evidence_id);
CREATE INDEX IF NOT EXISTS idx_weapon_access_weapon ON weapon_access_biometrics(weapon_id);
CREATE INDEX IF NOT EXISTS idx_weapon_access_personnel ON weapon_access_biometrics(personnel_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_personnel ON attendance_records(personnel_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_station ON attendance_records(station_id);
CREATE INDEX IF NOT EXISTS idx_attendance_records_date ON attendance_records(date);
CREATE INDEX IF NOT EXISTS idx_daily_attendance_personnel ON daily_attendance(personnel_id);
CREATE INDEX IF NOT EXISTS idx_daily_attendance_station ON daily_attendance(station_id);
CREATE INDEX IF NOT EXISTS idx_daily_attendance_date ON daily_attendance(date);
CREATE INDEX IF NOT EXISTS idx_leave_requests_personnel ON leave_requests(personnel_id);
CREATE INDEX IF NOT EXISTS idx_leave_requests_status ON leave_requests(status);
