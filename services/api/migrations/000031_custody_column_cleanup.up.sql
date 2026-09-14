-- Remove columns that 000030 added redundantly.
--
-- `evidence_custody.from_user` and `to_user` were already UUID references to
-- users; 000030 added from_user_id/to_user_id alongside them, which would have
-- left two places to record the same fact and a query joining the wrong one.
-- The original columns are used.

-- Carry across anything written in the short window the duplicates existed.
UPDATE evidence_custody SET from_user = from_user_id WHERE from_user IS NULL AND from_user_id IS NOT NULL;
UPDATE evidence_custody SET to_user   = to_user_id   WHERE to_user   IS NULL AND to_user_id   IS NOT NULL;

ALTER TABLE evidence_custody
    DROP COLUMN IF EXISTS from_user_id,
    DROP COLUMN IF EXISTS to_user_id;

-- Give the existing columns the foreign keys they lacked.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'evidence_custody_from_user_fkey') THEN
        ALTER TABLE evidence_custody
            ADD CONSTRAINT evidence_custody_from_user_fkey
            FOREIGN KEY (from_user) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'evidence_custody_to_user_fkey') THEN
        ALTER TABLE evidence_custody
            ADD CONSTRAINT evidence_custody_to_user_fkey
            FOREIGN KEY (to_user) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;
