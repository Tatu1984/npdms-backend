-- Remove the activity permissions. Grants go with them by cascade, so the
-- trigger that fixes rank-role grants stands aside here too.

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

DELETE FROM permissions WHERE key IN (
    'activity.create', 'activity.roll-up.create',
    'me.activity.view', 'officers.activity.view');

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
