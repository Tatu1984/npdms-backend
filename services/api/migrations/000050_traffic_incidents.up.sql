-- Phase 06 — Traffic Incident & Accident Reconstruction, functional without AI.
--
-- An incident is registered with its place, time and conditions; the vehicles
-- and persons involved, camera footage references, plate reads, signal-phase
-- observations and timeline facts are attached to it; a report is assembled
-- from those stored facts and approved by an officer other than its drafter.
--
-- The rule that shapes the schema: measured and estimated values are
-- different kinds of fact. Every timeline fact carries its provenance
-- (MEASURED, OBSERVED or ESTIMATED) and its source, and an estimate cannot be
-- stored without the method that produced it. A speed derived from camera
-- calibration is never stored — or shown — like a timestamp read from a
-- controller log.
--
-- Camera references are free identifiers here. The Phase 03 camera register
-- is built on a separate branch; the correlation joins it when both land.

CREATE TABLE IF NOT EXISTS traffic_incidents (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_number  VARCHAR(40)  NOT NULL UNIQUE,
    occurred_at      TIMESTAMPTZ  NOT NULL,
    location         TEXT         NOT NULL CHECK (btrim(location) <> ''),
    latitude         DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude        DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    station_id       UUID NOT NULL REFERENCES stations(id),
    fir_id           UUID REFERENCES firs(id),
    collision_type   VARCHAR(20)  NOT NULL
                     CHECK (collision_type IN ('HEAD_ON', 'REAR_END', 'SIDE_IMPACT', 'SIDESWIPE',
                                               'PEDESTRIAN', 'ROLLOVER', 'FIXED_OBJECT', 'OTHER')),
    road_condition   VARCHAR(20)  NOT NULL
                     CHECK (road_condition IN ('DRY', 'WET', 'WATERLOGGED', 'POTHOLED', 'UNDER_REPAIR', 'OTHER')),
    weather          VARCHAR(20)  NOT NULL
                     CHECK (weather IN ('CLEAR', 'RAIN', 'HEAVY_RAIN', 'FOG', 'HAZE', 'OTHER')),
    lighting         VARCHAR(20)  NOT NULL
                     CHECK (lighting IN ('DAYLIGHT', 'DUSK_DAWN', 'DARK_LIT', 'DARK_UNLIT')),
    description      TEXT         NOT NULL CHECK (btrim(description) <> ''),
    reported_by      UUID NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_traffic_incidents_occurred ON traffic_incidents (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_traffic_incidents_station ON traffic_incidents (station_id, occurred_at DESC);

-- Registration numbers are stored normalised (upper case, letters and digits
-- only) so WB-06-BC-2210 and WB06BC2210 are the same vehicle.
CREATE TABLE IF NOT EXISTS traffic_incident_vehicles (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id          UUID NOT NULL REFERENCES traffic_incidents(id),
    registration_number  VARCHAR(20) NOT NULL CHECK (registration_number ~ '^[A-Z0-9]{4,20}$'),
    vehicle_type         VARCHAR(20) NOT NULL
                         CHECK (vehicle_type IN ('TWO_WHEELER', 'THREE_WHEELER', 'E_RICKSHAW', 'CAR', 'TAXI',
                                                 'BUS', 'LCV', 'TRUCK', 'BICYCLE', 'OTHER')),
    description          TEXT NOT NULL DEFAULT '',
    driver_name          TEXT,
    created_by           UUID NOT NULL REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (incident_id, registration_number)
);

CREATE TABLE IF NOT EXISTS traffic_incident_persons (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id      UUID NOT NULL REFERENCES traffic_incidents(id),
    name             TEXT,  -- NULL when the person is not yet identified
    role             VARCHAR(20) NOT NULL
                     CHECK (role IN ('DRIVER', 'PASSENGER', 'PEDESTRIAN', 'CYCLIST', 'OTHER')),
    vehicle_id       UUID REFERENCES traffic_incident_vehicles(id),
    injury_severity  VARCHAR(10) NOT NULL CHECK (injury_severity IN ('FATAL', 'GRIEVOUS', 'MINOR', 'NONE')),
    hospital         TEXT,
    created_by       UUID NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Drivers and passengers belong to a vehicle; pedestrians do not.
    CONSTRAINT person_vehicle_matches_role CHECK (
        (role IN ('DRIVER', 'PASSENGER') AND vehicle_id IS NOT NULL)
        OR (role NOT IN ('DRIVER', 'PASSENGER'))
    )
);

CREATE TABLE IF NOT EXISTS traffic_incident_cameras (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id   UUID NOT NULL REFERENCES traffic_incidents(id),
    camera_ref    VARCHAR(80) NOT NULL CHECK (btrim(camera_ref) <> ''),
    camera_name   TEXT NOT NULL DEFAULT '',
    distance_m    INTEGER CHECK (distance_m >= 0),
    footage_from  TIMESTAMPTZ NOT NULL,
    footage_to    TIMESTAMPTZ NOT NULL,
    notes         TEXT NOT NULL DEFAULT '',
    created_by    UUID NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT camera_footage_window CHECK (footage_to > footage_from),
    UNIQUE (incident_id, camera_ref, footage_from)
);

CREATE TABLE IF NOT EXISTS traffic_incident_plate_reads (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id          UUID NOT NULL REFERENCES traffic_incidents(id),
    registration_number  VARCHAR(20) NOT NULL CHECK (registration_number ~ '^[A-Z0-9]{4,20}$'),
    read_at              TIMESTAMPTZ NOT NULL,
    location             TEXT NOT NULL CHECK (btrim(location) <> ''),
    camera_ref           VARCHAR(80),
    source               VARCHAR(20) NOT NULL CHECK (source IN ('ANPR_SYSTEM', 'CCTV_REVIEW', 'OFFICER')),
    source_detail        TEXT NOT NULL DEFAULT '',
    created_by           UUID NOT NULL REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A read from an ANPR system or from reviewed footage names its camera.
    CONSTRAINT plate_read_camera_named CHECK (source = 'OFFICER' OR btrim(COALESCE(camera_ref, '')) <> '')
);

CREATE TABLE IF NOT EXISTS traffic_incident_signal_phases (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id    UUID NOT NULL REFERENCES traffic_incidents(id),
    signal_ref     VARCHAR(80) NOT NULL CHECK (btrim(signal_ref) <> ''),
    approach       TEXT NOT NULL CHECK (btrim(approach) <> ''),
    phase          VARCHAR(10) NOT NULL CHECK (phase IN ('RED', 'AMBER', 'GREEN', 'FLASHING', 'OFF')),
    phase_from     TIMESTAMPTZ NOT NULL,
    phase_to       TIMESTAMPTZ,
    source         VARCHAR(20) NOT NULL
                   CHECK (source IN ('CONTROLLER_LOG', 'CCTV_REVIEW', 'OFFICER_OBSERVATION', 'WITNESS')),
    source_detail  TEXT NOT NULL CHECK (btrim(source_detail) <> ''),
    created_by     UUID NOT NULL REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT signal_phase_window CHECK (phase_to IS NULL OR phase_to > phase_from)
);

-- Timeline facts. A quantity (speed, distance) is a value with a unit; an
-- estimate may be a range, and must name the method that produced it.
CREATE TABLE IF NOT EXISTS traffic_incident_facts (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id  UUID NOT NULL REFERENCES traffic_incidents(id),
    occurred_at  TIMESTAMPTZ NOT NULL,
    description  TEXT NOT NULL CHECK (btrim(description) <> ''),
    provenance   VARCHAR(10) NOT NULL CHECK (provenance IN ('MEASURED', 'OBSERVED', 'ESTIMATED')),
    source       TEXT NOT NULL CHECK (btrim(source) <> ''),
    method       TEXT,
    vehicle_id   UUID REFERENCES traffic_incident_vehicles(id),
    quantity     VARCHAR(10) CHECK (quantity IN ('SPEED', 'DISTANCE', 'DURATION')),
    value        NUMERIC(12, 3),
    value_low    NUMERIC(12, 3),
    value_high   NUMERIC(12, 3),
    unit         VARCHAR(10) CHECK (unit IN ('km/h', 'm', 's')),
    created_by   UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fact_estimate_has_method CHECK (provenance <> 'ESTIMATED' OR btrim(COALESCE(method, '')) <> ''),
    -- A quantity has a unit and either a value or a range, never both.
    CONSTRAINT fact_quantity_complete CHECK (
        (quantity IS NULL AND value IS NULL AND value_low IS NULL AND value_high IS NULL AND unit IS NULL)
        OR (quantity IS NOT NULL AND unit IS NOT NULL AND (
              (value IS NOT NULL AND value_low IS NULL AND value_high IS NULL)
           OR (value IS NULL AND value_low IS NOT NULL AND value_high IS NOT NULL AND value_high >= value_low)))
    ),
    -- A range is an estimate by nature; a measured or observed value is a point.
    CONSTRAINT fact_range_only_estimated CHECK (value_low IS NULL OR provenance = 'ESTIMATED')
);
CREATE INDEX IF NOT EXISTS idx_traffic_incident_facts_incident ON traffic_incident_facts (incident_id, occurred_at);

-- Reports: DRAFT → SUBMITTED → APPROVED, or SUBMITTED → RETURNED → (edited) DRAFT.
-- The content is frozen at submission as a snapshot of the stored facts, with
-- its digest, so an approved report cannot drift from what was approved.
CREATE TABLE IF NOT EXISTS traffic_incident_reports (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id       UUID NOT NULL REFERENCES traffic_incidents(id),
    report_number     VARCHAR(40) NOT NULL UNIQUE,
    status            VARCHAR(10) NOT NULL DEFAULT 'DRAFT'
                      CHECK (status IN ('DRAFT', 'SUBMITTED', 'RETURNED', 'APPROVED')),
    findings          TEXT NOT NULL DEFAULT '',
    drafted_by        UUID NOT NULL REFERENCES users(id),
    snapshot          JSONB,
    snapshot_sha256   VARCHAR(64),
    submitted_at      TIMESTAMPTZ,
    reviewed_by       UUID REFERENCES users(id),
    reviewed_at       TIMESTAMPTZ,
    return_reason     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT report_submitted_has_snapshot CHECK (
        status = 'DRAFT' OR (snapshot IS NOT NULL AND snapshot_sha256 IS NOT NULL AND submitted_at IS NOT NULL)
    ),
    CONSTRAINT report_review_complete CHECK (
        (status IN ('DRAFT', 'SUBMITTED') AND reviewed_by IS NULL)
        OR (status IN ('APPROVED', 'RETURNED') AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    ),
    CONSTRAINT report_reviewed_independently CHECK (reviewed_by IS NULL OR reviewed_by <> drafted_by),
    CONSTRAINT report_return_has_reason CHECK (status <> 'RETURNED' OR btrim(COALESCE(return_reason, '')) <> '')
);
-- At most one report in progress per incident.
CREATE UNIQUE INDEX IF NOT EXISTS uq_traffic_incident_open_report
    ON traffic_incident_reports (incident_id) WHERE status IN ('DRAFT', 'SUBMITTED', 'RETURNED');

-- An approved report is final.
CREATE OR REPLACE FUNCTION traffic_report_approved_is_final() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status = 'APPROVED' THEN
        RAISE EXCEPTION 'traffic incident report % is approved and cannot be changed', OLD.report_number
            USING ERRCODE = 'check_violation';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS traffic_report_approved_final ON traffic_incident_reports;
CREATE TRIGGER traffic_report_approved_final
    BEFORE UPDATE OR DELETE ON traffic_incident_reports
    FOR EACH ROW EXECUTE FUNCTION traffic_report_approved_is_final();
