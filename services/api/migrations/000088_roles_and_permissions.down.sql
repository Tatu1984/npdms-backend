-- Remove roles and permissions, leaving the rank ladder as the only
-- authorisation. Assignments are lost: which officer held which role is not
-- recoverable from the rank alone, so take a copy of user_roles first if the
-- intention is to reapply.

DROP VIEW IF EXISTS user_permissions;

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;
DROP FUNCTION IF EXISTS rank_role_grants_are_not_edited();

DROP TRIGGER IF EXISTS trg_rank_roles_are_not_edited ON roles;
DROP FUNCTION IF EXISTS rank_roles_are_not_edited();

DROP TRIGGER IF EXISTS trg_rank_role_follows_rank ON users;
DROP FUNCTION IF EXISTS rank_role_follows_rank();

DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS permissions;

DROP FUNCTION IF EXISTS rank_level(user_role);
