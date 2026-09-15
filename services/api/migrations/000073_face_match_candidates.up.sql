-- Face recognition for missing persons — searches and match candidates.
--
--   * Every search (uploaded footage, uploaded still, camera snapshot) is a row
--     with its stated purpose, who submitted it, the SHA-256 of the media, the
--     model version and the threshold in force. Media that produced no
--     candidate is not kept; its hash is.
--   * A candidate is a lead, never a finding. It is PENDING until an officer
--     confirms or rejects it. Confirming creates a VERIFIED sighting through the
--     Phase 04 sighting table, recorded as reported by the submitter and
--     verified by the confirmer, who must be a different officer.
--   * The evidence behind a candidate — similarity, model version, threshold,
--     source frame and its hash — cannot be changed after it is written.
-- Idempotent: the bootstrap runs every migration twice.

CREATE TABLE IF NOT EXISTS face_match_searches (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    is_demo             BOOLEAN NOT NULL,
    authorisation_id    UUID NOT NULL REFERENCES fr_authorisations(id),
    source_media        VARCHAR(20) NOT NULL CHECK (source_media IN ('UPLOADED_FOOTAGE', 'UPLOADED_STILL', 'CAMERA_SNAPSHOT')),
    camera_id           UUID REFERENCES cameras(id),
    purpose             TEXT NOT NULL CHECK (char_length(btrim(purpose)) >= 10),
    original_filename   TEXT,
    content_type        VARCHAR(100),
    size_bytes          BIGINT,
    media_sha256        CHAR(64) NOT NULL,
    -- Kept only when the search produced at least one candidate.
    media_key           TEXT,
    -- When the footage starts / the still or snapshot was taken.
    recorded_at         TIMESTAMPTZ NOT NULL,
    location_text       TEXT,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    threshold_used      REAL NOT NULL CHECK (threshold_used > 0 AND threshold_used <= 1),
    model_version       VARCHAR(80),
    sample_fps          REAL,
    gallery_size        INTEGER,
    frames_analysed     INTEGER,
    faces_seen          INTEGER,
    faces_compared      INTEGER,
    candidates_created  INTEGER NOT NULL DEFAULT 0,
    status              VARCHAR(10) NOT NULL CHECK (status IN ('RUNNING', 'COMPLETED', 'FAILED')),
    error               TEXT,
    duration_ms         INTEGER,
    submitted_by        UUID NOT NULL REFERENCES users(id),
    submitted_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    client_ip           VARCHAR(64),
    CONSTRAINT fms_camera_for_snapshot CHECK (source_media <> 'CAMERA_SNAPSHOT' OR camera_id IS NOT NULL),
    CONSTRAINT fms_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CONSTRAINT fms_failure_explained CHECK (status <> 'FAILED' OR error IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_fms_submitted ON face_match_searches (submitted_at DESC);

CREATE TABLE IF NOT EXISTS face_match_candidates (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    search_id        UUID NOT NULL REFERENCES face_match_searches(id),
    report_id        UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    enrolment_id     UUID NOT NULL REFERENCES face_enrolments(id),
    -- The enrolled photo the face matched: a report photo, or under DEMO a
    -- synthetic test photo.
    photo_id            UUID REFERENCES missing_person_photos(id) ON DELETE SET NULL,
    synthetic_photo_id  UUID REFERENCES fr_synthetic_photos(id) ON DELETE SET NULL,
    is_demo          BOOLEAN NOT NULL,
    camera_id        UUID REFERENCES cameras(id),
    source_media     VARCHAR(20) NOT NULL CHECK (source_media IN ('UPLOADED_FOOTAGE', 'UPLOADED_STILL', 'CAMERA_SNAPSHOT')),
    media_key        TEXT NOT NULL,          -- the source frame as stored
    media_sha256     CHAR(64) NOT NULL,      -- SHA-256 of that frame
    crop_key         TEXT NOT NULL,          -- the face crop shown beside the enrolled photo
    frame_offset_ms  INTEGER,                -- into the footage; NULL for a still
    frame_time       TIMESTAMPTZ NOT NULL,
    location_text    TEXT,
    latitude         DOUBLE PRECISION,
    longitude        DOUBLE PRECISION,
    bbox_x           INTEGER NOT NULL,
    bbox_y           INTEGER NOT NULL,
    bbox_w           INTEGER NOT NULL CHECK (bbox_w > 0),
    bbox_h           INTEGER NOT NULL CHECK (bbox_h > 0),
    frame_width      INTEGER,
    frame_height     INTEGER,
    detection_score  REAL,
    quality          JSONB,
    similarity       REAL NOT NULL CHECK (similarity >= 0 AND similarity <= 1),
    model_version    VARCHAR(80) NOT NULL,
    threshold_used   REAL NOT NULL CHECK (threshold_used > 0 AND threshold_used <= 1),
    status           VARCHAR(10) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'CONFIRMED', 'REJECTED')),
    submitted_by     UUID NOT NULL REFERENCES users(id),
    reviewed_by      UUID REFERENCES users(id),
    reviewed_at      TIMESTAMPTZ,
    review_note      TEXT,
    sighting_id      UUID REFERENCES missing_person_sightings(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fmc_one_photo CHECK (NOT (photo_id IS NOT NULL AND synthetic_photo_id IS NOT NULL)),
    CONSTRAINT fmc_above_threshold CHECK (similarity >= threshold_used),
    CONSTRAINT fmc_review_complete CHECK (
        (status = 'PENDING') = (reviewed_by IS NULL) AND (reviewed_by IS NULL) = (reviewed_at IS NULL)),
    CONSTRAINT fmc_reviewed_independently CHECK (reviewed_by IS NULL OR reviewed_by <> submitted_by),
    CONSTRAINT fmc_rejection_explained CHECK (status <> 'REJECTED' OR (review_note IS NOT NULL AND btrim(review_note) <> '')),
    CONSTRAINT fmc_confirmation_creates_sighting CHECK ((status = 'CONFIRMED') = (sighting_id IS NOT NULL)),
    CONSTRAINT fmc_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_fmc_queue ON face_match_candidates (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fmc_report ON face_match_candidates (report_id, frame_time DESC);
CREATE INDEX IF NOT EXISTS idx_fmc_search ON face_match_candidates (search_id);

-- The evidence behind a candidate is fixed once written; only the review
-- fields change, and only once (PENDING to CONFIRMED or REJECTED).
CREATE OR REPLACE FUNCTION fmc_evidence_is_fixed() RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.search_id, NEW.report_id, NEW.enrolment_id, NEW.is_demo, NEW.source_media, NEW.media_key,
        NEW.media_sha256, NEW.crop_key, NEW.frame_offset_ms, NEW.frame_time, NEW.bbox_x, NEW.bbox_y,
        NEW.bbox_w, NEW.bbox_h, NEW.similarity, NEW.model_version, NEW.threshold_used, NEW.submitted_by)
       IS DISTINCT FROM
       (OLD.search_id, OLD.report_id, OLD.enrolment_id, OLD.is_demo, OLD.source_media, OLD.media_key,
        OLD.media_sha256, OLD.crop_key, OLD.frame_offset_ms, OLD.frame_time, OLD.bbox_x, OLD.bbox_y,
        OLD.bbox_w, OLD.bbox_h, OLD.similarity, OLD.model_version, OLD.threshold_used, OLD.submitted_by) THEN
        RAISE EXCEPTION 'the evidence behind a face match candidate cannot be changed'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fmc_evidence_is_fixed';
    END IF;
    IF OLD.status <> 'PENDING' AND (NEW.status, NEW.reviewed_by, NEW.reviewed_at, NEW.review_note, NEW.sighting_id)
       IS DISTINCT FROM (OLD.status, OLD.reviewed_by, OLD.reviewed_at, OLD.review_note, OLD.sighting_id) THEN
        RAISE EXCEPTION 'a reviewed face match candidate cannot be reviewed again'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fmc_reviewed_once';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fmc_evidence_is_fixed ON face_match_candidates;
CREATE TRIGGER trg_fmc_evidence_is_fixed
    BEFORE UPDATE ON face_match_candidates
    FOR EACH ROW EXECUTE FUNCTION fmc_evidence_is_fixed();
