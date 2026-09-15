-- Phase 10 — Public Safety Risk & Hotspots.
--
-- This scores PLACES, never people. Nothing here references a complainant,
-- accused, phone or vehicle: the tables hold area definitions, the weights
-- of a documented weighted sum, and which beat an FIR's incident occurred in.

-- Weights are versioned configuration, never hidden constants. Each change is
-- a new row with a stated reason; the highest version is the one in force.
CREATE TABLE IF NOT EXISTS risk_weight_sets (
    version     SERIAL PRIMARY KEY,
    weights     JSONB       NOT NULL,
    reason      TEXT        NOT NULL CHECK (length(btrim(reason)) > 0),
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The initial weights, stated so an officer can see where the numbers began.
INSERT INTO risk_weight_sets (version, weights, reason)
SELECT 1,
       '{"fir_count": 1, "serious_fir_count": 3, "night_fir_count": 2, "alert_count": 1, "fir_increase": 2}'::jsonb,
       'Initial documented weights: each FIR counts 1; high or critical priority FIRs add 3; FIRs with an incident time between 20:00 and 06:00 add 2; alerts issued for the station add 1; each FIR more than the previous period adds 2.'
WHERE NOT EXISTS (SELECT 1 FROM risk_weight_sets);
SELECT setval(pg_get_serial_sequence('risk_weight_sets', 'version'),
              GREATEST((SELECT MAX(version) FROM risk_weight_sets), 1));

-- Beats: officer-defined named areas inside a station's jurisdiction, as a
-- centre point and radius. Coordinates are what the officer entered.
CREATE TABLE IF NOT EXISTS risk_beats (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    station_id     UUID NOT NULL REFERENCES stations(id),
    name           VARCHAR(120) NOT NULL CHECK (length(btrim(name)) > 0),
    description    TEXT NOT NULL DEFAULT '',
    latitude       DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude      DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_meters  INTEGER NOT NULL CHECK (radius_meters BETWEEN 50 AND 10000),
    created_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (station_id, name)
);

-- FIRs store no coordinates, so an FIR counts towards a beat only when an
-- officer has placed its incident there. One beat per FIR.
CREATE TABLE IF NOT EXISTS risk_fir_placements (
    fir_id     UUID PRIMARY KEY REFERENCES firs(id) ON DELETE CASCADE,
    beat_id    UUID NOT NULL REFERENCES risk_beats(id),
    note       TEXT NOT NULL DEFAULT '',
    placed_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    placed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_risk_fir_placements_beat ON risk_fir_placements (beat_id);

-- A placement must stay inside the FIR's own station.
CREATE OR REPLACE FUNCTION risk_placement_same_station() RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM firs f JOIN risk_beats b ON b.id = NEW.beat_id
        WHERE f.id = NEW.fir_id AND f.station_id = b.station_id
    ) THEN
        RAISE EXCEPTION 'FIR and beat belong to different stations' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_risk_placement_same_station ON risk_fir_placements;
CREATE TRIGGER trg_risk_placement_same_station
    BEFORE INSERT OR UPDATE ON risk_fir_placements
    FOR EACH ROW EXECUTE FUNCTION risk_placement_same_station();

CREATE INDEX IF NOT EXISTS idx_firs_station_incident_date ON firs (station_id, incident_date);
