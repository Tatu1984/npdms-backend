-- Permissions for the fleet logs and the surety register.
--
-- Reading a vehicle's history is any officer's: a driver checks whether the
-- jeep is booked out before taking it. Recording a trip or a fill is a
-- constable's work — they are the ones drawing the fuel. Closing a maintenance
-- job and verifying a surety are not: one settles money, the other tells a
-- court that a named person stands good for an accused, which is an SI's
-- responsibility at the least.

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('vehicles.trips.view', 'vehicles', 'view',
     'View — Vehicles · trip log', NULL,
     ARRAY['GET /api/v1/vehicles/:id/trips']::TEXT[]),
    ('vehicles.trips.create', 'vehicles', 'create',
     'Create — Vehicles · book a vehicle out', 'CONSTABLE'::user_role,
     ARRAY['POST /api/v1/vehicles/:id/trips']::TEXT[]),
    ('vehicles.trips.close', 'vehicles', 'create',
     'Create — Vehicles · book a vehicle back in', 'CONSTABLE'::user_role,
     ARRAY['POST /api/v1/vehicles/:id/trips/:tripId/close']::TEXT[]),

    ('vehicles.fuel.view', 'vehicles', 'view',
     'View — Vehicles · fuel log', NULL,
     ARRAY['GET /api/v1/vehicles/:id/fuel']::TEXT[]),
    ('vehicles.fuel.create', 'vehicles', 'create',
     'Create — Vehicles · record fuel drawn', 'CONSTABLE'::user_role,
     ARRAY['POST /api/v1/vehicles/:id/fuel']::TEXT[]),

    ('vehicles.maintenance.view', 'vehicles', 'view',
     'View — Vehicles · service and repair history', NULL,
     ARRAY['GET /api/v1/vehicles/:id/maintenance']::TEXT[]),
    ('vehicles.maintenance.create', 'vehicles', 'create',
     'Create — Vehicles · report work needed', 'CONSTABLE'::user_role,
     ARRAY['POST /api/v1/vehicles/:id/maintenance']::TEXT[]),
    ('vehicles.maintenance.complete', 'vehicles', 'create',
     'Create — Vehicles · record work completed and its cost', 'SHO'::user_role,
     ARRAY['POST /api/v1/vehicles/:id/maintenance/:recordId/complete']::TEXT[]),

    ('bail.sureties.view', 'bail', 'view',
     'View — Bail · who stands surety', NULL,
     ARRAY['GET /api/v1/bail/:id/sureties']::TEXT[]),
    ('bail.sureties.create', 'bail', 'create',
     'Create — Bail · record a surety', 'SI'::user_role,
     ARRAY['POST /api/v1/bail/:id/sureties']::TEXT[]),
    ('bail.sureties.verify', 'bail', 'create',
     'Create — Bail · verify a surety stands good', 'SI'::user_role,
     ARRAY['POST /api/v1/bail/:id/sureties/:suretyId/verify']::TEXT[]),
    ('bail.sureties.delete', 'bail', 'delete',
     'Delete — Bail · remove a surety', 'SI'::user_role,
     ARRAY['DELETE /api/v1/bail/:id/sureties/:suretyId']::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module, action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank, routes = EXCLUDED.routes;

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p ON p.module IN ('vehicles', 'bail')
 AND p.key LIKE '%.%.%'
 AND (p.default_min_rank IS NULL
      OR rank_level(p.default_min_rank) <= rank_level(ro.rank))
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
