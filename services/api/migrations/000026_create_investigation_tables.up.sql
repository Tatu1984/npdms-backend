-- Phase 01 — Investigation Copilot
--
-- The investigation workspace binds an FIR or case to the working material an
-- investigating officer accumulates: persons, a chronology, evidence links,
-- tasks, recorded discrepancies and identified gaps.
--
-- AI is deliberately absent here. Every row is created by an officer or by a
-- deterministic rule; the `origin` and `review_state` columns exist so that AI
-- suggestions can be added later without reshaping the tables.

CREATE TABLE IF NOT EXISTS investigation_workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_number VARCHAR(80) UNIQUE NOT NULL,
    fir_id UUID REFERENCES firs(id) ON DELETE SET NULL,
    case_id UUID REFERENCES cases(id) ON DELETE SET NULL,

    title TEXT NOT NULL,
    title_bn TEXT,
    offence TEXT,
    offence_bn TEXT,
    sections TEXT[] DEFAULT '{}',

    station_id UUID REFERENCES stations(id) ON DELETE SET NULL,
    io_id UUID REFERENCES users(id) ON DELETE SET NULL,
    supervisor_id UUID REFERENCES users(id) ON DELETE SET NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'supervisory-review', 'chargesheet', 'closed')),
    priority VARCHAR(20) NOT NULL DEFAULT 'medium'
        CHECK (priority IN ('low', 'medium', 'high', 'critical')),

    registered_on DATE NOT NULL DEFAULT CURRENT_DATE,
    next_court_date DATE,
    closed_at TIMESTAMPTZ,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_station ON investigation_workspaces(station_id);
CREATE INDEX IF NOT EXISTS idx_ws_io ON investigation_workspaces(io_id);
CREATE INDEX IF NOT EXISTS idx_ws_status ON investigation_workspaces(status);
CREATE INDEX IF NOT EXISTS idx_ws_fir ON investigation_workspaces(fir_id);

-- Persons of interest -------------------------------------------------------
-- Deliberately separate from `accused` and `witnesses`: a workspace person may
-- be neither yet, and the role changes as the investigation develops.
CREATE TABLE IF NOT EXISTS workspace_persons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    name_bn TEXT,
    aliases TEXT[] DEFAULT '{}',
    role VARCHAR(20) NOT NULL DEFAULT 'suspect'
        CHECK (role IN ('accused', 'suspect', 'witness', 'complainant', 'victim')),
    age INTEGER,
    gender VARCHAR(20),
    address TEXT,
    address_bn TEXT,
    phone VARCHAR(30),
    vehicles TEXT[] DEFAULT '{}',
    risk_note TEXT,

    statements_count INTEGER NOT NULL DEFAULT 0,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_persons_ws ON workspace_persons(workspace_id);
CREATE INDEX IF NOT EXISTS idx_ws_persons_phone ON workspace_persons(phone);

-- Chronology ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workspace_timeline (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,

    occurred_at TIMESTAMPTZ NOT NULL,
    title TEXT NOT NULL,
    title_bn TEXT,
    detail TEXT,
    detail_bn TEXT,
    kind VARCHAR(20) NOT NULL DEFAULT 'incident'
        CHECK (kind IN ('movement', 'communication', 'incident', 'transaction', 'detection', 'report')),

    -- 'officer' now; 'derived' for rule output; 'ai' reserved for later phases.
    origin VARCHAR(10) NOT NULL DEFAULT 'officer'
        CHECK (origin IN ('officer', 'derived', 'ai')),
    confidence NUMERIC(4,3),
    review_state VARCHAR(10) NOT NULL DEFAULT 'accepted'
        CHECK (review_state IN ('pending', 'accepted', 'rejected')),
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_timeline_ws ON workspace_timeline(workspace_id, occurred_at);

-- What a timeline entry, contradiction or gap rests on.
CREATE TABLE IF NOT EXISTS workspace_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,

    -- Polymorphic parent: exactly one of these is set.
    timeline_id UUID REFERENCES workspace_timeline(id) ON DELETE CASCADE,
    contradiction_id UUID,
    gap_id UUID,

    label TEXT NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'document'
        CHECK (source_type IN ('document', 'statement', 'cctv', 'call', 'transaction', 'forensic', 'circular')),
    locator TEXT,
    evidence_id UUID REFERENCES evidence(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_sources_timeline ON workspace_sources(timeline_id);
CREATE INDEX IF NOT EXISTS idx_ws_sources_contradiction ON workspace_sources(contradiction_id);
CREATE INDEX IF NOT EXISTS idx_ws_sources_gap ON workspace_sources(gap_id);

-- Recorded discrepancies ----------------------------------------------------
CREATE TABLE IF NOT EXISTS workspace_contradictions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,

    title TEXT NOT NULL,
    title_bn TEXT,

    statement_a_label TEXT NOT NULL,
    statement_a_claim TEXT NOT NULL,
    statement_b_label TEXT NOT NULL,
    statement_b_claim TEXT NOT NULL,

    severity VARCHAR(20) NOT NULL DEFAULT 'medium'
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    origin VARCHAR(10) NOT NULL DEFAULT 'officer'
        CHECK (origin IN ('officer', 'derived', 'ai')),
    confidence NUMERIC(4,3),
    review_state VARCHAR(10) NOT NULL DEFAULT 'pending'
        CHECK (review_state IN ('pending', 'accepted', 'rejected')),
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    resolution_note TEXT,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ws_contradictions_ws ON workspace_contradictions(workspace_id);

-- Investigation gaps --------------------------------------------------------
-- Populated by deterministic rules (see investigation_service.RecomputeGaps)
-- as well as by officers. No model involvement.
CREATE TABLE IF NOT EXISTS workspace_gaps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,

    -- Stable key for rule-derived gaps so a recompute updates instead of duplicating.
    rule_key VARCHAR(80),

    title TEXT NOT NULL,
    title_bn TEXT,
    detail TEXT,
    detail_bn TEXT,
    kind VARCHAR(20) NOT NULL DEFAULT 'document'
        CHECK (kind IN ('witness', 'forensic', 'timeline', 'document', 'digital', 'seizure')),
    severity VARCHAR(20) NOT NULL DEFAULT 'medium'
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),

    origin VARCHAR(10) NOT NULL DEFAULT 'derived'
        CHECK (origin IN ('officer', 'derived', 'ai')),
    status VARCHAR(12) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'closed', 'dismissed')),
    due_by DATE,
    closed_at TIMESTAMPTZ,
    closed_by UUID REFERENCES users(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ws_gaps_rule ON workspace_gaps(workspace_id, rule_key)
    WHERE rule_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ws_gaps_ws ON workspace_gaps(workspace_id, status);

-- Tasks ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS investigation_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,
    gap_id UUID REFERENCES workspace_gaps(id) ON DELETE SET NULL,
    contradiction_id UUID REFERENCES workspace_contradictions(id) ON DELETE SET NULL,

    title TEXT NOT NULL,
    title_bn TEXT,
    detail TEXT,
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    due_date DATE,
    priority VARCHAR(20) NOT NULL DEFAULT 'medium'
        CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    status VARCHAR(15) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'in-progress', 'blocked', 'done')),
    origin VARCHAR(10) NOT NULL DEFAULT 'officer'
        CHECK (origin IN ('officer', 'supervisor', 'derived', 'ai')),

    completion_note TEXT,
    completed_at TIMESTAMPTZ,
    completed_by UUID REFERENCES users(id) ON DELETE SET NULL,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inv_tasks_ws ON investigation_tasks(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_inv_tasks_assignee ON investigation_tasks(assignee_id, status);

-- Evidence attached to a workspace -----------------------------------------
CREATE TABLE IF NOT EXISTS workspace_evidence (
    workspace_id UUID NOT NULL REFERENCES investigation_workspaces(id) ON DELETE CASCADE,
    evidence_id UUID NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,
    note TEXT,
    linked_by UUID REFERENCES users(id) ON DELETE SET NULL,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, evidence_id)
);

-- updated_at triggers -------------------------------------------------------
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ws_updated ON investigation_workspaces;
CREATE TRIGGER trg_ws_updated BEFORE UPDATE ON investigation_workspaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_ws_persons_updated ON workspace_persons;
CREATE TRIGGER trg_ws_persons_updated BEFORE UPDATE ON workspace_persons
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_ws_timeline_updated ON workspace_timeline;
CREATE TRIGGER trg_ws_timeline_updated BEFORE UPDATE ON workspace_timeline
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_ws_contradictions_updated ON workspace_contradictions;
CREATE TRIGGER trg_ws_contradictions_updated BEFORE UPDATE ON workspace_contradictions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_ws_gaps_updated ON workspace_gaps;
CREATE TRIGGER trg_ws_gaps_updated BEFORE UPDATE ON workspace_gaps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS trg_inv_tasks_updated ON investigation_tasks;
CREATE TRIGGER trg_inv_tasks_updated BEFORE UPDATE ON investigation_tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE investigation_workspaces IS 'Phase 01 — investigation workspace binding a case to its working material';
COMMENT ON COLUMN workspace_gaps.rule_key IS 'Stable identifier for rule-derived gaps so recomputation updates rather than duplicates';
COMMENT ON COLUMN workspace_timeline.origin IS 'officer | derived (deterministic rule) | ai (reserved for a later phase)';
