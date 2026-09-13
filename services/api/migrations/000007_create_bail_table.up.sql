-- Create bail table
CREATE TABLE IF NOT EXISTS bail (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_number VARCHAR(50) UNIQUE NOT NULL,
    accused_id UUID REFERENCES accused(id) ON DELETE CASCADE,
    case_id UUID REFERENCES cases(id) ON DELETE CASCADE,
    fir_id UUID REFERENCES firs(id) ON DELETE CASCADE,
    ipc_sections TEXT[] DEFAULT '{}',
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED', 'RELEASED')),
    bail_type VARCHAR(20) NOT NULL CHECK (bail_type IN ('REGULAR', 'ANTICIPATORY', 'INTERIM')),
    application_date TIMESTAMP NOT NULL,
    hearing_date TIMESTAMP,
    approval_date TIMESTAMP,
    rejection_date TIMESTAMP,
    cancellation_date TIMESTAMP,
    release_date TIMESTAMP,
    court VARCHAR(255) NOT NULL,
    judge_name VARCHAR(255),
    bail_amount BIGINT,
    surety_amount BIGINT,
    proposed_bail_amount BIGINT,
    conditions TEXT[] DEFAULT '{}',
    rejection_reason TEXT,
    cancellation_reason TEXT,
    lawyer VARCHAR(255),
    valid_until TIMESTAMP,
    order_summary TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for bail
CREATE INDEX idx_bail_status ON bail(status);
CREATE INDEX idx_bail_type ON bail(bail_type);
CREATE INDEX idx_bail_case_id ON bail(case_id);
CREATE INDEX idx_bail_fir_id ON bail(fir_id);
CREATE INDEX idx_bail_accused_id ON bail(accused_id);
CREATE INDEX idx_bail_application_date ON bail(application_date);
CREATE INDEX idx_bail_hearing_date ON bail(hearing_date);

-- Create updated_at trigger for bail
CREATE TRIGGER update_bail_updated_at
    BEFORE UPDATE ON bail
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
