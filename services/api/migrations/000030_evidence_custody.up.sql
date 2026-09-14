-- Phase 02 — Evidence & Chain of Custody.
--
-- The base schema's `evidence` table records what an item is but nothing about
-- the file that represents it, and `evidence_custody` records a movement but
-- nothing that makes the movement provable. This migration adds both, plus the
-- access log that lets a court ask who has handled an item.
--
-- Everything here is ordinary cryptography and bookkeeping: SHA-256 over the
-- stored bytes, a signature bound to an officer's account, and an append-only
-- log. Anchoring these hashes to a chain is a later layer; the columns reserved
-- for it are marked.

-- --------------------------------------------------------------- the file --
ALTER TABLE evidence
    ADD COLUMN IF NOT EXISTS object_key TEXT,
    ADD COLUMN IF NOT EXISTS original_filename TEXT,
    ADD COLUMN IF NOT EXISTS content_type VARCHAR(120),
    ADD COLUMN IF NOT EXISTS file_size BIGINT,
    ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20),

    -- The digest taken as the bytes were written. Everything downstream compares
    -- against this value; it is never recomputed from a client-supplied figure.
    ADD COLUMN IF NOT EXISTS sha256 CHAR(64),
    ADD COLUMN IF NOT EXISTS hash_algorithm VARCHAR(20) DEFAULT 'SHA-256',
    ADD COLUMN IF NOT EXISTS uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS uploaded_at TIMESTAMPTZ,

    -- Result of the most recent integrity check, so a list can show status
    -- without re-reading every file.
    ADD COLUMN IF NOT EXISTS integrity_state VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (integrity_state IN ('pending', 'verified', 'broken')),
    ADD COLUMN IF NOT EXISTS last_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_verified_by UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Reserved for the anchoring layer. Nothing writes these yet.
    ADD COLUMN IF NOT EXISTS blockchain_anchor_tx VARCHAR(120),
    ADD COLUMN IF NOT EXISTS blockchain_anchor_time TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_evidence_sha256 ON evidence(sha256) WHERE sha256 IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_evidence_integrity ON evidence(integrity_state);

-- ------------------------------------------------------ custody movements --
ALTER TABLE evidence_custody
    ADD COLUMN IF NOT EXISTS from_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS to_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS seal_number VARCHAR(80),
    ADD COLUMN IF NOT EXISTS seal_intact BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS condition_note TEXT,

    -- The signature binds this movement to the officer who attested it, and to
    -- the state of the item at that moment: the hash is recorded again here, so
    -- a later mismatch can be traced to the leg of the chain it happened on.
    ADD COLUMN IF NOT EXISTS signed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS signed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS signature CHAR(64),
    ADD COLUMN IF NOT EXISTS hash_at_transfer CHAR(64),
    ADD COLUMN IF NOT EXISTS sequence_number INTEGER;

CREATE INDEX IF NOT EXISTS idx_custody_evidence_seq
    ON evidence_custody(evidence_id, sequence_number);

-- ----------------------------------------------------------- access log --
-- Append-only. A court is entitled to ask who has seen an item, and the answer
-- has to be complete, including the views that led nowhere.
CREATE TABLE IF NOT EXISTS evidence_access_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id UUID NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,

    action VARCHAR(20) NOT NULL
        CHECK (action IN ('viewed', 'downloaded', 'verified', 'transferred',
                          'uploaded', 'metadata_changed', 'court_verified')),
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_name TEXT,
    purpose TEXT,
    ip_address INET,
    user_agent TEXT,
    outcome VARCHAR(12) NOT NULL DEFAULT 'success'
        CHECK (outcome IN ('success', 'failure', 'denied')),
    detail TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evidence_access_item
    ON evidence_access_log(evidence_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_evidence_access_actor
    ON evidence_access_log(actor_id, created_at DESC);

REVOKE UPDATE, DELETE ON evidence_access_log FROM PUBLIC;

-- --------------------------------------------------- integrity check log --
-- Every verification is kept, not only the latest. "It verified last week and
-- fails today" is the question this table answers.
CREATE TABLE IF NOT EXISTS evidence_integrity_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id UUID NOT NULL REFERENCES evidence(id) ON DELETE CASCADE,

    expected_hash CHAR(64),
    computed_hash CHAR(64),
    matched BOOLEAN NOT NULL,
    size_bytes BIGINT,
    checked_by UUID REFERENCES users(id) ON DELETE SET NULL,
    note TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integrity_checks_item
    ON evidence_integrity_checks(evidence_id, created_at DESC);

COMMENT ON COLUMN evidence.sha256 IS
    'SHA-256 taken while the upload streamed to storage. Integrity checks compare a fresh read against this.';
COMMENT ON COLUMN evidence.blockchain_anchor_tx IS
    'Reserved for the anchoring layer. The hash chain is already tamper-evident without it.';
COMMENT ON TABLE evidence_access_log IS
    'Append-only record of who handled an evidence item and why.';
