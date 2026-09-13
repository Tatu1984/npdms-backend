-- NPDMS Database Schema
-- National Police Department Management System

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =====================================================
-- ENUMS
-- =====================================================

CREATE TYPE user_role AS ENUM (
    'CONSTABLE', 'HEAD_CONSTABLE', 'ASI', 'SI', 'INSPECTOR',
    'DSP', 'SP', 'DIG', 'IG', 'SECRETARY', 'DGP', 'SHO'
);

CREATE TYPE fir_status AS ENUM (
    'DRAFT', 'REGISTERED', 'UNDER_INVESTIGATION',
    'CHARGESHEET_FILED', 'CLOSED', 'TRANSFERRED'
);

CREATE TYPE fir_priority AS ENUM ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL');

CREATE TYPE case_status AS ENUM (
    'REGISTERED', 'UNDER_INVESTIGATION', 'CHARGESHEET_FILED',
    'IN_COURT', 'CONVICTION', 'ACQUITTAL', 'CLOSED'
);

CREATE TYPE evidence_type AS ENUM (
    'PHYSICAL', 'DIGITAL', 'DOCUMENTARY', 'BIOLOGICAL', 'TRACE', 'TESTIMONIAL'
);

CREATE TYPE evidence_status AS ENUM (
    'COLLECTED', 'IN_CUSTODY', 'SENT_TO_FSL', 'AT_COURT', 'DISPOSED'
);

CREATE TYPE accused_status AS ENUM (
    'ABSCONDING', 'ARRESTED', 'ON_BAIL', 'IN_CUSTODY', 'RELEASED'
);

CREATE TYPE warrant_type AS ENUM ('ARREST', 'SEARCH', 'SUMMONS', 'NBW');

CREATE TYPE warrant_status AS ENUM ('ACTIVE', 'EXECUTED', 'EXPIRED', 'CANCELLED');

CREATE TYPE bail_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED', 'RELEASED');

CREATE TYPE bail_type AS ENUM ('REGULAR', 'ANTICIPATORY', 'INTERIM');

-- =====================================================
-- TABLES
-- =====================================================

-- Stations Table
CREATE TABLE stations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    code VARCHAR(20) UNIQUE NOT NULL,
    address TEXT,
    district VARCHAR(100),
    state VARCHAR(100) DEFAULT 'Karnataka',
    phone VARCHAR(20),
    email VARCHAR(255),
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Users Table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role user_role NOT NULL DEFAULT 'CONSTABLE',
    badge_number VARCHAR(50) UNIQUE,
    station_id UUID REFERENCES stations(id),
    phone VARCHAR(20),
    is_active BOOLEAN DEFAULT true,
    last_login TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- FIR Table
CREATE TABLE firs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    fir_number VARCHAR(50) UNIQUE NOT NULL,
    station_id UUID REFERENCES stations(id) NOT NULL,
    complainant_name VARCHAR(255) NOT NULL,
    complainant_phone VARCHAR(20),
    complainant_address TEXT,
    complainant_id_type VARCHAR(50),
    complainant_id_number VARCHAR(100),
    incident_date DATE NOT NULL,
    incident_time TIME,
    incident_location TEXT NOT NULL,
    incident_description TEXT NOT NULL,
    ipc_sections TEXT[] DEFAULT '{}',
    status fir_status DEFAULT 'DRAFT',
    priority fir_priority DEFAULT 'MEDIUM',
    registered_by UUID REFERENCES users(id),
    investigating_officer UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Cases Table
CREATE TABLE cases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_number VARCHAR(50) UNIQUE NOT NULL,
    fir_id UUID REFERENCES firs(id) NOT NULL,
    title VARCHAR(500) NOT NULL,
    synopsis TEXT,
    category VARCHAR(100),
    status case_status DEFAULT 'REGISTERED',
    priority fir_priority DEFAULT 'MEDIUM',
    ipc_sections TEXT[] DEFAULT '{}',
    investigating_officer UUID REFERENCES users(id),
    court_name VARCHAR(255),
    court_case_number VARCHAR(100),
    next_hearing_date DATE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Accused Table
CREATE TABLE accused (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    fir_id UUID REFERENCES firs(id),
    name VARCHAR(255) NOT NULL,
    alias VARCHAR(255),
    description TEXT,
    age INTEGER,
    gender VARCHAR(20),
    address TEXT,
    id_type VARCHAR(50),
    id_number VARCHAR(100),
    status accused_status DEFAULT 'ABSCONDING',
    arrest_date TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Witnesses Table
CREATE TABLE witnesses (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    fir_id UUID REFERENCES firs(id),
    name VARCHAR(255) NOT NULL,
    phone VARCHAR(20),
    address TEXT,
    witness_type VARCHAR(50), -- EYEWITNESS, VICTIM, EXPERT, etc.
    statement_recorded BOOLEAN DEFAULT false,
    statement_date TIMESTAMP WITH TIME ZONE,
    statement_text TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Evidence Table
CREATE TABLE evidence (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    evidence_number VARCHAR(50) UNIQUE NOT NULL,
    case_id UUID REFERENCES cases(id),
    fir_id UUID REFERENCES firs(id),
    evidence_type evidence_type NOT NULL,
    description TEXT NOT NULL,
    collection_location TEXT,
    collection_date TIMESTAMP WITH TIME ZONE,
    collected_by UUID REFERENCES users(id),
    storage_location VARCHAR(255),
    container_type VARCHAR(100),
    seal_number VARCHAR(100),
    weight VARCHAR(50),
    dimensions VARCHAR(100),
    condition VARCHAR(100),
    status evidence_status DEFAULT 'COLLECTED',
    requires_forensic BOOLEAN DEFAULT false,
    forensic_type VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Evidence Chain of Custody
CREATE TABLE evidence_custody (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    evidence_id UUID REFERENCES evidence(id) ON DELETE CASCADE,
    from_user UUID REFERENCES users(id),
    to_user UUID REFERENCES users(id),
    from_location VARCHAR(255),
    to_location VARCHAR(255),
    purpose TEXT,
    transfer_date TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    verified BOOLEAN DEFAULT false,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Warrants Table
CREATE TABLE warrants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    warrant_number VARCHAR(50) UNIQUE NOT NULL,
    case_id UUID REFERENCES cases(id),
    fir_id UUID REFERENCES firs(id),
    warrant_type warrant_type NOT NULL,
    status warrant_status DEFAULT 'ACTIVE',
    issued_for VARCHAR(255) NOT NULL,
    issued_by VARCHAR(255), -- Court name
    issued_date DATE NOT NULL,
    valid_until DATE,
    executed_date TIMESTAMP WITH TIME ZONE,
    charges TEXT[],
    last_known_location TEXT,
    priority fir_priority DEFAULT 'MEDIUM',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Bail Applications Table
CREATE TABLE bail_applications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bail_number VARCHAR(50) UNIQUE NOT NULL,
    case_id UUID REFERENCES cases(id),
    accused_id UUID REFERENCES accused(id),
    bail_type bail_type NOT NULL,
    status bail_status DEFAULT 'PENDING',
    application_date DATE NOT NULL,
    hearing_date DATE,
    court_name VARCHAR(255),
    judge_name VARCHAR(255),
    bail_amount DECIMAL(12, 2),
    surety_amount DECIMAL(12, 2),
    conditions TEXT[],
    rejection_reason TEXT,
    approval_date TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Sureties for Bail
CREATE TABLE bail_sureties (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bail_id UUID REFERENCES bail_applications(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    relation VARCHAR(100),
    phone VARCHAR(20),
    address TEXT,
    id_type VARCHAR(50),
    id_number VARCHAR(100),
    verified BOOLEAN DEFAULT false,
    verified_by UUID REFERENCES users(id),
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Court Hearings Table
CREATE TABLE court_hearings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    hearing_date DATE NOT NULL,
    hearing_time TIME,
    court_name VARCHAR(255),
    court_room VARCHAR(100),
    judge_name VARCHAR(255),
    hearing_type VARCHAR(100), -- ARGUMENTS, EVIDENCE, BAIL_HEARING, JUDGMENT
    outcome TEXT,
    next_hearing_date DATE,
    io_required BOOLEAN DEFAULT true,
    documents_required TEXT[],
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Forensic Requests Table
CREATE TABLE forensic_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_number VARCHAR(50) UNIQUE NOT NULL,
    evidence_id UUID REFERENCES evidence(id),
    case_id UUID REFERENCES cases(id),
    forensic_type VARCHAR(100) NOT NULL, -- FINGERPRINT, DNA, BALLISTICS, DIGITAL, NARCOTICS
    lab_name VARCHAR(255),
    analyst_name VARCHAR(255),
    status VARCHAR(50) DEFAULT 'PENDING', -- PENDING, IN_PROGRESS, COMPLETED, INCONCLUSIVE
    priority fir_priority DEFAULT 'MEDIUM',
    submitted_date DATE NOT NULL,
    expected_date DATE,
    completed_date DATE,
    findings TEXT,
    summary TEXT,
    report_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Audit Logs Table
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    action VARCHAR(50) NOT NULL, -- CREATE, UPDATE, DELETE, VIEW, LOGIN, etc.
    resource_type VARCHAR(100) NOT NULL, -- FIR, CASE, EVIDENCE, etc.
    resource_id UUID,
    description TEXT,
    ip_address INET,
    user_agent TEXT,
    success BOOLEAN DEFAULT true,
    failure_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Refresh Tokens for JWT
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    revoked BOOLEAN DEFAULT false
);

-- =====================================================
-- INDEXES
-- =====================================================

CREATE INDEX idx_users_station ON users(station_id);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_firs_station ON firs(station_id);
CREATE INDEX idx_firs_status ON firs(status);
CREATE INDEX idx_firs_date ON firs(incident_date);
CREATE INDEX idx_firs_io ON firs(investigating_officer);
CREATE INDEX idx_cases_fir ON cases(fir_id);
CREATE INDEX idx_cases_status ON cases(status);
CREATE INDEX idx_cases_io ON cases(investigating_officer);
CREATE INDEX idx_evidence_case ON evidence(case_id);
CREATE INDEX idx_evidence_fir ON evidence(fir_id);
CREATE INDEX idx_evidence_status ON evidence(status);
CREATE INDEX idx_accused_case ON accused(case_id);
CREATE INDEX idx_accused_status ON accused(status);
CREATE INDEX idx_warrants_status ON warrants(status);
CREATE INDEX idx_bail_status ON bail_applications(status);
CREATE INDEX idx_audit_user ON audit_logs(user_id);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_created ON audit_logs(created_at);

-- =====================================================
-- TRIGGERS
-- =====================================================

-- Update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply to all tables with updated_at
CREATE TRIGGER update_stations_updated_at BEFORE UPDATE ON stations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_firs_updated_at BEFORE UPDATE ON firs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_cases_updated_at BEFORE UPDATE ON cases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_accused_updated_at BEFORE UPDATE ON accused
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_witnesses_updated_at BEFORE UPDATE ON witnesses
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_evidence_updated_at BEFORE UPDATE ON evidence
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_warrants_updated_at BEFORE UPDATE ON warrants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_bail_updated_at BEFORE UPDATE ON bail_applications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_hearings_updated_at BEFORE UPDATE ON court_hearings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_forensic_updated_at BEFORE UPDATE ON forensic_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- =====================================================
-- SEED DATA
-- =====================================================

-- Insert default station
INSERT INTO stations (id, name, code, address, district, phone, latitude, longitude) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'Koramangala Police Station', 'KOR',
     '80 Feet Road, Koramangala, Bangalore', 'Bangalore Urban', '080-25520100',
     12.9352, 77.6245);

-- Insert demo users (password: Demo@123)
-- Password hash for 'Demo@123' using bcrypt
INSERT INTO users (id, username, email, password_hash, name, role, badge_number, station_id, is_active) VALUES
    ('550e8400-e29b-41d4-a716-446655440009', 'admin', 'admin@npdms.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'System Administrator', 'DGP', 'ADMIN-001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440010', 'constable', 'constable@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Ramesh Kumar', 'CONSTABLE', 'KAR-PC-1001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440011', 'hc', 'hc@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Suresh Patil', 'HEAD_CONSTABLE', 'KAR-HC-2001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440012', 'asi', 'asi@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Venkatesh Reddy', 'ASI', 'KAR-ASI-3001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440013', 'si', 'si@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Priya Sharma', 'SI', 'KAR-SI-4001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440014', 'inspector', 'inspector@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Rajendra Singh', 'INSPECTOR', 'KAR-INS-5001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440015', 'sho', 'sho@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Vikram Desai', 'SHO', 'KAR-SHO-6001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440016', 'dsp', 'dsp@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Anjali Menon', 'DSP', 'KAR-DSP-7001', '550e8400-e29b-41d4-a716-446655440001', true),
    ('550e8400-e29b-41d4-a716-446655440017', 'sp', 'sp@karpolice.gov.in',
     '$2b$10$yG1J9RcteL35i6x41ZCQUexMQx5eAEXjryaczK/cAc/fipIeEL0Ci',
     'Arun Kumar IPS', 'SP', 'KAR-SP-8001', '550e8400-e29b-41d4-a716-446655440001', true);

-- Insert sample FIRs
INSERT INTO firs (id, fir_number, station_id, complainant_name, complainant_phone, complainant_address,
    incident_date, incident_time, incident_location, incident_description, ipc_sections,
    status, priority, registered_by, investigating_officer) VALUES
    ('660e8400-e29b-41d4-a716-446655440001', 'KOR/2024/00001', '550e8400-e29b-41d4-a716-446655440001',
     'Anil Kumar', '9876543210', '123, MG Road, Bangalore',
     '2024-01-15', '10:30:00', 'SBI Main Branch, Koramangala',
     'Armed robbery at bank. Three masked individuals entered and stole approximately Rs. 18.5 lakhs.',
     ARRAY['IPC 392', 'IPC 397', 'Arms Act 25'], 'UNDER_INVESTIGATION', 'HIGH',
     '550e8400-e29b-41d4-a716-446655440015', '550e8400-e29b-41d4-a716-446655440013'),
    ('660e8400-e29b-41d4-a716-446655440002', 'KOR/2024/00002', '550e8400-e29b-41d4-a716-446655440001',
     'Priya Nair', '9876543211', '456, Indiranagar, Bangalore',
     '2024-01-16', '22:15:00', 'Near Forum Mall, Koramangala',
     'Chain snatching incident. Gold chain worth Rs. 2 lakhs snatched by two individuals on motorcycle.',
     ARRAY['IPC 379', 'IPC 356'], 'REGISTERED', 'MEDIUM',
     '550e8400-e29b-41d4-a716-446655440013', '550e8400-e29b-41d4-a716-446655440012');

COMMIT;
