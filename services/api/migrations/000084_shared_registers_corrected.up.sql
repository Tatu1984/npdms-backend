-- The shared registers, named after things that actually exist.
--
-- 000082 listed a register called VEHICLES as "stolen and wanted vehicles".
-- There is no such register. `/vehicles` in this platform is the departmental
-- FLEET — allocation, fuel, maintenance — which is emphatically not state-wide;
-- Kolkata Police has no business in West Bengal Police's vehicle maintenance.
-- Stolen and wanted vehicles are held as STOLEN_VEHICLE lookouts and in the
-- ANPR watchlist fed from them, both of which are shared and are named here.
--
-- Left as it was, the interface would have had to either mark a fleet page
-- "shared by all four departments", which is false, or quietly ignore the
-- table, which makes the table a lie. Naming the real registers is the fix.

DELETE FROM shared_registers WHERE register = 'VEHICLES';

INSERT INTO shared_registers (register, description) VALUES
    ('ANPR_WATCHLIST', 'Vehicles on the number-plate watchlist, including those from stolen-vehicle lookouts: a stolen car crosses a boundary within the hour.')
ON CONFLICT (register) DO NOTHING;

UPDATE shared_registers
   SET description = 'Lookout notices and their sightings, including stolen and wanted vehicles.'
 WHERE register = 'LOOKOUTS';
