-- Create forensics table
CREATE TABLE IF NOT EXISTS forensics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id UUID NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('FINGERPRINT', 'DNA', 'BALLISTICS', 'DIGITAL', 'NARCOTICS', 'DOCUMENT')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'INCONCLUSIVE')),
    priority VARCHAR(20) NOT NULL CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    submitted_date TIMESTAMP NOT NULL,
    completed_date TIMESTAMP,
    expected_date TIMESTAMP,
    lab VARCHAR(255) NOT NULL,
    analyst VARCHAR(255),
    summary TEXT,
    findings TEXT,
    progress INTEGER CHECK (progress >= 0 AND progress <= 100),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for forensics
CREATE INDEX idx_forensics_status ON forensics(status);
CREATE INDEX idx_forensics_type ON forensics(type);
CREATE INDEX idx_forensics_priority ON forensics(priority);
CREATE INDEX idx_forensics_case_id ON forensics(case_id);
CREATE INDEX idx_forensics_evidence_id ON forensics(evidence_id);
CREATE INDEX idx_forensics_submitted_date ON forensics(submitted_date);
CREATE INDEX idx_forensics_expected_date ON forensics(expected_date);

-- Create updated_at trigger for forensics
CREATE TRIGGER update_forensics_updated_at
    BEFORE UPDATE ON forensics
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
