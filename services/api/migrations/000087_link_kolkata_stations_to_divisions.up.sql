-- Kolkata's stations knew which division they were in, but nothing could read it.
--
-- Every Kolkata Police station carries its division as text — "Kolkata South",
-- "Kolkata Central" — while `district_id`, the column the hierarchy actually
-- joins on, was left NULL on all seventeen. The divisions themselves have been
-- in `districts` the whole time.
--
-- The effect was quiet rather than loud: the district and state dashboards and
-- the station rankings ran correct queries and returned nothing, because the
-- chain from station up to state was broken at its first link. A force with
-- sixty-five FIRs showed a ranking table of zeroes. That reads as "there is no
-- crime here" rather than "this is not wired up", which is the more dangerous
-- of the two.
--
-- This links each station to the division it already names. It invents no
-- relationship: where the text does not match a division, the station is left
-- unlinked rather than guessed at, and the check at the end reports how many.

UPDATE stations s
   SET district_id = d.id
  FROM districts d
 WHERE s.district_id IS NULL
   AND d.force_id = s.force_id
   AND (
        -- "Kolkata South East" is the "South East Division".
        lower(btrim(replace(s.district, 'Kolkata', ''))) = lower(btrim(replace(d.name, 'Division', '')))
        -- and a station that names the division outright.
        OR lower(btrim(s.district)) = lower(btrim(d.name))
       );

-- Say plainly what is still unlinked, so a gap shows up in the bootstrap output
-- rather than as an empty ranking table months later.
DO $$
DECLARE
    v_unlinked INTEGER;
    v_total    INTEGER;
BEGIN
    SELECT count(*) FILTER (WHERE district_id IS NULL), count(*)
      INTO v_unlinked, v_total
      FROM stations;

    IF v_unlinked > 0 THEN
        RAISE NOTICE '% of % stations are not linked to a district. The district, state and ranking screens will show nothing for them until they are.',
            v_unlinked, v_total;
    END IF;
END $$;
