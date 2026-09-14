-- Phase 02: make custody signatures checkable and the custody records append-only.
--
-- Two claims in the plan did not hold:
--
--   * "A leg cannot be inserted, reordered or backdated without the signature
--     failing." Nothing ever verified a signature, and none could be verified:
--     the HMAC was computed over a timestamp taken in Go, while signed_at was
--     written by the database's NOW() a moment later, so the signed moment was
--     never stored. The payload also left out the sequence number, the previous
--     holder and the seal state, so those could be altered without detection.
--   * "Append-only access log." No trigger or rule stopped UPDATE or DELETE on
--     evidence_custody, evidence_access_log or evidence_integrity_checks.
--
-- signature_version marks legs signed with the version-2 payload (see
-- CustodyService.signCustody). Legs written before it are reported as legacy:
-- they carry a signature that cannot be re-derived, and saying so is more
-- honest than calling them valid.

ALTER TABLE evidence_custody ADD COLUMN IF NOT EXISTS signature_version SMALLINT;

-- One leg per position. Concurrent transfers previously read MAX(sequence)
-- without a lock and could both take the same number.
CREATE UNIQUE INDEX IF NOT EXISTS uq_custody_evidence_sequence
    ON evidence_custody (evidence_id, sequence_number)
    WHERE sequence_number IS NOT NULL;

CREATE OR REPLACE FUNCTION forbid_custody_record_change() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only: % is not allowed', TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['evidence_custody', 'evidence_access_log', 'evidence_integrity_checks'] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = t || '_append_only') THEN
            EXECUTE format(
                'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION forbid_custody_record_change()',
                t || '_append_only', t);
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = t || '_no_truncate') THEN
            EXECUTE format(
                'CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION forbid_custody_record_change()',
                t || '_no_truncate', t);
        END IF;
    END LOOP;
END $$;
