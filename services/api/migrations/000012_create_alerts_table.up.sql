-- Create alerts table
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(20) NOT NULL CHECK (type IN ('FLASH', 'URGENT', 'BOLO', 'NOTICE')),
    scope VARCHAR(20) NOT NULL CHECK (scope IN ('STATION', 'DISTRICT', 'STATE', 'NATIONAL')),
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    issued_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    issued_by UUID REFERENCES users(id) ON DELETE SET NULL,
    acknowledged BOOLEAN NOT NULL DEFAULT FALSE,
    acknowledged_by UUID REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMP,
    priority INTEGER NOT NULL CHECK (priority >= 1 AND priority <= 3),
    has_image BOOLEAN NOT NULL DEFAULT FALSE,
    station_id UUID REFERENCES stations(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for alerts
CREATE INDEX idx_alerts_type ON alerts(type);
CREATE INDEX idx_alerts_scope ON alerts(scope);
CREATE INDEX idx_alerts_acknowledged ON alerts(acknowledged);
CREATE INDEX idx_alerts_issued_at ON alerts(issued_at);
CREATE INDEX idx_alerts_expires_at ON alerts(expires_at);
CREATE INDEX idx_alerts_station_id ON alerts(station_id);
CREATE INDEX idx_alerts_priority ON alerts(priority);
CREATE INDEX idx_alerts_active ON alerts(expires_at) WHERE expires_at > NOW();
CREATE INDEX idx_alerts_unacknowledged ON alerts(acknowledged, expires_at) WHERE acknowledged = FALSE AND expires_at > NOW();

-- Create updated_at trigger for alerts
CREATE TRIGGER update_alerts_updated_at
    BEFORE UPDATE ON alerts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
