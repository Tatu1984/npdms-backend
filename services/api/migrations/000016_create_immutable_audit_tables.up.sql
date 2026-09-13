-- Enhanced Immutable Audit Logging System
-- Implements: Hash chains, tamper detection, blockchain anchoring

-- Drop existing audit_logs table if exists and recreate with immutability
-- Note: In production, migrate data first
DROP TABLE IF EXISTS audit_logs CASCADE;

-- Create immutable audit_logs table
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sequence_number BIGSERIAL NOT NULL,

    -- Event identification
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    event_type VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL CHECK (action IN (
        'CREATE', 'READ', 'UPDATE', 'DELETE',
        'LOGIN', 'LOGOUT', 'EXPORT', 'PRINT',
        'APPROVE', 'REJECT', 'ESCALATE', 'TRANSFER',
        'VERIFY', 'SIGN', 'ENCRYPT', 'DECRYPT'
    )),

    -- Actor information
    actor_user_id UUID,
    actor_badge_number VARCHAR(50),
    actor_role VARCHAR(50),
    actor_station_id UUID,
    actor_district_id UUID,
    actor_state_id UUID,

    -- Device & Session
    device_id VARCHAR(255),
    device_type VARCHAR(50),
    device_fingerprint VARCHAR(255),
    session_id VARCHAR(255),
    ip_address INET,
    user_agent TEXT,
    geo_location JSONB,

    -- Resource affected
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID,
    resource_attributes JSONB,

    -- Changes (for UPDATE operations)
    previous_values JSONB,
    new_values JSONB,
    changed_fields TEXT[],

    -- Outcome
    outcome VARCHAR(20) NOT NULL CHECK (outcome IN ('SUCCESS', 'FAILURE', 'DENIED', 'PARTIAL')),
    outcome_reason TEXT,
    error_code VARCHAR(50),

    -- Context
    correlation_id UUID,
    trace_id VARCHAR(64),
    span_id VARCHAR(32),
    request_id VARCHAR(64),

    -- Cryptographic integrity
    previous_hash VARCHAR(128),
    current_hash VARCHAR(128) NOT NULL,
    hash_algorithm VARCHAR(20) NOT NULL DEFAULT 'SHA-512',
    signature BYTEA,
    signing_key_id VARCHAR(100),

    -- Timestamps
    event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Anchoring
    blockchain_anchor_tx VARCHAR(255),
    blockchain_anchor_time TIMESTAMPTZ,
    anchor_batch_id UUID
);

-- Create sequence for audit logs (explicit for reference)
CREATE SEQUENCE IF NOT EXISTS audit_log_sequence START 1;

-- Create append-only enforcement trigger
CREATE OR REPLACE FUNCTION prevent_audit_log_modification()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        -- Only allow updating blockchain anchor fields
        IF NEW.id != OLD.id OR
           NEW.sequence_number != OLD.sequence_number OR
           NEW.event_id != OLD.event_id OR
           NEW.current_hash != OLD.current_hash OR
           NEW.previous_hash IS DISTINCT FROM OLD.previous_hash OR
           NEW.event_timestamp != OLD.event_timestamp THEN
            RAISE EXCEPTION 'Audit log modification not allowed - immutable fields cannot be changed';
        END IF;

        -- Only allow anchor updates
        IF NEW.blockchain_anchor_tx IS NOT NULL AND OLD.blockchain_anchor_tx IS NULL THEN
            RETURN NEW;
        END IF;

        RAISE EXCEPTION 'Audit log modification not allowed';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'Audit log deletion not allowed';
    END IF;

    IF TG_OP = 'TRUNCATE' THEN
        RAISE EXCEPTION 'Audit log truncation not allowed';
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Apply triggers
CREATE TRIGGER audit_log_immutable_update
    BEFORE UPDATE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_log_modification();

CREATE TRIGGER audit_log_immutable_delete
    BEFORE DELETE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_log_modification();

-- Create hash chain verification function
CREATE OR REPLACE FUNCTION verify_audit_log_chain(start_seq BIGINT, end_seq BIGINT)
RETURNS TABLE (
    sequence_number BIGINT,
    is_valid BOOLEAN,
    error_message TEXT
) AS $$
DECLARE
    rec RECORD;
    expected_previous_hash VARCHAR(128);
    is_first BOOLEAN := TRUE;
BEGIN
    FOR rec IN
        SELECT al.sequence_number, al.current_hash, al.previous_hash
        FROM audit_logs al
        WHERE al.sequence_number BETWEEN start_seq AND end_seq
        ORDER BY al.sequence_number
    LOOP
        IF is_first THEN
            -- First record should have NULL or match known genesis hash
            IF rec.previous_hash IS NOT NULL AND rec.sequence_number > 1 THEN
                -- Verify against previous record
                SELECT al.current_hash INTO expected_previous_hash
                FROM audit_logs al
                WHERE al.sequence_number = rec.sequence_number - 1;

                IF expected_previous_hash != rec.previous_hash THEN
                    RETURN QUERY SELECT rec.sequence_number, FALSE,
                        'Previous hash mismatch: expected ' || expected_previous_hash || ' got ' || rec.previous_hash;
                    RETURN;
                END IF;
            END IF;
            is_first := FALSE;
        ELSE
            -- Subsequent records must match
            IF expected_previous_hash != rec.previous_hash THEN
                RETURN QUERY SELECT rec.sequence_number, FALSE,
                    'Chain broken at sequence ' || rec.sequence_number;
                RETURN;
            END IF;
        END IF;

        expected_previous_hash := rec.current_hash;
        sequence_number := rec.sequence_number;
        is_valid := TRUE;
        error_message := NULL;
        RETURN NEXT;
    END LOOP;

    RETURN;
END;
$$ LANGUAGE plpgsql;

-- Create audit log batches table for blockchain anchoring
CREATE TABLE audit_log_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_number SERIAL NOT NULL,
    start_sequence BIGINT NOT NULL,
    end_sequence BIGINT NOT NULL,
    record_count INTEGER NOT NULL,
    merkle_root VARCHAR(128) NOT NULL,
    batch_hash VARCHAR(128) NOT NULL,

    -- Blockchain anchoring
    blockchain_network VARCHAR(50),
    transaction_hash VARCHAR(255),
    block_number BIGINT,
    anchor_timestamp TIMESTAMPTZ,
    anchor_status VARCHAR(20) CHECK (anchor_status IN ('PENDING', 'CONFIRMED', 'FAILED')),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create tamper detection alerts table
CREATE TABLE audit_tamper_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_type VARCHAR(50) NOT NULL CHECK (alert_type IN (
        'HASH_MISMATCH', 'SEQUENCE_GAP', 'TIMESTAMP_ANOMALY',
        'SIGNATURE_INVALID', 'CHAIN_BROKEN', 'UNAUTHORIZED_ACCESS'
    )),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),

    affected_sequence_start BIGINT,
    affected_sequence_end BIGINT,
    affected_record_ids UUID[],

    detection_method VARCHAR(100) NOT NULL,
    details JSONB NOT NULL,

    investigated_by UUID,
    investigation_status VARCHAR(20) CHECK (investigation_status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'FALSE_POSITIVE')),
    investigation_notes TEXT,
    resolved_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for audit_logs
CREATE INDEX idx_audit_logs_sequence ON audit_logs(sequence_number);
CREATE INDEX idx_audit_logs_event_timestamp ON audit_logs(event_timestamp);
CREATE INDEX idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_outcome ON audit_logs(outcome);
CREATE INDEX idx_audit_logs_correlation ON audit_logs(correlation_id);
CREATE INDEX idx_audit_logs_trace ON audit_logs(trace_id);
CREATE INDEX idx_audit_logs_station ON audit_logs(actor_station_id);
CREATE INDEX idx_audit_logs_district ON audit_logs(actor_district_id);
CREATE INDEX idx_audit_logs_hash ON audit_logs(current_hash);

-- Create indexes for batches
CREATE INDEX idx_audit_batches_sequence ON audit_log_batches(start_sequence, end_sequence);
CREATE INDEX idx_audit_batches_status ON audit_log_batches(anchor_status);

-- Create indexes for tamper alerts
CREATE INDEX idx_tamper_alerts_severity ON audit_tamper_alerts(severity);
CREATE INDEX idx_tamper_alerts_status ON audit_tamper_alerts(investigation_status);
CREATE INDEX idx_tamper_alerts_created ON audit_tamper_alerts(created_at);

-- Revoke dangerous permissions
REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs FROM PUBLIC;
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log_batches FROM PUBLIC;

-- Create audit archival table for long-term storage
CREATE TABLE audit_logs_archive (
    LIKE audit_logs INCLUDING ALL
) PARTITION BY RANGE (event_timestamp);

-- Create partitions for archival (example for 2026)
CREATE TABLE audit_logs_archive_2026_q1 PARTITION OF audit_logs_archive
    FOR VALUES FROM ('2026-01-01') TO ('2026-04-01');
CREATE TABLE audit_logs_archive_2026_q2 PARTITION OF audit_logs_archive
    FOR VALUES FROM ('2026-04-01') TO ('2026-07-01');
CREATE TABLE audit_logs_archive_2026_q3 PARTITION OF audit_logs_archive
    FOR VALUES FROM ('2026-07-01') TO ('2026-10-01');
CREATE TABLE audit_logs_archive_2026_q4 PARTITION OF audit_logs_archive
    FOR VALUES FROM ('2026-10-01') TO ('2027-01-01');

COMMENT ON TABLE audit_logs IS 'Immutable audit log with cryptographic hash chain for tamper detection';
COMMENT ON COLUMN audit_logs.current_hash IS 'SHA-512 hash of event data + previous_hash';
COMMENT ON COLUMN audit_logs.previous_hash IS 'Hash of previous audit log entry for chain verification';
