-- Citizen Portal Tables Migration

-- Complaint Status Enum
CREATE TYPE complaint_status AS ENUM (
    'SUBMITTED', 'ACKNOWLEDGED', 'ASSIGNED', 'IN_PROGRESS', 'RESOLVED', 'CLOSED', 'REJECTED'
);

-- Complaint Category Enum
CREATE TYPE complaint_category AS ENUM (
    'THEFT', 'FRAUD', 'ASSAULT', 'CYBER_CRIME', 'MISSING_PERSON',
    'DOMESTIC_VIOLENCE', 'TRAFFIC', 'NOISE_COMPLAINT', 'DRUG_RELATED', 'OTHER'
);

-- Citizen Complaints Table
CREATE TABLE citizen_complaints (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tracking_number VARCHAR(50) UNIQUE NOT NULL,
    category complaint_category NOT NULL,
    status complaint_status DEFAULT 'SUBMITTED',

    -- Complainant Info
    is_anonymous BOOLEAN DEFAULT false,
    complainant_name VARCHAR(255),
    complainant_phone VARCHAR(20),
    complainant_email VARCHAR(255),
    complainant_address TEXT,

    -- Incident Details
    subject VARCHAR(500) NOT NULL,
    description TEXT NOT NULL,
    incident_date TIMESTAMP WITH TIME ZONE,
    incident_location TEXT,
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),

    -- Attachments
    attachments TEXT[],

    -- Assignment
    station_id UUID REFERENCES stations(id),
    assigned_to UUID REFERENCES users(id),

    -- Linked Records
    fir_id UUID REFERENCES firs(id),

    -- Response
    response_notes TEXT,
    rejection_reason TEXT,

    -- Verification
    otp_verified BOOLEAN DEFAULT false,
    verification_token VARCHAR(255),

    -- Timestamps
    submitted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    assigned_at TIMESTAMP WITH TIME ZONE,
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Complaint Updates Table
CREATE TABLE complaint_updates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id UUID REFERENCES citizen_complaints(id) ON DELETE CASCADE,
    status complaint_status NOT NULL,
    message TEXT NOT NULL,
    updated_by UUID REFERENCES users(id),
    is_public BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Grievances Table
CREATE TABLE grievances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    grievance_number VARCHAR(50) UNIQUE NOT NULL,
    type VARCHAR(50) NOT NULL, -- SERVICE_DELAY, MISCONDUCT, CORRUPTION, OTHER

    -- Complainant
    complainant_name VARCHAR(255) NOT NULL,
    complainant_phone VARCHAR(20),
    complainant_email VARCHAR(255),

    -- Against
    against_officer UUID REFERENCES users(id),
    against_station UUID REFERENCES stations(id),

    -- Details
    subject VARCHAR(500) NOT NULL,
    description TEXT NOT NULL,
    related_fir VARCHAR(50),
    related_complaint VARCHAR(50),

    -- Status
    status VARCHAR(50) DEFAULT 'SUBMITTED', -- SUBMITTED, UNDER_REVIEW, ESCALATED, RESOLVED, CLOSED
    priority VARCHAR(20) DEFAULT 'MEDIUM',
    assigned_to UUID REFERENCES users(id),

    -- Resolution
    resolution TEXT,
    action_taken TEXT,

    -- Timestamps
    submitted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Missing Person Reports Table
CREATE TABLE missing_person_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_number VARCHAR(50) UNIQUE NOT NULL,
    status VARCHAR(50) DEFAULT 'REPORTED', -- REPORTED, SEARCHING, FOUND, CLOSED

    -- Reporter Info
    reporter_name VARCHAR(255) NOT NULL,
    reporter_phone VARCHAR(20) NOT NULL,
    reporter_relation VARCHAR(100) NOT NULL,

    -- Missing Person Info
    person_name VARCHAR(255) NOT NULL,
    age INTEGER NOT NULL,
    gender VARCHAR(20) NOT NULL,
    height VARCHAR(50),
    weight VARCHAR(50),
    complexion VARCHAR(50),
    identifying_marks TEXT,
    last_seen_location TEXT NOT NULL,
    last_seen_date TIMESTAMP WITH TIME ZONE NOT NULL,
    last_seen_wearing TEXT,
    photo_url TEXT,

    -- Assignment
    station_id UUID REFERENCES stations(id),
    assigned_to UUID REFERENCES users(id),
    fir_id UUID REFERENCES firs(id),

    -- Found Details
    found_date TIMESTAMP WITH TIME ZONE,
    found_location TEXT,
    found_condition TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Public FIR Requests Table
CREATE TABLE public_fir_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_number VARCHAR(50) UNIQUE NOT NULL,
    fir_number VARCHAR(50) NOT NULL,
    requester_name VARCHAR(255) NOT NULL,
    requester_phone VARCHAR(20) NOT NULL,
    requester_email VARCHAR(255),
    relationship VARCHAR(50) NOT NULL, -- COMPLAINANT, VICTIM, ACCUSED, ADVOCATE
    purpose TEXT NOT NULL,
    id_proof VARCHAR(100) NOT NULL,
    status VARCHAR(50) DEFAULT 'SUBMITTED', -- SUBMITTED, VERIFIED, APPROVED, REJECTED, DELIVERED
    rejection_reason TEXT,
    approved_by UUID REFERENCES users(id),
    delivery_method VARCHAR(50) NOT NULL, -- DOWNLOAD, EMAIL, PHYSICAL
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_complaints_tracking ON citizen_complaints(tracking_number);
CREATE INDEX idx_complaints_status ON citizen_complaints(status);
CREATE INDEX idx_complaints_category ON citizen_complaints(category);
CREATE INDEX idx_complaints_station ON citizen_complaints(station_id);
CREATE INDEX idx_complaints_submitted ON citizen_complaints(submitted_at);

CREATE INDEX idx_complaint_updates_complaint ON complaint_updates(complaint_id);
CREATE INDEX idx_complaint_updates_public ON complaint_updates(is_public) WHERE is_public = true;

CREATE INDEX idx_grievances_number ON grievances(grievance_number);
CREATE INDEX idx_grievances_status ON grievances(status);
CREATE INDEX idx_grievances_officer ON grievances(against_officer);

CREATE INDEX idx_missing_reports_number ON missing_person_reports(report_number);
CREATE INDEX idx_missing_reports_status ON missing_person_reports(status);
CREATE INDEX idx_missing_reports_station ON missing_person_reports(station_id);

CREATE INDEX idx_fir_requests_number ON public_fir_requests(request_number);
CREATE INDEX idx_fir_requests_fir ON public_fir_requests(fir_number);
CREATE INDEX idx_fir_requests_status ON public_fir_requests(status);

-- Triggers
CREATE TRIGGER update_complaints_updated_at BEFORE UPDATE ON citizen_complaints
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_grievances_updated_at BEFORE UPDATE ON grievances
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_missing_reports_updated_at BEFORE UPDATE ON missing_person_reports
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_fir_requests_updated_at BEFORE UPDATE ON public_fir_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
