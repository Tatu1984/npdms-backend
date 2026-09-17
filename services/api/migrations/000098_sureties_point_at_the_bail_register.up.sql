-- The surety register referred to a table the platform does not use.
--
-- bail_sureties.bail_id has a foreign key to `bail_applications`, and every
-- bail record the platform writes goes to `bail`. The two are leftovers of the
-- same idea built twice: bail_applications is empty and nothing else in the
-- schema writes to it.
--
-- So no surety could ever be recorded — any insert failed the key — which is
-- why the table sat unreferenced by a single route since the bail module was
-- built. The failure would have said so plainly, but it arrived as a generic
-- 500 until handlers/pgfail.go started naming database refusals; with that in
-- place the cause read straight off the response: "Key (bail_id)=(...) is not
-- present in table bail_applications".
--
-- The key is repointed at the register in use. Nothing is migrated because
-- there is nothing to migrate: both the surety table and bail_applications are
-- empty.

ALTER TABLE bail_sureties DROP CONSTRAINT IF EXISTS bail_sureties_bail_id_fkey;

ALTER TABLE bail_sureties
    ADD CONSTRAINT bail_sureties_bail_id_fkey
    FOREIGN KEY (bail_id) REFERENCES bail(id) ON DELETE CASCADE;

COMMENT ON TABLE bail_sureties IS
    'Who stands surety on a bail application, and whether an officer has verified them. Keyed to `bail`; see migration 000098.';

-- A surety is named once per application. The same person recorded twice is a
-- clerical slip, and it would overstate the security the court actually holds.
CREATE UNIQUE INDEX IF NOT EXISTS idx_bail_sureties_named_once
    ON bail_sureties(bail_id, lower(name));

-- A verified surety names the officer who verified them and when. Verification
-- is what a court relies on; recorded with nobody behind it, it is worth
-- nothing at an inspection.
ALTER TABLE bail_sureties DROP CONSTRAINT IF EXISTS bail_sureties_verification_is_attributed;
ALTER TABLE bail_sureties ADD CONSTRAINT bail_sureties_verification_is_attributed
    CHECK (NOT verified OR (verified_by IS NOT NULL AND verified_at IS NOT NULL));
