-- Face photographs on missing-person reports, the database storage backend,
-- and the last-seen point.
--
-- 1. storage_objects holds file bytes for STORAGE_BACKEND=database — hosting
--    whose disk does not persist (the staging API on Vercel). Objects are capped
--    by the API (8 MB by default); recordings need MinIO/S3.
-- 2. missing_person_photos: photographs of the missing person, most often given
--    by family or friends. The face recognition layer enrols faces from this
--    table (its own tables start at 000072). A photo is never deleted: it is
--    retired with a reason, and the bytes stay in storage.
-- 3. missing_person_reports gains a last-seen coordinate pair so the map plots
--    only a stored point, never a guessed one.

CREATE TABLE IF NOT EXISTS storage_objects (
    key          TEXT PRIMARY KEY CHECK (length(key) BETWEEN 1 AND 512),
    content_type VARCHAR(128) NOT NULL,
    size_bytes   BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256       CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    data         BYTEA NOT NULL,
    stored_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_object_size_matches CHECK (octet_length(data) = size_bytes)
);

CREATE TABLE IF NOT EXISTS missing_person_photos (
    id                        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id                 UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE RESTRICT,
    storage_key               TEXT NOT NULL UNIQUE,
    storage_backend           VARCHAR(16) NOT NULL,
    sha256                    CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes                BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 8388608),
    content_type              VARCHAR(16) NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    width                     INTEGER NOT NULL CHECK (width > 0),
    height                    INTEGER NOT NULL CHECK (height > 0),
    thumbnail_key             TEXT,
    original_filename         VARCHAR(255),
    -- Location metadata (EXIF GPS, XMP) is removed before the bytes are stored;
    -- the hash is of the stored bytes. The note says what was found and removed.
    location_metadata_removed BOOLEAN NOT NULL DEFAULT FALSE,
    metadata_note             TEXT NOT NULL DEFAULT '',
    source                    VARCHAR(20) NOT NULL
                              CHECK (source IN ('FAMILY', 'FRIEND', 'REPORTING_PERSON', 'OFFICER', 'CCTV_STILL', 'OTHER')),
    provided_by_name          VARCHAR(255) NOT NULL CHECK (btrim(provided_by_name) <> ''),
    relationship              VARCHAR(100) NOT NULL CHECK (btrim(relationship) <> ''),
    consent_recorded          BOOLEAN NOT NULL DEFAULT FALSE,
    consent_note              TEXT,
    taken_on                  DATE,
    is_primary                BOOLEAN NOT NULL DEFAULT FALSE,
    quality_note              TEXT,
    uploaded_by               UUID NOT NULL REFERENCES users(id),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at                TIMESTAMPTZ,
    retired_by                UUID REFERENCES users(id),
    retire_reason             TEXT,
    CONSTRAINT mpp_retirement_complete CHECK (
        (retired_at IS NULL AND retired_by IS NULL AND retire_reason IS NULL)
        OR (retired_at IS NOT NULL AND retired_by IS NOT NULL AND retire_reason IS NOT NULL AND btrim(retire_reason) <> '')
    ),
    CONSTRAINT mpp_retired_not_primary CHECK (retired_at IS NULL OR NOT is_primary),
    CONSTRAINT mpp_taken_not_future CHECK (taken_on IS NULL OR taken_on <= (created_at AT TIME ZONE 'Asia/Kolkata')::date)
);

-- Exactly one primary among a report's active photos (the service keeps one
-- whenever any active photo exists; the index forbids two).
CREATE UNIQUE INDEX IF NOT EXISTS uq_mpp_one_primary
    ON missing_person_photos (report_id) WHERE is_primary AND retired_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_mpp_report ON missing_person_photos (report_id, created_at);

-- Photos are retired, never removed.
CREATE OR REPLACE FUNCTION mpp_forbid_delete() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'missing person photos are retired with a reason, never deleted';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_mpp_forbid_delete ON missing_person_photos;
CREATE TRIGGER trg_mpp_forbid_delete BEFORE DELETE ON missing_person_photos
    FOR EACH ROW EXECUTE FUNCTION mpp_forbid_delete();

ALTER TABLE missing_person_reports
    ADD COLUMN IF NOT EXISTS last_seen_latitude  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS last_seen_longitude DOUBLE PRECISION;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_last_seen_point_paired') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_last_seen_point_paired
            CHECK ((last_seen_latitude IS NULL) = (last_seen_longitude IS NULL));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_last_seen_point_range') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_last_seen_point_range
            CHECK (last_seen_latitude IS NULL OR (last_seen_latitude BETWEEN -90 AND 90 AND last_seen_longitude BETWEEN -180 AND 180));
    END IF;
END $$;
