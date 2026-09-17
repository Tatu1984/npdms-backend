DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;
DELETE FROM permissions WHERE key LIKE 'vehicles.trips.%' OR key LIKE 'vehicles.fuel.%'
    OR key LIKE 'vehicles.maintenance.%' OR key LIKE 'bail.sureties.%';
CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
