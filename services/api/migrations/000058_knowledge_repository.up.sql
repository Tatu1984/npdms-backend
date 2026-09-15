-- Phase 11 — Police Knowledge Assistant, functional without AI.
--
-- A document repository (SOPs, circulars, standing orders, manuals, statute
-- text) with versioning by supersession, classification enforced in SQL,
-- keyword search, and procedure checklists officers tick through for a case.
--
-- Search. No Bengali text-search configuration ships with Postgres, so the
-- tsvector uses 'simple' (no stemming; Bengali words are tokenised whole under
-- a UTF-8 locale) and pg_trgm word similarity catches inflected and partial
-- Bengali and mixed-script terms the tsvector cannot. Under a C/POSIX locale
-- non-ASCII letters are not word characters and both would degrade — the edge
-- database must be created with a UTF-8 locale.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS knowledge_documents (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_number     VARCHAR(40)  NOT NULL UNIQUE,
    doc_type            VARCHAR(20)  NOT NULL
                        CHECK (doc_type IN ('SOP', 'CIRCULAR', 'STANDING_ORDER', 'MANUAL', 'STATUTE', 'NOTIFICATION', 'OTHER')),
    title               VARCHAR(500) NOT NULL,
    title_bn            VARCHAR(500),
    description         TEXT NOT NULL DEFAULT '',
    issuing_authority   VARCHAR(255) NOT NULL,
    reference_number    VARCHAR(120),
    issued_on           DATE NOT NULL,
    applicable_to       TEXT[] NOT NULL DEFAULT '{}',

    -- Classification decides who may see the document at all. The rank floor is
    -- derived here, not chosen per row, so the mapping has one definition.
    classification      VARCHAR(20)  NOT NULL DEFAULT 'PUBLIC'
                        CHECK (classification IN ('PUBLIC', 'RESTRICTED', 'CONFIDENTIAL', 'SECRET')),
    min_rank_level      SMALLINT GENERATED ALWAYS AS (
                            CASE classification
                                WHEN 'PUBLIC' THEN 1        -- every officer
                                WHEN 'RESTRICTED' THEN 3    -- ASI and above
                                WHEN 'CONFIDENTIAL' THEN 6  -- SHO and above
                                ELSE 8                      -- SECRET: SP and above
                            END) STORED,

    -- Versioning: a new version supersedes the old, which stays readable.
    version             INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    supersedes_id       UUID UNIQUE REFERENCES knowledge_documents(id),
    status              VARCHAR(12)  NOT NULL DEFAULT 'EFFECTIVE'
                        CHECK (status IN ('EFFECTIVE', 'SUPERSEDED', 'WITHDRAWN')),
    superseded_by_id    UUID UNIQUE REFERENCES knowledge_documents(id),
    superseded_at       TIMESTAMPTZ,

    -- The file, hashed as it streams into storage.
    object_key          TEXT NOT NULL,
    original_filename   VARCHAR(255) NOT NULL,
    content_type        VARCHAR(120) NOT NULL,
    file_size           BIGINT NOT NULL CHECK (file_size > 0),
    sha256              CHAR(64) NOT NULL,

    -- What text, if any, could be read out of the file, and how.
    text_content        TEXT NOT NULL DEFAULT '',
    extraction_status   VARCHAR(20)  NOT NULL
                        CHECK (extraction_status IN ('TEXT_LAYER', 'PLAIN_TEXT', 'NO_TEXT_LAYER', 'OCR_UNAVAILABLE', 'UNSUPPORTED', 'FAILED')),
    extraction_note     TEXT NOT NULL DEFAULT '',

    search_vector       TSVECTOR GENERATED ALWAYS AS (
                            setweight(to_tsvector('simple'::regconfig, coalesce(title, '') || ' ' || coalesce(title_bn, '') || ' ' || coalesce(reference_number, '')), 'A') ||
                            setweight(to_tsvector('simple'::regconfig, coalesce(description, '') || ' ' || coalesce(issuing_authority, '')), 'B') ||
                            setweight(to_tsvector('simple'::regconfig, coalesce(text_content, '')), 'C')
                        ) STORED,

    uploaded_by         UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT knowledge_superseded_complete CHECK (
        (status = 'SUPERSEDED') = (superseded_by_id IS NOT NULL)
        AND (superseded_by_id IS NULL) = (superseded_at IS NULL)
    ),
    CONSTRAINT knowledge_version_chain CHECK ((version = 1) = (supersedes_id IS NULL)),
    CONSTRAINT knowledge_not_self_superseding CHECK (supersedes_id <> id AND superseded_by_id <> id)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_search ON knowledge_documents USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_knowledge_title_trgm ON knowledge_documents
    USING GIN ((coalesce(title, '') || ' ' || coalesce(title_bn, '') || ' ' || coalesce(description, '')) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_knowledge_text_trgm ON knowledge_documents USING GIN (text_content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_knowledge_visibility ON knowledge_documents (min_rank_level, status, doc_type);

-- ------------------------------------------------------------- checklists --

CREATE TABLE IF NOT EXISTS knowledge_checklists (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id     UUID NOT NULL REFERENCES knowledge_documents(id),
    section_ref     VARCHAR(120) NOT NULL,
    title           VARCHAR(255) NOT NULL,
    title_bn        VARCHAR(255),
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS knowledge_checklist_steps (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    checklist_id    UUID NOT NULL REFERENCES knowledge_checklists(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL CHECK (position >= 1),
    text            TEXT NOT NULL CHECK (length(trim(text)) > 0),
    text_bn         TEXT,
    UNIQUE (checklist_id, position)
);

-- A run is one use of a checklist for one case or FIR.
CREATE TABLE IF NOT EXISTS knowledge_checklist_runs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    checklist_id    UUID NOT NULL REFERENCES knowledge_checklists(id),
    case_id         UUID REFERENCES cases(id),
    fir_id          UUID REFERENCES firs(id),
    started_by      UUID NOT NULL REFERENCES users(id),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT checklist_run_subject CHECK (case_id IS NOT NULL OR fir_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_checklist_runs_case ON knowledge_checklist_runs (case_id);
CREATE INDEX IF NOT EXISTS idx_checklist_runs_fir ON knowledge_checklist_runs (fir_id);

-- A tick is a record of who confirmed a step and when. It is never edited or
-- removed; a step done in error is noted, not erased.
CREATE TABLE IF NOT EXISTS knowledge_checklist_ticks (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id          UUID NOT NULL REFERENCES knowledge_checklist_runs(id),
    step_id         UUID NOT NULL REFERENCES knowledge_checklist_steps(id),
    ticked_by       UUID NOT NULL REFERENCES users(id),
    ticked_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note            TEXT NOT NULL DEFAULT '',
    UNIQUE (run_id, step_id)
);

CREATE OR REPLACE FUNCTION knowledge_ticks_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'checklist ticks are append-only';
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'knowledge_ticks_no_update') THEN
        CREATE TRIGGER knowledge_ticks_no_update BEFORE UPDATE OR DELETE ON knowledge_checklist_ticks
            FOR EACH ROW EXECUTE FUNCTION knowledge_ticks_append_only();
    END IF;
END $$;
