-- Remove the roster's integrity rules. Records merged by the up migration are
-- not restored: which of two duplicates was which is not recoverable.

DROP VIEW IF EXISTS personnel_caseload;
ALTER TABLE personnel DROP CONSTRAINT IF EXISTS personnel_leave_belongs_to_leave;
DROP INDEX IF EXISTS idx_personnel_one_per_officer;
