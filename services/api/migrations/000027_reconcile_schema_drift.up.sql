-- Reconcile drift between services/db/init/001_schema.sql and the migration chain.
--
-- The base schema predates several migrations and creates narrower versions of
-- tables that later migrations then skip, because those migrations use
-- CREATE TABLE IF NOT EXISTS. The result is a database that looks complete but
-- is missing columns the application selects, and the failures only surface at
-- runtime as 500s.
--
-- This migration brings a database built from the base schema up to what the
-- code actually queries. It is written to be safe to run on a fresh database
-- and on one already patched by hand.

-- Trigger function used by several migrations but never defined by the base schema.
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- The user lookup joins stations to districts and states by id, but the base
-- schema stores them only as free text. Without these columns every sign-in
-- fails with "invalid credentials", because the query errors before the
-- password is ever compared.
ALTER TABLE stations
    ADD COLUMN IF NOT EXISTS district_id UUID REFERENCES districts(id),
    ADD COLUMN IF NOT EXISTS state_id UUID REFERENCES states(id);

-- Warrant columns introduced by 000006, skipped because the base schema had
-- already created a narrower `warrants`.
ALTER TABLE warrants
    ADD COLUMN IF NOT EXISTS type VARCHAR(20),
    ADD COLUMN IF NOT EXISTS judge_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS ipc_sections TEXT[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS executed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS description TEXT,
    ADD COLUMN IF NOT EXISTS age INTEGER,
    ADD COLUMN IF NOT EXISTS gender VARCHAR(20),
    ADD COLUMN IF NOT EXISTS address TEXT,
    ADD COLUMN IF NOT EXISTS identifying_marks TEXT,
    ADD COLUMN IF NOT EXISTS search_premises TEXT,
    ADD COLUMN IF NOT EXISTS search_scope TEXT,
    ADD COLUMN IF NOT EXISTS summons_purpose TEXT,
    ADD COLUMN IF NOT EXISTS hearing_date TIMESTAMP,
    ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

-- Court hearing columns introduced by 000011, skipped for the same reason.
ALTER TABLE court_hearings
    ADD COLUMN IF NOT EXISTS title VARCHAR(255),
    ADD COLUMN IF NOT EXISTS court VARCHAR(255),
    ADD COLUMN IF NOT EXISTS type VARCHAR(30),
    ADD COLUMN IF NOT EXISTS investigating_officer UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS ipc_sections TEXT[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS required_documents TEXT[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS priority VARCHAR(20);

-- Carry the legacy text values across so existing rows are not left blank.
UPDATE court_hearings SET court = court_name WHERE court IS NULL AND court_name IS NOT NULL;
UPDATE court_hearings SET type = hearing_type WHERE type IS NULL AND hearing_type IS NOT NULL;

COMMENT ON COLUMN stations.district_id IS 'Added by 000027 — the base schema stored district as text only';
