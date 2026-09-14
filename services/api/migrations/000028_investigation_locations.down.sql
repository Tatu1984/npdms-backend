DROP INDEX IF EXISTS idx_ws_timeline_location;
ALTER TABLE workspace_timeline
    DROP COLUMN IF EXISTS location,
    DROP COLUMN IF EXISTS location_bn,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude;
