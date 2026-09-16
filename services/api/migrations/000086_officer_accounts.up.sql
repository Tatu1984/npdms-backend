-- Officer accounts, administered rather than inserted by hand.
--
-- Every account in this platform exists because somebody ran SQL against the
-- users table. There has never been a way to create an officer, move one to a
-- new station, or stop one signing in after they left the force. For a force
-- of any size that is the thing that stops it being deployed: an officer
-- joins, transfers or retires every week, and none of those is a deployment.
--
-- This migration gives the users table the three columns that administration
-- needs and states two rules the database will keep whatever the application
-- does:
--
--   * an account is deactivated, never deleted;
--   * an officer's department follows their posting.
--
-- The second rule already existed, in the trg_user_force_matches_posting
-- trigger from 000082. What is added here is a single definition of what the
-- department should be, so that the service and the database do not each work
-- it out and disagree.

-- ------------------------------------------------- a first password ---------

-- An account created by an administrator starts with a password the
-- administrator has read off the screen and handed over. That password is
-- known to two people, which is one too many, so the account is marked as
-- owing a change. The mark is cleared when the officer changes it themselves.
ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN users.must_change_password IS
    'Set when an administrator issued the current password. Cleared when the officer sets their own.';

-- The mark clears itself when the password changes.
--
-- Putting it here rather than in the change-password path is deliberate:
-- there are two of those already (the officer's own /me/password, and an
-- administrator's reset) and there will be more — a first-sign-in flow, a
-- forced rotation. A mark that has to be cleared by hand in each of them is a
-- mark that will be left set in one of them, and an officer who is told
-- forever that they owe a change they already made stops reading the message.
--
-- An administrator's reset re-raises the mark in a second statement, which
-- does not touch password_hash and so does not trip this.
CREATE OR REPLACE FUNCTION password_change_clears_the_mark() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash THEN
        NEW.must_change_password := FALSE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_password_change_clears_the_mark ON users;
CREATE TRIGGER trg_password_change_clears_the_mark
    BEFORE UPDATE OF password_hash ON users
    FOR EACH ROW EXECUTE FUNCTION password_change_clears_the_mark();

-- ------------------------------------------------- leaving the force --------

-- Deactivation has to carry who and why. "This account is closed" is the
-- answer to a question an inspection asks years later, and without the name of
-- the officer who closed it the answer is worth nothing.
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_at      TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_by      UUID REFERENCES users(id);
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivation_reason TEXT;

-- Any account already switched off was switched off by hand and nobody wrote
-- down when. Record the migration's own time rather than leave the column
-- empty, so the rule below can be enforced from here on.
UPDATE users SET deactivated_at = NOW() WHERE is_active IS NOT TRUE AND deactivated_at IS NULL;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_deactivation_is_recorded;
ALTER TABLE users ADD CONSTRAINT users_deactivation_is_recorded
    CHECK (is_active IS TRUE OR deactivated_at IS NOT NULL);

COMMENT ON COLUMN users.deactivated_by IS
    'The officer who closed this account. An account is closed, never removed.';

-- ------------------------------------------------- never deleted ------------

-- Every register in this platform references the officer who wrote the record:
-- the FIR's recording officer, the investigating officer on a case, the
-- custody chain on a piece of evidence, and the actor on every line of the
-- audit trail. Deleting an officer either breaks those references or, worse,
-- succeeds and leaves records nobody appears to have made.
--
-- So there is no route to a delete: not in the API, which has no such
-- endpoint, and not in the database either. Taking this guard off is an act of
-- the database owner, and it leaves a trace in the server log when it happens.
CREATE OR REPLACE FUNCTION users_are_never_deleted() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'an officer account is deactivated, never deleted (%)', OLD.username
        USING ERRCODE = 'restrict_violation', CONSTRAINT = 'users_are_never_deleted';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_users_are_never_deleted ON users;
CREATE TRIGGER trg_users_are_never_deleted
    BEFORE DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION users_are_never_deleted();

-- ------------------------------------------------- department follows post --

-- Which department an officer belongs to, given the station they are posted to
-- and the department of the administrator making the posting.
--
-- Ordinarily it is simply the station's department. The exception is a wing:
-- a traffic sergeant works out of a Kolkata Police station and stays Kolkata
-- Traffic Police, which is why 000082's trigger allows a wing's officer at a
-- station of its parent force. Written once, here, so the service does not
-- have to guess at the same rule and drift from it.
CREATE OR REPLACE FUNCTION force_for_posting(p_station UUID, p_actor_force UUID) RETURNS UUID AS $$
    SELECT CASE
        -- The administrator's own department, where the station is theirs
        -- or their parent force's.
        WHEN s.force_id = p_actor_force THEN p_actor_force
        WHEN (SELECT f.parent_id FROM forces f WHERE f.id = p_actor_force) = s.force_id THEN p_actor_force
        -- Otherwise the station speaks for itself: a Kolkata Police
        -- administrator posting to a traffic guard makes a traffic officer.
        ELSE s.force_id
    END
    FROM stations s WHERE s.id = p_station;
$$ LANGUAGE sql STABLE;

COMMENT ON FUNCTION force_for_posting(UUID, UUID) IS
    'The department an officer posted to this station belongs to. A department is never chosen; it follows the posting.';

CREATE INDEX IF NOT EXISTS idx_users_force_active ON users (force_id, is_active);
