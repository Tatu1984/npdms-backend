ALTER TABLE bail_sureties DROP CONSTRAINT IF EXISTS bail_sureties_verification_is_attributed;
DROP INDEX IF EXISTS idx_bail_sureties_named_once;
ALTER TABLE bail_sureties DROP CONSTRAINT IF EXISTS bail_sureties_bail_id_fkey;
ALTER TABLE bail_sureties
    ADD CONSTRAINT bail_sureties_bail_id_fkey
    FOREIGN KEY (bail_id) REFERENCES bail_applications(id) ON DELETE CASCADE;
