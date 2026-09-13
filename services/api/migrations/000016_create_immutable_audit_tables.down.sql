-- Rollback immutable audit tables

DROP TABLE IF EXISTS audit_logs_archive_2026_q1;
DROP TABLE IF EXISTS audit_logs_archive_2026_q2;
DROP TABLE IF EXISTS audit_logs_archive_2026_q3;
DROP TABLE IF EXISTS audit_logs_archive_2026_q4;
DROP TABLE IF EXISTS audit_logs_archive;
DROP TABLE IF EXISTS audit_tamper_alerts;
DROP TABLE IF EXISTS audit_log_batches;

DROP TRIGGER IF EXISTS audit_log_immutable_update ON audit_logs;
DROP TRIGGER IF EXISTS audit_log_immutable_delete ON audit_logs;

DROP FUNCTION IF EXISTS prevent_audit_log_modification();
DROP FUNCTION IF EXISTS verify_audit_log_chain(BIGINT, BIGINT);

DROP TABLE IF EXISTS audit_logs;
DROP SEQUENCE IF EXISTS audit_log_sequence;

-- Recreate basic audit_logs table for rollback
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    action VARCHAR(50) NOT NULL,
    resource_type VARCHAR(100),
    resource_id UUID,
    description TEXT,
    ip_address VARCHAR(50),
    user_agent TEXT,
    success BOOLEAN DEFAULT TRUE,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
