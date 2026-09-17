-- Roles and permissions: what an officer may do, as distinct from how senior
-- they are.
--
-- Until now authorisation was a rank ladder and nothing else. Every check in
-- the API is "your rank number is at least this rank number", which can say
-- "a DSP and above may do this" and cannot say anything else. It cannot
-- express a job. A malkhana clerk who issues property but may not dispose of
-- it, a court liaison who amends orders but registers no FIR, a station
-- writer — none of those are ranks, and the platform had no way to describe
-- them. Worse, seniority granted everything: whatever a DSP could do, every
-- SP, DIG, IG and DGP could do too, because the comparison is numeric.
--
-- This migration adds the missing half. Rank stays, and keeps doing the one
-- thing it is right for — seniority rules, such as which ranks may issue a
-- state-wide alert. Day-to-day access becomes a named permission, granted to
-- a named role, assigned to an officer.
--
-- Nothing changes behaviour on the day it is applied. Twelve roles are
-- created, one per rank, each granted exactly the permissions that rank can
-- already reach, and every existing officer is assigned the role matching
-- their rank. The platform therefore behaves identically the moment this runs;
-- what it gains is the ability to say something the ladder could not.
--
-- The catalogue below was derived from the router rather than invented: all
-- 590 authenticated routes were read out of the router with the rank floor each
-- one carries today, and grouped into the smallest set of permissions in which
-- every route sharing a permission also shares a floor. Where routes under one
-- name disagreed — adding a camera is an SHO's, decommissioning one a DSP's,
-- logging a health check an ASI's — the permission was split until they
-- agreed. That is why some modules have three permissions and others fifteen:
-- the shape follows what the code actually enforces, not a tidy guess.

-- ------------------------------------------------- rank, in SQL -------------

-- The rank ladder exists in Go as models.RoleHierarchy. It has to exist here
-- too, because the seeding below is a set operation over ranks and because a
-- rule that lives only in application code is a rule until somebody writes a
-- second caller.
CREATE OR REPLACE FUNCTION rank_level(r user_role) RETURNS INT AS $$
    SELECT CASE r
        WHEN 'CONSTABLE'      THEN 1
        WHEN 'HEAD_CONSTABLE' THEN 2
        WHEN 'ASI'            THEN 3
        WHEN 'SI'             THEN 4
        WHEN 'INSPECTOR'      THEN 5
        WHEN 'SHO'            THEN 6
        WHEN 'DSP'            THEN 7
        WHEN 'SP'             THEN 8
        WHEN 'DIG'            THEN 9
        WHEN 'IG'             THEN 10
        WHEN 'SECRETARY'      THEN 11
        WHEN 'DGP'            THEN 12
    END;
$$ LANGUAGE sql IMMUTABLE;

COMMENT ON FUNCTION rank_level(user_role) IS
    'Seniority as a number. Must match models.RoleHierarchy in the Go service.';

-- ------------------------------------------------- the catalogue ------------

CREATE TABLE IF NOT EXISTS permissions (
    key         TEXT PRIMARY KEY,
    module      TEXT NOT NULL,
    action      TEXT NOT NULL,
    description TEXT NOT NULL,

    -- The rank floor this capability carried before roles existed, kept so the
    -- seeding below is reproducible and so an administrator can see what the
    -- platform used to require. It is not consulted when a request is
    -- authorised: that is the granted permission's job.
    default_min_rank user_role,

    -- The routes this permission covers, so the administration screen can
    -- answer "what does this actually allow?" without anyone reading Go.
    routes      TEXT[] NOT NULL DEFAULT '{}',

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE permissions IS
    'Every capability the API can be asked for. Derived from the router; see migration 000088.';

CREATE INDEX IF NOT EXISTS idx_permissions_module ON permissions(module);

INSERT INTO permissions (key, module, action, description, default_min_rank, routes) VALUES
    ('access-log.view', 'access-log', 'view', 'View — Access log', 'DSP'::user_role, ARRAY['GET /api/v1/access-log','GET /api/v1/access-log/stats']::TEXT[]),
    ('ai-review.acceptance.view', 'ai-review', 'view', 'View — AI review queue · acceptance', 'DSP'::user_role, ARRAY['GET /api/v1/ai-review/acceptance']::TEXT[]),
    ('ai-review.amend', 'ai-review', 'amend', 'Amend — AI review queue', 'SP'::user_role, ARRAY['PUT /api/v1/ai-review/models/:modelName','PUT /api/v1/ai-review/modules/:module']::TEXT[]),
    ('ai-review.bulk-review.create', 'ai-review', 'create', 'Create — AI review queue · bulk review', 'SHO'::user_role, ARRAY['POST /api/v1/ai-review/bulk-review']::TEXT[]),
    ('ai-review.decisions.assign', 'ai-review', 'assign', 'Assign — AI review queue · decisions', 'SHO'::user_role, ARRAY['POST /api/v1/ai-review/decisions/:id/assign']::TEXT[]),
    ('ai-review.decisions.feedback', 'ai-review', 'feedback', 'Feedback — AI review queue · decisions', NULL, ARRAY['POST /api/v1/ai-review/decisions/:id/feedback']::TEXT[]),
    ('ai-review.decisions.review', 'ai-review', 'review', 'Review — AI review queue · decisions', NULL, ARRAY['POST /api/v1/ai-review/decisions/:id/review']::TEXT[]),
    ('ai-review.decisions.view', 'ai-review', 'view', 'View — AI review queue · decisions', NULL, ARRAY['GET /api/v1/ai-review/decisions/:id','GET /api/v1/ai-review/decisions/:id/history']::TEXT[]),
    ('ai-review.evaluations.create', 'ai-review', 'create', 'Create — AI review queue · evaluations', 'SP'::user_role, ARRAY['POST /api/v1/ai-review/evaluations']::TEXT[]),
    ('ai-review.evaluations.view', 'ai-review', 'view', 'View — AI review queue · evaluations', 'DSP'::user_role, ARRAY['GET /api/v1/ai-review/evaluations']::TEXT[]),
    ('ai-review.expire.create', 'ai-review', 'create', 'Create — AI review queue · expire', 'SP'::user_role, ARRAY['POST /api/v1/ai-review/expire']::TEXT[]),
    ('ai-review.gateway.view', 'ai-review', 'view', 'View — AI review queue · gateway', 'DSP'::user_role, ARRAY['GET /api/v1/ai-review/gateway']::TEXT[]),
    ('ai-review.models.create', 'ai-review', 'create', 'Create — AI review queue · models', 'SP'::user_role, ARRAY['POST /api/v1/ai-review/models','POST /api/v1/ai-review/models/:modelName/retire']::TEXT[]),
    ('ai-review.models.view', 'ai-review', 'view', 'View — AI review queue · models', 'DSP'::user_role, ARRAY['GET /api/v1/ai-review/models','GET /api/v1/ai-review/models/:modelName']::TEXT[]),
    ('ai-review.modules.view', 'ai-review', 'view', 'View — AI review queue · modules', 'DSP'::user_role, ARRAY['GET /api/v1/ai-review/modules']::TEXT[]),
    ('ai-review.my-assignments.view', 'ai-review', 'view', 'View — AI review queue · my assignments', NULL, ARRAY['GET /api/v1/ai-review/my-assignments']::TEXT[]),
    ('ai-review.queue.view', 'ai-review', 'view', 'View — AI review queue · queue', NULL, ARRAY['GET /api/v1/ai-review/queue']::TEXT[]),
    ('ai-review.stats.view', 'ai-review', 'view', 'View — AI review queue · stats', NULL, ARRAY['GET /api/v1/ai-review/stats']::TEXT[]),
    ('alerts.acknowledge', 'alerts', 'acknowledge', 'Acknowledge — Alerts', NULL, ARRAY['POST /api/v1/alerts/:id/acknowledge']::TEXT[]),
    ('alerts.amend', 'alerts', 'amend', 'Amend — Alerts', 'SHO'::user_role, ARRAY['PUT /api/v1/alerts/:id']::TEXT[]),
    ('alerts.create', 'alerts', 'create', 'Create — Alerts', 'SHO'::user_role, ARRAY['POST /api/v1/alerts']::TEXT[]),
    ('alerts.delete', 'alerts', 'delete', 'Delete — Alerts', 'SHO'::user_role, ARRAY['DELETE /api/v1/alerts/:id']::TEXT[]),
    ('alerts.view', 'alerts', 'view', 'View — Alerts', NULL, ARRAY['GET /api/v1/alerts','GET /api/v1/alerts/:id','GET /api/v1/alerts/active','GET /api/v1/alerts/unacknowledged']::TEXT[]),
    ('anpr.access-log.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · access log', 'DSP'::user_role, ARRAY['GET /api/v1/anpr/access-log']::TEXT[]),
    ('anpr.amend', 'anpr', 'amend', 'Amend — Vehicle detection and ANPR', 'DSP'::user_role, ARRAY['PUT /api/v1/anpr/switch']::TEXT[]),
    ('anpr.analyses.create', 'anpr', 'create', 'Create — Vehicle detection and ANPR · analyses', 'ASI'::user_role, ARRAY['POST /api/v1/anpr/analyses','POST /api/v1/anpr/analyses/:id/access']::TEXT[]),
    ('anpr.analyses.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · analyses', 'ASI'::user_role, ARRAY['GET /api/v1/anpr/analyses','GET /api/v1/anpr/analyses/:id/frames/:frameId/image']::TEXT[]),
    ('anpr.hits.create', 'anpr', 'create', 'Create — Vehicle detection and ANPR · hits', 'SI'::user_role, ARRAY['POST /api/v1/anpr/hits/:id/review']::TEXT[]),
    ('anpr.hits.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · hits', 'ASI'::user_role, ARRAY['GET /api/v1/anpr/hits']::TEXT[]),
    ('anpr.map.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · map', 'ASI'::user_role, ARRAY['GET /api/v1/anpr/map']::TEXT[]),
    ('anpr.reads.create', 'anpr', 'create', 'Create — Vehicle detection and ANPR · reads', 'ASI'::user_role, ARRAY['POST /api/v1/anpr/reads/search']::TEXT[]),
    ('anpr.snapshots.create', 'anpr', 'create', 'Create — Vehicle detection and ANPR · snapshots', 'ASI'::user_role, ARRAY['POST /api/v1/anpr/snapshots']::TEXT[]),
    ('anpr.status.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · status', NULL, ARRAY['GET /api/v1/anpr/status']::TEXT[]),
    ('anpr.watchlist.create', 'anpr', 'create', 'Create — Vehicle detection and ANPR · watchlist', 'SI'::user_role, ARRAY['POST /api/v1/anpr/watchlist','POST /api/v1/anpr/watchlist/:id/remove']::TEXT[]),
    ('anpr.watchlist.view', 'anpr', 'view', 'View — Vehicle detection and ANPR · watchlist', 'ASI'::user_role, ARRAY['GET /api/v1/anpr/watchlist']::TEXT[]),
    ('armoury.amend', 'armoury', 'amend', 'Amend — Armoury', 'SHO'::user_role, ARRAY['PATCH /api/v1/armoury/weapons/:id/state']::TEXT[]),
    ('armoury.view', 'armoury', 'view', 'View — Armoury', NULL, ARRAY['GET /api/v1/armoury/issuances','GET /api/v1/armoury/weapons','GET /api/v1/armoury/weapons/:id','GET /api/v1/armoury/weapons/:id/issuances','GET /api/v1/armoury/weapons/stats']::TEXT[]),
    ('armoury.weapons.create', 'armoury', 'create', 'Create — Armoury · weapons', 'SHO'::user_role, ARRAY['POST /api/v1/armoury/weapons']::TEXT[]),
    ('armoury.weapons.issue', 'armoury', 'issue', 'Issue — Armoury · weapons', 'ASI'::user_role, ARRAY['POST /api/v1/armoury/weapons/:id/issue']::TEXT[]),
    ('armoury.weapons.return', 'armoury', 'return', 'Return — Armoury · weapons', 'ASI'::user_role, ARRAY['POST /api/v1/armoury/weapons/:id/return']::TEXT[]),
    ('audit.view', 'audit', 'view', 'View — Audit trail', 'DSP'::user_role, ARRAY['GET /api/v1/audit/logs','GET /api/v1/audit/stats','GET /api/v1/audit/verify']::TEXT[]),
    ('bail.amend', 'bail', 'amend', 'Amend — Bail', 'SI'::user_role, ARRAY['PUT /api/v1/bail/:id','PATCH /api/v1/bail/:id/status']::TEXT[]),
    ('bail.create', 'bail', 'create', 'Create — Bail', 'SI'::user_role, ARRAY['POST /api/v1/bail']::TEXT[]),
    ('bail.view', 'bail', 'view', 'View — Bail', NULL, ARRAY['GET /api/v1/bail','GET /api/v1/bail/:id','GET /api/v1/bail/stats']::TEXT[]),
    ('biometric.amend', 'biometric', 'amend', 'Amend — Face match review', 'SHO'::user_role, ARRAY['PATCH /api/v1/biometric/devices/:id/status']::TEXT[]),
    ('biometric.devices.create', 'biometric', 'create', 'Create — Face match review · devices', 'SHO'::user_role, ARRAY['POST /api/v1/biometric/devices']::TEXT[]),
    ('biometric.devices.heartbeat', 'biometric', 'heartbeat', 'Heartbeat — Face match review · devices', NULL, ARRAY['POST /api/v1/biometric/devices/:id/heartbeat']::TEXT[]),
    ('biometric.evidence.create', 'biometric', 'create', 'Create — Face match review · evidence', NULL, ARRAY['POST /api/v1/biometric/evidence/:evidenceId/verify']::TEXT[]),
    ('biometric.identifications.create', 'biometric', 'create', 'Create — Face match review · identifications', NULL, ARRAY['POST /api/v1/biometric/identifications/:id/confirm']::TEXT[]),
    ('biometric.identify.create', 'biometric', 'create', 'Create — Face match review · identify', NULL, ARRAY['POST /api/v1/biometric/identify']::TEXT[]),
    ('biometric.templates.create', 'biometric', 'create', 'Create — Face match review · templates', NULL, ARRAY['POST /api/v1/biometric/templates']::TEXT[]),
    ('biometric.verify.create', 'biometric', 'create', 'Create — Face match review · verify', NULL, ARRAY['POST /api/v1/biometric/verify','POST /api/v1/biometric/verify/aadhaar']::TEXT[]),
    ('biometric.view', 'biometric', 'view', 'View — Face match review', NULL, ARRAY['GET /api/v1/biometric/devices','GET /api/v1/biometric/devices/:id','GET /api/v1/biometric/stats','GET /api/v1/biometric/verifications/:subjectId/history']::TEXT[]),
    ('biometric.weapons.create', 'biometric', 'create', 'Create — Face match review · weapons', NULL, ARRAY['POST /api/v1/biometric/weapons/:weaponId/verify']::TEXT[]),
    ('bodycam.amend', 'bodycam', 'amend', 'Amend — Body-worn camera', 'SHO'::user_role, ARRAY['PATCH /api/v1/bodycam/devices/:id/status']::TEXT[]),
    ('bodycam.devices.assignments', 'bodycam', 'assignments', 'Assignments — Body-worn camera · devices', 'ASI'::user_role, ARRAY['POST /api/v1/bodycam/devices/:id/assignments']::TEXT[]),
    ('bodycam.devices.assignments.recordings', 'bodycam', 'recordings', 'Recordings — Body-worn camera · devices · assignments', NULL, ARRAY['POST /api/v1/bodycam/devices/:id/assignments/:assignmentId/recordings']::TEXT[]),
    ('bodycam.devices.assignments.return', 'bodycam', 'return', 'Return — Body-worn camera · devices · assignments', 'ASI'::user_role, ARRAY['POST /api/v1/bodycam/devices/:id/assignments/:assignmentId/return']::TEXT[]),
    ('bodycam.devices.create', 'bodycam', 'create', 'Create — Body-worn camera · devices', 'SHO'::user_role, ARRAY['POST /api/v1/bodycam/devices']::TEXT[]),
    ('bodycam.devices.readings', 'bodycam', 'readings', 'Readings — Body-worn camera · devices', 'ASI'::user_role, ARRAY['POST /api/v1/bodycam/devices/:id/readings']::TEXT[]),
    ('bodycam.devices.view', 'bodycam', 'view', 'View — Body-worn camera · devices', NULL, ARRAY['GET /api/v1/bodycam/devices','GET /api/v1/bodycam/devices/:id','GET /api/v1/bodycam/devices/:id/assignments','GET /api/v1/bodycam/devices/:id/readings','GET /api/v1/bodycam/devices/stats']::TEXT[]),
    ('bodycam.recordings.access', 'bodycam', 'access', 'Access — Body-worn camera · recordings', 'ASI'::user_role, ARRAY['POST /api/v1/bodycam/recordings/:id/access']::TEXT[]),
    ('bodycam.recordings.access-log.view', 'bodycam', 'view', 'View — Body-worn camera · recordings · access log', 'DSP'::user_role, ARRAY['GET /api/v1/bodycam/recordings/:id/access-log']::TEXT[]),
    ('bodycam.recordings.custody.view', 'bodycam', 'view', 'View — Body-worn camera · recordings · custody', 'ASI'::user_role, ARRAY['GET /api/v1/bodycam/recordings/:id/custody']::TEXT[]),
    ('bodycam.recordings.file.view', 'bodycam', 'view', 'View — Body-worn camera · recordings · file', 'ASI'::user_role, ARRAY['GET /api/v1/bodycam/recordings/:id/file']::TEXT[]),
    ('bodycam.recordings.link', 'bodycam', 'link', 'Link — Body-worn camera · recordings', 'SI'::user_role, ARRAY['POST /api/v1/bodycam/recordings/:id/link']::TEXT[]),
    ('bodycam.recordings.purge', 'bodycam', 'purge', 'Purge — Body-worn camera · recordings', 'DSP'::user_role, ARRAY['POST /api/v1/bodycam/recordings/:id/purge']::TEXT[]),
    ('bodycam.recordings.purge-expired.create', 'bodycam', 'create', 'Create — Body-worn camera · recordings · purge expired', 'DSP'::user_role, ARRAY['POST /api/v1/bodycam/recordings/purge-expired']::TEXT[]),
    ('bodycam.recordings.verify', 'bodycam', 'verify', 'Verify — Body-worn camera · recordings', 'ASI'::user_role, ARRAY['POST /api/v1/bodycam/recordings/:id/verify']::TEXT[]),
    ('bodycam.recordings.view', 'bodycam', 'view', 'View — Body-worn camera · recordings', 'ASI'::user_role, ARRAY['GET /api/v1/bodycam/recordings']::TEXT[]),
    ('case-files.charges', 'case-files', 'charges', 'Charges — Case file and court readiness', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/charges']::TEXT[]),
    ('case-files.charges.create', 'case-files', 'create', 'Create — Case file and court readiness · charges', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/charges/:chargeId/evidence']::TEXT[]),
    ('case-files.create', 'case-files', 'create', 'Create — Case file and court readiness', 'SI'::user_role, ARRAY['POST /api/v1/case-files']::TEXT[]),
    ('case-files.delete', 'case-files', 'delete', 'Delete — Case file and court readiness', 'SI'::user_role, ARRAY['DELETE /api/v1/case-files/:id/charges/:chargeId','DELETE /api/v1/case-files/:id/charges/:chargeId/evidence/:evidenceId','DELETE /api/v1/case-files/:id/witness-facts/:factId']::TEXT[]),
    ('case-files.entries', 'case-files', 'entries', 'Entries — Case file and court readiness', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/entries']::TEXT[]),
    ('case-files.entries.create', 'case-files', 'create', 'Create — Case file and court readiness · entries', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/entries/:entryId/remove','POST /api/v1/case-files/:id/entries/upload']::TEXT[]),
    ('case-files.packs', 'case-files', 'packs', 'Packs — Case file and court readiness', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/packs']::TEXT[]),
    ('case-files.packs.create', 'case-files', 'create', 'Create — Case file and court readiness · packs', 'INSPECTOR'::user_role, ARRAY['POST /api/v1/case-files/:id/packs/:packId/approve','POST /api/v1/case-files/:id/packs/:packId/return']::TEXT[]),
    ('case-files.view', 'case-files', 'view', 'View — Case file and court readiness', 'ASI'::user_role, ARRAY['GET /api/v1/case-files','GET /api/v1/case-files/:id','GET /api/v1/case-files/:id/completeness','GET /api/v1/case-files/:id/entries','GET /api/v1/case-files/:id/entries/:entryId/file','GET /api/v1/case-files/:id/evidence-matrix','GET /api/v1/case-files/:id/packs','GET /api/v1/case-files/:id/packs/:packId','GET /api/v1/case-files/:id/sources','GET /api/v1/case-files/:id/versions','GET /api/v1/case-files/:id/versions/:version','GET /api/v1/case-files/:id/witness-matrix','GET /api/v1/case-files/by-workspace/:workspaceId']::TEXT[]),
    ('case-files.witness-facts', 'case-files', 'witness-facts', 'Witness facts — Case file and court readiness', 'SI'::user_role, ARRAY['POST /api/v1/case-files/:id/witness-facts']::TEXT[]),
    ('cases.amend', 'cases', 'amend', 'Amend — Case register', 'SI'::user_role, ARRAY['PUT /api/v1/cases/:id']::TEXT[]),
    ('cases.create', 'cases', 'create', 'Create — Case register', 'SI'::user_role, ARRAY['POST /api/v1/cases','POST /api/v1/cases/:id/accused','POST /api/v1/cases/:id/witnesses']::TEXT[]),
    ('cases.view', 'cases', 'view', 'View — Case register', NULL, ARRAY['GET /api/v1/cases','GET /api/v1/cases/:id','GET /api/v1/cases/:id/accused','GET /api/v1/cases/:id/witnesses']::TEXT[]),
    ('complaints.categorise', 'complaints', 'categorise', 'Categorise — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/categorise']::TEXT[]),
    ('complaints.create', 'complaints', 'create', 'Create — Citizen grievance', NULL, ARRAY['POST /api/v1/complaints']::TEXT[]),
    ('complaints.link-duplicate', 'complaints', 'link-duplicate', 'Link duplicate — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/link-duplicate']::TEXT[]),
    ('complaints.link-fir', 'complaints', 'link-fir', 'Link fir — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/link-fir']::TEXT[]),
    ('complaints.notes', 'complaints', 'notes', 'Notes — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/notes']::TEXT[]),
    ('complaints.responses', 'complaints', 'responses', 'Responses — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/responses']::TEXT[]),
    ('complaints.responses.create', 'complaints', 'create', 'Create — Citizen grievance · responses', 'SI'::user_role, ARRAY['POST /api/v1/complaints/:id/responses/:responseId/review']::TEXT[]),
    ('complaints.route', 'complaints', 'route', 'Route — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/route']::TEXT[]),
    ('complaints.status', 'complaints', 'status', 'Status — Citizen grievance', 'ASI'::user_role, ARRAY['POST /api/v1/complaints/:id/status']::TEXT[]),
    ('complaints.view', 'complaints', 'view', 'View — Citizen grievance', NULL, ARRAY['GET /api/v1/complaints','GET /api/v1/complaints/:id','GET /api/v1/complaints/:id/duplicate-candidates','GET /api/v1/complaints/routing-targets','GET /api/v1/complaints/stats']::TEXT[]),
    ('court.amend', 'court', 'amend', 'Amend — Court diary', 'SI'::user_role, ARRAY['PUT /api/v1/court/hearings/:id']::TEXT[]),
    ('court.create', 'court', 'create', 'Create — Court diary', 'SI'::user_role, ARRAY['POST /api/v1/court/hearings','POST /api/v1/court/orders']::TEXT[]),
    ('court.view', 'court', 'view', 'View — Court diary', NULL, ARRAY['GET /api/v1/court/hearings','GET /api/v1/court/hearings/:id','GET /api/v1/court/orders','GET /api/v1/court/orders/:id','GET /api/v1/court/stats']::TEXT[]),
    ('custody.create', 'custody', 'create', 'Create — Custody', NULL, ARRAY['POST /api/v1/custody','POST /api/v1/custody/:id/file','POST /api/v1/custody/:id/transfer','POST /api/v1/custody/:id/verify']::TEXT[]),
    ('custody.view', 'custody', 'view', 'View — Custody', NULL, ARRAY['GET /api/v1/custody','GET /api/v1/custody/:id','GET /api/v1/custody/:id/access-log','GET /api/v1/custody/:id/chain','GET /api/v1/custody/:id/court-verification','GET /api/v1/custody/:id/file','GET /api/v1/custody/:id/verifications','GET /api/v1/custody/stats']::TEXT[]),
    ('cyber-crime.amend', 'cyber-crime', 'amend', 'Amend — Cyber and financial fraud', 'SI'::user_role, ARRAY['PUT /api/v1/cyber-crime/:id','PATCH /api/v1/cyber-crime/:id/status']::TEXT[]),
    ('cyber-crime.create', 'cyber-crime', 'create', 'Create — Cyber and financial fraud', 'SI'::user_role, ARRAY['POST /api/v1/cyber-crime']::TEXT[]),
    ('cyber-crime.delete', 'cyber-crime', 'delete', 'Delete — Cyber and financial fraud', 'SI'::user_role, ARRAY['DELETE /api/v1/cyber-crime/:id/entities/:linkId']::TEXT[]),
    ('cyber-crime.entities', 'cyber-crime', 'entities', 'Entities — Cyber and financial fraud', 'ASI'::user_role, ARRAY['POST /api/v1/cyber-crime/:id/entities']::TEXT[]),
    ('cyber-crime.freeze-requests', 'cyber-crime', 'freeze-requests', 'Freeze requests — Cyber and financial fraud', 'SI'::user_role, ARRAY['POST /api/v1/cyber-crime/:id/freeze-requests']::TEXT[]),
    ('cyber-crime.freeze-requests.create', 'cyber-crime', 'create', 'Create — Cyber and financial fraud · freeze requests', 'SI'::user_role, ARRAY['POST /api/v1/cyber-crime/:id/freeze-requests/:freezeId/transition']::TEXT[]),
    ('cyber-crime.recoveries', 'cyber-crime', 'recoveries', 'Recoveries — Cyber and financial fraud', 'SI'::user_role, ARRAY['POST /api/v1/cyber-crime/:id/recoveries']::TEXT[]),
    ('cyber-crime.transactions', 'cyber-crime', 'transactions', 'Transactions — Cyber and financial fraud', 'ASI'::user_role, ARRAY['POST /api/v1/cyber-crime/:id/transactions']::TEXT[]),
    ('cyber-crime.view', 'cyber-crime', 'view', 'View — Cyber and financial fraud', NULL, ARRAY['GET /api/v1/cyber-crime','GET /api/v1/cyber-crime/:id','GET /api/v1/cyber-crime/:id/entities','GET /api/v1/cyber-crime/:id/freeze-requests','GET /api/v1/cyber-crime/:id/network','GET /api/v1/cyber-crime/:id/recoveries','GET /api/v1/cyber-crime/:id/transactions','GET /api/v1/cyber-crime/clusters','GET /api/v1/cyber-crime/dashboard','GET /api/v1/cyber-crime/entities']::TEXT[]),
    ('dashboard.view', 'dashboard', 'view', 'View — Dashboard', NULL, ARRAY['GET /api/v1/dashboard/stats']::TEXT[]),
    ('dispatch.analytics.view', 'dispatch', 'view', 'View — Dispatch and response · analytics', 'SI'::user_role, ARRAY['GET /api/v1/dispatch/analytics']::TEXT[]),
    ('dispatch.assignments.acknowledge', 'dispatch', 'acknowledge', 'Acknowledge — Dispatch and response · assignments', NULL, ARRAY['POST /api/v1/dispatch/assignments/:assignmentId/acknowledge']::TEXT[]),
    ('dispatch.assignments.cancel', 'dispatch', 'cancel', 'Cancel — Dispatch and response · assignments', 'ASI'::user_role, ARRAY['POST /api/v1/dispatch/assignments/:assignmentId/cancel']::TEXT[]),
    ('dispatch.assignments.clear', 'dispatch', 'clear', 'Clear — Dispatch and response · assignments', NULL, ARRAY['POST /api/v1/dispatch/assignments/:assignmentId/clear']::TEXT[]),
    ('dispatch.assignments.on-scene', 'dispatch', 'on-scene', 'On scene — Dispatch and response · assignments', NULL, ARRAY['POST /api/v1/dispatch/assignments/:assignmentId/on-scene']::TEXT[]),
    ('dispatch.incidents.assign', 'dispatch', 'assign', 'Assign — Dispatch and response · incidents', 'ASI'::user_role, ARRAY['POST /api/v1/dispatch/incidents/:id/assign']::TEXT[]),
    ('dispatch.incidents.classify', 'dispatch', 'classify', 'Classify — Dispatch and response · incidents', 'ASI'::user_role, ARRAY['POST /api/v1/dispatch/incidents/:id/classify']::TEXT[]),
    ('dispatch.incidents.close', 'dispatch', 'close', 'Close — Dispatch and response · incidents', 'ASI'::user_role, ARRAY['POST /api/v1/dispatch/incidents/:id/close']::TEXT[]),
    ('dispatch.incidents.create', 'dispatch', 'create', 'Create — Dispatch and response · incidents', NULL, ARRAY['POST /api/v1/dispatch/incidents']::TEXT[]),
    ('dispatch.incidents.escalate', 'dispatch', 'escalate', 'Escalate — Dispatch and response · incidents', 'ASI'::user_role, ARRAY['POST /api/v1/dispatch/incidents/:id/escalate']::TEXT[]),
    ('dispatch.incidents.view', 'dispatch', 'view', 'View — Dispatch and response · incidents', NULL, ARRAY['GET /api/v1/dispatch/incidents','GET /api/v1/dispatch/incidents/:id','GET /api/v1/dispatch/incidents/:id/events']::TEXT[]),
    ('dispatch.policy.view', 'dispatch', 'view', 'View — Dispatch and response · policy', NULL, ARRAY['GET /api/v1/dispatch/policy']::TEXT[]),
    ('dispatch.stats.view', 'dispatch', 'view', 'View — Dispatch and response · stats', NULL, ARRAY['GET /api/v1/dispatch/stats']::TEXT[]),
    ('dispatch.units.view', 'dispatch', 'view', 'View — Dispatch and response · units', NULL, ARRAY['GET /api/v1/dispatch/units']::TEXT[]),
    ('district.amend', 'district', 'amend', 'Amend — District and stations', 'SP'::user_role, ARRAY['PUT /api/v1/district/:id']::TEXT[]),
    ('district.coordination.requests.approve', 'district', 'approve', 'Approve — District and stations · coordination · requests', 'DSP'::user_role, ARRAY['POST /api/v1/district/coordination/requests/:id/approve']::TEXT[]),
    ('district.coordination.requests.complete', 'district', 'complete', 'Complete — District and stations · coordination · requests', 'SHO'::user_role, ARRAY['POST /api/v1/district/coordination/requests/:id/complete']::TEXT[]),
    ('district.coordination.requests.create', 'district', 'create', 'Create — District and stations · coordination · requests', 'SHO'::user_role, ARRAY['POST /api/v1/district/coordination/requests']::TEXT[]),
    ('district.coordination.requests.reject', 'district', 'reject', 'Reject — District and stations · coordination · requests', 'DSP'::user_role, ARRAY['POST /api/v1/district/coordination/requests/:id/reject']::TEXT[]),
    ('district.create', 'district', 'create', 'Create — District and stations', 'SP'::user_role, ARRAY['POST /api/v1/district']::TEXT[]),
    ('district.meetings.amend', 'district', 'amend', 'Amend — District and stations · meetings', 'DSP'::user_role, ARRAY['PUT /api/v1/district/meetings/:id']::TEXT[]),
    ('district.meetings.create', 'district', 'create', 'Create — District and stations · meetings', 'DSP'::user_role, ARRAY['POST /api/v1/district/meetings']::TEXT[]),
    ('district.resources.allocate', 'district', 'allocate', 'Allocate — District and stations · resources', 'DSP'::user_role, ARRAY['POST /api/v1/district/resources/:id/allocate']::TEXT[]),
    ('district.resources.approve', 'district', 'approve', 'Approve — District and stations · resources', 'DSP'::user_role, ARRAY['POST /api/v1/district/resources/:id/approve']::TEXT[]),
    ('district.resources.create', 'district', 'create', 'Create — District and stations · resources', 'SHO'::user_role, ARRAY['POST /api/v1/district/resources']::TEXT[]),
    ('district.resources.return', 'district', 'return', 'Return — District and stations · resources', NULL, ARRAY['POST /api/v1/district/resources/:id/return']::TEXT[]),
    ('district.stations.amend', 'district', 'amend', 'Amend — District and stations · stations', 'SP'::user_role, ARRAY['PUT /api/v1/district/stations/:id']::TEXT[]),
    ('district.stations.create', 'district', 'create', 'Create — District and stations · stations', 'SP'::user_role, ARRAY['POST /api/v1/district/stations']::TEXT[]),
    ('district.view', 'district', 'view', 'View — District and stations', NULL, ARRAY['GET /api/v1/district','GET /api/v1/district/:id','GET /api/v1/district/:id/dashboard','GET /api/v1/district/:id/hotspots','GET /api/v1/district/:id/rankings','GET /api/v1/district/:id/tasks','GET /api/v1/district/coordination/requests','GET /api/v1/district/coordination/requests/:id','GET /api/v1/district/meetings','GET /api/v1/district/meetings/:id','GET /api/v1/district/resources','GET /api/v1/district/resources/:id','GET /api/v1/district/stations','GET /api/v1/district/stations/:id']::TEXT[]),
    ('evidence.amend', 'evidence', 'amend', 'Amend — Evidence and chain of custody', NULL, ARRAY['PUT /api/v1/evidence/:id']::TEXT[]),
    ('evidence.create', 'evidence', 'create', 'Create — Evidence and chain of custody', NULL, ARRAY['POST /api/v1/evidence']::TEXT[]),
    ('evidence.view', 'evidence', 'view', 'View — Evidence and chain of custody', NULL, ARRAY['GET /api/v1/evidence','GET /api/v1/evidence/:id','GET /api/v1/evidence/:id/custody']::TEXT[]),
    ('face-recognition.amend', 'face-recognition', 'amend', 'Amend — Face recognition', 'SP'::user_role, ARRAY['PUT /api/v1/face-recognition/settings']::TEXT[]),
    ('face-recognition.authorisations.create', 'face-recognition', 'create', 'Create — Face recognition · authorisations', 'SP'::user_role, ARRAY['POST /api/v1/face-recognition/authorisations','POST /api/v1/face-recognition/authorisations/:id/revoke']::TEXT[]),
    ('face-recognition.authorisations.view', 'face-recognition', 'view', 'View — Face recognition · authorisations', NULL, ARRAY['GET /api/v1/face-recognition/authorisations']::TEXT[]),
    ('face-recognition.camera-snapshots.create', 'face-recognition', 'create', 'Create — Face recognition · camera snapshots', 'ASI'::user_role, ARRAY['POST /api/v1/face-recognition/camera-snapshots']::TEXT[]),
    ('face-recognition.candidates.create', 'face-recognition', 'create', 'Create — Face recognition · candidates', 'ASI'::user_role, ARRAY['POST /api/v1/face-recognition/candidates/:id/confirm','POST /api/v1/face-recognition/candidates/:id/reject']::TEXT[]),
    ('face-recognition.candidates.view', 'face-recognition', 'view', 'View — Face recognition · candidates', 'ASI'::user_role, ARRAY['GET /api/v1/face-recognition/candidates','GET /api/v1/face-recognition/candidates/:id','GET /api/v1/face-recognition/candidates/:id/crop','GET /api/v1/face-recognition/candidates/:id/frame']::TEXT[]),
    ('face-recognition.enrolments.view', 'face-recognition', 'view', 'View — Face recognition · enrolments', NULL, ARRAY['GET /api/v1/face-recognition/enrolments/:id/face']::TEXT[]),
    ('face-recognition.searches.create', 'face-recognition', 'create', 'Create — Face recognition · searches', 'ASI'::user_role, ARRAY['POST /api/v1/face-recognition/searches']::TEXT[]),
    ('face-recognition.searches.view', 'face-recognition', 'view', 'View — Face recognition · searches', 'DSP'::user_role, ARRAY['GET /api/v1/face-recognition/searches']::TEXT[]),
    ('face-recognition.status.view', 'face-recognition', 'view', 'View — Face recognition · status', NULL, ARRAY['GET /api/v1/face-recognition/status']::TEXT[]),
    ('files.delete', 'files', 'delete', 'Delete — Files', 'SI'::user_role, ARRAY['DELETE /api/v1/files/*key']::TEXT[]),
    ('files.view', 'files', 'view', 'View — Files', NULL, ARRAY['GET /api/v1/files/*key']::TEXT[]),
    ('firs.amend', 'firs', 'amend', 'Amend — FIR and general diary', 'SI'::user_role, ARRAY['PUT /api/v1/firs/:id','PATCH /api/v1/firs/:id/status']::TEXT[]),
    ('firs.create', 'firs', 'create', 'Create — FIR and general diary', 'SI'::user_role, ARRAY['POST /api/v1/firs']::TEXT[]),
    ('firs.view', 'firs', 'view', 'View — FIR and general diary', NULL, ARRAY['GET /api/v1/firs','GET /api/v1/firs/:id','GET /api/v1/firs/:id/timeline']::TEXT[]),
    ('forensics.amend', 'forensics', 'amend', 'Amend — Forensics', NULL, ARRAY['PUT /api/v1/forensics/:id']::TEXT[]),
    ('forensics.create', 'forensics', 'create', 'Create — Forensics', NULL, ARRAY['POST /api/v1/forensics','POST /api/v1/forensics/:id/complete']::TEXT[]),
    ('forensics.view', 'forensics', 'view', 'View — Forensics', NULL, ARRAY['GET /api/v1/forensics','GET /api/v1/forensics/:id','GET /api/v1/forensics/stats']::TEXT[]),
    ('gazetteer.create', 'gazetteer', 'create', 'Create — Places', 'SP'::user_role, ARRAY['POST /api/v1/gazetteer/places','POST /api/v1/gazetteer/places/:id/retire']::TEXT[]),
    ('gazetteer.view', 'gazetteer', 'view', 'View — Places', NULL, ARRAY['GET /api/v1/gazetteer/nearest','GET /api/v1/gazetteer/places','GET /api/v1/gazetteer/search']::TEXT[]),
    ('graph.amend', 'graph', 'amend', 'Amend — Link analysis', 'SI'::user_role, ARRAY['PUT /api/v1/graph/nodes/:id']::TEXT[]),
    ('graph.edges.create', 'graph', 'create', 'Create — Link analysis · edges', 'SI'::user_role, ARRAY['POST /api/v1/graph/edges']::TEXT[]),
    ('graph.networks.create', 'graph', 'create', 'Create — Link analysis · networks', 'SHO'::user_role, ARRAY['POST /api/v1/graph/networks']::TEXT[]),
    ('graph.networks.members', 'graph', 'members', 'Members — Link analysis · networks', 'SI'::user_role, ARRAY['POST /api/v1/graph/networks/:id/members']::TEXT[]),
    ('graph.nodes.create', 'graph', 'create', 'Create — Link analysis · nodes', 'SI'::user_role, ARRAY['POST /api/v1/graph/nodes']::TEXT[]),
    ('graph.view', 'graph', 'view', 'View — Link analysis', NULL, ARRAY['GET /api/v1/graph/networks','GET /api/v1/graph/networks/:id','GET /api/v1/graph/networks/:id/members','GET /api/v1/graph/nodes','GET /api/v1/graph/nodes/:id','GET /api/v1/graph/nodes/:id/connections','GET /api/v1/graph/stats','GET /api/v1/graph/visualization']::TEXT[]),
    ('intel.view', 'intel', 'view', 'View — IP and OSINT lookup', NULL, ARRAY['GET /api/v1/intel/ip/:ip']::TEXT[]),
    ('investigation.amend', 'investigation', 'amend', 'Amend — Investigation copilot', NULL, ARRAY['PUT /api/v1/investigation/:id','PATCH /api/v1/investigation/:id/gaps/:gapId','PATCH /api/v1/investigation/:id/persons/:personId','PATCH /api/v1/investigation/:id/tasks/:taskId']::TEXT[]),
    ('investigation.create', 'investigation', 'create', 'Create — Investigation copilot', NULL, ARRAY['POST /api/v1/investigation','POST /api/v1/investigation/:id/contradictions','POST /api/v1/investigation/:id/contradictions/:contradictionId/review','POST /api/v1/investigation/:id/evidence','POST /api/v1/investigation/:id/gaps','POST /api/v1/investigation/:id/gaps/recompute','POST /api/v1/investigation/:id/persons','POST /api/v1/investigation/:id/tasks','POST /api/v1/investigation/:id/timeline','POST /api/v1/investigation/:id/timeline/:entryId/review']::TEXT[]),
    ('investigation.delete', 'investigation', 'delete', 'Delete — Investigation copilot', NULL, ARRAY['DELETE /api/v1/investigation/:id/evidence/:evidenceId','DELETE /api/v1/investigation/:id/persons/:personId','DELETE /api/v1/investigation/:id/tasks/:taskId','DELETE /api/v1/investigation/:id/timeline/:entryId']::TEXT[]),
    ('investigation.view', 'investigation', 'view', 'View — Investigation copilot', NULL, ARRAY['GET /api/v1/investigation','GET /api/v1/investigation/:id','GET /api/v1/investigation/:id/brief','GET /api/v1/investigation/:id/contradictions','GET /api/v1/investigation/:id/evidence','GET /api/v1/investigation/:id/gaps','GET /api/v1/investigation/:id/links','GET /api/v1/investigation/:id/persons','GET /api/v1/investigation/:id/tasks','GET /api/v1/investigation/:id/timeline','GET /api/v1/investigation/officers']::TEXT[]),
    ('knowledge.amend', 'knowledge', 'amend', 'Amend — Knowledge assistant', 'SP'::user_role, ARRAY['PATCH /api/v1/knowledge/documents/:id/classification']::TEXT[]),
    ('knowledge.checklists.create', 'knowledge', 'create', 'Create — Knowledge assistant · checklists', 'SI'::user_role, ARRAY['POST /api/v1/knowledge/checklists']::TEXT[]),
    ('knowledge.checklists.runs', 'knowledge', 'runs', 'Runs — Knowledge assistant · checklists', 'ASI'::user_role, ARRAY['POST /api/v1/knowledge/checklists/:id/runs']::TEXT[]),
    ('knowledge.documents.create', 'knowledge', 'create', 'Create — Knowledge assistant · documents', 'SI'::user_role, ARRAY['POST /api/v1/knowledge/documents']::TEXT[]),
    ('knowledge.documents.supersede', 'knowledge', 'supersede', 'Supersede — Knowledge assistant · documents', 'SI'::user_role, ARRAY['POST /api/v1/knowledge/documents/:id/supersede']::TEXT[]),
    ('knowledge.documents.withdraw', 'knowledge', 'withdraw', 'Withdraw — Knowledge assistant · documents', 'SP'::user_role, ARRAY['POST /api/v1/knowledge/documents/:id/withdraw']::TEXT[]),
    ('knowledge.runs.create', 'knowledge', 'create', 'Create — Knowledge assistant · runs', 'ASI'::user_role, ARRAY['POST /api/v1/knowledge/runs/:id/ticks']::TEXT[]),
    ('knowledge.view', 'knowledge', 'view', 'View — Knowledge assistant', NULL, ARRAY['GET /api/v1/knowledge/capabilities','GET /api/v1/knowledge/checklists','GET /api/v1/knowledge/checklists/:id','GET /api/v1/knowledge/checklists/:id/runs','GET /api/v1/knowledge/documents','GET /api/v1/knowledge/documents/:id','GET /api/v1/knowledge/documents/:id/file','GET /api/v1/knowledge/documents/stats','GET /api/v1/knowledge/runs/:id']::TEXT[]),
    ('legal.amend', 'legal', 'amend', 'Amend — Statute library', 'SP'::user_role, ARRAY['PUT /api/v1/legal/acts/:id','PUT /api/v1/legal/sections/:id']::TEXT[]),
    ('legal.create', 'legal', 'create', 'Create — Statute library', 'SP'::user_role, ARRAY['POST /api/v1/legal/acts','POST /api/v1/legal/acts/:id/retire','POST /api/v1/legal/acts/:id/sections','POST /api/v1/legal/sections/:id/retire']::TEXT[]),
    ('legal.view', 'legal', 'view', 'View — Statute library', NULL, ARRAY['GET /api/v1/legal/acts','GET /api/v1/legal/acts/:id','GET /api/v1/legal/acts/:id/sections','GET /api/v1/legal/correspondence','GET /api/v1/legal/sections','GET /api/v1/legal/sections/:id']::TEXT[]),
    ('lookouts.create', 'lookouts', 'create', 'Create — Lookout notices', 'SI'::user_role, ARRAY['POST /api/v1/lookouts']::TEXT[]),
    ('lookouts.resolve', 'lookouts', 'resolve', 'Resolve — Lookout notices', 'SI'::user_role, ARRAY['POST /api/v1/lookouts/:id/resolve']::TEXT[]),
    ('lookouts.sightings', 'lookouts', 'sightings', 'Sightings — Lookout notices', NULL, ARRAY['POST /api/v1/lookouts/:id/sightings']::TEXT[]),
    ('lookouts.sightings.create', 'lookouts', 'create', 'Create — Lookout notices · sightings', 'ASI'::user_role, ARRAY['POST /api/v1/lookouts/:id/sightings/:sightingId/verify']::TEXT[]),
    ('lookouts.view', 'lookouts', 'view', 'View — Lookout notices', NULL, ARRAY['GET /api/v1/lookouts','GET /api/v1/lookouts/:id','GET /api/v1/lookouts/:id/sightings','GET /api/v1/lookouts/stats']::TEXT[]),
    ('malkhana.items.create', 'malkhana', 'create', 'Create — Malkhana · items', 'ASI'::user_role, ARRAY['POST /api/v1/malkhana/items']::TEXT[]),
    ('malkhana.items.dispose', 'malkhana', 'dispose', 'Dispose — Malkhana · items', 'SHO'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/dispose']::TEXT[]),
    ('malkhana.items.movements', 'malkhana', 'movements', 'Movements — Malkhana · items', 'ASI'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/movements']::TEXT[]),
    ('malkhana.items.movements.create', 'malkhana', 'create', 'Create — Malkhana · items · movements', 'ASI'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/movements/:movementId/return']::TEXT[]),
    ('malkhana.items.relocate', 'malkhana', 'relocate', 'Relocate — Malkhana · items', 'SI'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/relocate']::TEXT[]),
    ('malkhana.items.reseal', 'malkhana', 'reseal', 'Reseal — Malkhana · items', 'SHO'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/reseal']::TEXT[]),
    ('malkhana.items.seal-checks', 'malkhana', 'seal-checks', 'Seal checks — Malkhana · items', 'ASI'::user_role, ARRAY['POST /api/v1/malkhana/items/:id/seal-checks']::TEXT[]),
    ('malkhana.locations.create', 'malkhana', 'create', 'Create — Malkhana · locations', 'SI'::user_role, ARRAY['POST /api/v1/malkhana/locations']::TEXT[]),
    ('malkhana.view', 'malkhana', 'view', 'View — Malkhana', NULL, ARRAY['GET /api/v1/malkhana/dashboard','GET /api/v1/malkhana/items','GET /api/v1/malkhana/items/:id','GET /api/v1/malkhana/items/:id/events','GET /api/v1/malkhana/items/:id/label','GET /api/v1/malkhana/items/:id/movements','GET /api/v1/malkhana/items/:id/movements/:movementId/forwarding-letter','GET /api/v1/malkhana/items/:id/seal-checks','GET /api/v1/malkhana/items/by-number/:number','GET /api/v1/malkhana/locations','GET /api/v1/malkhana/stations']::TEXT[]),
    ('me.amend', 'me', 'amend', 'Amend — Own profile', NULL, ARRAY['PUT /api/v1/me','PUT /api/v1/me/password']::TEXT[]),
    ('me.view', 'me', 'view', 'View — Own profile', NULL, ARRAY['GET /api/v1/me']::TEXT[]),
    ('missing-persons.amend', 'missing-persons', 'amend', 'Amend — Missing persons', 'SI'::user_role, ARRAY['PATCH /api/v1/missing-persons/:id']::TEXT[]),
    ('missing-persons.checklist.create', 'missing-persons', 'create', 'Create — Missing persons · checklist', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/checklist/:itemCode/complete']::TEXT[]),
    ('missing-persons.close', 'missing-persons', 'close', 'Close — Missing persons', 'SI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/close']::TEXT[]),
    ('missing-persons.create', 'missing-persons', 'create', 'Create — Missing persons', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons']::TEXT[]),
    ('missing-persons.face-recognition.enrol.create', 'missing-persons', 'create', 'Create — Missing persons · face recognition · enrol', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/face-recognition/enrol']::TEXT[]),
    ('missing-persons.face-recognition.enrolments.create', 'missing-persons', 'create', 'Create — Missing persons · face recognition · enrolments', 'SI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/face-recognition/enrolments/:enrolmentId/withdraw']::TEXT[]),
    ('missing-persons.face-recognition.synthetic-photos.create', 'missing-persons', 'create', 'Create — Missing persons · face recognition · synthetic photos', 'DGP'::user_role, ARRAY['POST /api/v1/missing-persons/:id/face-recognition/synthetic-photos']::TEXT[]),
    ('missing-persons.family-contacts', 'missing-persons', 'family-contacts', 'Family contacts — Missing persons', NULL, ARRAY['POST /api/v1/missing-persons/:id/family-contacts']::TEXT[]),
    ('missing-persons.lookout', 'missing-persons', 'lookout', 'Lookout — Missing persons', 'SI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/lookout']::TEXT[]),
    ('missing-persons.photos', 'missing-persons', 'photos', 'Photos — Missing persons', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/photos']::TEXT[]),
    ('missing-persons.photos.primary', 'missing-persons', 'primary', 'Primary — Missing persons · photos', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/photos/:photoId/primary']::TEXT[]),
    ('missing-persons.photos.retire', 'missing-persons', 'retire', 'Retire — Missing persons · photos', 'SI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/photos/:photoId/retire']::TEXT[]),
    ('missing-persons.sightings', 'missing-persons', 'sightings', 'Sightings — Missing persons', NULL, ARRAY['POST /api/v1/missing-persons/:id/sightings']::TEXT[]),
    ('missing-persons.sightings.create', 'missing-persons', 'create', 'Create — Missing persons · sightings', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/sightings/:sightingId/reject','POST /api/v1/missing-persons/:id/sightings/:sightingId/verify']::TEXT[]),
    ('missing-persons.start-search', 'missing-persons', 'start-search', 'Start search — Missing persons', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/start-search']::TEXT[]),
    ('missing-persons.station-checks', 'missing-persons', 'station-checks', 'Station checks — Missing persons', 'ASI'::user_role, ARRAY['POST /api/v1/missing-persons/:id/station-checks']::TEXT[]),
    ('missing-persons.view', 'missing-persons', 'view', 'View — Missing persons', NULL, ARRAY['GET /api/v1/missing-persons','GET /api/v1/missing-persons/:id','GET /api/v1/missing-persons/:id/checklist','GET /api/v1/missing-persons/:id/face-recognition','GET /api/v1/missing-persons/:id/face-recognition/candidates','GET /api/v1/missing-persons/:id/face-recognition/photos/:photoId/image','GET /api/v1/missing-persons/:id/family-contacts','GET /api/v1/missing-persons/:id/map','GET /api/v1/missing-persons/:id/movement','GET /api/v1/missing-persons/:id/photos','GET /api/v1/missing-persons/:id/photos/:photoId/image','GET /api/v1/missing-persons/:id/photos/:photoId/thumbnail','GET /api/v1/missing-persons/:id/sightings','GET /api/v1/missing-persons/:id/station-checks','GET /api/v1/missing-persons/board','GET /api/v1/missing-persons/stats']::TEXT[]),
    ('ml.create', 'ml', 'create', 'Create — Model registry', NULL, ARRAY['POST /api/v1/ml/classify','POST /api/v1/ml/ocr','POST /api/v1/ml/search']::TEXT[]),
    ('ml.view', 'ml', 'view', 'View — Model registry', NULL, ARRAY['GET /api/v1/ml/health']::TEXT[]),
    ('national.amend', 'national', 'amend', 'Amend — National coordination', 'IG'::user_role, ARRAY['PUT /api/v1/national/alerts/:id','PUT /api/v1/national/patterns/:id']::TEXT[]),
    ('national.create', 'national', 'create', 'Create — National coordination', 'IG'::user_role, ARRAY['POST /api/v1/national/alerts','POST /api/v1/national/alerts/:id/acknowledge','POST /api/v1/national/compare','POST /api/v1/national/coordination/requests','POST /api/v1/national/coordination/requests/:id/approve','POST /api/v1/national/coordination/requests/:id/reject','POST /api/v1/national/patterns']::TEXT[]),
    ('national.view', 'national', 'view', 'View — National coordination', 'IG'::user_role, ARRAY['GET /api/v1/national/alerts','GET /api/v1/national/alerts/:id','GET /api/v1/national/coordination/requests','GET /api/v1/national/coordination/requests/:id','GET /api/v1/national/dashboard','GET /api/v1/national/hotspots','GET /api/v1/national/metrics','GET /api/v1/national/patterns','GET /api/v1/national/patterns/:id','GET /api/v1/national/rankings','GET /api/v1/national/resources']::TEXT[]),
    ('ocr.create', 'ocr', 'create', 'Create — Document OCR', NULL, ARRAY['POST /api/v1/ocr/classify','POST /api/v1/ocr/enhance','POST /api/v1/ocr/extract','POST /api/v1/ocr/extract-batch','POST /api/v1/ocr/extract-entities']::TEXT[]),
    ('ocr.view', 'ocr', 'view', 'View — Document OCR', NULL, ARRAY['GET /api/v1/ocr/document-types','GET /api/v1/ocr/health','GET /api/v1/ocr/languages']::TEXT[]),
    ('officers.amend', 'officers', 'amend', 'Amend — Officer accounts', 'SP'::user_role, ARRAY['PATCH /api/v1/officers/:id']::TEXT[]),
    ('officers.create', 'officers', 'create', 'Create — Officer accounts', 'SP'::user_role, ARRAY['POST /api/v1/officers','POST /api/v1/officers/:id/deactivate','POST /api/v1/officers/:id/password-reset','POST /api/v1/officers/:id/reactivate']::TEXT[]),
    ('officers.view', 'officers', 'view', 'View — Officer accounts', 'DSP'::user_role, ARRAY['GET /api/v1/officers','GET /api/v1/officers/:id','GET /api/v1/officers/options']::TEXT[]),
    ('personnel.amend', 'personnel', 'amend', 'Amend — Personnel', 'SHO'::user_role, ARRAY['PUT /api/v1/personnel/:id']::TEXT[]),
    ('personnel.create', 'personnel', 'create', 'Create — Personnel', 'SHO'::user_role, ARRAY['POST /api/v1/personnel','POST /api/v1/personnel/:id/assign-duty']::TEXT[]),
    ('personnel.delete', 'personnel', 'delete', 'Delete — Personnel', 'DSP'::user_role, ARRAY['DELETE /api/v1/personnel/:id']::TEXT[]),
    ('personnel.view', 'personnel', 'view', 'View — Personnel', NULL, ARRAY['GET /api/v1/personnel','GET /api/v1/personnel/:id']::TEXT[]),
    ('referrals.create', 'referrals', 'create', 'Create — Referrals', 'SHO'::user_role, ARRAY['POST /api/v1/referrals','POST /api/v1/referrals/:id/decision','POST /api/v1/referrals/:id/withdraw']::TEXT[]),
    ('referrals.view', 'referrals', 'view', 'View — Referrals', NULL, ARRAY['GET /api/v1/referrals','GET /api/v1/referrals/:id']::TEXT[]),
    ('reports.create', 'reports', 'create', 'Create — Reports', NULL, ARRAY['POST /api/v1/reports/generate']::TEXT[]),
    ('reports.view', 'reports', 'view', 'View — Reports', NULL, ARRAY['GET /api/v1/reports/crime-statistics','GET /api/v1/reports/daily-summary','GET /api/v1/reports/download/:type','GET /api/v1/reports/fir-status','GET /api/v1/reports/officer-workload','GET /api/v1/reports/pending-investigation','GET /api/v1/reports/types']::TEXT[]),
    ('risk.beats.create', 'risk', 'create', 'Create — Public safety risk · beats', 'SHO'::user_role, ARRAY['POST /api/v1/risk/beats']::TEXT[]),
    ('risk.delete', 'risk', 'delete', 'Delete — Public safety risk', 'SHO'::user_role, ARRAY['DELETE /api/v1/risk/beats/:id','DELETE /api/v1/risk/placements/:firId']::TEXT[]),
    ('risk.placements.create', 'risk', 'create', 'Create — Public safety risk · placements', 'SHO'::user_role, ARRAY['POST /api/v1/risk/placements']::TEXT[]),
    ('risk.simulate.create', 'risk', 'create', 'Create — Public safety risk · simulate', 'SHO'::user_role, ARRAY['POST /api/v1/risk/simulate']::TEXT[]),
    ('risk.view', 'risk', 'view', 'View — Public safety risk', 'SHO'::user_role, ARRAY['GET /api/v1/risk/areas','GET /api/v1/risk/beats','GET /api/v1/risk/factors','GET /api/v1/risk/firs','GET /api/v1/risk/recommendations','GET /api/v1/risk/weights/history']::TEXT[]),
    ('risk.weights.create', 'risk', 'create', 'Create — Public safety risk · weights', 'SP'::user_role, ARRAY['POST /api/v1/risk/weights']::TEXT[]),
    ('search.view', 'search', 'view', 'View — Search', NULL, ARRAY['GET /api/v1/search']::TEXT[]),
    ('state.alerts.acknowledge', 'state', 'acknowledge', 'Acknowledge — State administration · alerts', NULL, ARRAY['POST /api/v1/state/alerts/:id/acknowledge']::TEXT[]),
    ('state.alerts.create', 'state', 'create', 'Create — State administration · alerts', 'DIG'::user_role, ARRAY['POST /api/v1/state/alerts']::TEXT[]),
    ('state.amend', 'state', 'amend', 'Amend — State administration', 'DIG'::user_role, ARRAY['PUT /api/v1/state/:id','PUT /api/v1/state/alerts/:id','PUT /api/v1/state/ranges/:id','PUT /api/v1/state/zones/:id']::TEXT[]),
    ('state.coordination.requests.approve', 'state', 'approve', 'Approve — State administration · coordination · requests', 'DIG'::user_role, ARRAY['POST /api/v1/state/coordination/requests/:id/approve']::TEXT[]),
    ('state.coordination.requests.create', 'state', 'create', 'Create — State administration · coordination · requests', 'SP'::user_role, ARRAY['POST /api/v1/state/coordination/requests']::TEXT[]),
    ('state.coordination.requests.reject', 'state', 'reject', 'Reject — State administration · coordination · requests', 'DIG'::user_role, ARRAY['POST /api/v1/state/coordination/requests/:id/reject']::TEXT[]),
    ('state.create', 'state', 'create', 'Create — State administration', 'DIG'::user_role, ARRAY['POST /api/v1/state']::TEXT[]),
    ('state.ranges.create', 'state', 'create', 'Create — State administration · ranges', 'DIG'::user_role, ARRAY['POST /api/v1/state/ranges']::TEXT[]),
    ('state.view', 'state', 'view', 'View — State administration', NULL, ARRAY['GET /api/v1/state','GET /api/v1/state/:id','GET /api/v1/state/:id/dashboard','GET /api/v1/state/:id/hotspots','GET /api/v1/state/:id/metrics','GET /api/v1/state/:id/resources','GET /api/v1/state/alerts','GET /api/v1/state/alerts/:id','GET /api/v1/state/code/:code','GET /api/v1/state/coordination/requests','GET /api/v1/state/coordination/requests/:id','GET /api/v1/state/ranges','GET /api/v1/state/ranges/:id','GET /api/v1/state/zones','GET /api/v1/state/zones/:id']::TEXT[]),
    ('state.zones.create', 'state', 'create', 'Create — State administration · zones', 'DIG'::user_role, ARRAY['POST /api/v1/state/zones']::TEXT[]),
    ('stations.view', 'stations', 'view', 'View — Stations', NULL, ARRAY['GET /api/v1/stations']::TEXT[]),
    ('traffic-incidents.amend', 'traffic-incidents', 'amend', 'Amend — Accident reconstruction', 'ASI'::user_role, ARRAY['PUT /api/v1/traffic-incidents/:id','PUT /api/v1/traffic-incidents/:id/reports/:reportId']::TEXT[]),
    ('traffic-incidents.cameras', 'traffic-incidents', 'cameras', 'Cameras — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/cameras']::TEXT[]),
    ('traffic-incidents.create', 'traffic-incidents', 'create', 'Create — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents']::TEXT[]),
    ('traffic-incidents.delete', 'traffic-incidents', 'delete', 'Delete — Accident reconstruction', 'ASI'::user_role, ARRAY['DELETE /api/v1/traffic-incidents/:id/cameras/:recordId','DELETE /api/v1/traffic-incidents/:id/facts/:recordId','DELETE /api/v1/traffic-incidents/:id/persons/:recordId','DELETE /api/v1/traffic-incidents/:id/plate-reads/:recordId','DELETE /api/v1/traffic-incidents/:id/signal-phases/:recordId','DELETE /api/v1/traffic-incidents/:id/vehicles/:recordId']::TEXT[]),
    ('traffic-incidents.facts', 'traffic-incidents', 'facts', 'Facts — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/facts']::TEXT[]),
    ('traffic-incidents.persons', 'traffic-incidents', 'persons', 'Persons — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/persons']::TEXT[]),
    ('traffic-incidents.plate-reads', 'traffic-incidents', 'plate-reads', 'Plate reads — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/plate-reads']::TEXT[]),
    ('traffic-incidents.plate-reads.create', 'traffic-incidents', 'create', 'Create — Accident reconstruction · plate reads', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/plate-reads/from-anpr']::TEXT[]),
    ('traffic-incidents.reports', 'traffic-incidents', 'reports', 'Reports — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/reports']::TEXT[]),
    ('traffic-incidents.reports.approve', 'traffic-incidents', 'approve', 'Approve — Accident reconstruction · reports', 'SI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/reports/:reportId/approve']::TEXT[]),
    ('traffic-incidents.reports.return', 'traffic-incidents', 'return', 'Return — Accident reconstruction · reports', 'SI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/reports/:reportId/return']::TEXT[]),
    ('traffic-incidents.reports.submit', 'traffic-incidents', 'submit', 'Submit — Accident reconstruction · reports', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/reports/:reportId/submit']::TEXT[]),
    ('traffic-incidents.signal-phases', 'traffic-incidents', 'signal-phases', 'Signal phases — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/signal-phases']::TEXT[]),
    ('traffic-incidents.vehicles', 'traffic-incidents', 'vehicles', 'Vehicles — Accident reconstruction', 'ASI'::user_role, ARRAY['POST /api/v1/traffic-incidents/:id/vehicles']::TEXT[]),
    ('traffic-incidents.view', 'traffic-incidents', 'view', 'View — Accident reconstruction', NULL, ARRAY['GET /api/v1/traffic-incidents','GET /api/v1/traffic-incidents/:id','GET /api/v1/traffic-incidents/:id/cameras','GET /api/v1/traffic-incidents/:id/facts','GET /api/v1/traffic-incidents/:id/persons','GET /api/v1/traffic-incidents/:id/plate-reads','GET /api/v1/traffic-incidents/:id/prior-challans','GET /api/v1/traffic-incidents/:id/report-draft','GET /api/v1/traffic-incidents/:id/reports','GET /api/v1/traffic-incidents/:id/reports/:reportId','GET /api/v1/traffic-incidents/:id/signal-phases','GET /api/v1/traffic-incidents/:id/timeline','GET /api/v1/traffic-incidents/:id/vehicles','GET /api/v1/traffic-incidents/:id/workspace','GET /api/v1/traffic-incidents/stats']::TEXT[]),
    ('traffic.amend', 'traffic', 'amend', 'Amend — Traffic challans', 'SI'::user_role, ARRAY['PATCH /api/v1/traffic/challans/:id/status']::TEXT[]),
    ('traffic.challans.create', 'traffic', 'create', 'Create — Traffic challans · challans', 'CONSTABLE'::user_role, ARRAY['POST /api/v1/traffic/challans']::TEXT[]),
    ('traffic.challans.dispute', 'traffic', 'dispute', 'Dispute — Traffic challans · challans', NULL, ARRAY['POST /api/v1/traffic/challans/:id/dispute']::TEXT[]),
    ('traffic.payments.create', 'traffic', 'create', 'Create — Traffic challans · payments', NULL, ARRAY['POST /api/v1/traffic/payments/callback','POST /api/v1/traffic/payments/initiate']::TEXT[]),
    ('traffic.view', 'traffic', 'view', 'View — Traffic challans', NULL, ARRAY['GET /api/v1/traffic/challans','GET /api/v1/traffic/challans/:id','GET /api/v1/traffic/challans/number/:number','GET /api/v1/traffic/defaulters','GET /api/v1/traffic/hotspots','GET /api/v1/traffic/stats','GET /api/v1/traffic/vehicle/:vehicleNumber','GET /api/v1/traffic/violation-types']::TEXT[]),
    ('upload.create', 'upload', 'create', 'Create — Uploads', NULL, ARRAY['POST /api/v1/upload','POST /api/v1/upload/presigned']::TEXT[]),
    ('vehicles.allocate', 'vehicles', 'allocate', 'Allocate — Vehicles', 'SHO'::user_role, ARRAY['POST /api/v1/vehicles/:id/allocate']::TEXT[]),
    ('vehicles.amend', 'vehicles', 'amend', 'Amend — Vehicles', 'SHO'::user_role, ARRAY['PUT /api/v1/vehicles/:id']::TEXT[]),
    ('vehicles.create', 'vehicles', 'create', 'Create — Vehicles', 'SHO'::user_role, ARRAY['POST /api/v1/vehicles']::TEXT[]),
    ('vehicles.delete', 'vehicles', 'delete', 'Delete — Vehicles', 'DSP'::user_role, ARRAY['DELETE /api/v1/vehicles/:id']::TEXT[]),
    ('vehicles.return', 'vehicles', 'return', 'Return — Vehicles', NULL, ARRAY['POST /api/v1/vehicles/:id/return']::TEXT[]),
    ('vehicles.view', 'vehicles', 'view', 'View — Vehicles', NULL, ARRAY['GET /api/v1/vehicles','GET /api/v1/vehicles/:id']::TEXT[]),
    ('video.access-log.view', 'video', 'view', 'View — Video intelligence · access log', 'DSP'::user_role, ARRAY['GET /api/v1/video/access-log']::TEXT[]),
    ('video.amend', 'video', 'amend', 'Amend — Video intelligence', 'SHO'::user_role, ARRAY['PUT /api/v1/video/cameras/:id']::TEXT[]),
    ('video.cameras.create', 'video', 'create', 'Create — Video intelligence · cameras', 'SHO'::user_role, ARRAY['POST /api/v1/video/cameras']::TEXT[]),
    ('video.cameras.decommission', 'video', 'decommission', 'Decommission — Video intelligence · cameras', 'DSP'::user_role, ARRAY['POST /api/v1/video/cameras/:id/decommission']::TEXT[]),
    ('video.cameras.health-check', 'video', 'health-check', 'Health check — Video intelligence · cameras', 'ASI'::user_role, ARRAY['POST /api/v1/video/cameras/:id/health-check']::TEXT[]),
    ('video.cameras.view', 'video', 'view', 'View — Video intelligence · cameras', NULL, ARRAY['GET /api/v1/video/cameras','GET /api/v1/video/cameras/:id','GET /api/v1/video/cameras/:id/health-checks','GET /api/v1/video/cameras/stats']::TEXT[]),
    ('video.events.access', 'video', 'access', 'Access — Video intelligence · events', 'ASI'::user_role, ARRAY['POST /api/v1/video/events/:id/access']::TEXT[]),
    ('video.events.create', 'video', 'create', 'Create — Video intelligence · events', NULL, ARRAY['POST /api/v1/video/events']::TEXT[]),
    ('video.events.link', 'video', 'link', 'Link — Video intelligence · events', 'SI'::user_role, ARRAY['POST /api/v1/video/events/:id/link']::TEXT[]),
    ('video.events.purge-expired.create', 'video', 'create', 'Create — Video intelligence · events · purge expired', 'DSP'::user_role, ARRAY['POST /api/v1/video/events/purge-expired']::TEXT[]),
    ('video.events.retention', 'video', 'retention', 'Retention — Video intelligence · events', 'SHO'::user_role, ARRAY['POST /api/v1/video/events/:id/retention']::TEXT[]),
    ('video.events.search.create', 'video', 'create', 'Create — Video intelligence · events · search', 'ASI'::user_role, ARRAY['POST /api/v1/video/events/search']::TEXT[]),
    ('video.events.triage', 'video', 'triage', 'Triage — Video intelligence · events', 'SI'::user_role, ARRAY['POST /api/v1/video/events/:id/triage']::TEXT[]),
    ('video.events.view', 'video', 'view', 'View — Video intelligence · events', NULL, ARRAY['GET /api/v1/video/events/stats']::TEXT[]),
    ('warrants.amend', 'warrants', 'amend', 'Amend — Warrants and summons', 'SI'::user_role, ARRAY['PUT /api/v1/warrants/:id','PATCH /api/v1/warrants/:id/status']::TEXT[]),
    ('warrants.create', 'warrants', 'create', 'Create — Warrants and summons', 'SI'::user_role, ARRAY['POST /api/v1/warrants']::TEXT[]),
    ('warrants.view', 'warrants', 'view', 'View — Warrants and summons', NULL, ARRAY['GET /api/v1/warrants','GET /api/v1/warrants/:id','GET /api/v1/warrants/stats']::TEXT[]),
    ('workload.backlog.view', 'workload', 'view', 'View — Station workload · backlog', 'SHO'::user_role, ARRAY['GET /api/v1/workload/backlog']::TEXT[]),
    ('workload.officers.view', 'workload', 'view', 'View — Station workload · officers', 'SHO'::user_role, ARRAY['GET /api/v1/workload/officers']::TEXT[]),
    ('workload.scopes.view', 'workload', 'view', 'View — Station workload · scopes', 'SHO'::user_role, ARRAY['GET /api/v1/workload/scopes']::TEXT[]),
    ('workload.sla.view', 'workload', 'view', 'View — Station workload · sla', 'SHO'::user_role, ARRAY['GET /api/v1/workload/sla']::TEXT[]),
    ('workload.stations.view', 'workload', 'view', 'View — Station workload · stations', 'DSP'::user_role, ARRAY['GET /api/v1/workload/stations']::TEXT[]),
    ('workload.summary.view', 'workload', 'view', 'View — Station workload · summary', 'SHO'::user_role, ARRAY['GET /api/v1/workload/summary']::TEXT[]),
    ('workload.trends.view', 'workload', 'view', 'View — Station workload · trends', 'SHO'::user_role, ARRAY['GET /api/v1/workload/trends']::TEXT[])
ON CONFLICT (key) DO UPDATE
    SET module = EXCLUDED.module,
        action = EXCLUDED.action,
        description = EXCLUDED.description,
        default_min_rank = EXCLUDED.default_min_rank,
        routes = EXCLUDED.routes;

-- ------------------------------------------------- roles --------------------

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    name_bn     TEXT,
    description TEXT NOT NULL DEFAULT '',

    -- A role belonging to one department is offered only there. NULL means
    -- every force may use it: 'Station writer' is a job in all four.
    force_id    UUID REFERENCES forces(id),

    -- The twelve roles this migration seeds, one per rank. They are what keeps
    -- the platform behaving as it did, so they are not editable and not
    -- deletable: an administrator who wants something different makes a role
    -- of their own rather than quietly redefining what a rank means.
    is_rank_default BOOLEAN NOT NULL DEFAULT FALSE,
    rank        user_role,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  UUID REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- A rank-default role names its rank; any other role must not.
    CONSTRAINT roles_rank_default_names_a_rank
        CHECK ((is_rank_default AND rank IS NOT NULL) OR (NOT is_rank_default AND rank IS NULL))
);

COMMENT ON TABLE roles IS
    'A named job. Rank says how senior an officer is; a role says what they do.';

CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_one_default_per_rank
    ON roles(rank) WHERE is_rank_default;

-- ------------------------------------------------- what a role may do -------

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id        UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_key TEXT NOT NULL REFERENCES permissions(key) ON DELETE CASCADE,
    granted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted_by     UUID REFERENCES users(id),
    PRIMARY KEY (role_id, permission_key)
);

CREATE INDEX IF NOT EXISTS idx_role_permissions_permission ON role_permissions(permission_key);

-- ------------------------------------------------- who holds a role ---------

CREATE TABLE IF NOT EXISTS user_roles (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_by UUID REFERENCES users(id),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);

-- ------------------------------------------------- seed the rank roles ------

-- Named rather than derived from the enum: initcap writes the abbreviated
-- ranks as "Dsp" and "Ig", and a rank is how an officer is addressed.
INSERT INTO roles (code, name, name_bn, description, is_rank_default, rank) VALUES
    ('constable',      'Constable',         'কনস্টেবল',            'What a constable could do before roles existed.',          TRUE, 'CONSTABLE'),
    ('head-constable', 'Head Constable',    'হেড কনস্টেবল',        'What a head constable could do before roles existed.',     TRUE, 'HEAD_CONSTABLE'),
    ('asi',            'ASI',               'এএসআই',               'What an ASI could do before roles existed.',               TRUE, 'ASI'),
    ('si',             'SI',                'এসআই',                'What a sub-inspector could do before roles existed.',      TRUE, 'SI'),
    ('inspector',      'Inspector',         'ইন্সপেক্টর',          'What an inspector could do before roles existed.',         TRUE, 'INSPECTOR'),
    ('sho',            'Officer in charge', 'ভারপ্রাপ্ত আধিকারিক', 'What an officer in charge could do before roles existed.', TRUE, 'SHO'),
    ('dsp',            'DSP',               'ডিএসপি',              'What a DSP could do before roles existed.',                TRUE, 'DSP'),
    ('sp',             'SP',                'এসপি',                'What an SP could do before roles existed.',                TRUE, 'SP'),
    ('dig',            'DIG',               'ডিআইজি',              'What a DIG could do before roles existed.',                TRUE, 'DIG'),
    ('ig',             'IG',                'আইজি',                'What an IG could do before roles existed.',                TRUE, 'IG'),
    ('secretary',      'Secretary',         'সচিব',                'What the secretary could do before roles existed.',        TRUE, 'SECRETARY'),
    ('dgp',            'Director General',  'ডিরেক্টর জেনারেল',    'What the Director General could do before roles existed.', TRUE, 'DGP')
-- Keyed on the rank, because that is what a rank-default role is. Keying on
-- the code would let a renamed code try to insert a second role for a rank
-- that already has one, which the partial unique index then rejects.
ON CONFLICT (rank) WHERE is_rank_default DO UPDATE
    SET code = EXCLUDED.code, name = EXCLUDED.name, name_bn = EXCLUDED.name_bn;

-- The trigger that fixes what a rank role grants is created at the end of this
-- file, and would otherwise refuse the seeding below on a second run: the
-- statement inserts nothing when the rows are already there, but a BEFORE
-- INSERT trigger fires per row whatever ON CONFLICT says. Dropped here so
-- applying this migration twice is a no-op, and recreated once seeding is done.
DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;

-- Each rank role is granted exactly what that rank can already reach: every
-- permission whose floor it meets, and every permission that had no floor at
-- all. This is the statement that makes the migration behaviour-preserving.
INSERT INTO role_permissions (role_id, permission_key)
SELECT ro.id, p.key
FROM roles ro
JOIN permissions p
  ON p.default_min_rank IS NULL
  OR rank_level(p.default_min_rank) <= rank_level(ro.rank)
WHERE ro.is_rank_default
ON CONFLICT DO NOTHING;

-- Every officer gets the role for the rank they hold.
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, ro.id
FROM users u
JOIN roles ro ON ro.is_rank_default AND ro.rank = u.role
ON CONFLICT DO NOTHING;

-- ------------------------------------------------- rank follows rank --------

-- An officer promoted from SI to inspector must stop holding the SI role and
-- start holding the inspector one, or the rank they are shown and the access
-- they have would drift apart. The same reasoning as trg_user_force_matches_
-- posting in 000082: the database keeps the relationship whatever the
-- application does, and there is exactly one definition of it.
--
-- Only the rank-default role is moved. A role an administrator assigned by
-- hand — 'Malkhana clerk' — is the officer's job and survives a promotion.
CREATE OR REPLACE FUNCTION rank_role_follows_rank() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM user_roles ur
    USING roles ro
    WHERE ur.user_id = NEW.id
      AND ur.role_id = ro.id
      AND ro.is_rank_default
      AND ro.rank IS DISTINCT FROM NEW.role;

    INSERT INTO user_roles (user_id, role_id)
    SELECT NEW.id, ro.id FROM roles ro
    WHERE ro.is_rank_default AND ro.rank = NEW.role
    ON CONFLICT DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_rank_role_follows_rank ON users;
CREATE TRIGGER trg_rank_role_follows_rank
    AFTER INSERT OR UPDATE OF role ON users
    FOR EACH ROW EXECUTE FUNCTION rank_role_follows_rank();

-- ------------------------------------------------- the rank roles are fixed -

-- Editing a rank-default role would change what a rank means for every officer
-- holding it, invisibly and everywhere at once. That is precisely the change
-- an administrator should have to make deliberately, by creating a role.
CREATE OR REPLACE FUNCTION rank_roles_are_not_edited() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.is_rank_default THEN
            RAISE EXCEPTION 'The % role mirrors a rank and cannot be deleted. '
                            'Create a role of your own instead.', OLD.name
                USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN OLD;
    END IF;

    IF OLD.is_rank_default AND (
        NEW.code IS DISTINCT FROM OLD.code OR
        NEW.rank IS DISTINCT FROM OLD.rank OR
        NEW.is_rank_default IS DISTINCT FROM OLD.is_rank_default
    ) THEN
        RAISE EXCEPTION 'The % role mirrors a rank; its code and rank are fixed.', OLD.name
            USING ERRCODE = 'restrict_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_rank_roles_are_not_edited ON roles;
CREATE TRIGGER trg_rank_roles_are_not_edited
    BEFORE UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION rank_roles_are_not_edited();

-- What a rank role grants is fixed for the same reason.
CREATE OR REPLACE FUNCTION rank_role_grants_are_not_edited() RETURNS TRIGGER AS $$
DECLARE
    target UUID := COALESCE(NEW.role_id, OLD.role_id);
    fixed  BOOLEAN;
    label  TEXT;
BEGIN
    SELECT is_rank_default, name INTO fixed, label FROM roles WHERE id = target;
    IF fixed THEN
        RAISE EXCEPTION 'The % role mirrors a rank; what it grants is fixed. '
                        'Create a role of your own instead.', label
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- Seeding above runs before this trigger exists, which is deliberate.
DROP TRIGGER IF EXISTS trg_rank_role_grants_are_not_edited ON role_permissions;
CREATE TRIGGER trg_rank_role_grants_are_not_edited
    BEFORE INSERT OR UPDATE OR DELETE ON role_permissions
    FOR EACH ROW EXECUTE FUNCTION rank_role_grants_are_not_edited();

-- ------------------------------------------------- reading it back ----------

-- Everything an officer may do, by name. The service resolves a request
-- against this view rather than against a token claim, so a permission removed
-- from a role stops working on the next request instead of an hour later when
-- the token expires.
CREATE OR REPLACE VIEW user_permissions AS
SELECT DISTINCT ur.user_id, rp.permission_key
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id;

COMMENT ON VIEW user_permissions IS
    'Resolved per request. Never cached in a token: revocation must take effect immediately.';
