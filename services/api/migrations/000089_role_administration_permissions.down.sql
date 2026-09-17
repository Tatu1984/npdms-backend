-- Remove the role-administration permissions. The grants go with them by the
-- foreign key's ON DELETE CASCADE, so the trigger that fixes rank-role grants
-- has to stand aside here too.

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

DELETE FROM permissions WHERE key IN (
    'roles.view', 'roles.create', 'roles.amend', 'roles.delete',
    'roles.permissions.amend', 'permissions.view',
    'officers.roles.view', 'officers.roles.create', 'officers.roles.delete',
    'me.permissions.view');

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
