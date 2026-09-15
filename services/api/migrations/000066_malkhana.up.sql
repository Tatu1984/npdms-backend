-- Phase 14 — Malkhana / seized property.
--
-- A register of property seized against an FIR or case, where it is kept, the
-- seal it is kept under, every movement out of the malkhana and back, and its
-- disposal under a court order recorded in the system.
--
-- The rules are held here as well as in the service:
--   * an item is kept in a named storage location while it is in the malkhana;
--   * only one movement may be open for an item at a time;
--   * nothing leaves the malkhana while its seal is recorded as broken, and a
--     broken seal is cleared only by an SHO-rank officer recording a reason;
--   * disposal requires a court order for the same case, and destroying
--     narcotics requires a witness of DSP rank or above;
--   * seal checks and the property history cannot be edited or deleted.
--
-- The custody record is the hash-chained audit trail every change is written
-- to. External anchoring is a later layer and nothing here claims it.

-- --------------------------------------------------------- storage locations --

CREATE TABLE IF NOT EXISTS malkhana_locations (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    station_id  UUID NOT NULL REFERENCES stations(id),
    room        VARCHAR(60)  NOT NULL,
    rack        VARCHAR(60)  NOT NULL,
    shelf       VARCHAR(60)  NOT NULL DEFAULT '',
    notes       TEXT,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT malkhana_location_named CHECK (btrim(room) <> '' AND btrim(rack) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_malkhana_location
    ON malkhana_locations (station_id, lower(room), lower(rack), lower(shelf));

-- ------------------------------------------------------------ property items --

CREATE TABLE IF NOT EXISTS property_items (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    property_number      VARCHAR(40)  NOT NULL UNIQUE,
    station_id           UUID NOT NULL REFERENCES stations(id),
    fir_id               UUID REFERENCES firs(id),
    case_id              UUID REFERENCES cases(id),
    evidence_id          UUID REFERENCES evidence(id),
    category             VARCHAR(20)  NOT NULL
                         CHECK (category IN ('CASH', 'JEWELLERY', 'NARCOTICS', 'ARMS', 'VEHICLE',
                                             'ELECTRONICS', 'DOCUMENTS', 'OTHER')),
    description          TEXT NOT NULL,
    quantity             NUMERIC(14, 3) NOT NULL CHECK (quantity > 0),
    unit                 VARCHAR(20)  NOT NULL,
    weight_grams         NUMERIC(14, 3) CHECK (weight_grams IS NULL OR weight_grams > 0),
    value_paise          BIGINT CHECK (value_paise IS NULL OR value_paise >= 0),
    seized_at            TIMESTAMPTZ NOT NULL,
    seized_place         TEXT NOT NULL,
    seized_by            UUID NOT NULL REFERENCES users(id),
    seizure_memo_ref     VARCHAR(120) NOT NULL,
    status               VARCHAR(20)  NOT NULL DEFAULT 'IN_MALKHANA'
                         CHECK (status IN ('IN_MALKHANA', 'MOVED_OUT', 'DISPOSED')),
    location_id          UUID REFERENCES malkhana_locations(id),
    seal_number          VARCHAR(80)  NOT NULL,
    seal_state           VARCHAR(10)  NOT NULL DEFAULT 'INTACT'
                         CHECK (seal_state IN ('INTACT', 'BROKEN')),
    seal_broken_at       TIMESTAMPTZ,
    deposited_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deposited_by         UUID NOT NULL REFERENCES users(id),
    disposal_type        VARCHAR(20)
                         CHECK (disposal_type IS NULL OR disposal_type IN
                                ('RETURNED_TO_OWNER', 'AUCTIONED', 'DESTROYED', 'CONFISCATED')),
    disposal_order_id    UUID REFERENCES court_orders(id),
    disposal_witness_id  UUID REFERENCES users(id),
    disposal_note        TEXT,
    disposed_at          TIMESTAMPTZ,
    disposed_by          UUID REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT property_linked_to_record CHECK (fir_id IS NOT NULL OR case_id IS NOT NULL),
    CONSTRAINT property_kept_somewhere CHECK (status <> 'IN_MALKHANA' OR location_id IS NOT NULL),
    CONSTRAINT property_seal_broken_dated CHECK ((seal_state = 'BROKEN') = (seal_broken_at IS NOT NULL)),
    CONSTRAINT property_disposal_complete CHECK (
        (status = 'DISPOSED' AND disposal_type IS NOT NULL AND disposal_order_id IS NOT NULL
            AND disposed_at IS NOT NULL AND disposed_by IS NOT NULL)
        OR (status <> 'DISPOSED' AND disposal_type IS NULL AND disposal_order_id IS NULL
            AND disposed_at IS NULL AND disposed_by IS NULL AND disposal_witness_id IS NULL)
    ),
    CONSTRAINT property_narcotics_destruction_witnessed CHECK (
        NOT (category = 'NARCOTICS' AND disposal_type = 'DESTROYED') OR disposal_witness_id IS NOT NULL
    )
);
-- An evidence item from the Phase 02 register is linked, never duplicated.
CREATE UNIQUE INDEX IF NOT EXISTS uq_property_evidence ON property_items (evidence_id) WHERE evidence_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_property_station_status ON property_items (station_id, status);
CREATE INDEX IF NOT EXISTS idx_property_case ON property_items (case_id);
CREATE INDEX IF NOT EXISTS idx_property_fir ON property_items (fir_id);

-- Disposal rules the check constraints cannot express: the order must belong
-- to the item's case, and a narcotics destruction witness must hold DSP rank.
CREATE OR REPLACE FUNCTION property_disposal_rules() RETURNS TRIGGER AS $$
DECLARE
    order_case UUID;
    witness_role TEXT;
BEGIN
    IF NEW.status = 'DISPOSED' AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'DISPOSED') THEN
        SELECT case_id INTO order_case FROM court_orders WHERE id = NEW.disposal_order_id;
        IF NEW.case_id IS NULL OR order_case IS DISTINCT FROM NEW.case_id THEN
            RAISE EXCEPTION 'disposal order must be recorded against the same case as the property'
                USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.category = 'NARCOTICS' AND NEW.disposal_type = 'DESTROYED' THEN
            SELECT role::text INTO witness_role FROM users WHERE id = NEW.disposal_witness_id;
            IF witness_role IS NULL OR witness_role NOT IN ('DSP', 'SP', 'DIG', 'IG', 'SECRETARY', 'DGP') THEN
                RAISE EXCEPTION 'narcotics destruction must be witnessed by an officer of DSP rank or above'
                    USING ERRCODE = 'check_violation';
            END IF;
        END IF;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.status = 'DISPOSED' THEN
        RAISE EXCEPTION 'a disposed property record cannot be changed' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_property_disposal_rules ON property_items;
CREATE TRIGGER trg_property_disposal_rules
    BEFORE INSERT OR UPDATE ON property_items
    FOR EACH ROW EXECUTE FUNCTION property_disposal_rules();

-- --------------------------------------------------------------- seal checks --

CREATE TABLE IF NOT EXISTS property_seal_checks (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id       UUID NOT NULL REFERENCES property_items(id),
    checked_by    UUID NOT NULL REFERENCES users(id),
    checked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    seal_number   VARCHAR(80) NOT NULL,
    seal_state    VARCHAR(10) NOT NULL CHECK (seal_state IN ('INTACT', 'BROKEN')),
    context       VARCHAR(20) NOT NULL CHECK (context IN ('DEPOSIT', 'VERIFICATION', 'MOVEMENT_OUT',
                                                            'MOVEMENT_BACK', 'RESEAL')),
    note          TEXT,
    CONSTRAINT seal_break_explained CHECK (seal_state = 'INTACT' OR (note IS NOT NULL AND btrim(note) <> ''))
);
CREATE INDEX IF NOT EXISTS idx_property_seal_checks_item ON property_seal_checks (item_id, checked_at DESC);

-- ----------------------------------------------------------------- movements --

CREATE TABLE IF NOT EXISTS property_movements (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id              UUID NOT NULL REFERENCES property_items(id),
    movement_type        VARCHAR(30) NOT NULL
                         CHECK (movement_type IN ('FORENSIC_EXAMINATION', 'COURT_PRODUCTION',
                                                  'INTER_STATION', 'INTERIM_CUSTODY')),
    destination          VARCHAR(200) NOT NULL,
    destination_station_id UUID REFERENCES stations(id),
    court_hearing_id     UUID REFERENCES court_hearings(id),
    purpose              TEXT NOT NULL,
    authority_ref        VARCHAR(120) NOT NULL,
    handed_to            VARCHAR(200) NOT NULL,
    expected_return_at   TIMESTAMPTZ NOT NULL,
    moved_out_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    moved_out_by         UUID NOT NULL REFERENCES users(id),
    seal_number_out      VARCHAR(80) NOT NULL,
    returned_at          TIMESTAMPTZ,
    received_back_by     UUID REFERENCES users(id),
    returned_by_name     VARCHAR(200),
    seal_number_back     VARCHAR(80),
    seal_intact_back     BOOLEAN,
    return_note          TEXT,
    return_location_id   UUID REFERENCES malkhana_locations(id),
    CONSTRAINT movement_return_complete CHECK (
        (returned_at IS NULL AND received_back_by IS NULL AND seal_number_back IS NULL AND seal_intact_back IS NULL)
        OR (returned_at IS NOT NULL AND received_back_by IS NOT NULL AND seal_number_back IS NOT NULL
            AND seal_intact_back IS NOT NULL AND return_location_id IS NOT NULL)
    ),
    CONSTRAINT movement_return_after_out CHECK (returned_at IS NULL OR returned_at >= moved_out_at),
    CONSTRAINT movement_expected_after_out CHECK (expected_return_at > moved_out_at),
    CONSTRAINT movement_broken_return_noted CHECK (
        seal_intact_back IS DISTINCT FROM FALSE OR (return_note IS NOT NULL AND btrim(return_note) <> '')
    ),
    CONSTRAINT movement_inter_station_named CHECK (movement_type <> 'INTER_STATION' OR destination_station_id IS NOT NULL)
);
-- One open movement per item, held by the database so two officers recording
-- a movement at once cannot both succeed.
CREATE UNIQUE INDEX IF NOT EXISTS uq_property_open_movement ON property_movements (item_id) WHERE returned_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_property_movements_item ON property_movements (item_id, moved_out_at DESC);

-- A movement may only start from the malkhana with an intact seal, and once
-- returned it is closed for good.
CREATE OR REPLACE FUNCTION property_movement_rules() RETURNS TRIGGER AS $$
DECLARE
    item_status TEXT;
    item_seal   TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT status, seal_state INTO item_status, item_seal FROM property_items WHERE id = NEW.item_id FOR UPDATE;
        IF item_status IS DISTINCT FROM 'IN_MALKHANA' THEN
            RAISE EXCEPTION 'property is not in the malkhana' USING ERRCODE = 'check_violation';
        END IF;
        IF item_seal IS DISTINCT FROM 'INTACT' THEN
            RAISE EXCEPTION 'property seal is recorded as broken' USING ERRCODE = 'check_violation';
        END IF;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.returned_at IS NOT NULL THEN
            RAISE EXCEPTION 'a returned movement cannot be changed' USING ERRCODE = 'check_violation';
        END IF;
        IF NEW.item_id <> OLD.item_id OR NEW.movement_type <> OLD.movement_type OR NEW.destination <> OLD.destination
           OR NEW.purpose <> OLD.purpose OR NEW.authority_ref <> OLD.authority_ref OR NEW.handed_to <> OLD.handed_to
           OR NEW.moved_out_at <> OLD.moved_out_at OR NEW.moved_out_by <> OLD.moved_out_by
           OR NEW.seal_number_out <> OLD.seal_number_out OR NEW.expected_return_at <> OLD.expected_return_at THEN
            RAISE EXCEPTION 'only the return of a movement may be recorded' USING ERRCODE = 'check_violation';
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'property movements cannot be deleted' USING ERRCODE = 'check_violation';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_property_movement_rules ON property_movements;
CREATE TRIGGER trg_property_movement_rules
    BEFORE INSERT OR UPDATE OR DELETE ON property_movements
    FOR EACH ROW EXECUTE FUNCTION property_movement_rules();

-- ------------------------------------------------------------------ history --

CREATE TABLE IF NOT EXISTS property_events (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id     UUID NOT NULL REFERENCES property_items(id),
    event_type  VARCHAR(30) NOT NULL,
    actor_id    UUID REFERENCES users(id),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    summary     TEXT NOT NULL,
    details     JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_property_events_item ON property_events (item_id, occurred_at);

-- Seal checks and the property history are append-only.
CREATE OR REPLACE FUNCTION property_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'check_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_property_seal_checks_append_only ON property_seal_checks;
CREATE TRIGGER trg_property_seal_checks_append_only
    BEFORE UPDATE OR DELETE ON property_seal_checks
    FOR EACH ROW EXECUTE FUNCTION property_append_only();

DROP TRIGGER IF EXISTS trg_property_events_append_only ON property_events;
CREATE TRIGGER trg_property_events_append_only
    BEFORE UPDATE OR DELETE ON property_events
    FOR EACH ROW EXECUTE FUNCTION property_append_only();

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_property_seal_checks_no_truncate') THEN
        CREATE TRIGGER trg_property_seal_checks_no_truncate BEFORE TRUNCATE ON property_seal_checks
            FOR EACH STATEMENT EXECUTE FUNCTION property_append_only();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_property_events_no_truncate') THEN
        CREATE TRIGGER trg_property_events_no_truncate BEFORE TRUNCATE ON property_events
            FOR EACH STATEMENT EXECUTE FUNCTION property_append_only();
    END IF;
END $$;
