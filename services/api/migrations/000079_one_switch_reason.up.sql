-- One column for why a module was switched.
--
-- 000076 folded the two switch tables into ai_module_switches and kept both
-- words for the same thing: `reason` from 000072 and `note` from 000074. Every
-- writer since has set both to the same sentence, so the oversight screen shows
-- it twice for one module and once for another. `reason` survives.
--
-- 000074's column carried NOT NULL and a non-blank check; that rule lives in
-- the services, which refuse a switch change without a reason, and it is not
-- reimposed here because face recognition's seeded row predates it.

-- Guarded, because the chain is applied twice and the column is gone after the
-- first pass: an unguarded UPDATE referencing it fails on the second.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = 'ai_module_switches'
                  AND column_name = 'note') THEN
        EXECUTE $q$
            UPDATE ai_module_switches
               SET reason = note
             WHERE (reason IS NULL OR btrim(reason) = '')
               AND note IS NOT NULL AND btrim(note) <> ''
        $q$;
        ALTER TABLE ai_module_switches DROP COLUMN note;
    END IF;
END $$;

COMMENT ON COLUMN ai_module_switches.reason IS
    'Why this module was last switched on or off, recorded with the officer who did it.';
