-- One personnel record per officer, and a caseload that is counted rather
-- than stored.
--
-- Three defects in the duty roster, all of the same kind: a number or a state
-- kept by hand that nothing kept correct.
--
-- Nothing prevented two personnel records for one officer. A roster with an
-- officer on it twice shows them available while they are on leave, and a
-- transfer recorded against one row leaves the other at the old station.
--
-- assigned_cases was a stored number that nothing maintained. It was set when
-- a record was created and never again, so an officer with eleven cases read
-- as having whatever was typed the day they were added — usually zero. It
-- becomes a view over the case register, which cannot drift because it is not
-- a copy.
--
-- And putting an officer on duty left leave_type and leave_until set, so the
-- roster showed them both on patrol and on leave. Being on duty and being on
-- leave are exclusive; the database now says so.

-- ------------------------------------------------- one record per officer --

-- Any officer with more than one record keeps the most recently updated and
-- loses the rest. Reported before anything is removed.
DO $$
DECLARE
    extra INT;
BEGIN
    SELECT COUNT(*) INTO extra FROM (
        SELECT user_id FROM personnel WHERE user_id IS NOT NULL
        GROUP BY user_id HAVING COUNT(*) > 1
    ) d;
    IF extra > 0 THEN
        RAISE NOTICE 'personnel: % officer(s) had more than one record; keeping the most recent of each', extra;
    END IF;
END$$;

DELETE FROM personnel p
USING personnel keep
WHERE p.user_id IS NOT NULL
  AND p.user_id = keep.user_id
  AND p.id <> keep.id
  AND (keep.updated_at, keep.id) > (p.updated_at, p.id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_personnel_one_per_officer
    ON personnel(user_id) WHERE user_id IS NOT NULL;

-- ------------------------------------------------- on duty or on leave -----

-- An officer on duty is not on leave. This was only ever true by convention,
-- and the convention was broken by the assign-duty path, which set the duty
-- and left the leave fields where they were.
UPDATE personnel
SET leave_type = NULL, leave_until = NULL
WHERE status <> 'ON_LEAVE' AND (leave_type IS NOT NULL OR leave_until IS NOT NULL);

ALTER TABLE personnel DROP CONSTRAINT IF EXISTS personnel_leave_belongs_to_leave;
ALTER TABLE personnel ADD CONSTRAINT personnel_leave_belongs_to_leave
    CHECK (status = 'ON_LEAVE' OR (leave_type IS NULL AND leave_until IS NULL));

-- ------------------------------------------------- caseload, counted -------

-- The live caseload, from the register itself. A view rather than a column,
-- because a copy of a count is a copy that goes stale, and this one did.
CREATE OR REPLACE VIEW personnel_caseload AS
SELECT p.id AS personnel_id,
       p.user_id,
       -- Still being worked: not closed, and not finished in court either
       -- way. A conviction and an acquittal both end the officer's work on it.
       COUNT(DISTINCT c.id) FILTER (
           WHERE c.status NOT IN ('CLOSED', 'CONVICTION', 'ACQUITTAL')
       )::INT AS open_cases,
       COUNT(DISTINCT c.id)::INT AS total_cases
FROM personnel p
LEFT JOIN cases c ON c.investigating_officer = p.user_id
GROUP BY p.id, p.user_id;

COMMENT ON VIEW personnel_caseload IS
    'An officer''s live caseload, counted from the case register. Replaces personnel.assigned_cases, which nothing maintained.';

COMMENT ON COLUMN personnel.assigned_cases IS
    'Superseded by the personnel_caseload view. Kept until every reader is known; see migration 000095.';
