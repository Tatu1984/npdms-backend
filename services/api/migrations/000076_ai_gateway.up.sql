-- AI layer A0 — foundations.
--
-- Everything a model produces reaches an officer through one path: the gateway
-- writes an ai_decisions row, and that row is a suggestion until an officer
-- acts on it. This migration makes the database hold the rules the plan states,
-- so they cannot be bypassed by a caller that forgets them:
--
--   * a model runs only while its module switch is on and the model itself is
--     enabled, and a model cannot be enabled until a held-out evaluation of
--     that exact version has passed its stated threshold;
--   * nothing is approved by confidence alone — the AUTO_APPROVED status and
--     the auto-approve threshold are removed, and a decision that leaves
--     PENDING must name the officer who decided it;
--   * every suggestion carries its model version, its confidence and the
--     record text it relied on.
--
-- Idempotent and safe to apply twice: bootstrap-db.sh runs the chain in two
-- passes.

-- ------------------------------------------------------------- registry ------

-- ai_model_configs arrived in 000019 as a thresholds table seeded with eight
-- models that were never built (ipc_classifier_v1 and friends, written around
-- the IPC rather than the BNS). A registry that lists models which do not exist
-- is worse than an empty one, so the fictional rows go and the table becomes
-- the real register: what the model is, where it runs, under what licence, and
-- whether it is allowed to run at all.
DELETE FROM ai_model_configs
 WHERE model_name IN (
        'ipc_classifier_v1', 'crime_category_v1', 'priority_assigner_v1',
        'suspect_matcher_v1', 'fraud_detector_v1', 'document_classifier_v1',
        'entity_extractor_v1', 'sentiment_analyzer_v1')
   AND NOT EXISTS (SELECT 1 FROM ai_decisions d WHERE d.model_name = ai_model_configs.model_name);

ALTER TABLE ai_model_configs
    ADD COLUMN IF NOT EXISTS module          VARCHAR(40),
    ADD COLUMN IF NOT EXISTS model_version   VARCHAR(50),
    ADD COLUMN IF NOT EXISTS task            VARCHAR(60),
    -- The environment variable holding this model's service URL. The API reads
    -- it at start-up; where it is unset the module reports "not connected"
    -- rather than inventing an answer.
    ADD COLUMN IF NOT EXISTS endpoint_env    VARCHAR(80),
    ADD COLUMN IF NOT EXISTS licence         VARCHAR(120),
    ADD COLUMN IF NOT EXISTS source_url      TEXT,
    ADD COLUMN IF NOT EXISTS registered_by   UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS retired_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS retired_reason  TEXT;

-- Confidence alone never approves anything.
ALTER TABLE ai_model_configs DROP COLUMN IF EXISTS auto_approve_threshold;

-- A model is off until someone measures it and switches it on.
ALTER TABLE ai_model_configs ALTER COLUMN is_enabled SET DEFAULT FALSE;
UPDATE ai_model_configs SET is_enabled = FALSE WHERE is_enabled IS NULL;
ALTER TABLE ai_model_configs ALTER COLUMN is_enabled SET NOT NULL;

-- Review is not optional, whatever a caller passes.
UPDATE ai_model_configs SET requires_review = TRUE WHERE requires_review IS DISTINCT FROM TRUE;
ALTER TABLE ai_model_configs ALTER COLUMN requires_review SET DEFAULT TRUE;
DO $$
BEGIN
    ALTER TABLE ai_model_configs ADD CONSTRAINT ai_model_review_is_mandatory CHECK (requires_review);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE ai_model_configs ADD CONSTRAINT ai_model_threshold_range
        CHECK (confidence_threshold > 0 AND confidence_threshold <= 1);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_ai_model_configs_module ON ai_model_configs (module) WHERE retired_at IS NULL;

-- ---------------------------------------------------------- evaluations ------

-- "Measured before enabled." One row per run of a held-out example set against
-- one model version. The set is named and its size recorded so a claim can be
-- checked later; the threshold is stated before the result, not after.
CREATE TABLE IF NOT EXISTS ai_model_evaluations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_name      VARCHAR(100) NOT NULL,
    model_version   VARCHAR(50)  NOT NULL CHECK (btrim(model_version) <> ''),
    -- What was measured, on what, and against what bar.
    dataset         TEXT NOT NULL CHECK (btrim(dataset) <> ''),
    dataset_size    INTEGER NOT NULL CHECK (dataset_size > 0),
    dataset_sha256  CHAR(64),
    metric          VARCHAR(40) NOT NULL CHECK (btrim(metric) <> ''),
    threshold       DOUBLE PRECISION NOT NULL CHECK (threshold > 0 AND threshold <= 1),
    measured        DOUBLE PRECISION NOT NULL CHECK (measured >= 0 AND measured <= 1),
    passed          BOOLEAN NOT NULL,
    -- Stated limits of the measurement: what it did not cover.
    limitations     TEXT,
    notes           TEXT,
    run_by          UUID NOT NULL REFERENCES users(id),
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_eval_model ON ai_model_evaluations (model_name, model_version, run_at DESC);

-- passed must agree with the numbers it reports.
CREATE OR REPLACE FUNCTION ai_evaluation_verdict() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.passed <> (NEW.measured >= NEW.threshold) THEN
        RAISE EXCEPTION 'an evaluation passes exactly when the measured value reaches its threshold (measured %, threshold %)',
            NEW.measured, NEW.threshold
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_evaluation_verdict';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_evaluation_verdict ON ai_model_evaluations;
CREATE TRIGGER trg_ai_evaluation_verdict
    BEFORE INSERT OR UPDATE ON ai_model_evaluations
    FOR EACH ROW EXECUTE FUNCTION ai_evaluation_verdict();

-- An evaluation is a record of a measurement. It is never edited or deleted.
CREATE OR REPLACE FUNCTION ai_evaluations_are_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ai_model_evaluations is append-only: a measurement is not rewritten'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_evaluations_append_only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_evaluations_append_only ON ai_model_evaluations;
CREATE TRIGGER trg_ai_evaluations_append_only
    BEFORE UPDATE OR DELETE ON ai_model_evaluations
    FOR EACH ROW EXECUTE FUNCTION ai_evaluations_are_append_only();

-- Has this exact model version passed an evaluation?
CREATE OR REPLACE FUNCTION ai_model_is_measured(p_model TEXT, p_version TEXT) RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM ai_model_evaluations
         WHERE model_name = p_model AND model_version = p_version AND passed
    )
$$ LANGUAGE sql STABLE;

-- A model cannot be switched on until that is true of the version it runs.
CREATE OR REPLACE FUNCTION ai_model_enable_requires_evaluation() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_enabled THEN
        IF NEW.model_version IS NULL OR btrim(NEW.model_version) = '' THEN
            RAISE EXCEPTION 'model % cannot be enabled without a version', NEW.model_name
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_version_required';
        END IF;
        IF NOT ai_model_is_measured(NEW.model_name, NEW.model_version) THEN
            RAISE EXCEPTION 'model % version % has not passed an evaluation and cannot be enabled',
                NEW.model_name, NEW.model_version
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_requires_evaluation';
        END IF;
        IF NEW.retired_at IS NOT NULL THEN
            RAISE EXCEPTION 'model % is retired and cannot be enabled', NEW.model_name
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_retired';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_model_enable_requires_evaluation ON ai_model_configs;
CREATE TRIGGER trg_ai_model_enable_requires_evaluation
    BEFORE INSERT OR UPDATE ON ai_model_configs
    FOR EACH ROW EXECUTE FUNCTION ai_model_enable_requires_evaluation();

-- ------------------------------------------------------- one switch table ----

-- 000074 created module_switches for vehicle detection two migrations after
-- 000072 created ai_module_switches for face recognition, so the platform grew
-- two tables for one job. ai_module_switches is the survivor: 000072 says so,
-- and it carries the config the thresholds live in.
ALTER TABLE ai_module_switches ADD COLUMN IF NOT EXISTS note TEXT;

DO $$
BEGIN
    IF to_regclass('public.module_switches') IS NOT NULL THEN
        -- DO NOTHING, not DO UPDATE: bootstrap-db.sh re-runs the whole chain,
        -- and 000074 recreates module_switches with its off-at-installation
        -- seed each time. Overwriting here would switch vehicle detection off
        -- on every bootstrap and erase which officer had switched it on.
        INSERT INTO ai_module_switches (module, enabled, config, reason, note, updated_by, updated_at)
        SELECT module, enabled, '{}'::jsonb, note, note, updated_by, updated_at
          FROM module_switches
        ON CONFLICT (module) DO NOTHING;
        DROP TABLE module_switches;
    END IF;
END $$;

-- Every module the gateway knows about has a row, off until someone switches
-- it on. No module is listed here that has no code behind it.
INSERT INTO ai_module_switches (module, enabled, config, reason)
VALUES ('VEHICLE_DETECTION', FALSE, '{}'::jsonb, 'Off at installation until a DSP or above switches it on')
ON CONFLICT (module) DO NOTHING;

-- --------------------------------------------------------- decisions ---------

-- What the suggestion was about, in which language, and the record text it
-- relied on — the three things the ground rules require on every suggestion.
ALTER TABLE ai_decisions
    ADD COLUMN IF NOT EXISTS module     VARCHAR(40),
    ADD COLUMN IF NOT EXISTS language   VARCHAR(10),
    -- [{"recordType": "...", "recordId": "...", "reference": "...", "excerpt": "..."}]
    ADD COLUMN IF NOT EXISTS sources    JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$
BEGIN
    ALTER TABLE ai_decisions ADD CONSTRAINT ai_decision_language
        CHECK (language IS NULL OR language IN ('en', 'bn', 'mixed', 'unknown'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE ai_decisions ADD CONSTRAINT ai_decision_confidence_range
        CHECK (confidence >= 0 AND confidence <= 1);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- No suggestion was ever auto-approved (nothing has written to this table), but
-- the status must stop existing so nothing can start.
UPDATE ai_decisions SET status = 'PENDING' WHERE status = 'AUTO_APPROVED';

DO $$
BEGIN
    ALTER TABLE ai_decisions ADD CONSTRAINT ai_decision_status
        CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'OVERRIDDEN', 'EXPIRED'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- A decision that is no longer pending was decided by a person, and an
-- overridden one says why. EXPIRED is the exception: nobody decided it, which
-- is the point of recording it.
CREATE OR REPLACE FUNCTION ai_decision_needs_an_officer() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status IN ('APPROVED', 'REJECTED', 'OVERRIDDEN') AND NEW.reviewed_by IS NULL THEN
        RAISE EXCEPTION 'an AI suggestion cannot be % without the officer who decided it', NEW.status
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_needs_an_officer';
    END IF;
    IF NEW.status = 'OVERRIDDEN' AND (NEW.override_reason IS NULL OR btrim(NEW.override_reason) = '') THEN
        RAISE EXCEPTION 'overriding an AI suggestion needs a reason'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_override_reason';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_decision_needs_an_officer ON ai_decisions;
CREATE TRIGGER trg_ai_decision_needs_an_officer
    BEFORE INSERT OR UPDATE ON ai_decisions
    FOR EACH ROW EXECUTE FUNCTION ai_decision_needs_an_officer();

-- A suggestion may only be recorded by a model that is switched on, at a
-- version that has passed its evaluation. The gateway checks this too; the
-- database checks it because the gateway is code and this is a rule.
CREATE OR REPLACE FUNCTION ai_decision_requires_enabled_model() RETURNS TRIGGER AS $$
DECLARE
    v_enabled BOOLEAN;
    v_version VARCHAR(50);
    v_module  VARCHAR(40);
BEGIN
    SELECT is_enabled, model_version, module INTO v_enabled, v_version, v_module
      FROM ai_model_configs WHERE model_name = NEW.model_name;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'model % is not in the registry', NEW.model_name
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_unregistered_model';
    END IF;
    IF NOT v_enabled THEN
        RAISE EXCEPTION 'model % is switched off and cannot produce suggestions', NEW.model_name
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_model_switched_off';
    END IF;
    IF NEW.model_version IS DISTINCT FROM v_version THEN
        RAISE EXCEPTION 'suggestion claims model % version %, but the registry has version %',
            NEW.model_name, NEW.model_version, v_version
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_model_version';
    END IF;
    IF v_module IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM ai_module_switches WHERE module = v_module AND enabled) THEN
        RAISE EXCEPTION 'module % is switched off', v_module
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_module_switched_off';
    END IF;

    NEW.module := COALESCE(NEW.module, v_module);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_decision_requires_enabled_model ON ai_decisions;
CREATE TRIGGER trg_ai_decision_requires_enabled_model
    BEFORE INSERT ON ai_decisions
    FOR EACH ROW EXECUTE FUNCTION ai_decision_requires_enabled_model();

CREATE INDEX IF NOT EXISTS idx_ai_decisions_module ON ai_decisions (module, created_at DESC);

-- ------------------------------------------------- monitoring, measured ------

-- 000019 added ai_performance_metrics and calculate_ai_metrics(), and nothing
-- ever called either: the numbers were a table waiting to be filled by hand.
-- Acceptance and override rates are counted from the decisions themselves, so
-- they cannot disagree with them.
DROP FUNCTION IF EXISTS calculate_ai_metrics(VARCHAR, VARCHAR, TIMESTAMP, TIMESTAMP);
DROP FUNCTION IF EXISTS calculate_ai_metrics(TEXT, TEXT, TIMESTAMP, TIMESTAMP);
DROP TABLE IF EXISTS ai_performance_metrics;

COMMENT ON TABLE ai_model_configs IS
    'The model registry: one row per model the gateway may call. is_enabled requires a passed evaluation of that exact version.';
COMMENT ON TABLE ai_model_evaluations IS
    'Append-only record of held-out measurements. A model is enabled only after one of these passes.';
COMMENT ON COLUMN ai_decisions.sources IS
    'The records and exact text the suggestion relied on: [{recordType, recordId, reference, excerpt}].';
