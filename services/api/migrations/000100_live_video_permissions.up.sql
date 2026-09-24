-- Permissions for live CCTV viewing and for the Edge Agent's ingest keys.
--
-- Migration 000099 added live streaming to the camera register. Its routes
-- were written before the permission model landed, guarded by rank; rank no
-- longer guards anything, so each one names a permission here or the service
-- refuses to start.
--
-- The floors are the ones Phase 03 already set for recorded footage, for the
-- same reasons. Seeing which cameras stream, and whether the store is even
-- configured, is any officer's — it is the register they can already read.
-- Watching is an ASI's, because live video is footage and Phase 03 puts
-- footage behind a stated purpose at that rank. Ending a session is nobody's
-- privilege: it is the viewer closing their own tab, and the service refuses
-- a session that is not theirs. Issuing, rotating and revoking a camera's
-- ingest token is an SHO's, alongside registering the camera itself — the
-- token is a credential that lets a machine publish video into the platform.
--
-- The ingest plane (/api/edge/ingest/...) names no permission because no
-- officer is there to hold one: the camera presents its own bearer token,
-- compared against the hash on its register row. It is declared in the authz
-- catalogue as a device-authenticated prefix, not as a public one.
--
-- Not a permission: a camera flagged for masking still needs SHO to watch
-- live, checked in the service. Masking is not applied to live video, so that
-- floor stands in for the mask itself rather than describing a job an officer
-- holds — and it is seniority, which is what rank is still for.

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('video.live.view', 'video', 'view',
     'View — CCTV · which cameras are streaming, and whether live video storage is configured', NULL,
     ARRAY[
        'GET /api/v1/video/live/cameras',
        'GET /api/v1/video/live/media-status'
     ]::TEXT[]),

    ('video.live.watch', 'video', 'create',
     'Create — CCTV · start watching live cameras, stating a purpose', 'ASI'::user_role,
     ARRAY['POST /api/v1/video/live/sessions']::TEXT[]),

    ('video.live.session.end', 'video', 'create',
     'Create — CCTV · end one''s own live viewing session', NULL,
     ARRAY['POST /api/v1/video/live/sessions/:id/end']::TEXT[]),

    ('video.live.streaming.manage', 'video', 'create',
     'Create — CCTV · enable live streaming on a camera, rotate or revoke its ingest token', 'SHO'::user_role,
     ARRAY[
        'POST /api/v1/video/cameras/:id/streaming/enable',
        'POST /api/v1/video/cameras/:id/streaming/rotate-token',
        'POST /api/v1/video/cameras/:id/streaming/disable'
     ]::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module, action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank, routes = EXCLUDED.routes;

DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p ON p.key LIKE 'video.live.%'
 AND (p.default_min_rank IS NULL
      OR rank_level(p.default_min_rank) <= rank_level(ro.rank))
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();
