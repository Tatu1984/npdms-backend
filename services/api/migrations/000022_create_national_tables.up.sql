-- National-Level Tables for Command Center

-- National Alerts
CREATE TABLE IF NOT EXISTS national_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_type VARCHAR(30) NOT NULL CHECK (alert_type IN ('BOLO', 'TERRORIST_THREAT', 'NATURAL_DISASTER', 'INTER_STATE_CRIME', 'CYBER_ATTACK', 'PUBLIC_ORDER', 'VIP_SECURITY', 'BORDER_ALERT', 'GENERAL')),
    severity VARCHAR(15) DEFAULT 'MEDIUM' CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    title VARCHAR(500) NOT NULL,
    description TEXT,
    target_states UUID[] DEFAULT '{}',
    acknowledged_by UUID[] DEFAULT '{}',
    expires_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'EXPIRED', 'CANCELLED', 'RESOLVED')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Inter-State Coordination Requests
CREATE TABLE IF NOT EXISTS inter_state_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_type VARCHAR(30) NOT NULL CHECK (request_type IN ('CRIMINAL_TRANSFER', 'INVESTIGATION_ASSISTANCE', 'INFORMATION_SHARING', 'JOINT_OPERATION', 'PRISONER_ESCORT', 'WITNESS_PROTECTION', 'ASSET_TRACING')),
    requesting_state UUID NOT NULL REFERENCES states(id),
    target_state UUID NOT NULL REFERENCES states(id),
    related_fir UUID REFERENCES firs(id),
    related_case UUID REFERENCES cases(id),
    priority VARCHAR(15) DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'URGENT', 'CRITICAL')),
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    subject VARCHAR(500) NOT NULL,
    description TEXT,
    requested_by UUID NOT NULL REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    approved_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- National Crime Patterns
CREATE TABLE IF NOT EXISTS national_crime_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pattern_type VARCHAR(50) NOT NULL CHECK (pattern_type IN ('ORGANIZED_CRIME', 'TERRORISM', 'CYBER_CRIME', 'DRUG_TRAFFICKING', 'HUMAN_TRAFFICKING', 'ECONOMIC_OFFENCE', 'SERIAL_CRIME', 'SEASONAL_TREND', 'REGIONAL_CLUSTER')),
    crime_category VARCHAR(50) NOT NULL,
    affected_states UUID[] DEFAULT '{}',
    description TEXT,
    start_date DATE NOT NULL,
    end_date DATE,
    trend VARCHAR(20) CHECK (trend IN ('INCREASING', 'DECREASING', 'STABLE', 'FLUCTUATING', 'EMERGING')),
    severity VARCHAR(15) DEFAULT 'MEDIUM' CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    recommendations TEXT,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'MONITORING', 'RESOLVED', 'ARCHIVED')),
    detected_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- National Performance Snapshots
CREATE TABLE IF NOT EXISTS national_performance_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_date DATE NOT NULL,
    snapshot_type VARCHAR(10) NOT NULL CHECK (snapshot_type IN ('DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY')),
    total_firs INTEGER DEFAULT 0,
    pending_firs INTEGER DEFAULT 0,
    resolved_firs INTEGER DEFAULT 0,
    total_cases INTEGER DEFAULT 0,
    conviction_count INTEGER DEFAULT 0,
    acquittal_count INTEGER DEFAULT 0,
    chargesheet_count INTEGER DEFAULT 0,
    total_states INTEGER DEFAULT 0,
    total_districts INTEGER DEFAULT 0,
    total_stations INTEGER DEFAULT 0,
    total_officers INTEGER DEFAULT 0,
    resolution_rate DECIMAL(5,2),
    conviction_rate DECIMAL(5,2),
    avg_resolution_days DECIMAL(10,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(snapshot_date, snapshot_type)
);

-- National Coordination Meetings
CREATE TABLE IF NOT EXISTS national_coordination_meetings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(500) NOT NULL,
    agenda TEXT,
    meeting_date TIMESTAMP WITH TIME ZONE NOT NULL,
    venue VARCHAR(300),
    meeting_type VARCHAR(30) DEFAULT 'REGULAR' CHECK (meeting_type IN ('DGPS_CONFERENCE', 'EMERGENCY', 'REVIEW', 'PLANNING', 'COORDINATION', 'INTER_AGENCY')),
    chairperson_id UUID NOT NULL REFERENCES users(id),
    invited_states UUID[] DEFAULT '{}',
    participants UUID[] DEFAULT '{}',
    minutes TEXT,
    decisions TEXT,
    action_items JSONB DEFAULT '[]',
    status VARCHAR(20) DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'POSTPONED')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Most Wanted List (National)
CREATE TABLE IF NOT EXISTS national_most_wanted (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_name VARCHAR(200) NOT NULL,
    alias VARCHAR(500),
    photo_url VARCHAR(500),
    description TEXT,
    crime_type VARCHAR(100) NOT NULL,
    ipc_sections VARCHAR(500),
    reward_amount DECIMAL(15,2),
    last_known_location VARCHAR(500),
    last_known_state UUID REFERENCES states(id),
    danger_level VARCHAR(15) DEFAULT 'HIGH' CHECK (danger_level IN ('MODERATE', 'HIGH', 'EXTREME')),
    related_cases UUID[] DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CAPTURED', 'DECEASED', 'SURRENDERED', 'REMOVED')),
    added_by UUID NOT NULL REFERENCES users(id),
    captured_at TIMESTAMP WITH TIME ZONE,
    captured_by_state UUID REFERENCES states(id),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_national_alerts_type ON national_alerts(alert_type);
CREATE INDEX idx_national_alerts_severity ON national_alerts(severity);
CREATE INDEX idx_national_alerts_status ON national_alerts(status);
CREATE INDEX idx_national_alerts_expires ON national_alerts(expires_at);
CREATE INDEX idx_national_alerts_created ON national_alerts(created_at);

CREATE INDEX idx_inter_state_requests_requesting ON inter_state_requests(requesting_state);
CREATE INDEX idx_inter_state_requests_target ON inter_state_requests(target_state);
CREATE INDEX idx_inter_state_requests_status ON inter_state_requests(status);
CREATE INDEX idx_inter_state_requests_priority ON inter_state_requests(priority);
CREATE INDEX idx_inter_state_requests_created ON inter_state_requests(created_at);

CREATE INDEX idx_national_crime_patterns_type ON national_crime_patterns(pattern_type);
CREATE INDEX idx_national_crime_patterns_category ON national_crime_patterns(crime_category);
CREATE INDEX idx_national_crime_patterns_status ON national_crime_patterns(status);
CREATE INDEX idx_national_crime_patterns_severity ON national_crime_patterns(severity);

CREATE INDEX idx_national_performance_date ON national_performance_snapshots(snapshot_date);
CREATE INDEX idx_national_performance_type ON national_performance_snapshots(snapshot_type);

CREATE INDEX idx_national_meetings_date ON national_coordination_meetings(meeting_date);
CREATE INDEX idx_national_meetings_status ON national_coordination_meetings(status);
CREATE INDEX idx_national_meetings_chairperson ON national_coordination_meetings(chairperson_id);

CREATE INDEX idx_national_most_wanted_status ON national_most_wanted(status);
CREATE INDEX idx_national_most_wanted_danger ON national_most_wanted(danger_level);
CREATE INDEX idx_national_most_wanted_state ON national_most_wanted(last_known_state);

-- Function to auto-expire national alerts
CREATE OR REPLACE FUNCTION expire_national_alerts()
RETURNS void AS $$
BEGIN
    UPDATE national_alerts
    SET status = 'EXPIRED', updated_at = NOW()
    WHERE status = 'ACTIVE'
      AND expires_at IS NOT NULL
      AND expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- Function to create daily national snapshot
CREATE OR REPLACE FUNCTION create_national_snapshot()
RETURNS void AS $$
BEGIN
    INSERT INTO national_performance_snapshots (
        snapshot_date, snapshot_type, total_firs, pending_firs, resolved_firs,
        total_cases, conviction_count, acquittal_count, chargesheet_count,
        total_states, total_districts, total_stations, total_officers,
        resolution_rate, conviction_rate
    )
    SELECT
        CURRENT_DATE,
        'DAILY',
        (SELECT COUNT(*) FROM firs WHERE DATE(created_at) = CURRENT_DATE),
        (SELECT COUNT(*) FROM firs WHERE status IN ('REGISTERED', 'UNDER_INVESTIGATION')),
        (SELECT COUNT(*) FROM firs WHERE status IN ('CLOSED', 'CHARGESHEETED')),
        (SELECT COUNT(*) FROM cases WHERE DATE(created_at) = CURRENT_DATE),
        (SELECT COUNT(*) FROM cases WHERE status = 'CONVICTED'),
        (SELECT COUNT(*) FROM cases WHERE status = 'ACQUITTED'),
        (SELECT COUNT(*) FROM cases WHERE status = 'CHARGESHEETED'),
        (SELECT COUNT(*) FROM states WHERE is_active = true),
        (SELECT COUNT(*) FROM districts WHERE is_active = true),
        (SELECT COUNT(*) FROM police_stations WHERE is_active = true),
        (SELECT COALESCE(SUM(total_officers), 0) FROM states WHERE is_active = true),
        CASE WHEN (SELECT COUNT(*) FROM firs) > 0 THEN
            (SELECT COUNT(*) FROM firs WHERE status IN ('CLOSED', 'CHARGESHEETED'))::float /
            (SELECT COUNT(*) FROM firs) * 100
        ELSE 0 END,
        CASE WHEN (SELECT COUNT(*) FROM cases) > 0 THEN
            (SELECT COUNT(*) FROM cases WHERE status = 'CONVICTED')::float /
            (SELECT COUNT(*) FROM cases) * 100
        ELSE 0 END
    ON CONFLICT (snapshot_date, snapshot_type) DO UPDATE SET
        total_firs = EXCLUDED.total_firs,
        pending_firs = EXCLUDED.pending_firs,
        resolved_firs = EXCLUDED.resolved_firs,
        total_cases = EXCLUDED.total_cases;
END;
$$ LANGUAGE plpgsql;
