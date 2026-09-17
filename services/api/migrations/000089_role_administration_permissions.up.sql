-- The permissions for administering roles themselves.
--
-- Migration 000088 derived the catalogue from the routes that existed then.
-- These eleven routes are new, and a route that names no permission stops the
-- service at startup, so they are declared here.
--
-- The floors mirror officer administration, which is the nearest comparable
-- thing: reading who may do what sits with reading the officer register (DSP),
-- and changing it sits with changing an officer's posting (SP). Seeing one's
-- own permissions is nobody's secret and has no floor.

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('roles.view', 'roles', 'view',
     'View — Roles and permissions', 'DSP'::user_role,
     ARRAY['GET /api/v1/roles', 'GET /api/v1/roles/:id']::TEXT[]),

    ('roles.create', 'roles', 'create',
     'Create — Roles and permissions', 'SP'::user_role,
     ARRAY['POST /api/v1/roles']::TEXT[]),

    ('roles.amend', 'roles', 'amend',
     'Amend — Roles and permissions', 'SP'::user_role,
     ARRAY['PATCH /api/v1/roles/:id']::TEXT[]),

    ('roles.delete', 'roles', 'delete',
     'Delete — Roles and permissions', 'SP'::user_role,
     ARRAY['DELETE /api/v1/roles/:id']::TEXT[]),

    ('roles.permissions.amend', 'roles', 'amend',
     'Amend — Roles and permissions · what a role grants', 'SP'::user_role,
     ARRAY['PUT /api/v1/roles/:id/permissions']::TEXT[]),

    ('permissions.view', 'permissions', 'view',
     'View — The permission catalogue', 'DSP'::user_role,
     ARRAY['GET /api/v1/permissions']::TEXT[]),

    ('officers.roles.view', 'officers', 'view',
     'View — Officer accounts · roles held', 'DSP'::user_role,
     ARRAY['GET /api/v1/officers/:id/roles']::TEXT[]),

    ('officers.roles.create', 'officers', 'create',
     'Create — Officer accounts · give a role', 'SP'::user_role,
     ARRAY['POST /api/v1/officers/:id/roles']::TEXT[]),

    ('officers.roles.delete', 'officers', 'delete',
     'Delete — Officer accounts · take a role away', 'SP'::user_role,
     ARRAY['DELETE /api/v1/officers/:id/roles/:roleId']::TEXT[]),

    -- An officer asking what they themselves may do. Refusing this would mean
    -- an officer cannot be told why a screen is empty.
    ('me.permissions.view', 'me', 'view',
     'View — Own profile · permissions held', NULL,
     ARRAY['GET /api/v1/me/permissions']::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module,
        action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank,
        routes = EXCLUDED.routes;

-- Grant them to the rank roles on the same rule 000088 used, so the ladder
-- keeps meaning what it meant. The trigger that fixes rank-role grants has to
-- stand aside for this, exactly as it does during the original seeding.
DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p
  ON p.key IN ('roles.view', 'roles.create', 'roles.amend', 'roles.delete',
               'roles.permissions.amend', 'permissions.view',
               'officers.roles.view', 'officers.roles.create',
               'officers.roles.delete', 'me.permissions.view')
 AND (p.default_min_rank IS NULL
      OR rank_level(p.default_min_rank) <= rank_level(ro.rank))
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
