DROP TABLE IF EXISTS evidence_integrity_checks;
DROP TABLE IF EXISTS evidence_access_log;

ALTER TABLE evidence_custody
    DROP COLUMN IF EXISTS from_user_id, DROP COLUMN IF EXISTS to_user_id,
    DROP COLUMN IF EXISTS seal_number, DROP COLUMN IF EXISTS seal_intact,
    DROP COLUMN IF EXISTS condition_note, DROP COLUMN IF EXISTS signed_by,
    DROP COLUMN IF EXISTS signed_at, DROP COLUMN IF EXISTS signature,
    DROP COLUMN IF EXISTS hash_at_transfer, DROP COLUMN IF EXISTS sequence_number;

ALTER TABLE evidence
    DROP COLUMN IF EXISTS object_key, DROP COLUMN IF EXISTS original_filename,
    DROP COLUMN IF EXISTS content_type, DROP COLUMN IF EXISTS file_size,
    DROP COLUMN IF EXISTS storage_backend, DROP COLUMN IF EXISTS sha256,
    DROP COLUMN IF EXISTS hash_algorithm, DROP COLUMN IF EXISTS uploaded_by,
    DROP COLUMN IF EXISTS uploaded_at, DROP COLUMN IF EXISTS integrity_state,
    DROP COLUMN IF EXISTS last_verified_at, DROP COLUMN IF EXISTS last_verified_by,
    DROP COLUMN IF EXISTS blockchain_anchor_tx, DROP COLUMN IF EXISTS blockchain_anchor_time;
