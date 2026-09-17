-- Remove activity monitoring. The detail and the monthly totals both go; there
-- is nowhere else they are held.

DROP FUNCTION IF EXISTS roll_up_officer_activity(INTERVAL);
DROP TABLE IF EXISTS officer_activity_monthly;
DROP TABLE IF EXISTS officer_activity;
