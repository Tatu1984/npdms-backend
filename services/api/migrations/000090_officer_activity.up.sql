-- Where an officer went in the platform, and for how long.
--
-- This is staff monitoring, and it is written down as such rather than dressed
-- up. A force asks for it because misuse of a police system is usually reading,
-- not writing: nothing is changed, so nothing appears in the audit trail, and
-- the only trace is that somebody spent eleven minutes on the record of a
-- person they have no case with. That question cannot be answered without this,
-- and this cannot be justified without that question.
--
-- Three decisions follow from that, and are enforced here rather than left to
-- whoever writes the next query:
--
--   * It is kept apart from audit_logs. The audit trail is hash-chained and
--     immutable because it records what was done; page visits are neither
--     state changes nor evidence, they run to tens of rows per officer per
--     day, and putting them in the chain would bury the events that matter.
--
--   * Detail is kept for ninety days and no longer. After that it is rolled up
--     to a month, an officer and a module — enough for "who works this
--     register" and not enough to reconstruct a person's day a year later.
--     roll_up_officer_activity() does both halves in one statement so the
--     aggregate cannot be written without the detail being removed.
--
--   * The duration is what the browser observed, and it is approximate. A tab
--     left open on a screen is not an officer reading it. Nothing here should
--     be read as time worked, and the column is named for what it is.

CREATE TABLE IF NOT EXISTS officer_activity (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- The session the visit belongs to, as the audit trail names it, so a
    -- visit and the events around it can be put side by side.
    session_id  TEXT,

    -- The path as visited, and the module it belongs to. The path is kept
    -- whole, record identifiers and all: "eleven minutes on this person's
    -- record" is the question, and a module alone cannot answer it.
    path        TEXT NOT NULL,
    module      TEXT NOT NULL,

    opened_at   TIMESTAMPTZ NOT NULL,
    closed_at   TIMESTAMPTZ NOT NULL,

    -- Seconds the page was open in front of this officer, as the browser
    -- measured it. Approximate by nature; see the note above.
    seconds     INTEGER NOT NULL CHECK (seconds >= 0 AND seconds <= 86400),

    ip_address  INET,
    force_id    UUID REFERENCES forces(id),
    station_id  UUID REFERENCES stations(id),

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT officer_activity_closed_after_opened CHECK (closed_at >= opened_at)
);

COMMENT ON TABLE officer_activity IS
    'Pages an officer opened and for how long. Staff monitoring; detail kept 90 days, then rolled up by roll_up_officer_activity().';
COMMENT ON COLUMN officer_activity.seconds IS
    'Observed by the browser. A tab left open is not an officer reading. Not time worked.';

CREATE INDEX IF NOT EXISTS idx_officer_activity_user_time
    ON officer_activity(user_id, opened_at DESC);
CREATE INDEX IF NOT EXISTS idx_officer_activity_opened
    ON officer_activity(opened_at);
CREATE INDEX IF NOT EXISTS idx_officer_activity_module
    ON officer_activity(module, opened_at DESC);

-- A visit is recorded once. The browser sends a batch on leaving a page and
-- again when the tab closes, and the two overlap; without this a slow network
-- would count the same minute twice and inflate every figure that follows.
CREATE UNIQUE INDEX IF NOT EXISTS idx_officer_activity_once
    ON officer_activity(user_id, path, opened_at);

-- ------------------------------------------------- what is kept after 90 days

CREATE TABLE IF NOT EXISTS officer_activity_monthly (
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month    DATE NOT NULL,
    module   TEXT NOT NULL,
    seconds  BIGINT NOT NULL DEFAULT 0,
    visits   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, month, module)
);

COMMENT ON TABLE officer_activity_monthly IS
    'What survives the 90-day cull: an officer, a month, a module, a total. Cannot reconstruct a day.';

-- ------------------------------------------------- the cull -----------------

-- Roll detail older than the window into the monthly totals and delete it.
--
-- One statement does both, in one transaction, because the failure that
-- matters is aggregating and then not deleting — which would leave the detail
-- in place while the screens report it as already summarised, and quietly
-- defeat the retention promise the force was given.
CREATE OR REPLACE FUNCTION roll_up_officer_activity(older_than INTERVAL DEFAULT INTERVAL '90 days')
RETURNS TABLE (rolled_rows BIGINT, months_touched BIGINT) AS $$
WITH cutoff AS (
    SELECT NOW() - older_than AS at
), doomed AS (
    DELETE FROM officer_activity a
    USING cutoff c
    WHERE a.opened_at < c.at
    RETURNING a.user_id, date_trunc('month', a.opened_at)::DATE AS month, a.module, a.seconds
), totals AS (
    SELECT user_id, month, module, SUM(seconds) AS seconds, COUNT(*) AS visits
    FROM doomed GROUP BY user_id, month, module
), merged AS (
    INSERT INTO officer_activity_monthly (user_id, month, module, seconds, visits)
    SELECT user_id, month, module, seconds, visits FROM totals
    ON CONFLICT (user_id, month, module) DO UPDATE
        SET seconds = officer_activity_monthly.seconds + EXCLUDED.seconds,
            visits  = officer_activity_monthly.visits  + EXCLUDED.visits
    RETURNING 1
)
SELECT COALESCE((SELECT SUM(visits) FROM totals), 0)::BIGINT,
       (SELECT COUNT(*) FROM merged)::BIGINT;
$$ LANGUAGE sql;

COMMENT ON FUNCTION roll_up_officer_activity(INTERVAL) IS
    'Aggregate and delete activity detail past the retention window. Run daily; safe to run repeatedly.';
