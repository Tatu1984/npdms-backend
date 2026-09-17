-- Permissions for the activity trail.
--
-- The floors say who this is for. Recording is something every officer's own
-- browser does, so it has none. Reading somebody else's trail is supervision
-- and sits with the audit trail at DSP. Reading your own needs nothing: being
-- able to see what is kept about you is part of being told it is kept.
--
-- Applying the retention cull is deliberately the same floor as reading,
-- rather than higher. It only ever removes detail, a scheduler must be able to
-- call it, and making it an SP's act would mean it did not happen nightly.

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('activity.create', 'activity', 'create',
     'Create — Activity trail · record one''s own page visits', NULL,
     ARRAY['POST /api/v1/activity']::TEXT[]),

    ('activity.roll-up.create', 'activity', 'create',
     'Create — Activity trail · apply the 90-day retention cull', 'DSP'::user_role,
     ARRAY['POST /api/v1/activity/roll-up']::TEXT[]),

    ('me.activity.view', 'me', 'view',
     'View — Own profile · where you have been in the platform', NULL,
     ARRAY['GET /api/v1/me/activity']::TEXT[]),

    ('officers.activity.view', 'officers', 'view',
     'View — Officer accounts · where an officer has been in the platform', 'DSP'::user_role,
     ARRAY['GET /api/v1/officers/:id/activity']::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module,
        action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank,
        routes = EXCLUDED.routes;

-- Grant to the rank roles on the same rule the original seeding used. The
-- trigger that fixes rank-role grants stands aside for this, as it does there.
DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p
  ON p.key IN ('activity.create', 'activity.roll-up.create',
               'me.activity.view', 'officers.activity.view')
 AND (p.default_min_rank IS NULL
      OR rank_level(p.default_min_rank) <= rank_level(ro.rank))
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
