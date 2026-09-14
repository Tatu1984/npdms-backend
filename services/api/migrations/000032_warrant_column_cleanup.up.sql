-- Remove the warrant columns left behind by the base schema.
--
-- The base schema created `warrants.warrant_type` (enum, NOT NULL) and
-- `warrants.charges`; 000027 then added `type` and `ipc_sections`, which are what
-- the application reads and writes. The originals were never populated by the
-- code, and because `warrant_type` is NOT NULL every insert failed — no warrant
-- could be created through the API.
--
-- The columns the code uses are kept. Anything held only in the originals is
-- carried across first, so this is safe on a database patched by hand.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'warrants' AND column_name = 'warrant_type') THEN
        UPDATE warrants SET type = warrant_type::text WHERE type IS NULL;
        ALTER TABLE warrants DROP COLUMN warrant_type;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'warrants' AND column_name = 'charges') THEN
        UPDATE warrants SET ipc_sections = charges
         WHERE (ipc_sections IS NULL OR ipc_sections = '{}') AND charges IS NOT NULL;
        ALTER TABLE warrants DROP COLUMN charges;
    END IF;
END $$;

DROP TYPE IF EXISTS warrant_type;

-- `type` now carries the constraint `warrant_type` used to.
UPDATE warrants SET ipc_sections = '{}' WHERE ipc_sections IS NULL;
ALTER TABLE warrants
    ALTER COLUMN type SET NOT NULL,
    ALTER COLUMN ipc_sections SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'warrants_type_check') THEN
        ALTER TABLE warrants ADD CONSTRAINT warrants_type_check
            CHECK (type IN ('ARREST', 'SEARCH', 'SUMMONS', 'NBW'));
    END IF;
END $$;
