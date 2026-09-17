-- The fleet's own records: where a vehicle went, what it was fuelled with,
-- and what was done to it.
--
-- `vehicles` held a current state — an odometer reading, a fuel level, a
-- driver, a maintenance note — and no history of how it got there. A station
-- could see that a jeep reads 84,120 km and nothing about the journey that
-- took it there, who drove it, or which of two officers put four thousand
-- rupees of diesel in it. Those are the questions a fleet is audited on, and
-- the ones a transfer or an accident enquiry starts with.
--
-- Three registers, each placed at a station so the force boundary covers them
-- like everything else, and each with its rules in the database rather than
-- only in the service.

-- ------------------------------------------------- trips -------------------

CREATE TABLE IF NOT EXISTS vehicle_trips (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id     UUID NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    station_id     UUID REFERENCES stations(id),

    driver_id      UUID REFERENCES users(id),
    authorised_by  UUID REFERENCES users(id),
    purpose        TEXT NOT NULL,
    destination    TEXT,

    started_at     TIMESTAMPTZ NOT NULL,
    ended_at       TIMESTAMPTZ,

    start_odometer INTEGER NOT NULL CHECK (start_odometer >= 0),
    end_odometer   INTEGER CHECK (end_odometer >= 0),

    note           TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- A journey cannot end before it began, and a vehicle cannot travel
    -- backwards. Both are the kind of entry a tired officer makes at the end
    -- of a shift, and both make every figure derived from the log wrong.
    CONSTRAINT vehicle_trips_ends_after_it_starts
        CHECK (ended_at IS NULL OR ended_at >= started_at),
    CONSTRAINT vehicle_trips_odometer_does_not_go_backwards
        CHECK (end_odometer IS NULL OR end_odometer >= start_odometer),
    -- A finished trip has both halves. A closing reading with no closing time
    -- is a trip nobody can say is over.
    CONSTRAINT vehicle_trips_closed_completely
        CHECK ((ended_at IS NULL) = (end_odometer IS NULL))
);

COMMENT ON TABLE vehicle_trips IS
    'Where a vehicle went and who drove it. One open trip per vehicle at a time.';

-- A vehicle is in one place at a time. Two open trips means one of them was
-- never closed, and the fleet's mileage has been wrong ever since.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_trips_one_open_per_vehicle
    ON vehicle_trips(vehicle_id) WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_vehicle_trips_vehicle ON vehicle_trips(vehicle_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_vehicle_trips_driver  ON vehicle_trips(driver_id, started_at DESC);

-- ------------------------------------------------- fuel --------------------

CREATE TABLE IF NOT EXISTS vehicle_fuel_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id    UUID NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    station_id    UUID REFERENCES stations(id),

    filled_at     TIMESTAMPTZ NOT NULL,
    litres        NUMERIC(8, 2) NOT NULL CHECK (litres > 0),
    cost_rupees   NUMERIC(10, 2) CHECK (cost_rupees IS NULL OR cost_rupees >= 0),
    odometer      INTEGER CHECK (odometer IS NULL OR odometer >= 0),

    vendor        TEXT,
    bill_number   TEXT,
    filled_by     UUID REFERENCES users(id),
    note          TEXT,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE vehicle_fuel_logs IS
    'Every fill: litres, cost, odometer and the officer who drew it. The fleet''s fuel account.';

-- The same bill cannot be claimed twice against the same vehicle.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_fuel_bill_once
    ON vehicle_fuel_logs(vehicle_id, bill_number)
    WHERE bill_number IS NOT NULL AND bill_number <> '';

CREATE INDEX IF NOT EXISTS idx_vehicle_fuel_vehicle ON vehicle_fuel_logs(vehicle_id, filled_at DESC);

-- ------------------------------------------------- maintenance -------------

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vehicle_maintenance_kind') THEN
        CREATE TYPE vehicle_maintenance_kind AS ENUM (
            'SERVICE', 'REPAIR', 'INSPECTION', 'TYRE', 'BODYWORK', 'OTHER');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS vehicle_maintenance (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id   UUID NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    station_id   UUID REFERENCES stations(id),

    kind         vehicle_maintenance_kind NOT NULL DEFAULT 'SERVICE',
    reported_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,

    odometer     INTEGER CHECK (odometer IS NULL OR odometer >= 0),
    garage       TEXT,
    cost_rupees  NUMERIC(10, 2) CHECK (cost_rupees IS NULL OR cost_rupees >= 0),
    description  TEXT NOT NULL,

    recorded_by  UUID REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT vehicle_maintenance_completed_after_reported
        CHECK (completed_at IS NULL OR completed_at >= reported_at)
);

COMMENT ON TABLE vehicle_maintenance IS
    'Service, repair and inspection history. An open row is work outstanding.';

CREATE INDEX IF NOT EXISTS idx_vehicle_maintenance_vehicle
    ON vehicle_maintenance(vehicle_id, reported_at DESC);
CREATE INDEX IF NOT EXISTS idx_vehicle_maintenance_open
    ON vehicle_maintenance(vehicle_id) WHERE completed_at IS NULL;

-- ------------------------------------------------- the vehicle follows -----

-- Closing a trip moves the vehicle's odometer, and a fill moves it too. The
-- current reading on `vehicles` was maintained by whoever remembered to, so it
-- drifted from the log that is supposed to explain it. It follows the records
-- now, and only ever forwards.
-- Two functions rather than one shared by both triggers. PL/pgSQL resolves a
-- record's fields against the table the trigger is on, so a single function
-- naming both NEW.end_odometer and NEW.odometer fails on whichever table lacks
-- the other — "record new has no field odometer" — however carefully the CASE
-- around it is written.
CREATE OR REPLACE FUNCTION vehicle_odometer_from_trip() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.end_odometer IS NOT NULL THEN
        UPDATE vehicles
        SET odometer_reading = GREATEST(COALESCE(odometer_reading, 0), NEW.end_odometer),
            updated_at = NOW()
        WHERE id = NEW.vehicle_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION vehicle_odometer_from_reading() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.odometer IS NOT NULL THEN
        UPDATE vehicles
        SET odometer_reading = GREATEST(COALESCE(odometer_reading, 0), NEW.odometer),
            updated_at = NOW()
        WHERE id = NEW.vehicle_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_trip_moves_the_odometer ON vehicle_trips;
CREATE TRIGGER trg_trip_moves_the_odometer
    AFTER INSERT OR UPDATE OF end_odometer ON vehicle_trips
    FOR EACH ROW EXECUTE FUNCTION vehicle_odometer_from_trip();

DROP TRIGGER IF EXISTS trg_fill_moves_the_odometer ON vehicle_fuel_logs;
CREATE TRIGGER trg_fill_moves_the_odometer
    AFTER INSERT ON vehicle_fuel_logs
    FOR EACH ROW EXECUTE FUNCTION vehicle_odometer_from_reading();

DROP FUNCTION IF EXISTS vehicle_odometer_follows_the_log();
