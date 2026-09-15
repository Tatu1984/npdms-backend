package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

// WorkloadRepository aggregates Phase 08 figures over records other modules
// own. It writes nothing. Every query takes the same three parameters:
// $1 the station ids in scope (NULL for all stations), $2 and $3 an optional
// window on when an item was opened.
type WorkloadRepository struct {
	db *pgxpool.Pool
}

func NewWorkloadRepository(db *pgxpool.Pool) *WorkloadRepository {
	return &WorkloadRepository{db: db}
}

// Stations are attributed through the FIR a record hangs off: a case through
// its FIR, a forensic request through its case or its evidence item, a warrant
// or bail application through its case or FIR. A record with no path to a
// station is counted only when the scope is all stations.
const (
	inScope = `($1::uuid[] IS NULL OR %s = ANY($1::uuid[]))`
	window  = `($2::timestamptz IS NULL OR %[1]s >= $2::timestamptz) AND ($3::timestamptz IS NULL OR %[1]s < $3::timestamptz)`
)

const forensicStation = `COALESCE(ff.station_id, ef.station_id, ecf.station_id)`

const forensicJoins = `
	LEFT JOIN cases fc ON fc.id = fr.case_id
	LEFT JOIN firs ff ON ff.id = fc.fir_id
	LEFT JOIN evidence ev ON ev.id = fr.evidence_id
	LEFT JOIN firs ef ON ef.id = ev.fir_id
	LEFT JOIN cases ec ON ec.id = ev.case_id
	LEFT JOIN firs ecf ON ecf.id = ec.fir_id
`

func scoped(column string) string   { return fmt.Sprintf(inScope, column) }
func windowed(column string) string { return fmt.Sprintf(window, column) }

// Summary returns the open-item counts in the order the service lists them.
func (r *WorkloadRepository) Summary(ctx context.Context, ids []string, from, to *time.Time) (map[string]int64, error) {
	q := `
	SELECT
		(SELECT COUNT(*) FROM firs f WHERE f.status::text = 'REGISTERED' AND ` + scoped("f.station_id") + ` AND ` + windowed("f.created_at") + `),
		(SELECT COUNT(*) FROM firs f WHERE f.status::text = 'UNDER_INVESTIGATION' AND ` + scoped("f.station_id") + ` AND ` + windowed("f.created_at") + `),
		(SELECT COUNT(*) FROM cases c LEFT JOIN firs f ON f.id = c.fir_id
		  WHERE c.status::text IN ('REGISTERED','UNDER_INVESTIGATION') AND ` + scoped("f.station_id") + ` AND ` + windowed("c.created_at") + `),
		(SELECT COUNT(*) FROM cases c LEFT JOIN firs f ON f.id = c.fir_id
		  WHERE c.status::text IN ('CHARGESHEET_FILED','IN_COURT') AND ` + scoped("f.station_id") + ` AND ` + windowed("c.created_at") + `),
		(SELECT COUNT(*) FROM forensics fr ` + forensicJoins + `
		  WHERE fr.status IN ('PENDING','IN_PROGRESS') AND ` + scoped(forensicStation) + ` AND ` + windowed("fr.submitted_date::timestamptz") + `),
		(SELECT COUNT(*) FROM forensics fr ` + forensicJoins + `
		  WHERE fr.status IN ('PENDING','IN_PROGRESS') AND fr.expected_date IS NOT NULL AND fr.expected_date::date < CURRENT_DATE
		    AND ` + scoped(forensicStation) + ` AND ` + windowed("fr.submitted_date::timestamptz") + `),
		(SELECT COUNT(*) FROM court_hearings h LEFT JOIN cases c ON c.id = h.case_id LEFT JOIN firs f ON f.id = c.fir_id
		  WHERE h.hearing_date >= CURRENT_DATE AND h.hearing_date < CURRENT_DATE + 7 AND ` + scoped("f.station_id") + `),
		(SELECT COUNT(*) FROM warrants w LEFT JOIN cases c ON c.id = w.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = w.fir_id
		  WHERE w.status::text = 'ACTIVE' AND ` + scoped("COALESCE(cf.station_id, f.station_id)") + ` AND ` + windowed("w.created_at") + `),
		(SELECT COUNT(*) FROM warrants w LEFT JOIN cases c ON c.id = w.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = w.fir_id
		  WHERE w.status::text = 'ACTIVE' AND w.valid_until IS NOT NULL AND w.valid_until < CURRENT_DATE
		    AND ` + scoped("COALESCE(cf.station_id, f.station_id)") + ` AND ` + windowed("w.created_at") + `),
		(SELECT COUNT(*) FROM bail b LEFT JOIN cases c ON c.id = b.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = b.fir_id
		  WHERE b.status = 'PENDING' AND ` + scoped("COALESCE(cf.station_id, f.station_id)") + ` AND ` + windowed("b.application_date::timestamptz") + `),
		(SELECT COUNT(*) FROM evidence e LEFT JOIN firs f ON f.id = e.fir_id LEFT JOIN cases c ON c.id = e.case_id LEFT JOIN firs cf ON cf.id = c.fir_id
		  WHERE e.integrity_state = 'broken' AND ` + scoped("COALESCE(f.station_id, cf.station_id)") + `),
		(SELECT COUNT(*) FROM evidence e LEFT JOIN firs f ON f.id = e.fir_id LEFT JOIN cases c ON c.id = e.case_id LEFT JOIN firs cf ON cf.id = c.fir_id
		  WHERE e.object_key IS NOT NULL AND e.integrity_state = 'pending' AND ` + scoped("COALESCE(f.station_id, cf.station_id)") + `),
		(SELECT COUNT(*) FROM lookouts l WHERE l.status = 'ACTIVE' AND ` + scoped("l.station_id") + ` AND ` + windowed("l.issued_at") + `),
		(SELECT COUNT(*) FROM investigation_tasks t JOIN investigation_workspaces w ON w.id = t.workspace_id
		  WHERE t.status <> 'done' AND ` + scoped("w.station_id") + ` AND ` + windowed("t.created_at") + `),
		(SELECT COUNT(*) FROM investigation_tasks t JOIN investigation_workspaces w ON w.id = t.workspace_id
		  WHERE t.status <> 'done' AND t.due_date IS NOT NULL AND t.due_date < CURRENT_DATE AND ` + scoped("w.station_id") + ` AND ` + windowed("t.created_at") + `),
		(SELECT COUNT(*) FROM weapon_issuances i JOIN weapons wp ON wp.id = i.weapon_id
		  WHERE i.returned_at IS NULL AND i.expected_return IS NOT NULL AND i.expected_return < NOW() AND ` + scoped("wp.station_id") + `),
		(SELECT COUNT(*) FROM personnel p WHERE p.status <> 'SUSPENDED' AND ` + scoped("p.station_id") + `),
		(SELECT COUNT(*) FROM personnel p WHERE p.status = 'ON_DUTY' AND ` + scoped("p.station_id") + `),
		(SELECT COUNT(*) FROM personnel p WHERE p.status = 'ON_LEAVE' AND ` + scoped("p.station_id") + `)
	`
	keys := []string{
		"firsRegistered", "firsUnderInvestigation", "casesOpen", "casesInCourt",
		"forensicsPending", "forensicsPastExpected", "hearingsNext7", "warrantsActive", "warrantsLapsed",
		"bailPending", "evidenceBroken", "evidenceUnverified", "lookoutsActive", "tasksOpen", "tasksOverdue",
		"weaponsOverdue", "rosterStrength", "onDuty", "onLeave",
	}
	vals := make([]int64, len(keys))
	dest := make([]interface{}, len(keys))
	for i := range vals {
		dest[i] = &vals[i]
	}
	if err := r.db.QueryRow(ctx, q, nullableIDs(ids), from, to).Scan(dest...); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(keys))
	for i, k := range keys {
		out[k] = vals[i]
	}
	return out, nil
}

// stageItems yields one row per open item with its pipeline stage, station and
// the date its age is measured from. Shared by the backlog and station views.
const stageItems = `
	stage_items AS (
		SELECT 'investigation' AS stage, f.station_id, f.created_at AS started_at
		FROM firs f
		WHERE f.status::text IN ('REGISTERED','UNDER_INVESTIGATION')
		  AND NOT EXISTS (SELECT 1 FROM cases c WHERE c.fir_id = f.id)
		UNION ALL
		SELECT 'investigation', f.station_id, COALESCE(f.created_at, c.created_at)
		FROM cases c LEFT JOIN firs f ON f.id = c.fir_id
		WHERE c.status::text IN ('REGISTERED','UNDER_INVESTIGATION')
		UNION ALL
		SELECT 'forensic', ` + forensicStation + `, fr.submitted_date::timestamptz
		FROM forensics fr ` + forensicJoins + `
		WHERE fr.status IN ('PENDING','IN_PROGRESS')
		UNION ALL
		SELECT 'court', f.station_id, COALESCE(f.created_at, c.created_at)
		FROM cases c LEFT JOIN firs f ON f.id = c.fir_id
		WHERE c.status::text IN ('CHARGESHEET_FILED','IN_COURT')
		UNION ALL
		SELECT 'tasks', w.station_id, t.created_at
		FROM investigation_tasks t JOIN investigation_workspaces w ON w.id = t.workspace_id
		WHERE t.status <> 'done'
	)
`

const bandColumns = `
	COUNT(*) FILTER (WHERE age <= 30),
	COUNT(*) FILTER (WHERE age BETWEEN 31 AND 90),
	COUNT(*) FILTER (WHERE age BETWEEN 91 AND 180),
	COUNT(*) FILTER (WHERE age > 180),
	COUNT(*)
`

func (r *WorkloadRepository) Backlog(ctx context.Context, ids []string, from, to *time.Time) (map[string]models.AgeBands, error) {
	rows, err := r.db.Query(ctx, `
		WITH `+stageItems+`
		SELECT stage, `+bandColumns+`
		FROM (
			SELECT stage, (CURRENT_DATE - started_at::date) AS age
			FROM stage_items
			WHERE `+scoped("station_id")+` AND `+windowed("started_at")+`
		) x
		GROUP BY stage
	`, nullableIDs(ids), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.AgeBands{}
	for rows.Next() {
		var stage string
		var b models.AgeBands
		if err := rows.Scan(&stage, &b.Days0to30, &b.Days31to90, &b.Days91to180, &b.Days181Plus, &b.Total); err != nil {
			return nil, err
		}
		out[stage] = b
	}
	return out, rows.Err()
}

func (r *WorkloadRepository) Stations(ctx context.Context, ids []string, from, to *time.Time) ([]models.StationWorkload, error) {
	rows, err := r.db.Query(ctx, `
		WITH `+stageItems+`,
		aged AS (
			SELECT station_id, stage, (CURRENT_DATE - started_at::date) AS age
			FROM stage_items WHERE `+windowed("started_at")+`
		)
		SELECT s.id::text, s.code, s.name, COALESCE(s.district, ''),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation'),
		       COUNT(a.*) FILTER (WHERE a.stage = 'forensic'),
		       COUNT(a.*) FILTER (WHERE a.stage = 'court'),
		       COUNT(a.*) FILTER (WHERE a.stage = 'tasks'),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation' AND a.age > 90),
		       (SELECT COUNT(*) FROM personnel p WHERE p.station_id = s.id AND p.status <> 'SUSPENDED'),
		       (SELECT COUNT(*) FROM personnel p WHERE p.station_id = s.id AND p.status IN ('ON_DUTY','OFF_DUTY')),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation' AND a.age <= 30),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation' AND a.age BETWEEN 31 AND 90),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation' AND a.age BETWEEN 91 AND 180),
		       COUNT(a.*) FILTER (WHERE a.stage = 'investigation' AND a.age > 180)
		FROM stations s
		LEFT JOIN aged a ON a.station_id = s.id
		WHERE `+scoped("s.id")+`
		GROUP BY s.id, s.code, s.name, s.district
		ORDER BY s.name
	`, nullableIDs(ids), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StationWorkload{}
	for rows.Next() {
		var w models.StationWorkload
		if err := rows.Scan(&w.StationID, &w.Code, &w.Name, &w.District,
			&w.OpenInvestigations, &w.PendingForensics, &w.InCourt, &w.OpenTasks, &w.Over90Days,
			&w.RosterStrength, &w.Available,
			&w.Backlog.Days0to30, &w.Backlog.Days31to90, &w.Backlog.Days91to180, &w.Backlog.Days181Plus); err != nil {
			return nil, err
		}
		w.Backlog.Total = w.OpenInvestigations
		if w.Available > 0 {
			ratio := float64(w.OpenInvestigations) / float64(w.Available)
			w.PerAvailableOfficer = &ratio
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Officers lists active officers posted to the stations in scope with the
// items currently assigned to each. Ordered by name: this is not a ranking.
func (r *WorkloadRepository) Officers(ctx context.Context, ids []string) ([]models.OfficerWorkload, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id::text, u.name, u.role::text, COALESCE(u.badge_number, ''), COALESCE(s.code, ''),
		       (SELECT COUNT(*) FROM firs f WHERE f.investigating_officer = u.id
		          AND f.status::text IN ('REGISTERED','UNDER_INVESTIGATION')),
		       (SELECT COUNT(*) FROM cases c WHERE c.investigating_officer = u.id
		          AND c.status::text IN ('REGISTERED','UNDER_INVESTIGATION')),
		       (SELECT COUNT(*) FROM cases c WHERE c.investigating_officer = u.id
		          AND c.status::text IN ('CHARGESHEET_FILED','IN_COURT')),
		       (SELECT COUNT(*) FROM investigation_workspaces w WHERE w.io_id = u.id AND w.status <> 'closed'),
		       (SELECT COUNT(*) FROM investigation_tasks t WHERE t.assignee_id = u.id AND t.status <> 'done'),
		       (SELECT COUNT(*) FROM investigation_tasks t WHERE t.assignee_id = u.id AND t.status <> 'done'
		          AND t.due_date IS NOT NULL AND t.due_date < CURRENT_DATE),
		       (SELECT COUNT(*) FROM forensics fr JOIN cases c ON c.id = fr.case_id
		          WHERE c.investigating_officer = u.id AND fr.status IN ('PENDING','IN_PROGRESS')),
		       (SELECT COUNT(*) FROM court_hearings h LEFT JOIN cases c ON c.id = h.case_id
		          WHERE COALESCE(h.investigating_officer, c.investigating_officer) = u.id
		            AND h.hearing_date >= CURRENT_DATE AND h.hearing_date < CURRENT_DATE + 14)
		FROM users u
		LEFT JOIN stations s ON s.id = u.station_id
		WHERE u.is_active AND `+scoped("u.station_id")+`
		ORDER BY u.name
	`, nullableIDs(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.OfficerWorkload{}
	for rows.Next() {
		var o models.OfficerWorkload
		if err := rows.Scan(&o.UserID, &o.Name, &o.Rank, &o.Badge, &o.StationCode,
			&o.FIRsAsIO, &o.CasesAsIO, &o.CasesInCourt, &o.WorkspacesAsIO,
			&o.OpenTasks, &o.OverdueTasks, &o.PendingForensics, &o.HearingsNext14); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SLA queries return every breaching item; the service caps the list it sends.
func (r *WorkloadRepository) slaItems(ctx context.Context, q string, ids []string, extra ...interface{}) ([]models.SLAItem, error) {
	rows, err := r.db.Query(ctx, q, append([]interface{}{nullableIDs(ids)}, extra...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SLAItem{}
	for rows.Next() {
		var it models.SLAItem
		if err := rows.Scan(&it.Module, &it.ID, &it.Ref, &it.Title, &it.Since, &it.DaysOver); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CustodyWithoutChargesheet lists accused recorded as arrested or in custody on
// a case that has not reached charge-sheet or beyond, with days since arrest.
func (r *WorkloadRepository) CustodyWithoutChargesheet(ctx context.Context, ids []string, fromDay int) ([]models.SLAItem, error) {
	return r.slaItems(ctx, `
		SELECT 'case', c.id::text, c.case_number, a.name || ' — ' || c.title, a.arrest_date,
		       (CURRENT_DATE - a.arrest_date::date)::bigint
		FROM accused a
		JOIN cases c ON c.id = a.case_id
		LEFT JOIN firs f ON f.id = c.fir_id
		WHERE a.arrest_date IS NOT NULL
		  AND a.status::text IN ('ARRESTED','IN_CUSTODY')
		  AND c.status::text IN ('REGISTERED','UNDER_INVESTIGATION')
		  AND (CURRENT_DATE - a.arrest_date::date) >= $2
		  AND `+scoped("f.station_id")+`
		ORDER BY a.arrest_date
	`, ids, fromDay)
}

func (r *WorkloadRepository) ForensicsPastExpected(ctx context.Context, ids []string) ([]models.SLAItem, error) {
	return r.slaItems(ctx, `
		SELECT 'forensic', fr.id::text, COALESCE(fc.case_number, ev.evidence_number, ''), fr.type || ' — ' || fr.lab,
		       fr.expected_date::timestamptz, (CURRENT_DATE - fr.expected_date::date)::bigint
		FROM forensics fr `+forensicJoins+`
		WHERE fr.status IN ('PENDING','IN_PROGRESS') AND fr.expected_date IS NOT NULL
		  AND fr.expected_date::date < CURRENT_DATE AND `+scoped(forensicStation)+`
		ORDER BY fr.expected_date
	`, ids)
}

func (r *WorkloadRepository) WarrantsLapsed(ctx context.Context, ids []string) ([]models.SLAItem, error) {
	return r.slaItems(ctx, `
		SELECT 'warrant', w.id::text, w.warrant_number, w.type || ' — ' || w.issued_for,
		       w.valid_until::timestamptz, (CURRENT_DATE - w.valid_until)::bigint
		FROM warrants w
		LEFT JOIN cases c ON c.id = w.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = w.fir_id
		WHERE w.status::text = 'ACTIVE' AND w.valid_until IS NOT NULL AND w.valid_until < CURRENT_DATE
		  AND `+scoped("COALESCE(cf.station_id, f.station_id)")+`
		ORDER BY w.valid_until
	`, ids)
}

func (r *WorkloadRepository) TasksOverdue(ctx context.Context, ids []string) ([]models.SLAItem, error) {
	return r.slaItems(ctx, `
		SELECT 'task', w.id::text, w.case_number, t.title,
		       t.due_date::timestamptz, (CURRENT_DATE - t.due_date)::bigint
		FROM investigation_tasks t JOIN investigation_workspaces w ON w.id = t.workspace_id
		WHERE t.status <> 'done' AND t.due_date IS NOT NULL AND t.due_date < CURRENT_DATE
		  AND `+scoped("w.station_id")+`
		ORDER BY t.due_date
	`, ids)
}

func (r *WorkloadRepository) WeaponsOverdue(ctx context.Context, ids []string) ([]models.SLAItem, error) {
	return r.slaItems(ctx, `
		SELECT 'weapon', wp.id::text, wp.weapon_number, wp.make || ' — ' || COALESCE(u.name, ''),
		       i.expected_return, (CURRENT_DATE - i.expected_return::date)::bigint
		FROM weapon_issuances i JOIN weapons wp ON wp.id = i.weapon_id
		LEFT JOIN users u ON u.id = i.issued_to
		WHERE i.returned_at IS NULL AND i.expected_return IS NOT NULL AND i.expected_return < NOW()
		  AND `+scoped("wp.station_id")+`
		ORDER BY i.expected_return
	`, ids)
}

// Trends counts dated events per bucket. $2/$3 bound the range; $4 is the
// bucket width ('week' or 'month').
func (r *WorkloadRepository) Trends(ctx context.Context, ids []string, from, to time.Time, interval string) ([]time.Time, map[string][]int64, error) {
	rows, err := r.db.Query(ctx, `
		WITH buckets AS (
			SELECT generate_series(date_trunc($4, $2::timestamptz), $3::timestamptz - interval '1 second', ('1 ' || $4)::interval) AS b
		),
		events AS (
			SELECT 'firsRegistered' AS k, f.created_at AS at FROM firs f WHERE `+scoped("f.station_id")+`
			UNION ALL
			SELECT 'casesRegistered', c.created_at FROM cases c LEFT JOIN firs f ON f.id = c.fir_id WHERE `+scoped("f.station_id")+`
			UNION ALL
			SELECT 'forensicsSubmitted', fr.submitted_date::timestamptz FROM forensics fr `+forensicJoins+` WHERE `+scoped(forensicStation)+`
			UNION ALL
			SELECT 'forensicsCompleted', fr.completed_date::timestamptz FROM forensics fr `+forensicJoins+`
			  WHERE fr.completed_date IS NOT NULL AND `+scoped(forensicStation)+`
			UNION ALL
			SELECT 'warrantsExecuted', w.executed_date FROM warrants w
			  LEFT JOIN cases c ON c.id = w.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = w.fir_id
			  WHERE w.executed_date IS NOT NULL AND `+scoped("COALESCE(cf.station_id, f.station_id)")+`
			UNION ALL
			SELECT 'workspacesClosed', w.closed_at FROM investigation_workspaces w
			  WHERE w.closed_at IS NOT NULL AND `+scoped("w.station_id")+`
			UNION ALL
			SELECT 'bailDecided', COALESCE(b.approval_date, b.rejection_date)::timestamptz FROM bail b
			  LEFT JOIN cases c ON c.id = b.case_id LEFT JOIN firs cf ON cf.id = c.fir_id LEFT JOIN firs f ON f.id = b.fir_id
			  WHERE COALESCE(b.approval_date, b.rejection_date) IS NOT NULL AND `+scoped("COALESCE(cf.station_id, f.station_id)")+`
		)
		SELECT bk.b, e.k, COUNT(e.at)
		FROM buckets bk
		LEFT JOIN events e ON e.at >= bk.b AND e.at < bk.b + ('1 ' || $4)::interval
		GROUP BY bk.b, e.k
		ORDER BY bk.b
	`, nullableIDs(ids), from, to, interval)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var buckets []time.Time
	index := map[time.Time]int{}
	counts := map[string]map[int]int64{}
	for rows.Next() {
		var b time.Time
		var k *string
		var n int64
		if err := rows.Scan(&b, &k, &n); err != nil {
			return nil, nil, err
		}
		if _, ok := index[b]; !ok {
			index[b] = len(buckets)
			buckets = append(buckets, b)
		}
		if k != nil {
			if counts[*k] == nil {
				counts[*k] = map[int]int64{}
			}
			counts[*k][index[b]] = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	series := map[string][]int64{}
	for _, key := range []string{"firsRegistered", "casesRegistered", "forensicsSubmitted", "forensicsCompleted", "warrantsExecuted", "workspacesClosed", "bailDecided"} {
		vals := make([]int64, len(buckets))
		for i := range buckets {
			vals[i] = counts[key][i]
		}
		series[key] = vals
	}
	return buckets, series, nil
}

func (r *WorkloadRepository) ScopeOptions(ctx context.Context) ([]models.WorkloadScopeOption, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text, code, name, COALESCE(district, '') FROM stations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.WorkloadScopeOption{}
	for rows.Next() {
		var o models.WorkloadScopeOption
		if err := rows.Scan(&o.StationID, &o.Code, &o.Name, &o.District); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// nullableIDs sends NULL rather than an empty array when every station is in scope.
func nullableIDs(ids []string) interface{} {
	if ids == nil {
		return nil
	}
	return ids
}
