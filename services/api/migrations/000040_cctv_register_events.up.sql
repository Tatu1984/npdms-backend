-- Phase 03 — CCTV & Video Intelligence, functional layer.
--
-- An intelligence layer over cameras Kolkata Police already operates: a camera
-- register with stream connection details, real reachability checks,
-- operator-raised events with independent triage, links to FIRs and cases,
-- and a purpose log for every search or view of event metadata.
--
-- Privacy governance ships with the first release rather than the AI layer:
-- retention classes with an enforced expiry, a masking flag, role floors in
-- the API, and purpose logging.

-- ------------------------------------------------------------------ cameras --

CREATE TABLE IF NOT EXISTS cameras (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_number        VARCHAR(40)  NOT NULL UNIQUE,
    -- The operating agency's own identifier, e.g. KP-ESP-014.
    code                 VARCHAR(40)  NOT NULL UNIQUE,
    name                 VARCHAR(255) NOT NULL,
    location             TEXT NOT NULL,
    latitude             DOUBLE PRECISION,
    longitude            DOUBLE PRECISION,
    station_id           UUID NOT NULL REFERENCES stations(id),
    owner_agency         VARCHAR(20)  NOT NULL
                         CHECK (owner_agency IN ('KP', 'KMC', 'TRAFFIC', 'PRIVATE', 'OTHER')),
    stream_type          VARCHAR(10)  NOT NULL DEFAULT 'NONE'
                         CHECK (stream_type IN ('RTSP', 'ONVIF', 'NVR', 'NONE')),
    stream_host          VARCHAR(255),
    stream_port          INTEGER CHECK (stream_port BETWEEN 1 AND 65535),
    -- RTSP path, ONVIF service path or NVR channel.
    stream_path          TEXT,
    -- Encrypted at rest (AES-256-GCM); never returned by the API.
    credential_username  TEXT,
    credential_secret    BYTEA,
    retention_class      VARCHAR(12)  NOT NULL DEFAULT 'STANDARD'
                         CHECK (retention_class IN ('SHORT', 'STANDARD', 'EXTENDED')),
    masking_required     BOOLEAN NOT NULL DEFAULT FALSE,
    status               VARCHAR(16)  NOT NULL DEFAULT 'ACTIVE'
                         CHECK (status IN ('ACTIVE', 'DECOMMISSIONED')),
    decommission_note    TEXT,
    last_checked_at      TIMESTAMPTZ,
    last_check_ok        BOOLEAN,
    last_seen_at         TIMESTAMPTZ,
    created_by           UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT camera_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL)),
    -- A camera with a stream type must say where the stream is.
    CONSTRAINT camera_stream_complete CHECK (
        stream_type = 'NONE' OR (stream_host IS NOT NULL AND stream_port IS NOT NULL)
    ),
    CONSTRAINT camera_decommission_noted CHECK (
        status = 'ACTIVE' OR (decommission_note IS NOT NULL AND length(trim(decommission_note)) > 0)
    ),
    -- last_seen only ever records a successful check.
    CONSTRAINT camera_seen_after_check CHECK (last_seen_at IS NULL OR last_checked_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_cameras_station_status ON cameras (station_id, status);

-- Every reachability check is kept, not just the latest.
CREATE TABLE IF NOT EXISTS camera_health_checks (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id    UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    checked_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reachable    BOOLEAN NOT NULL,
    latency_ms   INTEGER CHECK (latency_ms >= 0),
    error        TEXT,
    checked_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT health_check_outcome CHECK (
        (reachable AND latency_ms IS NOT NULL AND error IS NULL)
        OR (NOT reachable AND error IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_camera_health_camera ON camera_health_checks (camera_id, checked_at DESC);

-- ------------------------------------------------------------- video events --

CREATE TABLE IF NOT EXISTS video_events (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_number      VARCHAR(40)  NOT NULL UNIQUE,
    camera_id         UUID NOT NULL REFERENCES cameras(id),
    event_type        VARCHAR(32)  NOT NULL
                      CHECK (event_type IN ('SUSPICIOUS_ACTIVITY', 'ABANDONED_OBJECT', 'CROWD_BUILDUP',
                                            'TRAFFIC_VIOLATION', 'ACCIDENT', 'ASSAULT', 'THEFT',
                                            'VEHICLE_OF_INTEREST', 'PERSON_OF_INTEREST',
                                            'CAMERA_TAMPERING', 'OTHER')),
    severity          VARCHAR(10)  NOT NULL DEFAULT 'medium'
                      CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    -- When it happened on the footage, not when it was entered.
    occurred_at       TIMESTAMPTZ NOT NULL,
    description       TEXT NOT NULL CHECK (length(trim(description)) > 0),
    -- Matches the platform vocabulary; the AI layer will add 'ai' events later.
    origin            VARCHAR(12)  NOT NULL DEFAULT 'officer'
                      CHECK (origin IN ('officer', 'supervisor', 'derived', 'ai')),
    status            VARCHAR(10)  NOT NULL DEFAULT 'RAISED'
                      CHECK (status IN ('RAISED', 'CONFIRMED', 'DISMISSED')),
    raised_by         UUID NOT NULL REFERENCES users(id),
    triaged_by        UUID REFERENCES users(id),
    triaged_at        TIMESTAMPTZ,
    triage_note       TEXT,
    fir_id            UUID REFERENCES firs(id),
    case_id           UUID REFERENCES cases(id),
    linked_by         UUID REFERENCES users(id),
    linked_at         TIMESTAMPTZ,
    retention_class   VARCHAR(12)  NOT NULL
                      CHECK (retention_class IN ('SHORT', 'STANDARD', 'EXTENDED', 'EVIDENTIAL')),
    -- NULL only while held as evidence.
    retain_until      TIMESTAMPTZ,
    masking_required  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT video_event_triage_complete CHECK (
        (status = 'RAISED' AND triaged_by IS NULL AND triaged_at IS NULL)
        OR (status <> 'RAISED' AND triaged_by IS NOT NULL AND triaged_at IS NOT NULL)
    ),
    -- Confirmation or dismissal is by an officer other than the one who raised it.
    CONSTRAINT video_event_triaged_independently CHECK (triaged_by IS NULL OR triaged_by <> raised_by),
    CONSTRAINT video_event_dismissal_reasoned CHECK (
        status <> 'DISMISSED' OR (triage_note IS NOT NULL AND length(trim(triage_note)) > 0)
    ),
    -- Only a confirmed event can be linked to a FIR or case.
    CONSTRAINT video_event_link_confirmed CHECK (
        (fir_id IS NULL AND case_id IS NULL) OR status = 'CONFIRMED'
    ),
    CONSTRAINT video_event_link_recorded CHECK (
        (fir_id IS NULL AND case_id IS NULL) = (linked_by IS NULL AND linked_at IS NULL)
    ),
    -- Evidential events have no expiry; every other class has one.
    CONSTRAINT video_event_retention_expiry CHECK (
        (retention_class = 'EVIDENTIAL') = (retain_until IS NULL)
    ),
    -- An event linked to a FIR or case is held as evidence.
    CONSTRAINT video_event_linked_is_evidential CHECK (
        (fir_id IS NULL AND case_id IS NULL) OR retention_class = 'EVIDENTIAL'
    )
);
CREATE INDEX IF NOT EXISTS idx_video_events_status ON video_events (status, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_events_camera ON video_events (camera_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_events_retain ON video_events (retain_until) WHERE retain_until IS NOT NULL;

-- ------------------------------------------------------- purpose-logged access --

-- Every search of event metadata and every event opened is recorded with the
-- officer's stated purpose. Append-only: the triggers below refuse change.
CREATE TABLE IF NOT EXISTS video_access_log (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_user_id  UUID NOT NULL REFERENCES users(id),
    access_type    VARCHAR(12) NOT NULL CHECK (access_type IN ('SEARCH', 'VIEW_EVENT')),
    purpose        TEXT NOT NULL CHECK (length(trim(purpose)) >= 10),
    filters        JSONB NOT NULL DEFAULT '{}'::jsonb,
    event_id       UUID REFERENCES video_events(id) ON DELETE SET NULL,
    result_count   INTEGER CHECK (result_count >= 0),
    ip_address     INET,
    accessed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_video_access_actor ON video_access_log (actor_user_id, accessed_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_access_event ON video_access_log (event_id, accessed_at DESC);

CREATE OR REPLACE FUNCTION video_access_log_immutable() RETURNS TRIGGER AS $$
BEGIN
    -- Deleting a purged event nulls event_id through the foreign key; that is
    -- the only permitted change.
    IF TG_OP = 'UPDATE'
       AND NEW.event_id IS NULL AND OLD.event_id IS NOT NULL
       AND NEW.actor_user_id = OLD.actor_user_id AND NEW.access_type = OLD.access_type
       AND NEW.purpose = OLD.purpose AND NEW.filters = OLD.filters
       AND NEW.result_count IS NOT DISTINCT FROM OLD.result_count
       AND NEW.accessed_at = OLD.accessed_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'video_access_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS video_access_log_no_update ON video_access_log;
CREATE TRIGGER video_access_log_no_update BEFORE UPDATE ON video_access_log
    FOR EACH ROW EXECUTE FUNCTION video_access_log_immutable();
DROP TRIGGER IF EXISTS video_access_log_no_delete ON video_access_log;
CREATE TRIGGER video_access_log_no_delete BEFORE DELETE ON video_access_log
    FOR EACH ROW EXECUTE FUNCTION video_access_log_immutable();
