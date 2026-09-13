-- Federated Sync System Tables
-- Implements: Vector clocks, conflict resolution, sync queues

-- Hierarchy table for jurisdictions
CREATE TABLE jurisdictions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('CENTRAL', 'STATE', 'DISTRICT', 'STATION')),
    parent_id UUID REFERENCES jurisdictions(id) ON DELETE SET NULL,

    -- Location
    state_code VARCHAR(10),
    district_code VARCHAR(10),
    station_code VARCHAR(10),

    -- Sync configuration
    sync_enabled BOOLEAN DEFAULT TRUE,
    sync_priority INTEGER DEFAULT 5,
    sync_interval_seconds INTEGER DEFAULT 300,
    last_sync_at TIMESTAMPTZ,
    last_sync_status VARCHAR(20),

    -- Connection info
    api_endpoint VARCHAR(500),
    is_online BOOLEAN DEFAULT TRUE,
    last_heartbeat TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Vector clock table for conflict detection
CREATE TABLE vector_clocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,

    -- Vector clock as JSONB: {"station_id": version, ...}
    clock JSONB NOT NULL DEFAULT '{}',

    -- Current version
    version BIGINT NOT NULL DEFAULT 1,

    -- Last modifier
    last_modified_by UUID,
    last_modified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(resource_type, resource_id)
);

-- Sync outbox for reliable message delivery
CREATE TABLE sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sequence_number BIGSERIAL NOT NULL,

    -- Source jurisdiction
    source_jurisdiction_id UUID NOT NULL REFERENCES jurisdictions(id),
    source_node_id VARCHAR(100) NOT NULL,

    -- Target (NULL means broadcast to parent)
    target_jurisdiction_id UUID REFERENCES jurisdictions(id),

    -- Event details
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN (
        'CREATE', 'UPDATE', 'DELETE', 'SYNC_REQUEST', 'SYNC_RESPONSE',
        'CONFLICT_DETECTED', 'CONFLICT_RESOLVED', 'HEARTBEAT'
    )),
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,

    -- Payload
    payload JSONB NOT NULL,

    -- Vector clock at time of event
    vector_clock JSONB NOT NULL,

    -- Delivery status
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'IN_PROGRESS', 'DELIVERED', 'FAILED', 'RETRY'
    )),
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 5,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ
);

-- Sync inbox for received messages
CREATE TABLE sync_inbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Source
    source_jurisdiction_id UUID NOT NULL,
    source_node_id VARCHAR(100) NOT NULL,
    outbox_id UUID NOT NULL,

    -- Event details
    event_type VARCHAR(50) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,

    -- Payload
    payload JSONB NOT NULL,
    vector_clock JSONB NOT NULL,

    -- Processing status
    status VARCHAR(20) NOT NULL DEFAULT 'RECEIVED' CHECK (status IN (
        'RECEIVED', 'PROCESSING', 'APPLIED', 'CONFLICT', 'REJECTED', 'FAILED'
    )),

    -- Conflict info
    conflict_type VARCHAR(50),
    conflict_details JSONB,
    resolution_strategy VARCHAR(50),
    resolved_at TIMESTAMPTZ,
    resolved_by UUID,

    -- Timestamps
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

-- Conflicts table for manual resolution
CREATE TABLE sync_conflicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Resource in conflict
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,

    -- Conflict type
    conflict_type VARCHAR(50) NOT NULL CHECK (conflict_type IN (
        'CONCURRENT_UPDATE', 'DELETE_UPDATE', 'PARENT_MISSING',
        'CONSTRAINT_VIOLATION', 'VERSION_MISMATCH', 'SCHEMA_MISMATCH'
    )),

    -- Conflicting versions
    local_version JSONB NOT NULL,
    local_vector_clock JSONB NOT NULL,
    local_modified_by UUID,
    local_modified_at TIMESTAMPTZ,

    remote_version JSONB NOT NULL,
    remote_vector_clock JSONB NOT NULL,
    remote_source_id UUID,
    remote_modified_by UUID,
    remote_modified_at TIMESTAMPTZ,

    -- Current resolution
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'AUTO_RESOLVED', 'MANUAL_REQUIRED', 'RESOLVED', 'ESCALATED'
    )),

    -- Resolution details
    resolution_strategy VARCHAR(50),
    resolved_version JSONB,
    resolved_by UUID,
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,

    -- Auto-resolution attempt
    auto_resolution_attempted BOOLEAN DEFAULT FALSE,
    auto_resolution_failed_reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sync state tracking per jurisdiction pair
CREATE TABLE sync_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Jurisdiction pair
    local_jurisdiction_id UUID NOT NULL REFERENCES jurisdictions(id),
    remote_jurisdiction_id UUID NOT NULL REFERENCES jurisdictions(id),

    -- Sync progress
    last_sent_sequence BIGINT DEFAULT 0,
    last_received_sequence BIGINT DEFAULT 0,
    last_ack_sequence BIGINT DEFAULT 0,

    -- Sync status
    sync_status VARCHAR(20) NOT NULL DEFAULT 'IDLE' CHECK (sync_status IN (
        'IDLE', 'SYNCING', 'PAUSED', 'ERROR', 'RECOVERING'
    )),

    -- Stats
    total_sent BIGINT DEFAULT 0,
    total_received BIGINT DEFAULT 0,
    total_conflicts BIGINT DEFAULT 0,
    total_resolved BIGINT DEFAULT 0,

    -- Timing
    last_sync_started_at TIMESTAMPTZ,
    last_sync_completed_at TIMESTAMPTZ,
    last_error TEXT,
    last_error_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(local_jurisdiction_id, remote_jurisdiction_id)
);

-- Sync checkpoints for recovery
CREATE TABLE sync_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jurisdiction_id UUID NOT NULL REFERENCES jurisdictions(id),

    -- Checkpoint data
    checkpoint_type VARCHAR(50) NOT NULL,
    checkpoint_data JSONB NOT NULL,
    sequence_number BIGINT NOT NULL,

    -- Validity
    valid_until TIMESTAMPTZ,
    is_valid BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Entity change log for sync (CDC-like)
CREATE TABLE entity_change_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sequence_number BIGSERIAL NOT NULL,

    -- Source
    jurisdiction_id UUID NOT NULL REFERENCES jurisdictions(id),

    -- Entity
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID NOT NULL,

    -- Change type
    change_type VARCHAR(20) NOT NULL CHECK (change_type IN ('INSERT', 'UPDATE', 'DELETE')),

    -- Data
    old_data JSONB,
    new_data JSONB,
    changed_columns TEXT[],

    -- Vector clock
    vector_clock JSONB NOT NULL,

    -- Context
    changed_by UUID,
    change_reason TEXT,
    transaction_id UUID,

    -- Sync status
    synced_to JSONB DEFAULT '{}', -- {"jurisdiction_id": timestamp}

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes
CREATE INDEX idx_jurisdictions_parent ON jurisdictions(parent_id);
CREATE INDEX idx_jurisdictions_type ON jurisdictions(type);
CREATE INDEX idx_jurisdictions_sync_enabled ON jurisdictions(sync_enabled);

CREATE INDEX idx_vector_clocks_resource ON vector_clocks(resource_type, resource_id);

CREATE INDEX idx_sync_outbox_status ON sync_outbox(status);
CREATE INDEX idx_sync_outbox_source ON sync_outbox(source_jurisdiction_id);
CREATE INDEX idx_sync_outbox_target ON sync_outbox(target_jurisdiction_id);
CREATE INDEX idx_sync_outbox_sequence ON sync_outbox(sequence_number);
CREATE INDEX idx_sync_outbox_retry ON sync_outbox(next_retry_at) WHERE status = 'RETRY';

CREATE INDEX idx_sync_inbox_status ON sync_inbox(status);
CREATE INDEX idx_sync_inbox_source ON sync_inbox(source_jurisdiction_id);
CREATE INDEX idx_sync_inbox_resource ON sync_inbox(resource_type, resource_id);

CREATE INDEX idx_sync_conflicts_status ON sync_conflicts(status);
CREATE INDEX idx_sync_conflicts_resource ON sync_conflicts(resource_type, resource_id);

CREATE INDEX idx_sync_state_jurisdictions ON sync_state(local_jurisdiction_id, remote_jurisdiction_id);

CREATE INDEX idx_entity_change_log_entity ON entity_change_log(entity_type, entity_id);
CREATE INDEX idx_entity_change_log_sequence ON entity_change_log(sequence_number);
CREATE INDEX idx_entity_change_log_jurisdiction ON entity_change_log(jurisdiction_id);

-- Create triggers for entity change capture
CREATE OR REPLACE FUNCTION capture_entity_change()
RETURNS TRIGGER AS $$
DECLARE
    change_type VARCHAR(20);
    old_data JSONB;
    new_data JSONB;
    changed_cols TEXT[];
    jurisdiction_id UUID;
    vec_clock JSONB;
BEGIN
    -- Determine change type
    IF TG_OP = 'INSERT' THEN
        change_type := 'INSERT';
        old_data := NULL;
        new_data := to_jsonb(NEW);
    ELSIF TG_OP = 'UPDATE' THEN
        change_type := 'UPDATE';
        old_data := to_jsonb(OLD);
        new_data := to_jsonb(NEW);
        -- Calculate changed columns
        SELECT array_agg(key) INTO changed_cols
        FROM jsonb_each(new_data) n
        FULL OUTER JOIN jsonb_each(old_data) o USING (key)
        WHERE n.value IS DISTINCT FROM o.value;
    ELSIF TG_OP = 'DELETE' THEN
        change_type := 'DELETE';
        old_data := to_jsonb(OLD);
        new_data := NULL;
    END IF;

    -- Get jurisdiction from context or default
    jurisdiction_id := COALESCE(
        current_setting('npdms.current_jurisdiction_id', TRUE)::UUID,
        '00000000-0000-0000-0000-000000000000'::UUID
    );

    -- Get or create vector clock
    SELECT clock INTO vec_clock
    FROM vector_clocks
    WHERE resource_type = TG_TABLE_NAME AND resource_id = COALESCE(NEW.id, OLD.id);

    IF vec_clock IS NULL THEN
        vec_clock := jsonb_build_object(jurisdiction_id::TEXT, 1);
    ELSE
        vec_clock := jsonb_set(
            vec_clock,
            ARRAY[jurisdiction_id::TEXT],
            to_jsonb(COALESCE((vec_clock->>jurisdiction_id::TEXT)::BIGINT, 0) + 1)
        );
    END IF;

    -- Insert change log entry
    INSERT INTO entity_change_log (
        jurisdiction_id, entity_type, entity_id,
        change_type, old_data, new_data, changed_columns,
        vector_clock
    ) VALUES (
        jurisdiction_id, TG_TABLE_NAME, COALESCE(NEW.id, OLD.id),
        change_type, old_data, new_data, changed_cols,
        vec_clock
    );

    -- Update vector clock
    INSERT INTO vector_clocks (resource_type, resource_id, clock, version)
    VALUES (TG_TABLE_NAME, COALESCE(NEW.id, OLD.id), vec_clock, 1)
    ON CONFLICT (resource_type, resource_id)
    DO UPDATE SET
        clock = EXCLUDED.clock,
        version = vector_clocks.version + 1,
        last_modified_at = NOW();

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- Apply change capture triggers to main tables
DO $$
DECLARE
    tbl TEXT;
    tables TEXT[] := ARRAY['firs', 'cases', 'evidence', 'warrants', 'bail',
                           'personnel', 'vehicles', 'alerts', 'cyber_crimes',
                           'citizen_complaints'];
BEGIN
    FOREACH tbl IN ARRAY tables
    LOOP
        EXECUTE format('
            DROP TRIGGER IF EXISTS capture_changes_%I ON %I;
            CREATE TRIGGER capture_changes_%I
            AFTER INSERT OR UPDATE OR DELETE ON %I
            FOR EACH ROW EXECUTE FUNCTION capture_entity_change();
        ', tbl, tbl, tbl, tbl);
    END LOOP;
END $$;

-- Function to merge vector clocks
CREATE OR REPLACE FUNCTION merge_vector_clocks(clock1 JSONB, clock2 JSONB)
RETURNS JSONB AS $$
DECLARE
    result JSONB := '{}';
    key TEXT;
    val1 BIGINT;
    val2 BIGINT;
BEGIN
    -- Get all keys from both clocks
    FOR key IN SELECT DISTINCT jsonb_object_keys(clock1) UNION SELECT jsonb_object_keys(clock2)
    LOOP
        val1 := COALESCE((clock1->>key)::BIGINT, 0);
        val2 := COALESCE((clock2->>key)::BIGINT, 0);
        result := jsonb_set(result, ARRAY[key], to_jsonb(GREATEST(val1, val2)));
    END LOOP;

    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- Function to compare vector clocks
-- Returns: 'EQUAL', 'BEFORE', 'AFTER', 'CONCURRENT'
CREATE OR REPLACE FUNCTION compare_vector_clocks(clock1 JSONB, clock2 JSONB)
RETURNS TEXT AS $$
DECLARE
    key TEXT;
    val1 BIGINT;
    val2 BIGINT;
    has_greater BOOLEAN := FALSE;
    has_lesser BOOLEAN := FALSE;
BEGIN
    FOR key IN SELECT DISTINCT jsonb_object_keys(clock1) UNION SELECT jsonb_object_keys(clock2)
    LOOP
        val1 := COALESCE((clock1->>key)::BIGINT, 0);
        val2 := COALESCE((clock2->>key)::BIGINT, 0);

        IF val1 > val2 THEN
            has_greater := TRUE;
        ELSIF val1 < val2 THEN
            has_lesser := TRUE;
        END IF;
    END LOOP;

    IF has_greater AND has_lesser THEN
        RETURN 'CONCURRENT';
    ELSIF has_greater THEN
        RETURN 'AFTER';
    ELSIF has_lesser THEN
        RETURN 'BEFORE';
    ELSE
        RETURN 'EQUAL';
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Insert default jurisdictions
INSERT INTO jurisdictions (id, code, name, type, sync_priority) VALUES
    ('00000000-0000-0000-0000-000000000001', 'CENTRAL', 'National Command', 'CENTRAL', 1),
    ('00000000-0000-0000-0000-000000000002', 'STATE_KA', 'Karnataka State', 'STATE', 2),
    ('00000000-0000-0000-0000-000000000003', 'DIST_BLR', 'Bangalore District', 'DISTRICT', 3),
    ('00000000-0000-0000-0000-000000000004', 'STN_KOR', 'Koramangala Station', 'STATION', 4)
ON CONFLICT (code) DO NOTHING;

-- Set up hierarchy
UPDATE jurisdictions SET parent_id = '00000000-0000-0000-0000-000000000001' WHERE code = 'STATE_KA';
UPDATE jurisdictions SET parent_id = '00000000-0000-0000-0000-000000000002' WHERE code = 'DIST_BLR';
UPDATE jurisdictions SET parent_id = '00000000-0000-0000-0000-000000000003' WHERE code = 'STN_KOR';

COMMENT ON TABLE sync_outbox IS 'Outbox pattern for reliable sync message delivery';
COMMENT ON TABLE sync_conflicts IS 'Detected conflicts requiring resolution';
COMMENT ON TABLE vector_clocks IS 'Vector clocks for causality tracking';
