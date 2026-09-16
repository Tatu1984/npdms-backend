-- Anchoring the audit trail outside the platform (layer B0).
--
-- The trail is already tamper-evident: every entry carries the hash of its
-- own fields plus the previous entry's hash, so a removed or altered row
-- breaks the chain. What that cannot do is prove WHEN the chain existed — the
-- police hold the whole chain, and could in principle rebuild it. Anchoring
-- publishes a fingerprint of the chain outside the platform, so a court or an
-- auditor can see that a given state existed by a given time without trusting
-- the police at all.
--
-- What leaves the platform is one 32-byte Merkle root per batch. No record, no
-- personal data, no case content. The root reveals nothing about the entries
-- it commits to, and cannot be reversed into them.
--
-- audit_log_batches already existed, reserved for this, with the Merkle root
-- and the sequence range. What it assumed was a single blockchain. A batch is
-- better witnessed more than once, by mechanisms that fail differently, so the
-- receipts get their own table.

-- A batch may now be witnessed but not yet confirmed: an OpenTimestamps proof
-- exists within seconds, while the Bitcoin block that settles it takes hours.
DO $$
BEGIN
    ALTER TABLE audit_log_batches DROP CONSTRAINT IF EXISTS audit_log_batches_anchor_status_check;
    ALTER TABLE audit_log_batches ADD CONSTRAINT audit_log_batches_anchor_status_check
        CHECK (anchor_status IN ('PENDING', 'WITNESSED', 'CONFIRMED', 'FAILED'));
END $$;

ALTER TABLE audit_log_batches
    ADD COLUMN IF NOT EXISTS covers_from TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS covers_to   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES users(id);

CREATE TABLE IF NOT EXISTS anchor_receipts (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    batch_id     UUID NOT NULL REFERENCES audit_log_batches(id) ON DELETE CASCADE,

    -- How this batch was witnessed.
    --   RFC3161        a timestamping authority signs "this hash existed at
    --                  this time". Immediate, and verifiable offline against
    --                  the authority's certificate — but it is the authority's
    --                  word, so it is only as good as the authority.
    --   OPENTIMESTAMPS the hash is committed into the Bitcoin blockchain
    --                  through free public calendars. Nobody's word is
    --                  required, but settlement takes hours.
    witness      VARCHAR(20) NOT NULL CHECK (witness IN ('RFC3161', 'OPENTIMESTAMPS')),
    status       VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                 CHECK (status IN ('PENDING', 'WITNESSED', 'CONFIRMED', 'FAILED')),

    -- Who witnessed it: the TSA's URL, or the calendars that answered.
    authority    TEXT NOT NULL,
    -- The proof itself, exactly as the witness returned it: an RFC 3161 token,
    -- or an OpenTimestamps .ots proof. Both are verifiable by tools that know
    -- nothing about this platform, which is the point.
    proof        BYTEA,
    proof_sha256 CHAR(64),

    -- What the witness said about the time.
    witnessed_at TIMESTAMPTZ,
    -- Filled when a pending Bitcoin attestation is later upgraded to a block.
    confirmed_at TIMESTAMPTZ,
    block_height BIGINT,

    detail       TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_anchor_receipts_batch ON anchor_receipts (batch_id);
CREATE INDEX IF NOT EXISTS idx_anchor_receipts_pending
    ON anchor_receipts (witness, status) WHERE status IN ('PENDING', 'WITNESSED');

-- A receipt records what a witness said. It is not edited afterwards, except
-- to record that a pending attestation later settled.
CREATE OR REPLACE FUNCTION anchor_receipt_is_append_only() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.batch_id IS DISTINCT FROM OLD.batch_id
       OR NEW.witness IS DISTINCT FROM OLD.witness
       OR NEW.authority IS DISTINCT FROM OLD.authority
       OR (OLD.proof IS NOT NULL AND NEW.proof IS DISTINCT FROM OLD.proof
           AND OLD.status <> 'PENDING' AND NEW.status <> 'CONFIRMED')
       OR NEW.witnessed_at IS DISTINCT FROM OLD.witnessed_at AND OLD.witnessed_at IS NOT NULL THEN
        RAISE EXCEPTION 'a witness receipt is not rewritten; only its confirmation may be added'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'anchor_receipt_append_only';
    END IF;
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_anchor_receipt_append_only ON anchor_receipts;
CREATE TRIGGER trg_anchor_receipt_append_only
    BEFORE UPDATE ON anchor_receipts
    FOR EACH ROW EXECUTE FUNCTION anchor_receipt_is_append_only();

COMMENT ON TABLE anchor_receipts IS
    'What an outside witness said about a batch''s Merkle root. Proves that the root existed by a time — not that the records under it are true, and not that they were not altered before the batch was made.';
