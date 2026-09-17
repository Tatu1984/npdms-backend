DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;
DELETE FROM permissions WHERE key IN ('court.orders.amend', 'court.orders.compliance');
CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
