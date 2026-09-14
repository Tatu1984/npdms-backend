-- Armoury and lookout notices.
--
-- Both screens previously held their records in the browser: a weapon issued
-- on one terminal was still "in armoury" on every other, and nothing survived
-- clearing site data. These tables make both the platform's record.

-- ------------------------------------------------------------------ armoury --

CREATE TABLE IF NOT EXISTS weapons (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    weapon_number     VARCHAR(40)  NOT NULL UNIQUE,
    weapon_type       VARCHAR(80)  NOT NULL,
    make              VARCHAR(120) NOT NULL,
    serial_number     VARCHAR(120) NOT NULL UNIQUE,
    station_id        UUID NOT NULL REFERENCES stations(id),
    status            VARCHAR(20)  NOT NULL DEFAULT 'IN_ARMOURY'
                      CHECK (status IN ('IN_ARMOURY', 'ISSUED', 'MAINTENANCE', 'CONDEMNED')),
    condition         VARCHAR(20)  NOT NULL DEFAULT 'SERVICEABLE'
                      CHECK (condition IN ('SERVICEABLE', 'UNDER_REPAIR', 'UNSERVICEABLE')),
    maintenance_note  TEXT,
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_weapons_station_status ON weapons (station_id, status);

-- Each issue is a row that is later closed by its return. Rounds are recorded
-- both ways so a shortfall is a stored fact, not something re-derived.
CREATE TABLE IF NOT EXISTS weapon_issuances (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    weapon_id         UUID NOT NULL REFERENCES weapons(id),
    issued_to         UUID NOT NULL REFERENCES users(id),
    issued_by         UUID NOT NULL REFERENCES users(id),
    purpose           TEXT NOT NULL,
    rounds_issued     INTEGER NOT NULL DEFAULT 0 CHECK (rounds_issued >= 0),
    expected_return   TIMESTAMPTZ,
    issued_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    returned_at       TIMESTAMPTZ,
    received_by       UUID REFERENCES users(id),
    rounds_returned   INTEGER CHECK (rounds_returned >= 0),
    return_condition  VARCHAR(20)
                      CHECK (return_condition IN ('SERVICEABLE', 'UNDER_REPAIR', 'UNSERVICEABLE')),
    return_note       TEXT,
    CONSTRAINT weapon_issuance_return_complete CHECK (
        (returned_at IS NULL AND received_by IS NULL AND rounds_returned IS NULL)
        OR (returned_at IS NOT NULL AND received_by IS NOT NULL AND rounds_returned IS NOT NULL)
    ),
    CONSTRAINT weapon_issuance_rounds_accounted CHECK (rounds_returned IS NULL OR rounds_returned <= rounds_issued)
);
-- A weapon can be out with only one officer at a time. Enforced by the
-- database so two armourers issuing the same weapon at once cannot both win.
CREATE UNIQUE INDEX IF NOT EXISTS uq_weapon_open_issuance
    ON weapon_issuances (weapon_id) WHERE returned_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_weapon_issuances_weapon ON weapon_issuances (weapon_id, issued_at DESC);
CREATE INDEX IF NOT EXISTS idx_weapon_issuances_officer ON weapon_issuances (issued_to, issued_at DESC);

-- ----------------------------------------------------------------- lookouts --

CREATE TABLE IF NOT EXISTS lookouts (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    lookout_number    VARCHAR(40)  NOT NULL UNIQUE,
    lookout_type      VARCHAR(20)  NOT NULL
                      CHECK (lookout_type IN ('WANTED', 'MISSING', 'STOLEN_VEHICLE', 'SUSPECT', 'WITNESS')),
    subject           VARCHAR(255) NOT NULL,
    description       TEXT NOT NULL,
    details           JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority          VARCHAR(10)  NOT NULL DEFAULT 'NORMAL'
                      CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'CRITICAL')),
    status            VARCHAR(10)  NOT NULL DEFAULT 'ACTIVE'
                      CHECK (status IN ('ACTIVE', 'LOCATED', 'CLOSED')),
    fir_id            UUID REFERENCES firs(id),
    station_id        UUID NOT NULL REFERENCES stations(id),
    issued_by         UUID NOT NULL REFERENCES users(id),
    issued_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at       TIMESTAMPTZ,
    resolved_by       UUID REFERENCES users(id),
    resolution_note   TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT lookout_resolution_complete CHECK (
        (status = 'ACTIVE' AND resolved_at IS NULL)
        OR (status <> 'ACTIVE' AND resolved_at IS NOT NULL AND resolved_by IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_lookouts_status_priority ON lookouts (status, priority, issued_at DESC);

-- A sighting is a report, not a fact, until an officer other than the
-- reporter verifies it.
CREATE TABLE IF NOT EXISTS lookout_sightings (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    lookout_id        UUID NOT NULL REFERENCES lookouts(id) ON DELETE CASCADE,
    reported_by       UUID NOT NULL REFERENCES users(id),
    location          TEXT NOT NULL,
    latitude          DOUBLE PRECISION,
    longitude         DOUBLE PRECISION,
    sighted_at        TIMESTAMPTZ NOT NULL,
    details           TEXT NOT NULL DEFAULT '',
    verified_by       UUID REFERENCES users(id),
    verified_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sighting_verification_complete CHECK ((verified_by IS NULL) = (verified_at IS NULL)),
    CONSTRAINT sighting_verified_independently CHECK (verified_by IS NULL OR verified_by <> reported_by),
    CONSTRAINT sighting_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_lookout_sightings_lookout ON lookout_sightings (lookout_id, sighted_at DESC);
