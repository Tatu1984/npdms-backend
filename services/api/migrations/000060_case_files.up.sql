-- Phase 12 — Case File & Court Readiness.
--
-- A case file is assembled over an investigation workspace (Phase 01) and the
-- signed evidence register (Phase 02). It owns no persons, evidence or
-- chronology of its own: entries reference those records, or a document
-- uploaded through the evidence storage with its SHA-256.
--
-- Serial numbers are not stored. They are derived on read from the order the
-- court expects (FIR, statements, seizure lists, forensic reports, custody
-- records, charge-sheet, other) and frozen into a pack's manifest when the file
-- is submitted.

CREATE TABLE IF NOT EXISTS case_files (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_number   VARCHAR(40) NOT NULL UNIQUE,
    workspace_id  UUID NOT NULL UNIQUE REFERENCES investigation_workspaces(id),
    -- Incremented by every change to the index, charges, support matrix or
    -- witness facts. A pack records the version it froze.
    version       INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS case_file_entries (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_file_id      UUID NOT NULL REFERENCES case_files(id),
    category          VARCHAR(20) NOT NULL CHECK (category IN
                      ('FIR', 'STATEMENT', 'SEIZURE_LIST', 'FORENSIC_REPORT', 'CUSTODY_RECORD', 'CHARGESHEET', 'OTHER')),
    title             TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    source_kind       VARCHAR(10) NOT NULL CHECK (source_kind IN ('evidence', 'fir', 'forensic', 'upload')),
    evidence_id       UUID REFERENCES evidence(id),
    fir_id            UUID REFERENCES firs(id),
    forensic_id       UUID REFERENCES forensics(id),
    object_key        TEXT,
    original_filename TEXT,
    content_type      TEXT,
    file_size         BIGINT,
    storage_backend   VARCHAR(20),
    sha256            CHAR(64),
    witness_person_id UUID REFERENCES workspace_persons(id),
    statement_section VARCHAR(40),
    statement_date    DATE,
    document_date     DATE,
    added_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    added_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_at        TIMESTAMPTZ,
    removed_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    removal_reason    TEXT,

    -- Each source kind carries exactly its own reference.
    CONSTRAINT case_file_entry_source CHECK (
        (source_kind = 'evidence' AND evidence_id IS NOT NULL AND fir_id IS NULL AND forensic_id IS NULL AND object_key IS NULL)
     OR (source_kind = 'fir'      AND fir_id IS NOT NULL AND evidence_id IS NULL AND forensic_id IS NULL AND object_key IS NULL)
     OR (source_kind = 'forensic' AND forensic_id IS NOT NULL AND evidence_id IS NULL AND fir_id IS NULL AND object_key IS NULL)
     OR (source_kind = 'upload'   AND object_key IS NOT NULL AND sha256 IS NOT NULL AND evidence_id IS NULL AND fir_id IS NULL AND forensic_id IS NULL)
    ),
    CONSTRAINT case_file_entry_sha256 CHECK (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$'),
    -- A statement names its witness and the BNSS provision it was recorded under.
    CONSTRAINT case_file_entry_statement CHECK (
        category <> 'STATEMENT'
        OR (witness_person_id IS NOT NULL AND statement_section IS NOT NULL AND statement_date IS NOT NULL)
    ),
    CONSTRAINT case_file_entry_fir_category CHECK (source_kind <> 'fir' OR category = 'FIR'),
    CONSTRAINT case_file_entry_forensic_category CHECK (source_kind <> 'forensic' OR category = 'FORENSIC_REPORT'),
    CONSTRAINT case_file_entry_removal CHECK (
        (removed_at IS NULL AND removed_by IS NULL AND removal_reason IS NULL)
        OR (removed_at IS NOT NULL AND removal_reason IS NOT NULL AND length(btrim(removal_reason)) > 0)
    )
);
CREATE INDEX IF NOT EXISTS idx_case_file_entries_file ON case_file_entries (case_file_id) WHERE removed_at IS NULL;

CREATE TABLE IF NOT EXISTS case_file_charges (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_file_id  UUID NOT NULL REFERENCES case_files(id),
    section       VARCHAR(60) NOT NULL CHECK (length(btrim(section)) > 0),
    description   TEXT,
    added_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (case_file_id, section)
);

-- Evidence supporting a charge. The evidence must be attached to the file's
-- workspace: the composite key into workspace_evidence enforces it, and
-- detaching evidence from the workspace removes its support rows.
CREATE TABLE IF NOT EXISTS case_file_evidence_support (
    charge_id     UUID NOT NULL REFERENCES case_file_charges(id) ON DELETE CASCADE,
    case_file_id  UUID NOT NULL REFERENCES case_files(id),
    workspace_id  UUID NOT NULL,
    evidence_id   UUID NOT NULL,
    note          TEXT,
    linked_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    linked_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (charge_id, evidence_id),
    FOREIGN KEY (workspace_id, evidence_id) REFERENCES workspace_evidence (workspace_id, evidence_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS case_file_witness_facts (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    case_file_id       UUID NOT NULL REFERENCES case_files(id),
    person_id          UUID NOT NULL REFERENCES workspace_persons(id) ON DELETE CASCADE,
    fact               TEXT NOT NULL CHECK (length(btrim(fact)) > 0),
    statement_entry_id UUID REFERENCES case_file_entries(id),
    added_by           UUID REFERENCES users(id) ON DELETE SET NULL,
    added_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_case_file_facts_file ON case_file_witness_facts (case_file_id, person_id);

-- Every change writes a version with a snapshot of the whole index, so an
-- earlier state of the file stays readable.
CREATE TABLE IF NOT EXISTS case_file_versions (
    case_file_id  UUID NOT NULL REFERENCES case_files(id),
    version       INTEGER NOT NULL,
    summary       TEXT NOT NULL,
    snapshot      JSONB NOT NULL,
    changed_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    changed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (case_file_id, version)
);

-- A submission pack freezes the file. The manifest is stored as the exact text
-- that was hashed, so anyone can recompute manifest_sha256 from it.
CREATE TABLE IF NOT EXISTS case_file_packs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pack_number     VARCHAR(40) NOT NULL UNIQUE,
    case_file_id    UUID NOT NULL REFERENCES case_files(id),
    file_version    INTEGER NOT NULL,
    manifest_json   TEXT NOT NULL,
    manifest_sha256 CHAR(64) NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    blocking_findings INTEGER NOT NULL DEFAULT 0 CHECK (blocking_findings >= 0),
    status          VARCHAR(10) NOT NULL DEFAULT 'SUBMITTED' CHECK (status IN ('SUBMITTED', 'APPROVED', 'RETURNED')),
    submitted_by    UUID NOT NULL REFERENCES users(id),
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_by      UUID REFERENCES users(id),
    decided_at      TIMESTAMPTZ,
    return_reason   TEXT,
    CONSTRAINT case_file_pack_decision CHECK (
        (status = 'SUBMITTED' AND decided_by IS NULL AND decided_at IS NULL AND return_reason IS NULL)
     OR (status = 'APPROVED'  AND decided_by IS NOT NULL AND decided_at IS NOT NULL AND return_reason IS NULL)
     OR (status = 'RETURNED'  AND decided_by IS NOT NULL AND decided_at IS NOT NULL
                              AND return_reason IS NOT NULL AND length(btrim(return_reason)) > 0)
    ),
    -- The officer who submitted a file cannot approve or return it.
    CONSTRAINT case_file_pack_independent_decision CHECK (decided_by IS NULL OR decided_by <> submitted_by),
    CONSTRAINT case_file_pack_approval_clean CHECK (status <> 'APPROVED' OR blocking_findings = 0)
);
CREATE INDEX IF NOT EXISTS idx_case_file_packs_file ON case_file_packs (case_file_id, submitted_at DESC);
-- One undecided submission per file at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_case_file_open_pack ON case_file_packs (case_file_id) WHERE status = 'SUBMITTED';

-- A pack's frozen content cannot change, and a decision is final.
CREATE OR REPLACE FUNCTION case_file_pack_guard() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'case file packs are permanent';
    END IF;
    IF NEW.manifest_json IS DISTINCT FROM OLD.manifest_json
       OR NEW.manifest_sha256 IS DISTINCT FROM OLD.manifest_sha256
       OR NEW.file_version IS DISTINCT FROM OLD.file_version
       OR NEW.blocking_findings IS DISTINCT FROM OLD.blocking_findings
       OR NEW.case_file_id IS DISTINCT FROM OLD.case_file_id
       OR NEW.pack_number IS DISTINCT FROM OLD.pack_number
       OR NEW.submitted_by IS DISTINCT FROM OLD.submitted_by
       OR NEW.submitted_at IS DISTINCT FROM OLD.submitted_at THEN
        RAISE EXCEPTION 'a submitted case file pack is frozen';
    END IF;
    IF OLD.status <> 'SUBMITTED' THEN
        RAISE EXCEPTION 'a decided case file pack cannot change';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_case_file_pack_guard ON case_file_packs;
CREATE TRIGGER trg_case_file_pack_guard BEFORE UPDATE OR DELETE ON case_file_packs
    FOR EACH ROW EXECUTE FUNCTION case_file_pack_guard();

-- Versions are history.
CREATE OR REPLACE FUNCTION case_file_version_guard() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'case file versions are permanent';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_case_file_version_guard ON case_file_versions;
CREATE TRIGGER trg_case_file_version_guard BEFORE UPDATE OR DELETE ON case_file_versions
    FOR EACH ROW EXECUTE FUNCTION case_file_version_guard();
