-- Create warrants table
CREATE TABLE IF NOT EXISTS warrants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    warrant_number VARCHAR(50) UNIQUE NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('ARREST', 'SEARCH', 'SUMMONS', 'NBW')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('ACTIVE', 'EXECUTED', 'EXPIRED', 'CANCELLED')),
    issued_for VARCHAR(255) NOT NULL,
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    fir_id UUID REFERENCES firs(id) ON DELETE CASCADE,
    issued_by VARCHAR(255) NOT NULL,
    judge_name VARCHAR(255),
    issued_date TIMESTAMP NOT NULL,
    valid_until TIMESTAMP NOT NULL,
    ipc_sections TEXT[] DEFAULT '{}',
    last_known_location TEXT,
    priority VARCHAR(20) NOT NULL CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    executed_date TIMESTAMP,
    executed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    description TEXT,
    age INTEGER,
    gender VARCHAR(20),
    address TEXT,
    identifying_marks TEXT,
    search_premises TEXT,
    search_scope TEXT,
    summons_purpose TEXT,
    hearing_date TIMESTAMP,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for warrants
CREATE INDEX idx_warrants_status ON warrants(status);
CREATE INDEX idx_warrants_type ON warrants(type);
CREATE INDEX idx_warrants_priority ON warrants(priority);
CREATE INDEX idx_warrants_case_id ON warrants(case_id);
CREATE INDEX idx_warrants_fir_id ON warrants(fir_id);
CREATE INDEX idx_warrants_issued_date ON warrants(issued_date);
CREATE INDEX idx_warrants_valid_until ON warrants(valid_until);

-- Create updated_at trigger for warrants
CREATE TRIGGER update_warrants_updated_at
    BEFORE UPDATE ON warrants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
