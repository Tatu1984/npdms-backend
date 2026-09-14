-- Phase 01 completion.
--
-- The workspace is specified to carry a location count alongside evidence,
-- witness, person and vehicle counts. Nothing recorded a location, so the
-- figure could never be produced. Each chronology entry happened somewhere;
-- that is the natural place to record it.

ALTER TABLE workspace_timeline
    ADD COLUMN IF NOT EXISTS location TEXT,
    ADD COLUMN IF NOT EXISTS location_bn TEXT,
    ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_ws_timeline_location
    ON workspace_timeline(workspace_id, location)
    WHERE location IS NOT NULL;

COMMENT ON COLUMN workspace_timeline.location IS
    'Where the event occurred. Distinct values give the workspace location count.';
