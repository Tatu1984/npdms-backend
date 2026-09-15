-- Phase 13 — Body-Worn Camera Evidence (functional layer, no AI).
--
-- A body-worn camera is issued to an officer for a shift and docked at the end
-- of it. Recordings are uploaded on docking against that shift's assignment.
-- The bytes go through the Phase 02 storage abstraction, hashed as they stream
-- in; nothing here is a second evidence store.
--
-- The Phase 02 register only accepts items linked to a case or FIR, and most
-- footage never becomes evidence. So a docked recording is held here with its
-- ingest digest under a short, non-evidential retention class. Linking it to
-- an FIR or case registers it in the Phase 02 register — a signed first
-- custody leg and the same bytes, re-hashed and required to match the digest
-- taken at docking — after which it is evidential and never purged.

-- ------------------------------------------------------------------ devices --

CREATE TABLE IF NOT EXISTS bwc_devices (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_number   VARCHAR(40)  NOT NULL UNIQUE,
    serial_number   VARCHAR(120) NOT NULL UNIQUE,
    model           VARCHAR(120) NOT NULL,
    station_id      UUID NOT NULL REFERENCES stations(id),
    status          VARCHAR(12)  NOT NULL DEFAULT 'IN_SERVICE'
                    CHECK (status IN ('IN_SERVICE', 'CHARGING', 'FAULTY', 'RETIRED')),
    status_note     TEXT,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bwc_device_fault_explained CHECK (
        status NOT IN ('FAULTY', 'RETIRED') OR (status_note IS NOT NULL AND length(trim(status_note)) > 0)
    )
);
CREATE INDEX IF NOT EXISTS idx_bwc_devices_station ON bwc_devices (station_id, status);

-- --------------------------------------------------------------- assignments --

CREATE TABLE IF NOT EXISTS bwc_assignments (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id        UUID NOT NULL REFERENCES bwc_devices(id),
    officer_id       UUID NOT NULL REFERENCES users(id),
    issued_by        UUID NOT NULL REFERENCES users(id),
    shift_label      VARCHAR(80) NOT NULL,
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expected_return  TIMESTAMPTZ,
    returned_at      TIMESTAMPTZ,
    received_by      UUID REFERENCES users(id),
    return_note      TEXT,
    CONSTRAINT bwc_assignment_return_complete CHECK (
        (returned_at IS NULL AND received_by IS NULL) OR (returned_at IS NOT NULL AND received_by IS NOT NULL)
    ),
    CONSTRAINT bwc_assignment_return_after_issue CHECK (returned_at IS NULL OR returned_at >= issued_at)
);
-- One open assignment per device, and one camera per officer at a time. Held
-- by the database so two issues racing for the same device cannot both win.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bwc_open_assignment_device
    ON bwc_assignments (device_id) WHERE returned_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_bwc_open_assignment_officer
    ON bwc_assignments (officer_id) WHERE returned_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bwc_assignments_device ON bwc_assignments (device_id, issued_at DESC);

-- ------------------------------------------------------------------ readings --

-- Battery and storage are reported — by a dock or an officer — never polled.
-- A reading is a statement at a moment, so the screen shows its age.
CREATE TABLE IF NOT EXISTS bwc_readings (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id         UUID NOT NULL REFERENCES bwc_devices(id),
    battery_percent   INTEGER NOT NULL CHECK (battery_percent BETWEEN 0 AND 100),
    storage_percent   INTEGER NOT NULL CHECK (storage_percent BETWEEN 0 AND 100),
    source            VARCHAR(8) NOT NULL CHECK (source IN ('DOCK', 'OFFICER')),
    reported_by       UUID NOT NULL REFERENCES users(id),
    observed_at       TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bwc_readings_device ON bwc_readings (device_id, observed_at DESC);

-- ---------------------------------------------------------------- recordings --

CREATE TABLE IF NOT EXISTS bwc_recordings (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_number      VARCHAR(40)  NOT NULL UNIQUE,
    device_id             UUID NOT NULL REFERENCES bwc_devices(id),
    assignment_id         UUID NOT NULL REFERENCES bwc_assignments(id),
    officer_id            UUID NOT NULL REFERENCES users(id),
    uploaded_by           UUID NOT NULL REFERENCES users(id),
    started_at            TIMESTAMPTZ NOT NULL,
    ended_at              TIMESTAMPTZ NOT NULL,
    object_key            TEXT,
    original_filename     VARCHAR(255) NOT NULL,
    content_type          VARCHAR(120) NOT NULL,
    size_bytes            BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256                CHAR(64) NOT NULL,
    storage_backend       VARCHAR(20) NOT NULL,
    uploaded_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retention_class       VARCHAR(16) NOT NULL DEFAULT 'NON_EVIDENTIAL'
                          CHECK (retention_class IN ('NON_EVIDENTIAL', 'EVIDENTIAL')),
    retain_until          TIMESTAMPTZ,
    evidence_id           UUID UNIQUE REFERENCES evidence(id),
    fir_id                UUID REFERENCES firs(id),
    case_id               UUID REFERENCES cases(id),
    dispatch_incident_id  UUID REFERENCES dispatch_incidents(id),
    link_note             TEXT,
    linked_by             UUID REFERENCES users(id),
    linked_at             TIMESTAMPTZ,
    purged_at             TIMESTAMPTZ,
    purged_by             UUID REFERENCES users(id),
    purge_reason          TEXT,
    CONSTRAINT bwc_recording_span CHECK (ended_at > started_at),
    CONSTRAINT bwc_recording_retention CHECK (
        (retention_class = 'NON_EVIDENTIAL' AND retain_until IS NOT NULL AND evidence_id IS NULL)
        OR (retention_class = 'EVIDENTIAL' AND retain_until IS NULL AND evidence_id IS NOT NULL
            AND (fir_id IS NOT NULL OR case_id IS NOT NULL) AND linked_by IS NOT NULL AND linked_at IS NOT NULL)
    ),
    -- Evidence is never purged; a purge removes the bytes and keeps the record.
    CONSTRAINT bwc_recording_purge CHECK (
        (purged_at IS NULL AND purged_by IS NULL AND object_key IS NOT NULL)
        OR (purged_at IS NOT NULL AND purged_by IS NOT NULL AND retention_class = 'NON_EVIDENTIAL'
            AND object_key IS NULL AND purge_reason IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_bwc_recordings_device ON bwc_recordings (device_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_bwc_recordings_officer ON bwc_recordings (officer_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_bwc_recordings_retention ON bwc_recordings (retention_class, retain_until)
    WHERE purged_at IS NULL;

-- A recording, once taken, keeps its identity: the digest, the officer, the
-- device and its time span cannot be rewritten, and an evidential recording
-- cannot go back to being purgeable.
CREATE OR REPLACE FUNCTION bwc_recording_guard() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'bwc_recordings rows are never deleted; purge removes the bytes only';
    END IF;
    IF NEW.sha256 <> OLD.sha256 OR NEW.device_id <> OLD.device_id OR NEW.assignment_id <> OLD.assignment_id
       OR NEW.officer_id <> OLD.officer_id OR NEW.started_at <> OLD.started_at OR NEW.ended_at <> OLD.ended_at
       OR NEW.recording_number <> OLD.recording_number OR NEW.size_bytes <> OLD.size_bytes THEN
        RAISE EXCEPTION 'a recording''s identity and digest cannot be changed';
    END IF;
    IF OLD.retention_class = 'EVIDENTIAL' AND NEW.retention_class <> 'EVIDENTIAL' THEN
        RAISE EXCEPTION 'an evidential recording cannot return to non-evidential retention';
    END IF;
    IF OLD.purged_at IS NOT NULL AND NEW.purged_at IS DISTINCT FROM OLD.purged_at THEN
        RAISE EXCEPTION 'a purge cannot be undone';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS bwc_recordings_guard_update ON bwc_recordings;
CREATE TRIGGER bwc_recordings_guard_update BEFORE UPDATE ON bwc_recordings
    FOR EACH ROW EXECUTE FUNCTION bwc_recording_guard();
DROP TRIGGER IF EXISTS bwc_recordings_guard_delete ON bwc_recordings;
CREATE TRIGGER bwc_recordings_guard_delete BEFORE DELETE ON bwc_recordings
    FOR EACH ROW EXECUTE FUNCTION bwc_recording_guard();

-- ---------------------------------------------------------------- access log --

-- Every look at footage states a purpose first, and the record of it cannot
-- be edited or removed. Refusals are logged too.
CREATE TABLE IF NOT EXISTS bwc_access_log (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id   UUID NOT NULL REFERENCES bwc_recordings(id),
    actor_user_id  UUID NOT NULL REFERENCES users(id),
    access_type    VARCHAR(10) NOT NULL CHECK (access_type IN ('VIEW', 'DOWNLOAD', 'DENIED')),
    purpose        TEXT NOT NULL,
    ip_address     INET,
    user_agent     TEXT,
    accessed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bwc_access_purpose_stated CHECK (access_type = 'DENIED' OR length(trim(purpose)) >= 10)
);
CREATE INDEX IF NOT EXISTS idx_bwc_access_recording ON bwc_access_log (recording_id, accessed_at DESC);

CREATE OR REPLACE FUNCTION bwc_access_log_immutable() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'bwc_access_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS bwc_access_log_no_update ON bwc_access_log;
CREATE TRIGGER bwc_access_log_no_update BEFORE UPDATE OR DELETE ON bwc_access_log
    FOR EACH ROW EXECUTE FUNCTION bwc_access_log_immutable();
