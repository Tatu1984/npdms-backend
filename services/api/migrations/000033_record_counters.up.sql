-- Atomic record numbering.
--
-- Record numbers were generated two broken ways:
--
--   * Cases and evidence took `UnixNano() % 100000` — a random draw from
--     100,000 values, so a collision is more likely than not by about the
--     370th record. Where the clock resolves only microseconds (observed on
--     macOS) every suffix ends in 000 and only 100 values exist. Each
--     collision surfaced as a unique-key 500.
--   * FIRs, warrants and bail counted this year's rows and added one. Two
--     officers registering at the same moment get the same number, and a
--     deleted row makes the next number repeat an existing one.
--
-- A counter row per scope and year is incremented with an upsert, which
-- Postgres serialises on the row, so each call returns a number no other call
-- can receive. FIR scopes carry the station code (`FIR:BHW`) because FIR
-- numbers are sequential per station.

CREATE TABLE IF NOT EXISTS record_counters (
    scope      VARCHAR(40) NOT NULL,
    year       INTEGER     NOT NULL,
    last_value BIGINT      NOT NULL,
    PRIMARY KEY (scope, year)
);

-- Start each counter above every number already issued, so nothing collides
-- with records created by the old generators.
INSERT INTO record_counters (scope, year, last_value)
SELECT scope, year, MAX(n) FROM (
    SELECT 'CASE' AS scope, (m)[1]::int AS year, (m)[2]::bigint AS n
      FROM (SELECT regexp_match(case_number, '^CASE-(\d{4})-(\d+)$') AS m FROM cases) s WHERE m IS NOT NULL
    UNION ALL
    SELECT 'EVD', (m)[1]::int, (m)[2]::bigint
      FROM (SELECT regexp_match(evidence_number, '^EVD-(\d{4})-(\d+)$') AS m FROM evidence) s WHERE m IS NOT NULL
    UNION ALL
    SELECT 'WAR', (m)[1]::int, (m)[2]::bigint
      FROM (SELECT regexp_match(warrant_number, '^WAR-(\d{4})-(\d+)$') AS m FROM warrants) s WHERE m IS NOT NULL
    UNION ALL
    SELECT 'BAIL', (m)[1]::int, (m)[2]::bigint
      FROM (SELECT regexp_match(application_number, '^BAIL-(\d{4})-(\d+)$') AS m FROM bail) s WHERE m IS NOT NULL
    UNION ALL
    SELECT 'FIR:' || (m)[1], (m)[2]::int, (m)[3]::bigint
      FROM (SELECT regexp_match(fir_number, '^([A-Za-z0-9]+)/(\d{4})/(\d+)$') AS m FROM firs) s WHERE m IS NOT NULL
) issued
GROUP BY scope, year
ON CONFLICT (scope, year) DO UPDATE
    SET last_value = GREATEST(record_counters.last_value, EXCLUDED.last_value);
