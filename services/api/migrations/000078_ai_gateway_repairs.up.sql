-- Repairs to the A0 gateway found by reviewing 000076 against the live schema.
--
-- Idempotent and safe to apply twice, like the rest of the chain.

-- --------------------------------------------- suggestions and stations ------

-- ai_decisions.station_id referenced police_stations, which is empty and has
-- three other references; the platform's station table is `stations`, which
-- holds the seventeen Kolkata stations and is what users.station_id points at.
-- So any suggestion that named the officer's station was refused by the
-- foreign key, and acceptance rates per station — which the plan asks for —
-- could never have a row. Only suggestions with no station could exist at all.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'ai_decisions_station_id_fkey'
           AND conrelid = 'ai_decisions'::regclass
           AND confrelid = 'police_stations'::regclass) THEN

        -- Nothing can be pointing at police_stations: it has no rows.
        ALTER TABLE ai_decisions DROP CONSTRAINT ai_decisions_station_id_fkey;
        ALTER TABLE ai_decisions
            ADD CONSTRAINT ai_decisions_station_id_fkey
            FOREIGN KEY (station_id) REFERENCES stations(id);
    END IF;
END $$;

-- ------------------------------------------- which model was measured --------

-- ai_model_is_measured() asked whether the pair (name, version) had ever
-- passed. Evaluations are append-only and had no link to the registry, so a
-- pass outlived the entry it measured: remove a model and register the same
-- name and version again — a different model entirely — and it reported itself
-- as measured, ready to be switched on without anybody measuring it.
--
-- Each registration now carries its own identifier, and an evaluation belongs
-- to the registration that was in the table when it was recorded.
ALTER TABLE ai_model_configs
    ADD COLUMN IF NOT EXISTS registration_id UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE ai_model_evaluations
    ADD COLUMN IF NOT EXISTS registration_id UUID;

-- Existing measurements belong to the registration that is there now.
UPDATE ai_model_evaluations e
   SET registration_id = m.registration_id
  FROM ai_model_configs m
 WHERE e.registration_id IS NULL
   AND e.model_name = m.model_name;

CREATE INDEX IF NOT EXISTS idx_ai_eval_registration
    ON ai_model_evaluations (registration_id, model_version) WHERE passed;

-- An evaluation is stamped with the registration it measured, by the database,
-- so a caller cannot attribute one to a different model.
CREATE OR REPLACE FUNCTION ai_evaluation_belongs_to_a_registration() RETURNS TRIGGER AS $$
DECLARE
    v_registration UUID;
BEGIN
    SELECT registration_id INTO v_registration
      FROM ai_model_configs WHERE model_name = NEW.model_name;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'model % is not in the registry, so it cannot be evaluated', NEW.model_name
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_evaluation_unregistered_model';
    END IF;

    NEW.registration_id := v_registration;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_evaluation_registration ON ai_model_evaluations;
CREATE TRIGGER trg_ai_evaluation_registration
    BEFORE INSERT ON ai_model_evaluations
    FOR EACH ROW EXECUTE FUNCTION ai_evaluation_belongs_to_a_registration();

-- Measured now means: this registration, at this version, has passed.
DROP FUNCTION IF EXISTS ai_model_is_measured(TEXT, TEXT);

CREATE OR REPLACE FUNCTION ai_model_is_measured(p_registration UUID, p_version TEXT) RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM ai_model_evaluations
         WHERE registration_id = p_registration AND model_version = p_version AND passed
    )
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION ai_model_enable_requires_evaluation() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_enabled THEN
        IF NEW.model_version IS NULL OR btrim(NEW.model_version) = '' THEN
            RAISE EXCEPTION 'model % cannot be enabled without a version', NEW.model_name
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_version_required';
        END IF;
        IF NOT ai_model_is_measured(NEW.registration_id, NEW.model_version) THEN
            RAISE EXCEPTION 'model % version % has not passed an evaluation and cannot be enabled',
                NEW.model_name, NEW.model_version
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_requires_evaluation';
        END IF;
        IF NEW.retired_at IS NOT NULL THEN
            RAISE EXCEPTION 'model % is retired and cannot be enabled', NEW.model_name
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_retired';
        END IF;

        -- What was measured is a model at an address. Repointing a switched-on
        -- model at a different service would keep the measurement and change
        -- the thing being called, so the address moves only while it is off.
        IF TG_OP = 'UPDATE' AND NEW.endpoint_env IS DISTINCT FROM OLD.endpoint_env
           AND OLD.is_enabled THEN
            RAISE EXCEPTION 'switch model % off before changing the service it calls', NEW.model_name
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_model_address_while_enabled';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_model_enable_requires_evaluation ON ai_model_configs;
CREATE TRIGGER trg_ai_model_enable_requires_evaluation
    BEFORE INSERT OR UPDATE ON ai_model_configs
    FOR EACH ROW EXECUTE FUNCTION ai_model_enable_requires_evaluation();

-- ------------------------------------------------ a suggestion is fixed ------

-- 000076 checked a suggestion's model, version and module when it was written
-- and never again, so the provenance could be rewritten afterwards: an
-- approved suggestion could be made to assert something the approving officer
-- never saw. What the model said is now settled at the moment it is recorded.
CREATE OR REPLACE FUNCTION ai_decision_provenance_is_fixed() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.model_name IS DISTINCT FROM OLD.model_name
       OR NEW.model_version IS DISTINCT FROM OLD.model_version
       OR NEW.prediction IS DISTINCT FROM OLD.prediction
       OR NEW.prediction_data IS DISTINCT FROM OLD.prediction_data
       OR NEW.confidence IS DISTINCT FROM OLD.confidence
       OR NEW.confidence_threshold IS DISTINCT FROM OLD.confidence_threshold
       OR NEW.sources IS DISTINCT FROM OLD.sources
       OR NEW.module IS DISTINCT FROM OLD.module
       OR NEW.source_type IS DISTINCT FROM OLD.source_type
       OR NEW.source_id IS DISTINCT FROM OLD.source_id
       OR NEW.requested_by IS DISTINCT FROM OLD.requested_by THEN
        RAISE EXCEPTION 'what a model suggested, and what it relied on, cannot be changed afterwards'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_provenance_fixed';
    END IF;

    -- A review is not undone by writing PENDING over it.
    IF OLD.status IN ('APPROVED', 'REJECTED', 'OVERRIDDEN') AND NEW.status = 'PENDING' THEN
        RAISE EXCEPTION 'a suggestion that an officer has decided cannot be returned to the queue'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_decision_review_stands';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_decision_provenance_is_fixed ON ai_decisions;
CREATE TRIGGER trg_ai_decision_provenance_is_fixed
    BEFORE UPDATE ON ai_decisions
    FOR EACH ROW EXECUTE FUNCTION ai_decision_provenance_is_fixed();

-- ------------------------------------------------------ append-only, fully ---

-- A row trigger does not fire on TRUNCATE, so the whole measurement history
-- could be emptied in one statement without the append-only rule noticing.
CREATE OR REPLACE FUNCTION ai_evaluations_refuse_truncate() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ai_model_evaluations is append-only: it is not truncated'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'ai_evaluations_append_only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_evaluations_refuse_truncate ON ai_model_evaluations;
CREATE TRIGGER trg_ai_evaluations_refuse_truncate
    BEFORE TRUNCATE ON ai_model_evaluations
    FOR EACH STATEMENT EXECUTE FUNCTION ai_evaluations_refuse_truncate();

-- ------------------------------------------------------ times with a zone ---

-- 000019 made every time column `TIMESTAMP` — no zone. The API writes local
-- time into them and serialises it with a trailing Z, so a suggestion recorded
-- at 08:58 in Kolkata was read back as 08:58 UTC and shown as 2:28 pm: five and
-- a half hours in the future. An overdue suggestion would not have been flagged
-- until it was five and a half hours late.
--
-- Existing values are read as Kolkata wall-clock time, which is what wrote
-- them. All four tables are empty on both the edge database and Neon at the
-- time of this migration, so nothing is being reinterpreted in practice.
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT table_name, column_name
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name IN ('ai_decisions', 'ai_decision_feedback',
                              'ai_review_assignments', 'ai_model_configs')
           AND data_type = 'timestamp without time zone'
    LOOP
        EXECUTE format(
            'ALTER TABLE %I ALTER COLUMN %I TYPE TIMESTAMPTZ USING %I AT TIME ZONE ''Asia/Kolkata''',
            r.table_name, r.column_name, r.column_name);
    END LOOP;
END $$;

COMMENT ON COLUMN ai_model_configs.registration_id IS
    'Identifies this registration. An evaluation measures a registration, not a name, so a pass does not survive the entry it was recorded against.';
