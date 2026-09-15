-- AI layer A4 — vehicle detection and number-plate reading (ANPR).
--
-- Machine output is stored as machine output. Every detection and plate read
-- carries its confidence, the exact model version that produced it and the
-- frame it came from (object key and SHA-256), so an officer can always see
-- what the model saw and a court can see which model said so. Nothing in these
-- tables is an officer's observation until an officer acts on it.
--
-- The module is behind a switch that starts OFF: analysis cannot run until an
-- officer of DSP rank or above switches it on, with a recorded reason.

-- ------------------------------------------------------------ module switch --

CREATE TABLE IF NOT EXISTS module_switches (
    module      VARCHAR(40) PRIMARY KEY,
    enabled     BOOLEAN NOT NULL,
    note        TEXT NOT NULL CHECK (btrim(note) <> ''),
    updated_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO module_switches (module, enabled, note)
VALUES ('VEHICLE_DETECTION', FALSE, 'Off at installation until a DSP or above switches it on')
ON CONFLICT (module) DO NOTHING;

-- ---------------------------------------------------------------- analyses --

-- One submission: an uploaded still, uploaded footage, or a camera snapshot.
--
-- A still or snapshot is kept: it is the frame the detections refer to.
-- Footage is never stored whole. Its SHA-256, size and name are recorded so the
-- source can be matched to the original held elsewhere, and only the sampled
-- frames that produced a detection or plate read are kept as evidence of it.
CREATE TABLE IF NOT EXISTS anpr_analyses (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    analysis_number       VARCHAR(40) NOT NULL UNIQUE,
    source_kind           VARCHAR(10) NOT NULL CHECK (source_kind IN ('STILL', 'FOOTAGE', 'SNAPSHOT')),
    camera_id             UUID REFERENCES cameras(id),
    purpose               TEXT NOT NULL CHECK (length(btrim(purpose)) >= 10),
    -- When the still was taken or the footage starts, as stated at submission.
    captured_at           TIMESTAMPTZ NOT NULL,
    media_object_key      TEXT,
    media_sha256          CHAR(64) NOT NULL CHECK (media_sha256 ~ '^[0-9a-f]{64}$'),
    media_filename        TEXT NOT NULL DEFAULT '',
    media_content_type    VARCHAR(100) NOT NULL,
    media_size_bytes      BIGINT NOT NULL CHECK (media_size_bytes > 0),
    storage_backend       VARCHAR(20) NOT NULL,
    detector_version      TEXT NOT NULL CHECK (btrim(detector_version) <> ''),
    plate_reader_version  TEXT NOT NULL CHECK (btrim(plate_reader_version) <> ''),
    sampled_frames        INTEGER NOT NULL CHECK (sampled_frames >= 1),
    sample_seconds        NUMERIC(6, 2),
    processing_ms         INTEGER CHECK (processing_ms >= 0),
    submitted_by          UUID NOT NULL REFERENCES users(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A camera snapshot always names its camera.
    CONSTRAINT anpr_snapshot_has_camera CHECK (source_kind <> 'SNAPSHOT' OR camera_id IS NOT NULL),
    CONSTRAINT anpr_footage_not_stored_whole CHECK ((source_kind = 'FOOTAGE') = (media_object_key IS NULL)),
    CONSTRAINT anpr_footage_sampled CHECK ((source_kind = 'FOOTAGE') = (sample_seconds IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_anpr_analyses_created ON anpr_analyses (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_anpr_analyses_camera ON anpr_analyses (camera_id, captured_at DESC);

-- A frame the models looked at and that produced at least one output. For a
-- still or snapshot it is the uploaded image itself; for footage, a JPEG of the
-- sampled frame.
CREATE TABLE IF NOT EXISTS anpr_frames (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    analysis_id     UUID NOT NULL REFERENCES anpr_analyses(id) ON DELETE CASCADE,
    frame_index     INTEGER NOT NULL CHECK (frame_index >= 0),
    offset_seconds  NUMERIC(10, 3) NOT NULL CHECK (offset_seconds >= 0),
    frame_time      TIMESTAMPTZ NOT NULL,
    object_key      TEXT NOT NULL,
    sha256          CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    width           INTEGER NOT NULL CHECK (width > 0),
    height          INTEGER NOT NULL CHECK (height > 0),
    UNIQUE (analysis_id, frame_index)
);

CREATE TABLE IF NOT EXISTS vehicle_detections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    analysis_id     UUID NOT NULL REFERENCES anpr_analyses(id) ON DELETE CASCADE,
    frame_id        UUID NOT NULL REFERENCES anpr_frames(id) ON DELETE CASCADE,
    camera_id       UUID REFERENCES cameras(id),
    frame_time      TIMESTAMPTZ NOT NULL,
    -- Only the classes the detector has. It has no auto-rickshaw class.
    vehicle_class   VARCHAR(12) NOT NULL CHECK (vehicle_class IN ('CAR', 'MOTORCYCLE', 'BUS', 'TRUCK', 'BICYCLE')),
    box_x1          INTEGER NOT NULL,
    box_y1          INTEGER NOT NULL,
    box_x2          INTEGER NOT NULL,
    box_y2          INTEGER NOT NULL,
    confidence      NUMERIC(5, 4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    model_version   TEXT NOT NULL CHECK (btrim(model_version) <> ''),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT vehicle_detection_box CHECK (box_x2 > box_x1 AND box_y2 > box_y1)
);
CREATE INDEX IF NOT EXISTS idx_vehicle_detections_analysis ON vehicle_detections (analysis_id);
CREATE INDEX IF NOT EXISTS idx_vehicle_detections_camera_time ON vehicle_detections (camera_id, frame_time DESC);

-- Registration numbers use the Phase 06 normalisation (upper case, letters and
-- digits only), so a read joins the traffic plate-read register and lookouts.
CREATE TABLE IF NOT EXISTS anpr_plate_reads (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    analysis_id          UUID NOT NULL REFERENCES anpr_analyses(id) ON DELETE CASCADE,
    frame_id             UUID NOT NULL REFERENCES anpr_frames(id) ON DELETE CASCADE,
    -- NULL when the plate was read from the frame without a detected vehicle.
    detection_id         UUID REFERENCES vehicle_detections(id) ON DELETE CASCADE,
    camera_id            UUID REFERENCES cameras(id),
    frame_time           TIMESTAMPTZ NOT NULL,
    registration_number  VARCHAR(20) NOT NULL CHECK (registration_number ~ '^[A-Z0-9]{4,20}$'),
    display_number       VARCHAR(30) NOT NULL,
    raw_text             TEXT NOT NULL,
    plate_format         VARCHAR(12) NOT NULL CHECK (plate_format IN ('STANDARD', 'BH', 'OLD')),
    confidence           NUMERIC(5, 4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    min_char_confidence  NUMERIC(5, 4) NOT NULL CHECK (min_char_confidence >= 0 AND min_char_confidence <= 1),
    -- [{char, confidence, corrected, raw}] as returned by the reader.
    characters           JSONB NOT NULL,
    corrections          JSONB NOT NULL DEFAULT '[]'::jsonb,
    box_x1               INTEGER NOT NULL,
    box_y1               INTEGER NOT NULL,
    box_x2               INTEGER NOT NULL,
    box_y2               INTEGER NOT NULL,
    model_version        TEXT NOT NULL CHECK (btrim(model_version) <> ''),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT anpr_plate_read_box CHECK (box_x2 > box_x1 AND box_y2 > box_y1)
);
CREATE INDEX IF NOT EXISTS idx_anpr_plate_reads_number ON anpr_plate_reads (registration_number, frame_time DESC);
CREATE INDEX IF NOT EXISTS idx_anpr_plate_reads_time ON anpr_plate_reads (frame_time DESC);
CREATE INDEX IF NOT EXISTS idx_anpr_plate_reads_camera ON anpr_plate_reads (camera_id, frame_time DESC);
CREATE INDEX IF NOT EXISTS idx_anpr_plate_reads_analysis ON anpr_plate_reads (analysis_id);

-- --------------------------------------------------------- purpose-logged --

-- Every submission, snapshot, search of plate reads and opening of an analysis
-- is recorded with the officer's stated purpose. Append-only.
CREATE TABLE IF NOT EXISTS anpr_access_log (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_user_id  UUID NOT NULL REFERENCES users(id),
    access_type    VARCHAR(20) NOT NULL
                   CHECK (access_type IN ('SUBMIT_ANALYSIS', 'INGEST_SNAPSHOT', 'SEARCH_READS', 'VIEW_ANALYSIS')),
    purpose        TEXT NOT NULL CHECK (length(btrim(purpose)) >= 10),
    filters        JSONB NOT NULL DEFAULT '{}'::jsonb,
    analysis_id    UUID REFERENCES anpr_analyses(id) ON DELETE SET NULL,
    result_count   INTEGER CHECK (result_count >= 0),
    ip_address     INET,
    accessed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_anpr_access_actor ON anpr_access_log (actor_user_id, accessed_at DESC);
CREATE INDEX IF NOT EXISTS idx_anpr_access_analysis ON anpr_access_log (analysis_id, actor_user_id, accessed_at DESC);

CREATE OR REPLACE FUNCTION anpr_access_log_immutable() RETURNS TRIGGER AS $$
BEGIN
    -- Deleting an analysis nulls analysis_id through the foreign key; that is
    -- the only permitted change.
    IF TG_OP = 'UPDATE'
       AND NEW.analysis_id IS NULL AND OLD.analysis_id IS NOT NULL
       AND NEW.actor_user_id = OLD.actor_user_id AND NEW.access_type = OLD.access_type
       AND NEW.purpose = OLD.purpose AND NEW.filters = OLD.filters
       AND NEW.result_count IS NOT DISTINCT FROM OLD.result_count
       AND NEW.accessed_at = OLD.accessed_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'anpr_access_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS anpr_access_log_no_update ON anpr_access_log;
CREATE TRIGGER anpr_access_log_no_update BEFORE UPDATE ON anpr_access_log
    FOR EACH ROW EXECUTE FUNCTION anpr_access_log_immutable();
DROP TRIGGER IF EXISTS anpr_access_log_no_delete ON anpr_access_log;
CREATE TRIGGER anpr_access_log_no_delete BEFORE DELETE ON anpr_access_log
    FOR EACH ROW EXECUTE FUNCTION anpr_access_log_immutable();
