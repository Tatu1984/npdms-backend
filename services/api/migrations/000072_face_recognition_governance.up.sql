-- Face recognition for missing persons — authorisation, module switch,
-- synthetic demo photos and face enrolments.
--
-- Face recognition is a biometric use. The rules are held here as well as in
-- the service, so a bug or a direct write cannot get around them:
--   * FR can be switched on only while an authorisation is active (not revoked,
--     within its validity dates) and its scope covers MISSING_PERSONS;
--   * an authorisation is either an ORDER (reference, date and issuing authority
--     all required) or DEMO, which covers only photos flagged as synthetic test
--     images uploaded through the admin demo path;
--   * a real report photo can be enrolled only under an ORDER, a synthetic test
--     photo only under DEMO, and the two never mix;
--   * an embedding exists only while its enrolment is ENROLLED — retiring or
--     rejecting an enrolment leaves no biometric template behind.
--
-- Embeddings are REAL[] (float4, 128 dimensions for SFace) compared by brute
-- force cosine similarity in the face recognition service. pgvector is not
-- used: it is not installed on the edge server's Postgres, and the gallery
-- (open missing-person reports only) is small enough that an index would not
-- matter. Idempotent: the bootstrap runs every migration twice.

-- Photos come from missing_person_photos (migration 000070). Synthetic test
-- photos for the DEMO path are kept apart in fr_synthetic_photos below.

-- ------------------------------------------------------------ authorisations --

CREATE TABLE IF NOT EXISTS fr_authorisations (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind               VARCHAR(8) NOT NULL CHECK (kind IN ('ORDER', 'DEMO')),
    order_reference    VARCHAR(120),
    issuing_authority  VARCHAR(255),
    order_date         DATE,
    -- What the order permits FR to be used for. Only MISSING_PERSONS is
    -- implemented; any other scope must be named explicitly in an order.
    scope              TEXT[] NOT NULL,
    scope_note         TEXT,
    valid_from         DATE NOT NULL DEFAULT CURRENT_DATE,
    valid_until        DATE NOT NULL,
    recorded_by        UUID NOT NULL REFERENCES users(id),
    recorded_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at         TIMESTAMPTZ,
    revoked_by         UUID REFERENCES users(id),
    revocation_reason  TEXT,
    CONSTRAINT fr_auth_scope_named CHECK (cardinality(scope) > 0 AND array_to_string(scope, ',') ~ '^[A-Z_]+(,[A-Z_]+)*$'),
    CONSTRAINT fr_auth_order_complete CHECK (
        kind = 'DEMO' OR (
            order_reference IS NOT NULL AND btrim(order_reference) <> '' AND
            issuing_authority IS NOT NULL AND btrim(issuing_authority) <> '' AND
            order_date IS NOT NULL)),
    CONSTRAINT fr_auth_validity CHECK (valid_until >= valid_from),
    CONSTRAINT fr_auth_order_before_validity CHECK (order_date IS NULL OR order_date <= valid_until),
    CONSTRAINT fr_auth_revocation_complete CHECK (
        (revoked_at IS NULL) = (revoked_by IS NULL) AND
        (revoked_at IS NULL OR (revocation_reason IS NOT NULL AND btrim(revocation_reason) <> '')))
);
CREATE INDEX IF NOT EXISTS idx_fr_auth_active ON fr_authorisations (kind, valid_until) WHERE revoked_at IS NULL;

-- Is there an authorisation of this kind (NULL = any) active now for the scope?
CREATE OR REPLACE FUNCTION fr_active_authorisation(p_kind TEXT, p_scope TEXT) RETURNS UUID AS $$
    SELECT id FROM fr_authorisations
     WHERE revoked_at IS NULL
       AND (p_kind IS NULL OR kind = p_kind)
       AND p_scope = ANY(scope)
       AND CURRENT_DATE BETWEEN valid_from AND valid_until
     ORDER BY (kind = 'ORDER') DESC, recorded_at DESC
     LIMIT 1
$$ LANGUAGE sql STABLE;

-- -------------------------------------------------------------- module switch --

-- One row per AI module. Face recognition is the first; the A0 gateway can add
-- rows for the others. Thresholds live in config and are shown on every result.
CREATE TABLE IF NOT EXISTS ai_module_switches (
    module      VARCHAR(40) PRIMARY KEY,
    enabled     BOOLEAN NOT NULL DEFAULT FALSE,
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason      TEXT,
    updated_by  UUID REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO ai_module_switches (module, enabled, config, reason)
VALUES ('FACE_RECOGNITION', FALSE,
        '{"matchThreshold": 0.50, "sampleFps": 1.0, "mergeWindowSeconds": 5}'::jsonb,
        'Off until an authorisation is recorded')
ON CONFLICT (module) DO NOTHING;

CREATE OR REPLACE FUNCTION fr_switch_requires_authorisation() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.module = 'FACE_RECOGNITION' AND NEW.enabled
       AND fr_active_authorisation(NULL, 'MISSING_PERSONS') IS NULL THEN
        RAISE EXCEPTION 'face recognition cannot be switched on without an active authorisation covering MISSING_PERSONS'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fr_switch_requires_authorisation';
    END IF;
    IF NEW.module = 'FACE_RECOGNITION' THEN
        IF NOT (NEW.config ? 'matchThreshold')
           OR (NEW.config->>'matchThreshold')::numeric <= 0 OR (NEW.config->>'matchThreshold')::numeric > 1 THEN
            RAISE EXCEPTION 'face recognition matchThreshold must be in (0, 1]'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'fr_threshold_range';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fr_switch_requires_authorisation ON ai_module_switches;
CREATE TRIGGER trg_fr_switch_requires_authorisation
    BEFORE INSERT OR UPDATE ON ai_module_switches
    FOR EACH ROW EXECUTE FUNCTION fr_switch_requires_authorisation();

-- ------------------------------------------------------- synthetic demo photos --

-- Photos explicitly declared to be synthetic test faces (generated, not of any
-- real person), uploaded by an administrator through the demo path. They are
-- attached to a report only so the demo exercises the real workflow, and are
-- the only photos a DEMO authorisation can enrol.
CREATE TABLE IF NOT EXISTS fr_synthetic_photos (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id           UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    object_key          TEXT NOT NULL,
    sha256              CHAR(64) NOT NULL,
    content_type        VARCHAR(100) NOT NULL,
    width               INTEGER,
    height              INTEGER,
    -- Where the synthetic image came from and under what licence.
    synthetic_source    TEXT NOT NULL CHECK (btrim(synthetic_source) <> ''),
    declared_synthetic  BOOLEAN NOT NULL CHECK (declared_synthetic),
    uploaded_by         UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_fr_synthetic_photos_report ON fr_synthetic_photos (report_id) WHERE retired_at IS NULL;

-- ------------------------------------------------------------------ enrolments --

CREATE TABLE IF NOT EXISTS face_enrolments (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id           UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    photo_id            UUID REFERENCES missing_person_photos(id) ON DELETE CASCADE,
    synthetic_photo_id  UUID REFERENCES fr_synthetic_photos(id) ON DELETE CASCADE,
    is_demo             BOOLEAN GENERATED ALWAYS AS (synthetic_photo_id IS NOT NULL) STORED,
    authorisation_id    UUID NOT NULL REFERENCES fr_authorisations(id),
    status              VARCHAR(10) NOT NULL CHECK (status IN ('ENROLLED', 'REJECTED', 'RETIRED')),
    rejection_reason    VARCHAR(40),
    rejection_message   TEXT,
    quality             JSONB,
    quality_score       REAL,
    face_box            JSONB,
    embedding           REAL[],
    embedding_dim       SMALLINT,
    model_version       VARCHAR(80) NOT NULL,
    photo_sha256        CHAR(64) NOT NULL,
    face_crop_key       TEXT,
    automatic           BOOLEAN NOT NULL DEFAULT FALSE,
    enrolled_by         UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at          TIMESTAMPTZ,
    retired_by          UUID REFERENCES users(id),
    retire_reason       TEXT,
    CONSTRAINT fe_one_photo CHECK ((photo_id IS NULL) <> (synthetic_photo_id IS NULL)),
    CONSTRAINT fe_embedding_only_when_enrolled CHECK (
        (status = 'ENROLLED') = (embedding IS NOT NULL) AND
        (embedding IS NULL OR (embedding_dim IS NOT NULL AND array_length(embedding, 1) = embedding_dim))),
    CONSTRAINT fe_rejection_explained CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL),
    CONSTRAINT fe_retirement_complete CHECK ((status = 'RETIRED') = (retired_at IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fe_photo_enrolled
    ON face_enrolments (photo_id, model_version) WHERE status = 'ENROLLED' AND photo_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_fe_synthetic_enrolled
    ON face_enrolments (synthetic_photo_id, model_version) WHERE status = 'ENROLLED' AND synthetic_photo_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fe_report ON face_enrolments (report_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fe_gallery ON face_enrolments (is_demo, model_version) WHERE status = 'ENROLLED';

-- A real photo needs an active ORDER; a synthetic photo needs an active DEMO.
CREATE OR REPLACE FUNCTION fe_authorisation_matches_photo() RETURNS TRIGGER AS $$
DECLARE
    auth fr_authorisations%ROWTYPE;
BEGIN
    SELECT * INTO auth FROM fr_authorisations WHERE id = NEW.authorisation_id;
    IF auth.revoked_at IS NOT NULL OR CURRENT_DATE NOT BETWEEN auth.valid_from AND auth.valid_until
       OR NOT ('MISSING_PERSONS' = ANY(auth.scope)) THEN
        RAISE EXCEPTION 'the authorisation is not active for missing persons'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fe_authorisation_active';
    END IF;
    IF NEW.photo_id IS NOT NULL AND auth.kind <> 'ORDER' THEN
        RAISE EXCEPTION 'a real report photo can only be enrolled under an ORDER authorisation, not %', auth.kind
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fe_real_photo_needs_order';
    END IF;
    IF NEW.synthetic_photo_id IS NOT NULL AND auth.kind <> 'DEMO' THEN
        RAISE EXCEPTION 'a synthetic test photo is enrolled only under a DEMO authorisation'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fe_synthetic_photo_needs_demo';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fe_authorisation_matches_photo ON face_enrolments;
CREATE TRIGGER trg_fe_authorisation_matches_photo
    BEFORE INSERT ON face_enrolments
    FOR EACH ROW EXECUTE FUNCTION fe_authorisation_matches_photo();
