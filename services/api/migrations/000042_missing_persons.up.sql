-- Phase 04 — Missing & Vulnerable Persons.
--
-- `missing_person_reports` already exists and the citizen portal writes to it
-- (status REPORTED, number MIS/YYYY/NNNNN). It is extended rather than
-- duplicated, so a report a family files online and a report registered at the
-- station are the same record and follow the same workflow:
--
--   REPORTED   filed by a citizen, not yet taken up by an officer
--   SEARCHING  taken up (or registered at the station); the checklist runs
--   FOUND      closed as TRACED or RETURNED
--   CLOSED     closed as DECEASED or OTHER
--
-- FOUND and CLOSED are the values the citizen portal already reads, so its
-- tracking page and statistics keep working unchanged.

ALTER TABLE missing_person_reports
    ADD COLUMN IF NOT EXISTS source            VARCHAR(10) NOT NULL DEFAULT 'CITIZEN',
    ADD COLUMN IF NOT EXISTS registered_by     UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS vulnerabilities   TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS priority          VARCHAR(10) NOT NULL DEFAULT 'NORMAL',
    ADD COLUMN IF NOT EXISTS circumstances     TEXT,
    ADD COLUMN IF NOT EXISTS search_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS search_started_by UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS closure_outcome   VARCHAR(20),
    ADD COLUMN IF NOT EXISTS closed_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS closed_by         UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS closure_note      TEXT,
    ADD COLUMN IF NOT EXISTS lookout_id        UUID REFERENCES lookouts(id);

-- Rows written before this migration cannot satisfy the workflow constraints
-- below unless their state is made explicit. A closed report stays closed and
-- an open one stays open; only the missing detail is filled, and the note says
-- so rather than inventing an outcome.
UPDATE missing_person_reports
   SET status = 'SEARCHING'
 WHERE status NOT IN ('REPORTED', 'SEARCHING', 'FOUND', 'CLOSED');
UPDATE missing_person_reports
   SET closure_outcome = 'TRACED', closed_at = COALESCE(closed_at, found_date, updated_at, created_at),
       closure_note = COALESCE(closure_note, 'Recorded as found before closure outcomes existed')
 WHERE status = 'FOUND' AND closure_outcome IS NULL;
UPDATE missing_person_reports
   SET closure_outcome = 'OTHER', closed_at = COALESCE(closed_at, updated_at, created_at),
       closure_note = COALESCE(closure_note, 'Recorded as closed before closure outcomes existed')
 WHERE status = 'CLOSED' AND closure_outcome IS NULL;
UPDATE missing_person_reports
   SET search_started_at = COALESCE(search_started_at, created_at)
 WHERE status <> 'REPORTED' AND search_started_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_status_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_status_check
            CHECK (status IN ('REPORTED', 'SEARCHING', 'FOUND', 'CLOSED'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_source_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_source_check
            CHECK (source IN ('CITIZEN', 'OFFICER'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_priority_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_priority_check
            CHECK (priority IN ('NORMAL', 'HIGH', 'CRITICAL'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_age_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_age_check
            CHECK (age BETWEEN 0 AND 130);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_vulnerabilities_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_vulnerabilities_check
            CHECK (vulnerabilities <@ ARRAY['CHILD', 'ELDERLY', 'DISABILITY', 'MENTAL_HEALTH', 'TRAFFICKING_RISK']::text[]);
    END IF;
    -- A minor is always flagged as a child; the flag cannot be dropped.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_minor_flagged') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_minor_flagged
            CHECK (age >= 18 OR 'CHILD' = ANY (vulnerabilities));
    END IF;
    -- Any vulnerability raises priority; a child or trafficking risk is critical.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_vulnerability_raises_priority') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_vulnerability_raises_priority
            CHECK (
                (NOT (vulnerabilities && ARRAY['CHILD', 'TRAFFICKING_RISK']::text[]) OR priority = 'CRITICAL')
                AND (cardinality(vulnerabilities) = 0 OR priority <> 'NORMAL')
            );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_closure_outcome_check') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_closure_outcome_check
            CHECK (closure_outcome IN ('TRACED', 'RETURNED', 'DECEASED', 'OTHER'));
    END IF;
    -- Closure is complete or absent, and the status agrees with the outcome.
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_closure_consistent') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_closure_consistent
            CHECK (
                (status IN ('REPORTED', 'SEARCHING') AND closure_outcome IS NULL)
                OR (status = 'FOUND' AND closure_outcome IN ('TRACED', 'RETURNED') AND closed_at IS NOT NULL AND closure_note IS NOT NULL)
                OR (status = 'CLOSED' AND closure_outcome IN ('DECEASED', 'OTHER') AND closed_at IS NOT NULL AND closure_note IS NOT NULL)
            );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'mpr_search_started') THEN
        ALTER TABLE missing_person_reports ADD CONSTRAINT mpr_search_started
            CHECK (status = 'REPORTED' OR search_started_at IS NOT NULL);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_mpr_status_priority ON missing_person_reports (status, priority, last_seen_date DESC);

-- ---------------------------------------------------- first 24 hours checklist
-- Items are created when the search starts, each with a due time measured from
-- that moment. Overdue is computed on read, never stored.
CREATE TABLE IF NOT EXISTS missing_person_checklist (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id     UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    item_code     VARCHAR(40) NOT NULL,
    label         TEXT NOT NULL,
    sequence      INTEGER NOT NULL,
    due_at        TIMESTAMPTZ NOT NULL,
    completed_at  TIMESTAMPTZ,
    completed_by  UUID REFERENCES users(id),
    note          TEXT,
    UNIQUE (report_id, item_code),
    CONSTRAINT mpc_completion_complete CHECK ((completed_at IS NULL) = (completed_by IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_mpc_report ON missing_person_checklist (report_id, sequence);

-- ------------------------------------------------------------------ sightings
-- A sighting is a report until an officer other than the reporter verifies or
-- rejects it. Movement reconstruction uses verified sightings only.
CREATE TABLE IF NOT EXISTS missing_person_sightings (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id     UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    reported_by   UUID NOT NULL REFERENCES users(id),
    source        VARCHAR(20) NOT NULL
                  CHECK (source IN ('OFFICER_OBSERVATION', 'PUBLIC_TIP', 'CCTV_REVIEW', 'OTHER')),
    location      TEXT NOT NULL,
    latitude      DOUBLE PRECISION,
    longitude     DOUBLE PRECISION,
    sighted_at    TIMESTAMPTZ NOT NULL,
    details       TEXT NOT NULL DEFAULT '',
    decision      VARCHAR(10) CHECK (decision IN ('VERIFIED', 'REJECTED')),
    decided_by    UUID REFERENCES users(id),
    decided_at    TIMESTAMPTZ,
    decision_note TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT mps_decision_complete CHECK ((decision IS NULL) = (decided_by IS NULL) AND (decision IS NULL) = (decided_at IS NULL)),
    CONSTRAINT mps_decided_independently CHECK (decided_by IS NULL OR decided_by <> reported_by),
    CONSTRAINT mps_rejection_explained CHECK (decision IS DISTINCT FROM 'REJECTED' OR (decision_note IS NOT NULL AND btrim(decision_note) <> '')),
    CONSTRAINT mps_coordinates_paired CHECK ((latitude IS NULL) = (longitude IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_mps_report_time ON missing_person_sightings (report_id, sighted_at);

-- ----------------------------------------------------- family communication
CREATE TABLE IF NOT EXISTS missing_person_family_contacts (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id     UUID NOT NULL REFERENCES missing_person_reports(id) ON DELETE CASCADE,
    officer_id    UUID NOT NULL REFERENCES users(id),
    direction     VARCHAR(10) NOT NULL CHECK (direction IN ('OUTBOUND', 'INBOUND')),
    channel       VARCHAR(20) NOT NULL
                  CHECK (channel IN ('IN_PERSON', 'PHONE', 'SMS', 'WHATSAPP', 'EMAIL', 'LETTER')),
    contact_name  VARCHAR(255) NOT NULL,
    summary       TEXT NOT NULL CHECK (btrim(summary) <> ''),
    contacted_at  TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mpfc_report_time ON missing_person_family_contacts (report_id, contacted_at DESC);

-- Report numbers come from the shared counter (000033), so citizen-filed and
-- station-registered reports share one sequence and cannot collide. Seed above
-- any number already issued by the old count-based generator.
INSERT INTO record_counters (scope, year, last_value)
SELECT 'MIS', (m)[1]::int, MAX((m)[2]::bigint)
  FROM (SELECT regexp_match(report_number, '^MIS/(\d{4})/(\d+)$') AS m FROM missing_person_reports) s
 WHERE m IS NOT NULL
 GROUP BY (m)[1]
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);
