-- AI Decisions table for human-in-loop review
CREATE TABLE IF NOT EXISTS ai_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    priority VARCHAR(20) DEFAULT 'MEDIUM',

    -- Source context
    source_type VARCHAR(50) NOT NULL,
    source_id UUID NOT NULL,
    source_reference VARCHAR(100),

    -- AI prediction
    model_name VARCHAR(100),
    model_version VARCHAR(50),
    prediction TEXT NOT NULL,
    prediction_data JSONB,
    confidence FLOAT NOT NULL,
    confidence_threshold FLOAT NOT NULL,
    alternatives JSONB,

    -- Human review
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMP,
    review_notes VARCHAR(1000),
    human_decision VARCHAR(500),
    override_reason VARCHAR(500),

    -- Assignment
    assigned_to UUID REFERENCES users(id),
    assigned_at TIMESTAMP,
    due_by TIMESTAMP,

    -- Metadata
    requested_by UUID NOT NULL REFERENCES users(id),
    station_id UUID REFERENCES police_stations(id),
    processing_time_ms BIGINT DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for ai_decisions
CREATE INDEX idx_ai_decisions_type ON ai_decisions(type);
CREATE INDEX idx_ai_decisions_status ON ai_decisions(status);
CREATE INDEX idx_ai_decisions_source ON ai_decisions(source_type, source_id);
CREATE INDEX idx_ai_decisions_assigned ON ai_decisions(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_ai_decisions_pending ON ai_decisions(priority, created_at) WHERE status = 'PENDING';
CREATE INDEX idx_ai_decisions_station ON ai_decisions(station_id) WHERE station_id IS NOT NULL;

-- AI Decision Feedback for model improvement
CREATE TABLE IF NOT EXISTS ai_decision_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    decision_id UUID NOT NULL REFERENCES ai_decisions(id) ON DELETE CASCADE,
    feedback_type VARCHAR(50) NOT NULL,
    feedback_by UUID NOT NULL REFERENCES users(id),
    correct_value TEXT,
    comments VARCHAR(1000),
    used_for_training BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for ai_decision_feedback
CREATE INDEX idx_ai_decision_feedback_decision ON ai_decision_feedback(decision_id);
CREATE INDEX idx_ai_decision_feedback_type ON ai_decision_feedback(feedback_type);
CREATE INDEX idx_ai_decision_feedback_training ON ai_decision_feedback(used_for_training) WHERE NOT used_for_training;

-- AI Model Configuration
CREATE TABLE IF NOT EXISTS ai_model_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_name VARCHAR(100) UNIQUE NOT NULL,
    decision_type VARCHAR(50) NOT NULL,
    confidence_threshold FLOAT NOT NULL DEFAULT 0.85,
    auto_approve_threshold FLOAT NOT NULL DEFAULT 0.95,
    is_enabled BOOLEAN DEFAULT TRUE,
    requires_review BOOLEAN DEFAULT TRUE,
    review_timeout INTEGER DEFAULT 24,
    max_queue_size INTEGER DEFAULT 1000,
    description VARCHAR(500),
    config_data JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert default model configurations
INSERT INTO ai_model_configs (model_name, decision_type, confidence_threshold, auto_approve_threshold, description) VALUES
    ('ipc_classifier_v1', 'IPC_CLASSIFICATION', 0.85, 0.95, 'IPC Section Classification Model'),
    ('crime_category_v1', 'CRIME_CATEGORY', 0.80, 0.92, 'Crime Category Detection Model'),
    ('priority_assigner_v1', 'PRIORITY_ASSIGNMENT', 0.75, 0.90, 'Case Priority Assignment Model'),
    ('suspect_matcher_v1', 'SUSPECT_MATCH', 0.90, 0.98, 'Suspect Matching Model'),
    ('fraud_detector_v1', 'FRAUD_DETECTION', 0.88, 0.96, 'Fraud Detection Model'),
    ('document_classifier_v1', 'DOCUMENT_CLASSIFICATION', 0.82, 0.94, 'Document Classification Model'),
    ('entity_extractor_v1', 'ENTITY_EXTRACTION', 0.78, 0.91, 'Named Entity Extraction Model'),
    ('sentiment_analyzer_v1', 'SENTIMENT_ANALYSIS', 0.80, 0.93, 'Sentiment Analysis Model')
ON CONFLICT (model_name) DO NOTHING;

-- AI Performance Metrics
CREATE TABLE IF NOT EXISTS ai_performance_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_name VARCHAR(100) NOT NULL,
    decision_type VARCHAR(50) NOT NULL,
    period VARCHAR(20) NOT NULL,
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,

    -- Counts
    total_decisions INTEGER DEFAULT 0,
    auto_approved INTEGER DEFAULT 0,
    human_approved INTEGER DEFAULT 0,
    human_rejected INTEGER DEFAULT 0,
    overridden INTEGER DEFAULT 0,
    expired INTEGER DEFAULT 0,

    -- Accuracy metrics
    accuracy_rate FLOAT DEFAULT 0,
    precision_rate FLOAT DEFAULT 0,
    recall_rate FLOAT DEFAULT 0,
    f1_score FLOAT DEFAULT 0,

    -- Performance metrics
    avg_confidence FLOAT DEFAULT 0,
    avg_processing_ms FLOAT DEFAULT 0,
    avg_review_time_hrs FLOAT DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(model_name, period, period_start)
);

-- Indexes for ai_performance_metrics
CREATE INDEX idx_ai_performance_model ON ai_performance_metrics(model_name);
CREATE INDEX idx_ai_performance_period ON ai_performance_metrics(period, period_start);

-- AI Review Assignments
CREATE TABLE IF NOT EXISTS ai_review_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reviewer_id UUID NOT NULL REFERENCES users(id),
    decision_id UUID NOT NULL REFERENCES ai_decisions(id) ON DELETE CASCADE,
    assigned_by UUID NOT NULL REFERENCES users(id),
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    due_by TIMESTAMP,
    completed_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'PENDING',
    notes VARCHAR(500),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for ai_review_assignments
CREATE INDEX idx_ai_review_assignments_reviewer ON ai_review_assignments(reviewer_id);
CREATE INDEX idx_ai_review_assignments_decision ON ai_review_assignments(decision_id);
CREATE INDEX idx_ai_review_assignments_status ON ai_review_assignments(status);
CREATE INDEX idx_ai_review_assignments_pending ON ai_review_assignments(reviewer_id, due_by) WHERE status = 'PENDING';

-- Function to calculate AI model performance metrics
CREATE OR REPLACE FUNCTION calculate_ai_metrics(
    p_model_name VARCHAR,
    p_period VARCHAR,
    p_start_date TIMESTAMP,
    p_end_date TIMESTAMP
) RETURNS void AS $$
DECLARE
    v_decision_type VARCHAR;
    v_total INTEGER;
    v_auto_approved INTEGER;
    v_human_approved INTEGER;
    v_human_rejected INTEGER;
    v_overridden INTEGER;
    v_expired INTEGER;
    v_avg_confidence FLOAT;
    v_avg_processing FLOAT;
    v_avg_review_time FLOAT;
    v_accuracy FLOAT;
BEGIN
    -- Get decision type for model
    SELECT decision_type INTO v_decision_type FROM ai_model_configs WHERE model_name = p_model_name;

    -- Calculate counts
    SELECT
        COUNT(*),
        COUNT(*) FILTER (WHERE status = 'AUTO_APPROVED'),
        COUNT(*) FILTER (WHERE status = 'APPROVED'),
        COUNT(*) FILTER (WHERE status = 'REJECTED'),
        COUNT(*) FILTER (WHERE status = 'OVERRIDDEN'),
        COUNT(*) FILTER (WHERE status = 'EXPIRED'),
        COALESCE(AVG(confidence), 0),
        COALESCE(AVG(processing_time_ms), 0)
    INTO
        v_total, v_auto_approved, v_human_approved, v_human_rejected,
        v_overridden, v_expired, v_avg_confidence, v_avg_processing
    FROM ai_decisions
    WHERE model_name = p_model_name
    AND created_at >= p_start_date
    AND created_at < p_end_date;

    -- Calculate average review time
    SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (reviewed_at - created_at)) / 3600), 0)
    INTO v_avg_review_time
    FROM ai_decisions
    WHERE model_name = p_model_name
    AND created_at >= p_start_date
    AND created_at < p_end_date
    AND reviewed_at IS NOT NULL;

    -- Calculate accuracy (approved / (approved + rejected + overridden))
    IF (v_human_approved + v_human_rejected + v_overridden) > 0 THEN
        v_accuracy := v_human_approved::FLOAT / (v_human_approved + v_human_rejected + v_overridden)::FLOAT;
    ELSE
        v_accuracy := 0;
    END IF;

    -- Insert or update metrics
    INSERT INTO ai_performance_metrics (
        model_name, decision_type, period, period_start, period_end,
        total_decisions, auto_approved, human_approved, human_rejected, overridden, expired,
        accuracy_rate, avg_confidence, avg_processing_ms, avg_review_time_hrs
    ) VALUES (
        p_model_name, v_decision_type, p_period, p_start_date, p_end_date,
        v_total, v_auto_approved, v_human_approved, v_human_rejected, v_overridden, v_expired,
        v_accuracy, v_avg_confidence, v_avg_processing, v_avg_review_time
    )
    ON CONFLICT (model_name, period, period_start)
    DO UPDATE SET
        total_decisions = EXCLUDED.total_decisions,
        auto_approved = EXCLUDED.auto_approved,
        human_approved = EXCLUDED.human_approved,
        human_rejected = EXCLUDED.human_rejected,
        overridden = EXCLUDED.overridden,
        expired = EXCLUDED.expired,
        accuracy_rate = EXCLUDED.accuracy_rate,
        avg_confidence = EXCLUDED.avg_confidence,
        avg_processing_ms = EXCLUDED.avg_processing_ms,
        avg_review_time_hrs = EXCLUDED.avg_review_time_hrs;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update timestamps
CREATE OR REPLACE FUNCTION update_ai_decision_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ai_decisions_updated_at
    BEFORE UPDATE ON ai_decisions
    FOR EACH ROW
    EXECUTE FUNCTION update_ai_decision_timestamp();

CREATE TRIGGER ai_model_configs_updated_at
    BEFORE UPDATE ON ai_model_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_ai_decision_timestamp();
