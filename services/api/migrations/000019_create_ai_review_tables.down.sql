-- Drop triggers
DROP TRIGGER IF EXISTS ai_decisions_updated_at ON ai_decisions;
DROP TRIGGER IF EXISTS ai_model_configs_updated_at ON ai_model_configs;

-- Drop functions
DROP FUNCTION IF EXISTS update_ai_decision_timestamp();
DROP FUNCTION IF EXISTS calculate_ai_metrics(VARCHAR, VARCHAR, TIMESTAMP, TIMESTAMP);

-- Drop tables
DROP TABLE IF EXISTS ai_review_assignments;
DROP TABLE IF EXISTS ai_performance_metrics;
DROP TABLE IF EXISTS ai_decision_feedback;
DROP TABLE IF EXISTS ai_model_configs;
DROP TABLE IF EXISTS ai_decisions;
