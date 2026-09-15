-- AI layer A4 — vehicle watchlist, watchlist hits, and ANPR reads in the
-- Phase 06 plate-read register.
--
-- A plate read that matches the watchlist is an ALERT CANDIDATE, never an
-- alert. It waits as PENDING until an operator other than the officer who
-- submitted the footage confirms or dismisses it. Only confirmation raises an
-- alert and, for a stolen-vehicle lookout, records a sighting.
--
-- The watchlist has two sources. Active STOLEN_VEHICLE lookouts are included
-- automatically (read at match time, so a lookout closed a minute ago no longer
-- matches). Officers add other entries here with a reason and an expiry.

CREATE TABLE IF NOT EXISTS vehicle_watchlist (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    registration_number  VARCHAR(20) NOT NULL CHECK (registration_number ~ '^[A-Z0-9]{4,20}$'),
    reason               TEXT NOT NULL CHECK (length(btrim(reason)) >= 10),
    priority             VARCHAR(10) NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'CRITICAL')),
    fir_id               UUID REFERENCES firs(id),
    expires_at           TIMESTAMPTZ NOT NULL,
    added_by             UUID NOT NULL REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_at           TIMESTAMPTZ,
    removed_by           UUID REFERENCES users(id),
    removal_note         TEXT,
    CONSTRAINT watchlist_expiry_after_creation CHECK (expires_at > created_at),
    CONSTRAINT watchlist_removal_complete CHECK (
        (removed_at IS NULL AND removed_by IS NULL AND removal_note IS NULL)
        OR (removed_at IS NOT NULL AND removed_by IS NOT NULL AND length(btrim(removal_note)) > 0)
    )
);
-- One open entry per registration; expired entries are ignored at match time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_vehicle_watchlist_open
    ON vehicle_watchlist (registration_number) WHERE removed_at IS NULL;

CREATE TABLE IF NOT EXISTS anpr_watchlist_hits (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hit_number           VARCHAR(40) NOT NULL UNIQUE,
    plate_read_id        UUID NOT NULL REFERENCES anpr_plate_reads(id) ON DELETE CASCADE,
    registration_number  VARCHAR(20) NOT NULL,
    source               VARCHAR(10) NOT NULL CHECK (source IN ('LOOKOUT', 'WATCHLIST')),
    lookout_id           UUID REFERENCES lookouts(id),
    watchlist_entry_id   UUID REFERENCES vehicle_watchlist(id),
    -- The officer who submitted the footage or snapshot; never the reviewer.
    submitted_by         UUID NOT NULL REFERENCES users(id),
    status               VARCHAR(10) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'CONFIRMED', 'DISMISSED')),
    reviewed_by          UUID REFERENCES users(id),
    reviewed_at          TIMESTAMPTZ,
    review_note          TEXT,
    alert_id             UUID REFERENCES alerts(id) ON DELETE SET NULL,
    sighting_id          UUID REFERENCES lookout_sightings(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT hit_source_named CHECK (
        (source = 'LOOKOUT' AND lookout_id IS NOT NULL AND watchlist_entry_id IS NULL)
        OR (source = 'WATCHLIST' AND watchlist_entry_id IS NOT NULL AND lookout_id IS NULL)
    ),
    CONSTRAINT hit_review_complete CHECK (
        (status = 'PENDING' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR (status <> 'PENDING' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    ),
    CONSTRAINT hit_reviewed_independently CHECK (reviewed_by IS NULL OR reviewed_by <> submitted_by),
    CONSTRAINT hit_dismissal_reasoned CHECK (status <> 'DISMISSED' OR length(btrim(COALESCE(review_note, ''))) > 0),
    CONSTRAINT hit_actions_need_confirmation CHECK (status = 'CONFIRMED' OR (alert_id IS NULL AND sighting_id IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_anpr_hit_read_lookout
    ON anpr_watchlist_hits (plate_read_id, lookout_id) WHERE lookout_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_anpr_hit_read_watchlist
    ON anpr_watchlist_hits (plate_read_id, watchlist_entry_id) WHERE watchlist_entry_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_anpr_hits_status ON anpr_watchlist_hits (status, created_at DESC);

-- ---------------------------------------- Phase 06 plate-read register link --

-- An ANPR read attached to a traffic incident keeps its machine provenance:
-- the read it came from, the model version and the confidence.
ALTER TABLE traffic_incident_plate_reads
    ADD COLUMN IF NOT EXISTS anpr_plate_read_id UUID REFERENCES anpr_plate_reads(id),
    ADD COLUMN IF NOT EXISTS model_version TEXT,
    ADD COLUMN IF NOT EXISTS read_confidence NUMERIC(5, 4);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'plate_read_anpr_provenance') THEN
        ALTER TABLE traffic_incident_plate_reads ADD CONSTRAINT plate_read_anpr_provenance CHECK (
            anpr_plate_read_id IS NULL
            OR (source = 'ANPR_SYSTEM' AND btrim(COALESCE(model_version, '')) <> '' AND read_confidence IS NOT NULL)
        );
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_traffic_plate_read_anpr
    ON traffic_incident_plate_reads (incident_id, anpr_plate_read_id) WHERE anpr_plate_read_id IS NOT NULL;
