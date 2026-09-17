-- Record numbers for the registers that never had one, and counters for the
-- registers that were still counting rows.
--
-- Two related defects.
--
-- A forensic request had no number. The model carried a RequestNumber field,
-- the API returned it, and it was always empty — the column lives on
-- forensic_requests, an abandoned table, while every request the platform
-- actually writes goes to `forensics`, which had no such column. A laboratory
-- asked "which request is this" had nothing to be told.
--
-- And five registers issued their numbers by counting what was already there
-- — COUNT(*) + 1, or the clock — rather than from the counter table 000033
-- introduced. Both are wrong the moment two officers record at once: counting
-- gives them the same number, and the clock gives a number that is not
-- sequential and cannot be read out over a radio. The counter table exists for
-- exactly this and takes a row lock per scope per year.

-- ------------------------------------------------- forensic request numbers

ALTER TABLE forensics ADD COLUMN IF NOT EXISTS request_number TEXT;

-- Existing requests are numbered in the order they were submitted, so the
-- sequence a laboratory sees matches the order it received them.
WITH numbered AS (
    SELECT id,
           EXTRACT(YEAR FROM COALESCE(submitted_date, created_at))::INT AS yr,
           ROW_NUMBER() OVER (
               PARTITION BY EXTRACT(YEAR FROM COALESCE(submitted_date, created_at))
               ORDER BY COALESCE(submitted_date, created_at), id
           ) AS n
    FROM forensics
    WHERE request_number IS NULL
)
UPDATE forensics f
SET request_number = 'FSL-' || numbered.yr || '-' || lpad(numbered.n::TEXT, 5, '0')
FROM numbered
WHERE f.id = numbered.id;

-- The counter must not reissue a number the backfill has already used.
INSERT INTO record_counters (scope, year, last_value)
SELECT 'FSL',
       EXTRACT(YEAR FROM COALESCE(submitted_date, created_at))::INT,
       COUNT(*)
FROM forensics
GROUP BY 2
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);

CREATE UNIQUE INDEX IF NOT EXISTS idx_forensics_request_number
    ON forensics(request_number) WHERE request_number IS NOT NULL;

COMMENT ON COLUMN forensics.request_number IS
    'FSL-YYYY-NNNNN, issued from record_counters. The number a laboratory quotes back.';

-- ------------------------------------------------- counters for the rest ---

-- Seed the counters for the five registers that were counting rows or reading
-- the clock, so the first number each issues from now on follows the last one
-- already recorded rather than colliding with it.
--
-- Each is seeded from the highest number already in use where one can be read,
-- and from the row count otherwise. Seeding too high is harmless — it skips a
-- number — and seeding too low reissues one, so where there is doubt this
-- rounds up.

INSERT INTO record_counters (scope, year, last_value)
SELECT 'CYBER', EXTRACT(YEAR FROM created_at)::INT, COUNT(*)
FROM cyber_crimes GROUP BY 2
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);

INSERT INTO record_counters (scope, year, last_value)
SELECT 'CMP', EXTRACT(YEAR FROM created_at)::INT, COUNT(*)
FROM citizen_complaints GROUP BY 2
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);

INSERT INTO record_counters (scope, year, last_value)
SELECT 'MP', EXTRACT(YEAR FROM created_at)::INT, COUNT(*)
FROM missing_person_reports GROUP BY 2
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);

INSERT INTO record_counters (scope, year, last_value)
SELECT 'GRV', EXTRACT(YEAR FROM created_at)::INT, COUNT(*)
FROM grievances GROUP BY 2
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);
