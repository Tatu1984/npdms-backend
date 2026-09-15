-- Phase 09 — citizen complaint register.
--
-- `citizen_complaints` and `complaint_updates` already existed but carried
-- only a public web submission: no record of the channel a complaint arrived
-- through, who entered it, how it was routed, which complaints duplicate one
-- another, or any response approval. The officer screen ran on fixture data.
--
-- This migration extends the existing register rather than creating a second
-- one. It is written to be safe on a database that already holds complaints.

-- ---------------------------------------------------------------- complaint --

ALTER TABLE citizen_complaints
    ADD COLUMN IF NOT EXISTS channel              VARCHAR(20) NOT NULL DEFAULT 'WEB',
    ADD COLUMN IF NOT EXISTS source_reference     VARCHAR(200),
    ADD COLUMN IF NOT EXISTS recorded_by          UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS text_script          VARCHAR(10) NOT NULL DEFAULT 'LATIN',
    ADD COLUMN IF NOT EXISTS priority             VARCHAR(10) NOT NULL DEFAULT 'NORMAL',
    ADD COLUMN IF NOT EXISTS categorised_by       UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS categorised_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS duplicate_of         UUID REFERENCES citizen_complaints(id),
    ADD COLUMN IF NOT EXISTS duplicate_linked_by  UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS duplicate_linked_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS duplicate_note       TEXT,
    -- Anonymous complainants have no phone to prove ownership when tracking,
    -- so they are given an access code once; only its SHA-256 is stored.
    ADD COLUMN IF NOT EXISTS access_code_hash     VARCHAR(64);

UPDATE citizen_complaints SET status = 'SUBMITTED' WHERE status IS NULL;
ALTER TABLE citizen_complaints ALTER COLUMN status SET NOT NULL;

-- Last ten digits of the phone, for the duplicate rule and the tracking check.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'citizen_complaints' AND column_name = 'phone_normalized') THEN
        ALTER TABLE citizen_complaints ADD COLUMN phone_normalized VARCHAR(10)
            GENERATED ALWAYS AS (NULLIF(right(regexp_replace(COALESCE(complainant_phone, ''), '\D', '', 'g'), 10), '')) STORED;
    END IF;

    -- Full-text search over Bengali, English and mixed text. The 'simple'
    -- configuration splits on whitespace and punctuation without stemming, so
    -- বাংলা words are indexed whole, as they are written.
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'citizen_complaints' AND column_name = 'search_vector') THEN
        ALTER TABLE citizen_complaints ADD COLUMN search_vector tsvector
            GENERATED ALWAYS AS (to_tsvector('simple',
                COALESCE(tracking_number, '') || ' ' || COALESCE(subject, '') || ' ' ||
                COALESCE(description, '') || ' ' || COALESCE(incident_location, ''))) STORED;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_citizen_complaints_search ON citizen_complaints USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_citizen_complaints_phone ON citizen_complaints (phone_normalized, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_citizen_complaints_status ON citizen_complaints (status, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_citizen_complaints_station ON citizen_complaints (station_id, status);
CREATE INDEX IF NOT EXISTS idx_citizen_complaints_duplicate ON citizen_complaints (duplicate_of);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_channel_valid') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_channel_valid
            CHECK (channel IN ('WEB', 'MOBILE', 'WHATSAPP', 'EMAIL', 'CALL_CENTRE', 'COUNTER'));
    END IF;
    -- Only the web portal receives complaints directly. The counter is an
    -- officer taking a complaint in person; every other channel is an officer
    -- entering what arrived elsewhere, with the reference it arrived under.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_channel_provenance') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_channel_provenance CHECK (
            channel = 'WEB'
            OR (channel = 'COUNTER' AND recorded_by IS NOT NULL)
            OR (recorded_by IS NOT NULL AND source_reference IS NOT NULL AND btrim(source_reference) <> '')
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_script_valid') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_script_valid
            CHECK (text_script IN ('LATIN', 'BENGALI', 'MIXED'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_priority_valid') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_priority_valid
            CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_contact_or_code') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_contact_or_code CHECK (
            (NOT is_anonymous AND complainant_phone IS NOT NULL)
            OR (is_anonymous AND access_code_hash IS NOT NULL)
        ) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_rejection_reason') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_rejection_reason
            CHECK (status <> 'REJECTED' OR (rejection_reason IS NOT NULL AND btrim(rejection_reason) <> ''));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_duplicate_complete') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_duplicate_complete CHECK (
            (duplicate_of IS NULL AND duplicate_linked_by IS NULL AND duplicate_linked_at IS NULL)
            OR (duplicate_of IS NOT NULL AND duplicate_of <> id
                AND duplicate_linked_by IS NOT NULL AND duplicate_linked_at IS NOT NULL)
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'complaint_text_bounds') THEN
        ALTER TABLE citizen_complaints ADD CONSTRAINT complaint_text_bounds CHECK (
            char_length(btrim(subject)) BETWEEN 3 AND 200
            AND char_length(btrim(description)) BETWEEN 10 AND 5000
        ) NOT VALID;
    END IF;
END $$;

-- ------------------------------------------------------------------ routing --

-- Every change of jurisdiction, with its reason. The complaint row holds the
-- current station; this table holds how it got there.
CREATE TABLE IF NOT EXISTS complaint_routings (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id  UUID NOT NULL REFERENCES citizen_complaints(id),
    from_station  UUID REFERENCES stations(id),
    to_station    UUID NOT NULL REFERENCES stations(id),
    to_unit       VARCHAR(120),
    reason        TEXT NOT NULL CHECK (char_length(btrim(reason)) >= 5),
    routed_by     UUID NOT NULL REFERENCES users(id),
    routed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_complaint_routings_complaint ON complaint_routings (complaint_id, routed_at);

-- ---------------------------------------------------------------- responses --

-- A response to the citizen is drafted by one officer and approved by
-- another of rank SI or above before the citizen can see it.
CREATE TABLE IF NOT EXISTS complaint_responses (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    complaint_id  UUID NOT NULL REFERENCES citizen_complaints(id),
    body          TEXT NOT NULL CHECK (char_length(btrim(body)) BETWEEN 10 AND 4000),
    status        VARCHAR(10) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'APPROVED', 'REJECTED')),
    drafted_by    UUID NOT NULL REFERENCES users(id),
    drafted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_by   UUID REFERENCES users(id),
    reviewed_at   TIMESTAMPTZ,
    review_note   TEXT,
    CONSTRAINT response_review_complete CHECK (
        (status = 'DRAFT' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR (status <> 'DRAFT' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    ),
    CONSTRAINT response_reviewed_independently CHECK (reviewed_by IS NULL OR reviewed_by <> drafted_by),
    CONSTRAINT response_rejection_note CHECK (
        status <> 'REJECTED' OR (review_note IS NOT NULL AND btrim(review_note) <> '')
    )
);
CREATE INDEX IF NOT EXISTS idx_complaint_responses_complaint ON complaint_responses (complaint_id, drafted_at);

-- ----------------------------------------------------------- history rules --

-- Routing history and status history are records of what happened; a
-- reviewed response is what the citizen was told. None of them change.
CREATE OR REPLACE FUNCTION forbid_complaint_history_change() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION forbid_reviewed_response_change() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'complaint responses cannot be deleted' USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF OLD.status <> 'DRAFT' THEN
        RAISE EXCEPTION 'a reviewed response cannot be changed' USING ERRCODE = 'insufficient_privilege';
    END IF;
    IF NEW.body IS DISTINCT FROM OLD.body OR NEW.drafted_by IS DISTINCT FROM OLD.drafted_by
       OR NEW.complaint_id IS DISTINCT FROM OLD.complaint_id THEN
        RAISE EXCEPTION 'a response is reviewed as drafted; draft a new one instead' USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['complaint_routings', 'complaint_updates'] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = t || '_append_only') THEN
            EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION forbid_complaint_history_change()',
                           t || '_append_only', t);
        END IF;
    END LOOP;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'complaint_responses_reviewed_immutable') THEN
        CREATE TRIGGER complaint_responses_reviewed_immutable
            BEFORE UPDATE OR DELETE ON complaint_responses
            FOR EACH ROW EXECUTE FUNCTION forbid_reviewed_response_change();
    END IF;
END $$;
