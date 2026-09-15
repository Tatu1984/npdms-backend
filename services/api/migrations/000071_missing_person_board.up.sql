-- City-wide missing-person board: each station's check of an open report, and
-- a link from an alert to the record it concerns.
--
-- A station check is appended, never edited: a station that first recorded
-- "no match" and later finds a possible match records a second check, and the
-- board shows each station's latest. A SIGHTING check points at the sighting it
-- created in missing_person_sightings, which still needs verification by an
-- officer other than the one who recorded it (000042).

CREATE TABLE IF NOT EXISTS missing_person_station_checks (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id   UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE RESTRICT,
    station_id  UUID NOT NULL REFERENCES stations(id),
    outcome     VARCHAR(16) NOT NULL CHECK (outcome IN ('NO_MATCH', 'POSSIBLE_MATCH', 'SIGHTING')),
    details     TEXT,
    sighting_id UUID REFERENCES missing_person_sightings(id),
    recorded_by UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT mpsc_possible_match_described CHECK (outcome <> 'POSSIBLE_MATCH' OR (details IS NOT NULL AND btrim(details) <> '')),
    CONSTRAINT mpsc_sighting_linked CHECK ((outcome = 'SIGHTING') = (sighting_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_mpsc_report_station ON missing_person_station_checks (report_id, station_id, created_at DESC);

CREATE OR REPLACE FUNCTION mpsc_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'station checks are appended, never changed or removed';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_mpsc_append_only ON missing_person_station_checks;
CREATE TRIGGER trg_mpsc_append_only BEFORE UPDATE OR DELETE ON missing_person_station_checks
    FOR EACH ROW EXECUTE FUNCTION mpsc_append_only();

-- Alerts raised from a record carry a link back to it.
ALTER TABLE alerts
    ADD COLUMN IF NOT EXISTS resource_type VARCHAR(40),
    ADD COLUMN IF NOT EXISTS resource_id   UUID;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'alerts_resource_paired') THEN
        ALTER TABLE alerts ADD CONSTRAINT alerts_resource_paired CHECK ((resource_type IS NULL) = (resource_id IS NULL));
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_alerts_resource ON alerts (resource_type, resource_id) WHERE resource_id IS NOT NULL;
