-- Phase 07 — Dispatch.
--
-- Incidents reported to the control room, the units sent to them, and every
-- step between. Units are not a new register: a dispatchable unit is either a
-- fleet vehicle (with its current driver as crew) or an on-duty officer from
-- the personnel register. These tables only record what was asked of them.
--
-- Severity is chosen by the operator from a stated four-level scale. Nothing
-- here computes a severity or predicts an arrival time.

CREATE TABLE IF NOT EXISTS dispatch_incidents (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_number   VARCHAR(40)  NOT NULL UNIQUE,
    source            VARCHAR(20)  NOT NULL
                      CHECK (source IN ('CONTROL_ROOM', 'PHONE_100', 'PHONE_112', 'APP', 'WALK_IN', 'OFFICER')),
    caller_name       VARCHAR(255),
    caller_phone      VARCHAR(20),
    description       TEXT NOT NULL,
    location_text     TEXT NOT NULL,
    latitude          DOUBLE PRECISION,
    longitude         DOUBLE PRECISION,
    station_id        UUID NOT NULL REFERENCES stations(id),
    received_at       TIMESTAMPTZ  NOT NULL,
    -- Operator classification. Both are set together, by a named officer.
    incident_type     VARCHAR(40),
    severity          VARCHAR(10)  CHECK (severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW')),
    classified_by     UUID REFERENCES users(id),
    classified_at     TIMESTAMPTZ,
    status            VARCHAR(12)  NOT NULL DEFAULT 'NEW'
                      CHECK (status IN ('NEW', 'CLASSIFIED', 'DISPATCHED', 'ON_SCENE', 'CLEARED', 'CLOSED')),
    -- Highest escalation level reached on the acknowledgement ladder (0 = none).
    escalation_level  SMALLINT     NOT NULL DEFAULT 0 CHECK (escalation_level BETWEEN 0 AND 3),
    outcome           VARCHAR(20)
                      CHECK (outcome IN ('RESOLVED_ON_SCENE', 'FIR_REGISTERED', 'REFERRED', 'FALSE_ALARM', 'NO_TRACE', 'DUPLICATE', 'OTHER')),
    outcome_note      TEXT,
    fir_id            UUID REFERENCES firs(id),
    closed_by         UUID REFERENCES users(id),
    closed_at         TIMESTAMPTZ,
    created_by        UUID NOT NULL REFERENCES users(id),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT dispatch_incident_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CONSTRAINT dispatch_incident_classification_complete CHECK (
        (incident_type IS NULL AND severity IS NULL AND classified_by IS NULL AND classified_at IS NULL)
        OR (incident_type IS NOT NULL AND severity IS NOT NULL AND classified_by IS NOT NULL AND classified_at IS NOT NULL)
    ),
    -- Nothing moves past NEW without a classification.
    CONSTRAINT dispatch_incident_classified_before_dispatch CHECK (status = 'NEW' OR severity IS NOT NULL OR status = 'CLOSED'),
    CONSTRAINT dispatch_incident_closure_complete CHECK (
        (status <> 'CLOSED' AND closed_at IS NULL AND outcome IS NULL)
        OR (status = 'CLOSED' AND closed_at IS NOT NULL AND closed_by IS NOT NULL AND outcome IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_dispatch_incidents_status ON dispatch_incidents (status, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_dispatch_incidents_station_received ON dispatch_incidents (station_id, received_at DESC);

CREATE TABLE IF NOT EXISTS dispatch_assignments (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id        UUID NOT NULL REFERENCES dispatch_incidents(id),
    unit_kind          VARCHAR(10) NOT NULL CHECK (unit_kind IN ('VEHICLE', 'OFFICER')),
    vehicle_id         UUID REFERENCES vehicles(id),
    -- For a vehicle, the driver at the moment of assignment; for an officer unit, the officer.
    officer_id         UUID REFERENCES users(id),
    status             VARCHAR(12) NOT NULL DEFAULT 'ASSIGNED'
                       CHECK (status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE', 'CLEARED', 'CANCELLED')),
    assigned_by        UUID NOT NULL REFERENCES users(id),
    assigned_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    distance_km        DOUBLE PRECISION,
    acknowledged_at    TIMESTAMPTZ,
    acknowledged_by    UUID REFERENCES users(id),
    on_scene_at        TIMESTAMPTZ,
    on_scene_by        UUID REFERENCES users(id),
    on_scene_alerted_at TIMESTAMPTZ,
    cleared_at         TIMESTAMPTZ,
    cleared_by         UUID REFERENCES users(id),
    cancelled_at       TIMESTAMPTZ,
    cancelled_by       UUID REFERENCES users(id),
    cancel_reason      TEXT,
    CONSTRAINT dispatch_assignment_unit_reference CHECK (
        (unit_kind = 'VEHICLE' AND vehicle_id IS NOT NULL)
        OR (unit_kind = 'OFFICER' AND vehicle_id IS NULL AND officer_id IS NOT NULL)
    ),
    -- Each stage carries its time and officer, and the times run forwards.
    CONSTRAINT dispatch_assignment_ack_complete CHECK ((acknowledged_at IS NULL) = (acknowledged_by IS NULL)),
    CONSTRAINT dispatch_assignment_scene_complete CHECK ((on_scene_at IS NULL) = (on_scene_by IS NULL)),
    CONSTRAINT dispatch_assignment_clear_complete CHECK ((cleared_at IS NULL) = (cleared_by IS NULL)),
    CONSTRAINT dispatch_assignment_cancel_complete CHECK (
        (cancelled_at IS NULL AND cancelled_by IS NULL AND cancel_reason IS NULL)
        OR (cancelled_at IS NOT NULL AND cancelled_by IS NOT NULL AND cancel_reason IS NOT NULL)
    ),
    CONSTRAINT dispatch_assignment_times_ordered CHECK (
        (acknowledged_at IS NULL OR acknowledged_at >= assigned_at)
        AND (on_scene_at IS NULL OR (acknowledged_at IS NOT NULL AND on_scene_at >= acknowledged_at))
        AND (cleared_at IS NULL OR (on_scene_at IS NOT NULL AND cleared_at >= on_scene_at))
    ),
    CONSTRAINT dispatch_assignment_status_matches_times CHECK (
        (status = 'ASSIGNED'     AND acknowledged_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'ACKNOWLEDGED' AND acknowledged_at IS NOT NULL AND on_scene_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'ON_SCENE'     AND on_scene_at IS NOT NULL AND cleared_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'CLEARED'      AND cleared_at IS NOT NULL AND cancelled_at IS NULL)
        OR (status = 'CANCELLED'    AND cancelled_at IS NOT NULL AND on_scene_at IS NULL)
    )
);
-- A unit can be committed to only one incident at a time. Enforced here so two
-- operators assigning the same unit at the same moment cannot both succeed.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_vehicle_active
    ON dispatch_assignments (vehicle_id) WHERE status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE');
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_officer_active
    ON dispatch_assignments (officer_id) WHERE status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE');
CREATE INDEX IF NOT EXISTS idx_dispatch_assignments_incident ON dispatch_assignments (incident_id, assigned_at);

-- Every step, including escalations raised by the stated rules (actor NULL).
CREATE TABLE IF NOT EXISTS dispatch_events (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id     UUID NOT NULL REFERENCES dispatch_incidents(id),
    assignment_id   UUID REFERENCES dispatch_assignments(id),
    event_type      VARCHAR(24) NOT NULL
                    CHECK (event_type IN ('INTAKE', 'CLASSIFIED', 'ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE', 'CLEARED',
                                          'ASSIGNMENT_CANCELLED', 'ESCALATED', 'ON_SCENE_OVERDUE', 'CLOSED')),
    level           SMALLINT,
    detail          TEXT NOT NULL,
    actor_id        UUID REFERENCES users(id),
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Several events can share a transaction's NOW(); seq gives them a stable order,
-- and clock_timestamp() a distinct time.
ALTER TABLE dispatch_events ADD COLUMN IF NOT EXISTS seq BIGSERIAL;
ALTER TABLE dispatch_events ALTER COLUMN occurred_at SET DEFAULT clock_timestamp();
CREATE INDEX IF NOT EXISTS idx_dispatch_events_incident ON dispatch_events (incident_id, seq);

-- The event log is a record of what happened; it is never rewritten.
CREATE OR REPLACE FUNCTION dispatch_events_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'dispatch_events is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS dispatch_events_no_update ON dispatch_events;
CREATE TRIGGER dispatch_events_no_update BEFORE UPDATE OR DELETE ON dispatch_events
    FOR EACH ROW EXECUTE FUNCTION dispatch_events_append_only();
