-- Permissions for the routes added while repairing the registers.
--
-- A court order is corrected by whoever may record one, and its compliance is
-- settled by the same rank: both are the court diary's own work, and splitting
-- them would mean an officer could record a direction and not record that it
-- had been carried out.

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('court.orders.amend', 'court', 'amend',
     'Amend — Court diary · correct a recorded order', 'SI'::user_role,
     ARRAY['PATCH /api/v1/court/orders/:id']::TEXT[]),

    ('court.orders.compliance', 'court', 'create',
     'Create — Court diary · record whether an order was complied with', 'SI'::user_role,
     ARRAY['POST /api/v1/court/orders/:id/compliance']::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module,
        action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank,
        routes = EXCLUDED.routes;

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p
  ON p.key IN ('court.orders.amend', 'court.orders.compliance')
 AND (p.default_min_rank IS NULL
      OR rank_level(p.default_min_rank) <= rank_level(ro.rank))
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
