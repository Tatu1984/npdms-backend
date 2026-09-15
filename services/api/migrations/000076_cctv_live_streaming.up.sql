-- Phase 03 — live CCTV streaming through the Edge Agent.
--
-- The same design as the Live Feed Portal and KMCP: an on-site Edge Agent pulls
-- a camera's RTSP on the local network and PUTs HLS (a playlist and 2-second
-- segments) outbound to /api/edge/ingest/<ingest_key>/<file>. The bytes go to a
-- media store (Cloudflare R2 in production); officers watch through
-- purpose-logged, rank-gated playback.
--
-- Streaming is part of the existing camera register, not a second camera table.
-- Each camera that streams has:
--   ingest_key         an unguessable path segment naming the camera in the URL
--                      (the Edge Agent's "Camera ID" / stream key)
--   ingest_token_hash  SHA-256 of the camera's upload token. The token itself is
--                      shown once, when streaming is enabled or the token is
--                      rotated, and is never stored.
--
-- Liveness is not stored: it is judged on every read from the playlist the
-- agent last wrote. last_segment_at only records that uploads are arriving.
--
-- Idempotent: safe to re-run.

ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_key              VARCHAR(64);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_token_hash       CHAR(64);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_enabled          BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_enabled_at       TIMESTAMPTZ;
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_enabled_by       UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS ingest_token_rotated_at TIMESTAMPTZ;
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS last_segment_at         TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cameras_ingest_key ON cameras (ingest_key) WHERE ingest_key IS NOT NULL;

DO $$
BEGIN
    -- A camera that accepts uploads has both a key and a token hash.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'camera_ingest_complete') THEN
        ALTER TABLE cameras ADD CONSTRAINT camera_ingest_complete CHECK (
            NOT ingest_enabled OR (ingest_key IS NOT NULL AND ingest_token_hash IS NOT NULL)
        );
    END IF;
    -- The key is a single safe URL path segment.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'camera_ingest_key_safe') THEN
        ALTER TABLE cameras ADD CONSTRAINT camera_ingest_key_safe CHECK (
            ingest_key IS NULL OR ingest_key ~ '^[A-Za-z0-9_-]{12,64}$'
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'camera_ingest_hash_hex') THEN
        ALTER TABLE cameras ADD CONSTRAINT camera_ingest_hash_hex CHECK (
            ingest_token_hash IS NULL OR ingest_token_hash ~ '^[0-9a-f]{64}$'
        );
    END IF;
    -- A decommissioned camera never accepts uploads.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'camera_ingest_active_only') THEN
        ALTER TABLE cameras ADD CONSTRAINT camera_ingest_active_only CHECK (
            NOT ingest_enabled OR status = 'ACTIVE'
        );
    END IF;
END $$;

-- ------------------------------------------------------ purpose-logged viewing --

-- Watching live video is an access like searching or opening an event: it is
-- recorded in the append-only video_access_log with the officer's purpose. It is
-- recorded once per viewing session, not once per two-second segment.
ALTER TABLE video_access_log ADD COLUMN IF NOT EXISTS camera_ids UUID[];

ALTER TABLE video_access_log DROP CONSTRAINT IF EXISTS video_access_log_access_type_check;
ALTER TABLE video_access_log ADD CONSTRAINT video_access_log_access_type_check
    CHECK (access_type IN ('SEARCH', 'VIEW_EVENT', 'VIEW_LIVE'));

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'video_access_live_names_cameras') THEN
        ALTER TABLE video_access_log ADD CONSTRAINT video_access_live_names_cameras CHECK (
            access_type <> 'VIEW_LIVE' OR cardinality(camera_ids) > 0
        );
    END IF;
END $$;

-- The immutability trigger compares every column an update could touch; keep it
-- in step with the new column so camera_ids cannot be rewritten either.
CREATE OR REPLACE FUNCTION video_access_log_immutable() RETURNS TRIGGER AS $$
BEGIN
    -- Deleting a purged event nulls event_id through the foreign key; that is
    -- the only permitted change.
    IF TG_OP = 'UPDATE'
       AND NEW.event_id IS NULL AND OLD.event_id IS NOT NULL
       AND NEW.actor_user_id = OLD.actor_user_id AND NEW.access_type = OLD.access_type
       AND NEW.purpose = OLD.purpose AND NEW.filters = OLD.filters
       AND NEW.result_count IS NOT DISTINCT FROM OLD.result_count
       AND NEW.camera_ids IS NOT DISTINCT FROM OLD.camera_ids
       AND NEW.accessed_at = OLD.accessed_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'video_access_log is append-only';
END;
$$ LANGUAGE plpgsql;

-- A live viewing session: one officer, one stated purpose, one or more cameras.
-- Playback of a segment is allowed only inside an open session for that camera.
-- The session slides forward while the officer is watching (idle expiry) and
-- ends outright at max_until, after which a new purpose must be stated.
CREATE TABLE IF NOT EXISTS live_view_sessions (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    viewer_id      UUID NOT NULL REFERENCES users(id),
    access_log_id  UUID NOT NULL REFERENCES video_access_log(id),
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL,
    max_until      TIMESTAMPTZ NOT NULL,
    ended_at       TIMESTAMPTZ,
    last_played_at TIMESTAMPTZ,
    CONSTRAINT live_view_expiry_bounded CHECK (expires_at <= max_until AND max_until > started_at)
);
CREATE INDEX IF NOT EXISTS idx_live_view_sessions_viewer ON live_view_sessions (viewer_id, expires_at DESC);

CREATE TABLE IF NOT EXISTS live_view_session_cameras (
    session_id UUID NOT NULL REFERENCES live_view_sessions(id) ON DELETE CASCADE,
    camera_id  UUID NOT NULL REFERENCES cameras(id),
    PRIMARY KEY (session_id, camera_id)
);
CREATE INDEX IF NOT EXISTS idx_live_view_session_cameras_camera ON live_view_session_cameras (camera_id);
