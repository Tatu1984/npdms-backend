-- State-Level Tables for Inter-District Coordination

-- Inter-District Coordination Requests
CREATE TABLE IF NOT EXISTS inter_district_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    request_type VARCHAR(30) NOT NULL CHECK (request_type IN ('TRANSFER', 'ASSISTANCE', 'INFORMATION', 'JOINT_OPERATION', 'PRISONER_ESCORT', 'WITNESS_ESCORT')),
    requesting_district UUID NOT NULL REFERENCES districts(id),
    target_district UUID NOT NULL REFERENCES districts(id),
    related_fir UUID REFERENCES firs(id),
    related_case UUID REFERENCES cases(id),
    priority VARCHAR(10) DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'URGENT', 'CRITICAL')),
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

-- State-Wide Alerts
CREATE TABLE IF NOT EXISTS state_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    alert_type VARCHAR(30) NOT NULL CHECK (alert_type IN ('BOLO', 'MISSING_PERSON', 'STOLEN_VEHICLE', 'ESCAPED_PRISONER', 'TERRORIST_THREAT', 'NATURAL_DISASTER', 'LAW_AND_ORDER', 'VIP_MOVEMENT', 'GENERAL')),
    severity VARCHAR(15) DEFAULT 'MEDIUM' CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    title VARCHAR(500) NOT NULL,
    description TEXT,
    target_districts UUID[] DEFAULT '{}',
    acknowledged_by UUID[] DEFAULT '{}',
    expires_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'EXPIRED', 'CANCELLED', 'RESOLVED')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- State-Level Meetings
CREATE TABLE IF NOT EXISTS state_coordination_meetings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    agenda TEXT,
    meeting_date TIMESTAMP WITH TIME ZONE NOT NULL,
    venue VARCHAR(300),
    meeting_type VARCHAR(30) DEFAULT 'REGULAR' CHECK (meeting_type IN ('REGULAR', 'EMERGENCY', 'REVIEW', 'PLANNING', 'COORDINATION')),
    chairperson_id UUID NOT NULL REFERENCES users(id),
    invited_districts UUID[] DEFAULT '{}',
    participants UUID[] DEFAULT '{}',
    minutes TEXT,
    decisions TEXT,
    action_items JSONB DEFAULT '[]',
    status VARCHAR(20) DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'POSTPONED')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- State Crime Patterns (for analytics)
CREATE TABLE IF NOT EXISTS state_crime_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    pattern_type VARCHAR(50) NOT NULL,
    crime_category VARCHAR(50) NOT NULL,
    affected_districts UUID[] DEFAULT '{}',
    description TEXT,
    start_date DATE NOT NULL,
    end_date DATE,
    trend VARCHAR(20) CHECK (trend IN ('INCREASING', 'DECREASING', 'STABLE', 'FLUCTUATING')),
    severity VARCHAR(15) DEFAULT 'MEDIUM' CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    recommendations TEXT,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'MONITORING', 'RESOLVED', 'ARCHIVED')),
    detected_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- State Performance Snapshots (daily/weekly/monthly)
CREATE TABLE IF NOT EXISTS state_performance_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    snapshot_date DATE NOT NULL,
    snapshot_type VARCHAR(10) NOT NULL CHECK (snapshot_type IN ('DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY')),
    total_firs INTEGER DEFAULT 0,
    pending_firs INTEGER DEFAULT 0,
    resolved_firs INTEGER DEFAULT 0,
    total_cases INTEGER DEFAULT 0,
    conviction_count INTEGER DEFAULT 0,
    acquittal_count INTEGER DEFAULT 0,
    chargesheet_count INTEGER DEFAULT 0,
    total_officers INTEGER DEFAULT 0,
    officers_on_duty INTEGER DEFAULT 0,
    resolution_rate DECIMAL(5,2),
    avg_resolution_days DECIMAL(10,2),
    conviction_rate DECIMAL(5,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(state_id, snapshot_date, snapshot_type)
);

-- State Resource Requests
CREATE TABLE IF NOT EXISTS state_resource_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_id UUID NOT NULL REFERENCES states(id) ON DELETE CASCADE,
    requesting_district UUID NOT NULL REFERENCES districts(id),
    resource_type VARCHAR(30) NOT NULL CHECK (resource_type IN ('PERSONNEL', 'VEHICLE', 'WEAPON', 'EQUIPMENT', 'SPECIAL_FORCE')),
    quantity INTEGER NOT NULL,
    purpose TEXT NOT NULL,
    urgency VARCHAR(15) DEFAULT 'NORMAL' CHECK (urgency IN ('NORMAL', 'URGENT', 'CRITICAL')),
    required_from DATE NOT NULL,
    required_until DATE,
    status VARCHAR(20) DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'PARTIALLY_APPROVED', 'REJECTED', 'FULFILLED', 'RETURNED')),
    approved_quantity INTEGER,
    allocated_from UUID[], -- districts from which resources are allocated
    approved_by UUID REFERENCES users(id),
    approved_at TIMESTAMP WITH TIME ZONE,
    requested_by UUID NOT NULL REFERENCES users(id),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_inter_district_requests_state ON inter_district_requests(state_id);
CREATE INDEX idx_inter_district_requests_requesting ON inter_district_requests(requesting_district);
CREATE INDEX idx_inter_district_requests_target ON inter_district_requests(target_district);
CREATE INDEX idx_inter_district_requests_status ON inter_district_requests(status);
CREATE INDEX idx_inter_district_requests_priority ON inter_district_requests(priority);
CREATE INDEX idx_inter_district_requests_created ON inter_district_requests(created_at);

CREATE INDEX idx_state_alerts_state ON state_alerts(state_id);
CREATE INDEX idx_state_alerts_type ON state_alerts(alert_type);
CREATE INDEX idx_state_alerts_severity ON state_alerts(severity);
CREATE INDEX idx_state_alerts_status ON state_alerts(status);
CREATE INDEX idx_state_alerts_expires ON state_alerts(expires_at);

CREATE INDEX idx_state_meetings_state ON state_coordination_meetings(state_id);
CREATE INDEX idx_state_meetings_date ON state_coordination_meetings(meeting_date);
CREATE INDEX idx_state_meetings_status ON state_coordination_meetings(status);
CREATE INDEX idx_state_meetings_chairperson ON state_coordination_meetings(chairperson_id);

CREATE INDEX idx_state_crime_patterns_state ON state_crime_patterns(state_id);
CREATE INDEX idx_state_crime_patterns_category ON state_crime_patterns(crime_category);
CREATE INDEX idx_state_crime_patterns_status ON state_crime_patterns(status);

CREATE INDEX idx_state_performance_state ON state_performance_snapshots(state_id);
CREATE INDEX idx_state_performance_date ON state_performance_snapshots(snapshot_date);
CREATE INDEX idx_state_performance_type ON state_performance_snapshots(snapshot_type);

CREATE INDEX idx_state_resource_requests_state ON state_resource_requests(state_id);
CREATE INDEX idx_state_resource_requests_district ON state_resource_requests(requesting_district);
CREATE INDEX idx_state_resource_requests_status ON state_resource_requests(status);
CREATE INDEX idx_state_resource_requests_type ON state_resource_requests(resource_type);

-- Function to expire old state alerts
CREATE OR REPLACE FUNCTION expire_state_alerts()
RETURNS void AS $$
BEGIN
    UPDATE state_alerts
    SET status = 'EXPIRED', updated_at = NOW()
    WHERE status = 'ACTIVE'
      AND expires_at IS NOT NULL
      AND expires_at < NOW();
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-expire alerts (can be called via cron)
CREATE OR REPLACE FUNCTION check_and_expire_alerts()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM expire_state_alerts();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
