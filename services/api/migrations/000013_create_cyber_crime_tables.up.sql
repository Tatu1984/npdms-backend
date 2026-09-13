-- Cyber Crime Tables Migration

-- Cyber Crime Type Enum
CREATE TYPE cyber_crime_type AS ENUM (
    'PHISHING', 'RANSOMWARE', 'IDENTITY_THEFT', 'ONLINE_FRAUD',
    'CYBER_STALKING', 'DATA_BREACH', 'SOCIAL_ENGINEERING',
    'CRYPTO_FRAUD', 'CHILD_EXPLOITATION', 'HACKING', 'MALWARE', 'OTHER'
);

-- Cyber Crime Status Enum
CREATE TYPE cyber_crime_status AS ENUM (
    'REPORTED', 'ANALYZING', 'INVESTIGATING', 'ESCALATED', 'RESOLVED', 'CLOSED'
);

-- Platform Type Enum
CREATE TYPE platform_type AS ENUM (
    'WEBSITE', 'MOBILE_APP', 'SOCIAL_MEDIA', 'EMAIL',
    'MESSAGING', 'BANKING', 'ECOMMERCE', 'CRYPTO', 'OTHER'
);

-- Main Cyber Crime Table
CREATE TABLE cyber_crimes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_number VARCHAR(50) UNIQUE NOT NULL,
    fir_id UUID REFERENCES firs(id),
    type cyber_crime_type NOT NULL,
    status cyber_crime_status DEFAULT 'REPORTED',
    priority fir_priority DEFAULT 'MEDIUM',

    -- Complainant Info
    complainant_name VARCHAR(255) NOT NULL,
    complainant_email VARCHAR(255),
    complainant_phone VARCHAR(20),

    -- Incident Details
    incident_date TIMESTAMP WITH TIME ZONE NOT NULL,
    incident_description TEXT NOT NULL,
    platform platform_type NOT NULL,
    platform_name VARCHAR(255),
    url TEXT,

    -- Suspect Info
    suspect_name VARCHAR(255),
    suspect_email VARCHAR(255),
    suspect_phone VARCHAR(20),
    suspect_social_media TEXT,

    -- Financial Details
    financial_loss DECIMAL(15, 2),
    currency VARCHAR(10) DEFAULT 'INR',
    bank_account_involved TEXT,
    crypto_wallet_address VARCHAR(255),
    transaction_ids TEXT[],

    -- Technical Evidence
    ip_addresses TEXT[],
    device_info TEXT,
    malware_sample TEXT,
    phishing_email TEXT,
    screenshots TEXT[],

    -- Investigation
    investigating_officer UUID REFERENCES users(id),
    analyst_notes TEXT,
    escalated_to VARCHAR(255),

    -- Applicable Laws
    it_act_sections TEXT[],
    ipc_sections TEXT[],

    -- Timestamps
    reported_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    analysis_started_at TIMESTAMP WITH TIME ZONE,
    investigation_started_at TIMESTAMP WITH TIME ZONE,
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Digital Evidence Table
CREATE TABLE digital_evidence (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cyber_crime_id UUID REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    evidence_type VARCHAR(50) NOT NULL, -- LOG, SCREENSHOT, EMAIL, MALWARE, NETWORK_CAPTURE
    description TEXT NOT NULL,
    file_path TEXT,
    file_hash VARCHAR(128),
    hash_algorithm VARCHAR(20),
    collected_by UUID REFERENCES users(id),
    collection_method VARCHAR(255),
    chain_of_custody TEXT DEFAULT '',
    is_verified BOOLEAN DEFAULT false,
    verified_by UUID REFERENCES users(id),
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- OSINT Reports Table
CREATE TABLE osint_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cyber_crime_id UUID REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    source VARCHAR(255) NOT NULL,
    report_type VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    reliability VARCHAR(20) DEFAULT 'UNVERIFIED', -- HIGH, MEDIUM, LOW, UNVERIFIED
    findings TEXT,
    linked_entities TEXT[],
    generated_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Financial Trail Table
CREATE TABLE financial_trails (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cyber_crime_id UUID REFERENCES cyber_crimes(id) ON DELETE CASCADE,
    transaction_type VARCHAR(50) NOT NULL, -- BANK, UPI, CRYPTO, CARD
    from_account VARCHAR(255) NOT NULL,
    to_account VARCHAR(255) NOT NULL,
    amount DECIMAL(15, 2) NOT NULL,
    currency VARCHAR(10) DEFAULT 'INR',
    transaction_date TIMESTAMP WITH TIME ZONE NOT NULL,
    transaction_ref VARCHAR(255),
    bank_name VARCHAR(255),
    ifsc VARCHAR(20),
    wallet_provider VARCHAR(100),
    blockchain_tx_id VARCHAR(255),
    is_suspicious BOOLEAN DEFAULT false,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_cyber_crimes_type ON cyber_crimes(type);
CREATE INDEX idx_cyber_crimes_status ON cyber_crimes(status);
CREATE INDEX idx_cyber_crimes_fir ON cyber_crimes(fir_id);
CREATE INDEX idx_cyber_crimes_io ON cyber_crimes(investigating_officer);
CREATE INDEX idx_cyber_crimes_reported ON cyber_crimes(reported_at);
CREATE INDEX idx_digital_evidence_crime ON digital_evidence(cyber_crime_id);
CREATE INDEX idx_osint_crime ON osint_reports(cyber_crime_id);
CREATE INDEX idx_financial_trails_crime ON financial_trails(cyber_crime_id);

-- Trigger for updated_at
CREATE TRIGGER update_cyber_crimes_updated_at BEFORE UPDATE ON cyber_crimes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
