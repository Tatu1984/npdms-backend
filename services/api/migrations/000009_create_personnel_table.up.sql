-- Create personnel table
CREATE TABLE IF NOT EXISTS personnel (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_number VARCHAR(50) UNIQUE NOT NULL,
    rank VARCHAR(50) NOT NULL CHECK (rank IN ('CONSTABLE', 'HEAD_CONSTABLE', 'ASI', 'SI', 'INSPECTOR', 'SHO', 'DSP', 'SP', 'DIG', 'IG', 'SECRETARY', 'DGP')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('ON_DUTY', 'OFF_DUTY', 'ON_LEAVE', 'TRAINING', 'SUSPENDED')),
    station_id UUID NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    joining_date DATE NOT NULL,
    assigned_cases INTEGER DEFAULT 0,
    current_duty TEXT,
    shift VARCHAR(50),
    leave_type VARCHAR(50),
    leave_until DATE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for personnel
CREATE INDEX idx_personnel_status ON personnel(status);
CREATE INDEX idx_personnel_rank ON personnel(rank);
CREATE INDEX idx_personnel_station_id ON personnel(station_id);
CREATE INDEX idx_personnel_user_id ON personnel(user_id);
CREATE INDEX idx_personnel_badge_number ON personnel(badge_number);

-- Create updated_at trigger for personnel
CREATE TRIGGER update_personnel_updated_at
    BEFORE UPDATE ON personnel
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
