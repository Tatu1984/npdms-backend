package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrIncidentNotFound     = errors.New("incident not found")
	ErrAssignmentNotFound   = errors.New("assignment not found")
	ErrUnitNotFound         = errors.New("unit not found")
	ErrUnitUnavailable      = errors.New("unit is not available")
	ErrIncidentState        = errors.New("incident is not in a state that allows this")
	ErrAssignmentState      = errors.New("assignment is not in a state that allows this")
	ErrIncidentUnclassified = errors.New("classify the incident before dispatching a unit")
	ErrStationNotFound      = errors.New("station not found")
)

// ConflictError carries a specific reason for a 409.
type ConflictError struct {
	Base   error
	Reason string
}

func (e *ConflictError) Error() string { return e.Reason }
func (e *ConflictError) Unwrap() error { return e.Base }

func conflict(base error, format string, args ...interface{}) error {
	return &ConflictError{Base: base, Reason: fmt.Sprintf(format, args...)}
}

type DispatchRepository struct {
	db *pgxpool.Pool
}

func NewDispatchRepository(db *pgxpool.Pool) *DispatchRepository {
	return &DispatchRepository{db: db}
}

// overdueSQL decides on read whether an assignment has passed its threshold.
// Thresholds are the stated policy constants, rendered as integers.
func ackOverdueSQL(alias string) string {
	return fmt.Sprintf("(%s.status = 'ASSIGNED' AND %s.assigned_at < NOW() - INTERVAL '%d minutes')",
		alias, alias, models.AcknowledgementLadder[0].AfterMinutes)
}

func onSceneOverdueSQL(assignment, incident string) string {
	var cases []string
	for _, d := range models.SeverityScale {
		cases = append(cases, fmt.Sprintf("WHEN '%s' THEN %d", d.Severity, d.OnSceneMinutes))
	}
	return fmt.Sprintf("(%s.status = 'ACKNOWLEDGED' AND %s.acknowledged_at < NOW() - make_interval(mins => CASE %s.severity %s ELSE %d END))",
		assignment, assignment, incident, strings.Join(cases, " "), models.SeverityScale[len(models.SeverityScale)-1].OnSceneMinutes)
}

func incidentSelect() string {
	return `
	SELECT i.id, i.incident_number, i.source, i.caller_name, i.caller_phone, i.description,
	       i.location_text, i.latitude, i.longitude, i.station_id, COALESCE(s.name, ''),
	       i.received_at, i.incident_type, i.severity, i.classified_by, COALESCE(cb.name, ''), i.classified_at,
	       i.status, i.escalation_level, i.outcome, i.outcome_note, i.fir_id, COALESCE(f.fir_number, ''),
	       COALESCE(clb.name, ''), i.closed_at, COALESCE(crb.name, ''), i.created_at, i.updated_at,
	       (SELECT COUNT(*) FROM dispatch_assignments a WHERE a.incident_id = i.id
	          AND a.status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE')),
	       CASE WHEN i.status IN ('NEW', 'CLASSIFIED') THEN EXTRACT(EPOCH FROM NOW() - i.received_at) / 60.0 END,
	       EXISTS (SELECT 1 FROM dispatch_assignments a WHERE a.incident_id = i.id
	          AND (` + ackOverdueSQL("a") + ` OR ` + onSceneOverdueSQL("a", "i") + `))
	FROM dispatch_incidents i
	LEFT JOIN stations s ON s.id = i.station_id
	LEFT JOIN users cb ON cb.id = i.classified_by
	LEFT JOIN users clb ON clb.id = i.closed_by
	LEFT JOIN users crb ON crb.id = i.created_by
	LEFT JOIN firs f ON f.id = i.fir_id
`
}

func scanIncident(row pgx.Row) (*models.DispatchIncident, error) {
	var i models.DispatchIncident
	err := row.Scan(
		&i.ID, &i.IncidentNumber, &i.Source, &i.CallerName, &i.CallerPhone, &i.Description,
		&i.LocationText, &i.Latitude, &i.Longitude, &i.StationID, &i.StationName,
		&i.ReceivedAt, &i.IncidentType, &i.Severity, &i.ClassifiedBy, &i.ClassifiedByName, &i.ClassifiedAt,
		&i.Status, &i.EscalationLevel, &i.Outcome, &i.OutcomeNote, &i.FIRID, &i.FIRNumber,
		&i.ClosedByName, &i.ClosedAt, &i.CreatedByName, &i.CreatedAt, &i.UpdatedAt,
		&i.ActiveUnits, &i.WaitingMinutes, &i.Overdue,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func assignmentSelect() string {
	return `
	SELECT a.id, a.incident_id, a.unit_kind, a.vehicle_id, COALESCE(v.registration_number, ''), COALESCE(v.type, ''),
	       a.officer_id, COALESCE(o.name, ''), COALESCE(o.badge_number, ''), a.status,
	       COALESCE(ab.name, ''), a.assigned_at, a.distance_km,
	       a.acknowledged_at, COALESCE(ak.name, ''), a.on_scene_at, COALESCE(os.name, ''), a.on_scene_alerted_at,
	       a.cleared_at, COALESCE(cl.name, ''), a.cancelled_at, COALESCE(cn.name, ''), a.cancel_reason,
	       ` + ackOverdueSQL("a") + `, ` + onSceneOverdueSQL("a", "i") + `
	FROM dispatch_assignments a
	JOIN dispatch_incidents i ON i.id = a.incident_id
	LEFT JOIN vehicles v ON v.id = a.vehicle_id
	LEFT JOIN users o ON o.id = a.officer_id
	LEFT JOIN users ab ON ab.id = a.assigned_by
	LEFT JOIN users ak ON ak.id = a.acknowledged_by
	LEFT JOIN users os ON os.id = a.on_scene_by
	LEFT JOIN users cl ON cl.id = a.cleared_by
	LEFT JOIN users cn ON cn.id = a.cancelled_by
`
}

func scanAssignment(row pgx.Row) (*models.DispatchAssignment, error) {
	var a models.DispatchAssignment
	err := row.Scan(
		&a.ID, &a.IncidentID, &a.UnitKind, &a.VehicleID, &a.VehicleNumber, &a.VehicleType,
		&a.OfficerID, &a.OfficerName, &a.OfficerBadge, &a.Status,
		&a.AssignedByName, &a.AssignedAt, &a.DistanceKm,
		&a.AcknowledgedAt, &a.AcknowledgedByName, &a.OnSceneAt, &a.OnSceneByName, &a.OnSceneAlertedAt,
		&a.ClearedAt, &a.ClearedByName, &a.CancelledAt, &a.CancelledByName, &a.CancelReason,
		&a.AckOverdue, &a.OnSceneOverdue,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

/* -------------------------------- incidents ------------------------------- */

type IncidentFilter struct {
	// View: "queue" (NEW, CLASSIFIED), "active" (DISPATCHED, ON_SCENE),
	// "open" (anything not closed), "closed", or "" for all.
	View      string
	Status    string
	Severity  string
	StationID *uuid.UUID
	Search    string
	Page      int
	PageSize  int
}

func (r *DispatchRepository) ListIncidents(ctx context.Context, f IncidentFilter) ([]models.DispatchIncident, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	switch f.View {
	case "queue":
		where = append(where, "i.status IN ('NEW', 'CLASSIFIED')")
	case "active":
		where = append(where, "i.status IN ('DISPATCHED', 'ON_SCENE')")
	case "open":
		where = append(where, "i.status <> 'CLOSED'")
	case "closed":
		where = append(where, "i.status = 'CLOSED'")
	}
	if f.Status != "" {
		add("i.status = $%d", f.Status)
	}
	if f.Severity != "" {
		add("i.severity = $%d", f.Severity)
	}
	if f.StationID != nil {
		add("i.station_id = $%d", *f.StationID)
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(
			"(i.incident_number ILIKE $%d OR i.location_text ILIKE $%d OR i.description ILIKE $%d OR i.caller_phone ILIKE $%d)", n, n, n, n))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM dispatch_incidents i WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	// Open incidents first; unclassified before classified by severity; then the
	// longest waiting. Closed incidents newest first.
	rows, err := r.db.Query(ctx, incidentSelect()+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (i.status = 'CLOSED'),
		         CASE WHEN i.status = 'CLOSED' THEN NULL
		              ELSE CASE i.severity WHEN 'CRITICAL' THEN 1 WHEN 'HIGH' THEN 2 WHEN 'MEDIUM' THEN 3 WHEN 'LOW' THEN 4 ELSE 0 END END,
		         CASE WHEN i.status = 'CLOSED' THEN NULL ELSE i.received_at END ASC,
		         i.closed_at DESC NULLS LAST
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.DispatchIncident{}
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *DispatchRepository) GetIncident(ctx context.Context, id uuid.UUID) (*models.DispatchIncident, error) {
	i, err := scanIncident(r.db.QueryRow(ctx, incidentSelect()+" WHERE i.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIncidentNotFound
	}
	if err != nil {
		return nil, err
	}
	if i.Assignments, err = r.Assignments(ctx, id); err != nil {
		return nil, err
	}
	return i, nil
}

func (r *DispatchRepository) Assignments(ctx context.Context, incidentID uuid.UUID) ([]models.DispatchAssignment, error) {
	rows, err := r.db.Query(ctx, assignmentSelect()+" WHERE a.incident_id = $1 ORDER BY a.assigned_at", incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DispatchAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *DispatchRepository) GetAssignment(ctx context.Context, id uuid.UUID) (*models.DispatchAssignment, error) {
	a, err := scanAssignment(r.db.QueryRow(ctx, assignmentSelect()+" WHERE a.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssignmentNotFound
	}
	return a, err
}

func (r *DispatchRepository) Events(ctx context.Context, incidentID uuid.UUID) ([]models.DispatchEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.incident_id, e.assignment_id, e.event_type, e.level, e.detail, e.actor_id,
		       CASE WHEN e.actor_id IS NULL THEN 'Escalation rule' ELSE COALESCE(u.name, '') END,
		       e.occurred_at
		FROM dispatch_events e
		LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.incident_id = $1
		ORDER BY e.seq`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DispatchEvent{}
	for rows.Next() {
		var e models.DispatchEvent
		var level *int16
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.AssignmentID, &e.EventType, &level, &e.Detail,
			&e.ActorID, &e.ActorName, &e.OccurredAt); err != nil {
			return nil, err
		}
		if level != nil {
			l := int(*level)
			e.Level = &l
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func addEvent(ctx context.Context, tx pgx.Tx, incidentID uuid.UUID, assignmentID *uuid.UUID, eventType string, level *int, detail string, actor *uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO dispatch_events (incident_id, assignment_id, event_type, level, detail, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6)`, incidentID, assignmentID, eventType, level, detail, actor)
	return err
}

func (r *DispatchRepository) CreateIncident(ctx context.Context, req models.CreateIncidentRequest, stationID uuid.UUID, receivedAt time.Time, actor uuid.UUID) (uuid.UUID, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM stations WHERE id = $1)", stationID).Scan(&exists); err != nil {
		return uuid.Nil, err
	}
	if !exists {
		return uuid.Nil, ErrStationNotFound
	}
	number, err := formatRecordNumber(ctx, r.db, "INC")
	if err != nil {
		return uuid.Nil, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO dispatch_incidents (id, incident_number, source, caller_name, caller_phone, description,
		                                location_text, latitude, longitude, station_id, received_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		id, number, req.Source, req.CallerName, req.CallerPhone, strings.TrimSpace(req.Description),
		strings.TrimSpace(req.LocationText), req.Latitude, req.Longitude, stationID, receivedAt, actor); err != nil {
		return uuid.Nil, err
	}
	if err := addEvent(ctx, tx, id, nil, "INTAKE", nil,
		fmt.Sprintf("%s logged via %s at %s", number, req.Source, strings.TrimSpace(req.LocationText)), &actor); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// lockIncident takes the incident row for the rest of the transaction.
func lockIncident(ctx context.Context, tx pgx.Tx, id uuid.UUID) (status models.IncidentStatus, severity *models.IncidentSeverity, number string, err error) {
	err = tx.QueryRow(ctx,
		"SELECT status, severity, incident_number FROM dispatch_incidents WHERE id = $1 FOR UPDATE", id,
	).Scan(&status, &severity, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrIncidentNotFound
	}
	return
}

func (r *DispatchRepository) Classify(ctx context.Context, id uuid.UUID, incidentType string, severity models.IncidentSeverity, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	status, previous, number, err := lockIncident(ctx, tx, id)
	if err != nil {
		return err
	}
	if status == models.IncidentClosed {
		return conflict(ErrIncidentState, "%s is closed and cannot be reclassified", number)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE dispatch_incidents
		SET incident_type = $2, severity = $3, classified_by = $4, classified_at = NOW(),
		    status = CASE WHEN status = 'NEW' THEN 'CLASSIFIED' ELSE status END, updated_at = NOW()
		WHERE id = $1`, id, strings.TrimSpace(incidentType), severity, actor); err != nil {
		return err
	}
	detail := fmt.Sprintf("Classified as %s, severity %s", strings.TrimSpace(incidentType), severity)
	if previous != nil {
		detail = fmt.Sprintf("Reclassified as %s, severity %s (was %s)", strings.TrimSpace(incidentType), severity, *previous)
	}
	if err := addEvent(ctx, tx, id, nil, "CLASSIFIED", nil, detail, &actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// syncIncidentStatus derives an open incident's status from its assignments.
// Once any unit has reached the scene the incident stays ON_SCENE while units
// remain committed.
func syncIncidentStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE dispatch_incidents i SET status = s.next, updated_at = NOW()
		FROM (
			SELECT CASE
			         WHEN active > 0 AND (on_scene > 0 OR cleared > 0) THEN 'ON_SCENE'
			         WHEN active > 0 THEN 'DISPATCHED'
			         WHEN cleared > 0 THEN 'CLEARED'
			         ELSE 'CLASSIFIED'
			       END AS next
			FROM (
				SELECT COUNT(*) FILTER (WHERE status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE')) AS active,
				       COUNT(*) FILTER (WHERE status = 'ON_SCENE') AS on_scene,
				       COUNT(*) FILTER (WHERE status = 'CLEARED') AS cleared
				FROM dispatch_assignments WHERE incident_id = $1
			) c
		) s
		WHERE i.id = $1 AND i.status <> 'CLOSED' AND i.status IS DISTINCT FROM s.next`, id)
	return err
}

/* ---------------------------------- units --------------------------------- */

// unitRow is the raw material for one dispatchable unit.
const unitsQuery = `
	SELECT 'VEHICLE' AS kind, v.id, v.registration_number, v.type || ' · ' || v.make,
	       v.station_id, COALESCE(s.name, ''), COALESCE(d.name, ''), v.status, v.current_driver,
	       NULL::varchar AS personnel_status, NULL::varchar AS leave_type, NULL::date AS leave_until,
	       COALESCE(v.gps_latitude, s.latitude::double precision), COALESCE(v.gps_longitude, s.longitude::double precision),
	       CASE WHEN v.gps_latitude IS NOT NULL THEN 'VEHICLE_GPS'
	            WHEN s.latitude IS NOT NULL THEN 'STATION' ELSE 'NONE' END,
	       act.incident_id, COALESCE(act.incident_number, ''),
	       NULL::varchar AS crewing
	FROM vehicles v
	LEFT JOIN stations s ON s.id = v.station_id
	LEFT JOIN users d ON d.id = v.current_driver
	LEFT JOIN LATERAL (
		SELECT a.incident_id, i.incident_number FROM dispatch_assignments a
		JOIN dispatch_incidents i ON i.id = a.incident_id
		WHERE a.status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE')
		  AND (a.vehicle_id = v.id OR (v.current_driver IS NOT NULL AND a.officer_id = v.current_driver))
		LIMIT 1
	) act ON true
	UNION ALL
	SELECT 'OFFICER', u.id, u.name, p.rank || ' · ' || p.badge_number,
	       p.station_id, COALESCE(s.name, ''), u.name, NULL, NULL,
	       p.status, p.leave_type, p.leave_until,
	       s.latitude::double precision, s.longitude::double precision,
	       CASE WHEN s.latitude IS NOT NULL THEN 'STATION' ELSE 'NONE' END,
	       act.incident_id, COALESCE(act.incident_number, ''),
	       crew.registration_number
	FROM personnel p
	JOIN users u ON u.id = p.user_id
	LEFT JOIN stations s ON s.id = p.station_id
	LEFT JOIN LATERAL (
		SELECT a.incident_id, i.incident_number FROM dispatch_assignments a
		JOIN dispatch_incidents i ON i.id = a.incident_id
		WHERE a.status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE') AND a.officer_id = u.id
		LIMIT 1
	) act ON true
	LEFT JOIN LATERAL (
		SELECT v.registration_number FROM vehicles v
		WHERE v.current_driver = u.id AND v.status = 'ON_DUTY' LIMIT 1
	) crew ON true
`

func buildUnit(kind, label, detail string, id, stationID uuid.UUID, stationName, crew string,
	vehicleStatus *string, driver *uuid.UUID, personnelStatus, leaveType *string, leaveUntil *time.Time,
	lat, lng *float64, positionSource string, engagedID *uuid.UUID, engagedNumber string, crewing *string) models.DispatchUnit {

	u := models.DispatchUnit{
		Kind: models.UnitKind(kind), ID: id, Label: label, Detail: detail,
		StationID: stationID, StationName: stationName, CrewName: crew,
		Latitude: lat, Longitude: lng, PositionSource: positionSource,
	}
	switch {
	case engagedID != nil:
		u.Availability, u.EngagedIncidentID, u.EngagedIncident = "ENGAGED", engagedID, engagedNumber
		u.Reason = "Committed to " + engagedNumber
	case kind == "VEHICLE" && vehicleStatus != nil && *vehicleStatus == "MAINTENANCE":
		u.Availability, u.Reason = "UNAVAILABLE", "In maintenance"
	case kind == "VEHICLE" && vehicleStatus != nil && *vehicleStatus == "RESERVED":
		u.Availability, u.Reason = "UNAVAILABLE", "Reserved"
	case kind == "VEHICLE" && driver == nil:
		u.Availability, u.Reason = "UNAVAILABLE", "No crew — allocate a driver in the fleet register"
	case kind == "VEHICLE" && vehicleStatus != nil && *vehicleStatus != "ON_DUTY":
		u.Availability, u.Reason = "UNAVAILABLE", "Not on duty"
	case kind == "OFFICER" && crewing != nil:
		u.Availability, u.Reason = "UNAVAILABLE", "Crewing "+*crewing+" — dispatch the vehicle"
	case kind == "OFFICER" && (personnelStatus == nil || *personnelStatus != "ON_DUTY"):
		status := "unknown"
		if personnelStatus != nil {
			status = strings.ToLower(strings.ReplaceAll(*personnelStatus, "_", " "))
		}
		u.Availability, u.Reason = "UNAVAILABLE", "Officer is "+status
	case kind == "OFFICER" && leaveType != nil && leaveUntil != nil && !leaveUntil.Before(time.Now().Truncate(24*time.Hour)):
		u.Availability, u.Reason = "UNAVAILABLE", "On "+*leaveType+" until "+leaveUntil.Format("2 Jan")
	default:
		u.Availability = "AVAILABLE"
	}
	return u
}

// Units lists every vehicle and officer as a dispatchable unit, with
// availability derived from the fleet and personnel registers and from
// active assignments. When an incident position is given, units are ranked by
// straight-line distance to it.
func (r *DispatchRepository) Units(ctx context.Context, stationID *uuid.UUID, fromLat, fromLng *float64) ([]models.DispatchUnit, error) {
	query := "SELECT * FROM (" + unitsQuery + ") units"
	args := []interface{}{}
	if stationID != nil {
		query += " WHERE station_id = $1"
		args = append(args, *stationID)
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []models.DispatchUnit{}
	for rows.Next() {
		var (
			kind, label, detail, stationName, crew, positionSource, engagedNumber string
			id, sid                                                               uuid.UUID
			vehicleStatus, personnelStatus, leaveType, crewing                    *string
			driver, engagedID                                                     *uuid.UUID
			leaveUntil                                                            *time.Time
			lat, lng                                                              *float64
		)
		if err := rows.Scan(&kind, &id, &label, &detail, &sid, &stationName, &crew, &vehicleStatus, &driver,
			&personnelStatus, &leaveType, &leaveUntil, &lat, &lng, &positionSource,
			&engagedID, &engagedNumber, &crewing); err != nil {
			return nil, err
		}
		u := buildUnit(kind, label, detail, id, sid, stationName, crew, vehicleStatus, driver,
			personnelStatus, leaveType, leaveUntil, lat, lng, positionSource, engagedID, engagedNumber, crewing)
		if fromLat != nil && fromLng != nil && lat != nil && lng != nil {
			d := math.Round(HaversineKm(*fromLat, *fromLng, *lat, *lng)*100) / 100
			u.DistanceKm = &d
		}
		units = append(units, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rank := map[string]int{"AVAILABLE": 0, "ENGAGED": 1, "UNAVAILABLE": 2}
	sort.SliceStable(units, func(a, b int) bool {
		ua, ub := units[a], units[b]
		if rank[ua.Availability] != rank[ub.Availability] {
			return rank[ua.Availability] < rank[ub.Availability]
		}
		if (ua.DistanceKm == nil) != (ub.DistanceKm == nil) {
			return ua.DistanceKm != nil
		}
		if ua.DistanceKm != nil && *ua.DistanceKm != *ub.DistanceKm {
			return *ua.DistanceKm < *ub.DistanceKm
		}
		return ua.Label < ub.Label
	})
	return units, nil
}

// HaversineKm is the great-circle distance between two points. It is a
// straight line, not a route.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLng := rad(lat2-lat1), rad(lng2-lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
}

/* ------------------------------- assignments ------------------------------ */

// officerLock serialises every commitment of one officer, whether as an
// officer unit or as the driver of a vehicle unit.
func officerLock(ctx context.Context, tx pgx.Tx, officer uuid.UUID) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('dispatch-officer:' || $1::text, 0))", officer)
	return err
}

// Assign commits a unit to an incident. The unit's state is re-read under
// lock inside the transaction, so a unit that became unavailable after the
// operator opened the recommendation is refused rather than double-booked.
func (r *DispatchRepository) Assign(ctx context.Context, incidentID uuid.UUID, req models.AssignUnitRequest, actor uuid.UUID) (*models.DispatchAssignment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	status, _, number, err := lockIncident(ctx, tx, incidentID)
	if err != nil {
		return nil, err
	}
	switch status {
	case models.IncidentNew:
		return nil, ErrIncidentUnclassified
	case models.IncidentCleared, models.IncidentClosed:
		return nil, conflict(ErrIncidentState, "%s is %s; units can no longer be dispatched to it", number, strings.ToLower(string(status)))
	}

	var incLat, incLng *float64
	if err := tx.QueryRow(ctx, "SELECT latitude, longitude FROM dispatch_incidents WHERE id = $1", incidentID).Scan(&incLat, &incLng); err != nil {
		return nil, err
	}

	var (
		vehicleID *uuid.UUID
		officerID uuid.UUID
		label     string
		unitLat   *float64
		unitLng   *float64
	)
	switch req.Kind {
	case models.UnitVehicle:
		var vStatus, reg string
		var driver *uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT v.status, v.registration_number, v.current_driver,
			       COALESCE(v.gps_latitude, s.latitude::double precision), COALESCE(v.gps_longitude, s.longitude::double precision)
			FROM vehicles v LEFT JOIN stations s ON s.id = v.station_id
			WHERE v.id = $1 FOR UPDATE OF v`, req.ID).Scan(&vStatus, &reg, &driver, &unitLat, &unitLng)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnitNotFound
		}
		if err != nil {
			return nil, err
		}
		label = reg
		if vStatus != "ON_DUTY" {
			return nil, conflict(ErrUnitUnavailable, "%s is %s, not on duty", reg, strings.ToLower(strings.ReplaceAll(vStatus, "_", " ")))
		}
		if driver == nil {
			return nil, conflict(ErrUnitUnavailable, "%s has no crew allocated", reg)
		}
		if err := officerLock(ctx, tx, *driver); err != nil {
			return nil, err
		}
		id := req.ID
		vehicleID, officerID = &id, *driver
	case models.UnitOfficer:
		var pStatus, name string
		var leaveType *string
		var leaveUntil *time.Time
		err := tx.QueryRow(ctx, `
			SELECT p.status, u.name, p.leave_type, p.leave_until, s.latitude::double precision, s.longitude::double precision
			FROM personnel p JOIN users u ON u.id = p.user_id LEFT JOIN stations s ON s.id = p.station_id
			WHERE p.user_id = $1 FOR UPDATE OF p`, req.ID).Scan(&pStatus, &name, &leaveType, &leaveUntil, &unitLat, &unitLng)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnitNotFound
		}
		if err != nil {
			return nil, err
		}
		label = name
		if err := officerLock(ctx, tx, req.ID); err != nil {
			return nil, err
		}
		if pStatus != "ON_DUTY" {
			return nil, conflict(ErrUnitUnavailable, "%s is %s, not on duty", name, strings.ToLower(strings.ReplaceAll(pStatus, "_", " ")))
		}
		if leaveType != nil && leaveUntil != nil && !leaveUntil.Before(time.Now().Truncate(24*time.Hour)) {
			return nil, conflict(ErrUnitUnavailable, "%s is on %s until %s", name, *leaveType, leaveUntil.Format("2 Jan"))
		}
		var crewing string
		if err := tx.QueryRow(ctx,
			"SELECT registration_number FROM vehicles WHERE current_driver = $1 AND status = 'ON_DUTY' LIMIT 1", req.ID,
		).Scan(&crewing); err == nil {
			return nil, conflict(ErrUnitUnavailable, "%s is crewing %s — dispatch the vehicle instead", name, crewing)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		officerID = req.ID
	default:
		return nil, conflict(ErrUnitUnavailable, "unknown unit kind %q", req.Kind)
	}

	var engaged string
	err = tx.QueryRow(ctx, `
		SELECT i.incident_number FROM dispatch_assignments a JOIN dispatch_incidents i ON i.id = a.incident_id
		WHERE a.status IN ('ASSIGNED', 'ACKNOWLEDGED', 'ON_SCENE')
		  AND (a.officer_id = $1 OR ($2::uuid IS NOT NULL AND a.vehicle_id = $2))
		LIMIT 1`, officerID, vehicleID).Scan(&engaged)
	if err == nil {
		return nil, conflict(ErrUnitUnavailable, "%s is already committed to %s", label, engaged)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var distance *float64
	if incLat != nil && incLng != nil && unitLat != nil && unitLng != nil {
		d := math.Round(HaversineKm(*incLat, *incLng, *unitLat, *unitLng)*100) / 100
		distance = &d
	}

	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO dispatch_assignments (id, incident_id, unit_kind, vehicle_id, officer_id, assigned_by, distance_km)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, incidentID, req.Kind, vehicleID, officerID, actor, distance); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, conflict(ErrUnitUnavailable, "%s was committed to another incident a moment ago", label)
		}
		return nil, err
	}
	detail := fmt.Sprintf("%s dispatched to %s", label, number)
	if distance != nil {
		detail += fmt.Sprintf(" (%.2f km straight-line)", *distance)
	}
	if err := addEvent(ctx, tx, incidentID, &id, "ASSIGNED", nil, detail, &actor); err != nil {
		return nil, err
	}
	if err := syncIncidentStatus(ctx, tx, incidentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetAssignment(ctx, id)
}

// Progress moves an assignment one step: ACKNOWLEDGED, ON_SCENE or CLEARED.
// Only the next step from the current one is accepted.
func (r *DispatchRepository) Progress(ctx context.Context, assignmentID uuid.UUID, to models.AssignmentStatus, actor uuid.UUID, mayActForOthers bool) (*models.DispatchAssignment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var incidentID uuid.UUID
	var current models.AssignmentStatus
	var officer *uuid.UUID
	err = tx.QueryRow(ctx, "SELECT incident_id, status, officer_id FROM dispatch_assignments WHERE id = $1", assignmentID).
		Scan(&incidentID, &current, &officer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, err
	}
	// Lock order: incident, then assignment — the same order Assign uses.
	if _, _, _, err := lockIncident(ctx, tx, incidentID); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, "SELECT status FROM dispatch_assignments WHERE id = $1 FOR UPDATE", assignmentID).Scan(&current); err != nil {
		return nil, err
	}
	if !mayActForOthers && (officer == nil || *officer != actor) {
		return nil, conflict(ErrAssignmentState, "only the assigned officer, or an ASI or above on their behalf, can record this")
	}

	var from models.AssignmentStatus
	var set, event, verb string
	switch to {
	case models.AssignmentAcknowledged:
		from, set, event, verb = models.AssignmentAssigned, "acknowledged_at = NOW(), acknowledged_by = $2", "ACKNOWLEDGED", "acknowledged"
	case models.AssignmentOnScene:
		from, set, event, verb = models.AssignmentAcknowledged, "on_scene_at = NOW(), on_scene_by = $2", "ON_SCENE", "reached the scene"
	case models.AssignmentCleared:
		from, set, event, verb = models.AssignmentOnScene, "cleared_at = NOW(), cleared_by = $2", "CLEARED", "cleared the scene"
	default:
		return nil, conflict(ErrAssignmentState, "unknown step %q", to)
	}
	if current != from {
		return nil, conflict(ErrAssignmentState, "cannot mark %s: the unit is %s, expected %s",
			strings.ToLower(strings.ReplaceAll(string(to), "_", " ")),
			strings.ToLower(strings.ReplaceAll(string(current), "_", " ")),
			strings.ToLower(strings.ReplaceAll(string(from), "_", " ")))
	}
	if _, err := tx.Exec(ctx, "UPDATE dispatch_assignments SET status = $3, "+set+" WHERE id = $1",
		assignmentID, actor, to); err != nil {
		return nil, err
	}
	a, err := scanAssignment(tx.QueryRow(ctx, assignmentSelect()+" WHERE a.id = $1", assignmentID))
	if err != nil {
		return nil, err
	}
	unit := a.VehicleNumber
	if unit == "" {
		unit = a.OfficerName
	}
	if err := addEvent(ctx, tx, incidentID, &assignmentID, event, nil, unit+" "+verb, &actor); err != nil {
		return nil, err
	}
	if err := syncIncidentStatus(ctx, tx, incidentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetAssignment(ctx, assignmentID)
}

// CancelAssignment stands a unit down before it reaches the scene.
func (r *DispatchRepository) CancelAssignment(ctx context.Context, assignmentID uuid.UUID, reason string, actor uuid.UUID) (*models.DispatchAssignment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var incidentID uuid.UUID
	err = tx.QueryRow(ctx, "SELECT incident_id FROM dispatch_assignments WHERE id = $1", assignmentID).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, _, _, err := lockIncident(ctx, tx, incidentID); err != nil {
		return nil, err
	}
	var current models.AssignmentStatus
	if err := tx.QueryRow(ctx, "SELECT status FROM dispatch_assignments WHERE id = $1 FOR UPDATE", assignmentID).Scan(&current); err != nil {
		return nil, err
	}
	if current != models.AssignmentAssigned && current != models.AssignmentAcknowledged {
		return nil, conflict(ErrAssignmentState, "a unit that is %s cannot be stood down; clear it from the scene instead",
			strings.ToLower(strings.ReplaceAll(string(current), "_", " ")))
	}
	if _, err := tx.Exec(ctx, `
		UPDATE dispatch_assignments SET status = 'CANCELLED', cancelled_at = NOW(), cancelled_by = $2, cancel_reason = $3
		WHERE id = $1`, assignmentID, actor, reason); err != nil {
		return nil, err
	}
	a, err := scanAssignment(tx.QueryRow(ctx, assignmentSelect()+" WHERE a.id = $1", assignmentID))
	if err != nil {
		return nil, err
	}
	unit := a.VehicleNumber
	if unit == "" {
		unit = a.OfficerName
	}
	if err := addEvent(ctx, tx, incidentID, &assignmentID, "ASSIGNMENT_CANCELLED", nil, unit+" stood down: "+reason, &actor); err != nil {
		return nil, err
	}
	if err := syncIncidentStatus(ctx, tx, incidentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetAssignment(ctx, assignmentID)
}

func (r *DispatchRepository) Escalate(ctx context.Context, incidentID uuid.UUID, reason string, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, _, number, err := lockIncident(ctx, tx, incidentID)
	if err != nil {
		return err
	}
	if status == models.IncidentClosed {
		return conflict(ErrIncidentState, "%s is closed", number)
	}
	var level int
	if err := tx.QueryRow(ctx, `
		UPDATE dispatch_incidents SET escalation_level = LEAST(escalation_level + 1, 3), updated_at = NOW()
		WHERE id = $1 RETURNING escalation_level`, incidentID).Scan(&level); err != nil {
		return err
	}
	action := models.AcknowledgementLadder[level-1].Action
	if err := addEvent(ctx, tx, incidentID, nil, "ESCALATED", &level,
		fmt.Sprintf("Escalated by the operator to level %d (%s): %s", level, action, reason), &actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *DispatchRepository) Close(ctx context.Context, incidentID uuid.UUID, req models.CloseIncidentRequest, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, _, number, err := lockIncident(ctx, tx, incidentID)
	if err != nil {
		return err
	}
	switch status {
	case models.IncidentClosed:
		return conflict(ErrIncidentState, "%s is already closed", number)
	case models.IncidentDispatched, models.IncidentOnScene:
		return conflict(ErrIncidentState, "%s still has units committed; clear or stand them down before closing", number)
	}
	if req.FIRID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM firs WHERE id = $1)", *req.FIRID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrFIRNotFound
		}
	}
	// A NEW incident closed without classification (a hoax, a duplicate) keeps
	// no severity; the constraint allows CLOSED without one.
	if _, err := tx.Exec(ctx, `
		UPDATE dispatch_incidents
		SET status = 'CLOSED', outcome = $2, outcome_note = NULLIF($3, ''), fir_id = $4,
		    closed_by = $5, closed_at = NOW(), updated_at = NOW()
		WHERE id = $1`, incidentID, req.Outcome, strings.TrimSpace(req.Note), req.FIRID, actor); err != nil {
		return err
	}
	detail := fmt.Sprintf("Closed: %s", strings.ToLower(strings.ReplaceAll(string(req.Outcome), "_", " ")))
	if n := strings.TrimSpace(req.Note); n != "" {
		detail += " — " + n
	}
	if err := addEvent(ctx, tx, incidentID, nil, "CLOSED", nil, detail, &actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

/* ------------------------------- escalations ------------------------------ */

// EscalationRecord describes one escalation raised by the stated rules.
type EscalationRecord struct {
	IncidentID uuid.UUID
	Number     string
	Detail     string
}

// ApplyEscalations raises any escalation the stated rules now require and
// records each as an event. It is idempotent: a level is recorded once per
// incident and an on-scene alert once per assignment, and incidents another
// caller is already processing are skipped.
func (r *DispatchRepository) ApplyEscalations(ctx context.Context) ([]EscalationRecord, error) {
	var records []EscalationRecord

	// Acknowledgement ladder.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT i.id, i.incident_number, i.escalation_level,
		       EXTRACT(EPOCH FROM NOW() - MIN(a.assigned_at)) / 60.0
		FROM dispatch_incidents i
		JOIN dispatch_assignments a ON a.incident_id = i.id AND a.status = 'ASSIGNED'
		WHERE i.status IN ('DISPATCHED', 'ON_SCENE')
		GROUP BY i.id
		HAVING EXTRACT(EPOCH FROM NOW() - MIN(a.assigned_at)) / 60.0 >= $1`,
		models.AcknowledgementLadder[0].AfterMinutes)
	if err != nil {
		return nil, err
	}
	type ladderRow struct {
		id      uuid.UUID
		number  string
		level   int
		waiting float64
	}
	var pending []ladderRow
	for rows.Next() {
		var lr ladderRow
		var level int16
		if err := rows.Scan(&lr.id, &lr.number, &level, &lr.waiting); err != nil {
			rows.Close()
			return nil, err
		}
		lr.level = int(level)
		pending = append(pending, lr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, lr := range pending {
		required := 0
		for _, step := range models.AcknowledgementLadder {
			if lr.waiting >= float64(step.AfterMinutes) {
				required = step.Level
			}
		}
		if required <= lr.level {
			continue
		}
		// Re-check under lock; skip an incident another caller holds.
		var current int16
		err := tx.QueryRow(ctx, "SELECT escalation_level FROM dispatch_incidents WHERE id = $1 FOR UPDATE SKIP LOCKED", lr.id).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for level := int(current) + 1; level <= required; level++ {
			step := models.AcknowledgementLadder[level-1]
			detail := fmt.Sprintf("Unacknowledged for more than %d minutes — escalated to level %d: %s",
				step.AfterMinutes, level, step.Action)
			l := level
			if err := addEvent(ctx, tx, lr.id, nil, "ESCALATED", &l, detail, nil); err != nil {
				return nil, err
			}
			records = append(records, EscalationRecord{IncidentID: lr.id, Number: lr.number, Detail: detail})
		}
		if int(current) < required {
			if _, err := tx.Exec(ctx, "UPDATE dispatch_incidents SET escalation_level = $2, updated_at = NOW() WHERE id = $1",
				lr.id, required); err != nil {
				return nil, err
			}
		}
	}

	// Arrival threshold: acknowledged but not on scene within the severity's minutes.
	scene, err := tx.Query(ctx, `
		SELECT a.id, a.incident_id, i.incident_number, COALESCE(v.registration_number, o.name, ''), i.severity
		FROM dispatch_assignments a
		JOIN dispatch_incidents i ON i.id = a.incident_id
		LEFT JOIN vehicles v ON v.id = a.vehicle_id
		LEFT JOIN users o ON o.id = a.officer_id
		WHERE a.on_scene_alerted_at IS NULL AND `+onSceneOverdueSQL("a", "i")+`
		FOR UPDATE OF a SKIP LOCKED`)
	if err != nil {
		return nil, err
	}
	type sceneRow struct {
		assignment, incident uuid.UUID
		number, unit         string
		severity             *string
	}
	var overdue []sceneRow
	for scene.Next() {
		var sr sceneRow
		if err := scene.Scan(&sr.assignment, &sr.incident, &sr.number, &sr.unit, &sr.severity); err != nil {
			scene.Close()
			return nil, err
		}
		overdue = append(overdue, sr)
	}
	scene.Close()
	if err := scene.Err(); err != nil {
		return nil, err
	}
	for _, sr := range overdue {
		minutes := models.SeverityScale[len(models.SeverityScale)-1].OnSceneMinutes
		if sr.severity != nil {
			minutes = models.OnSceneMinutes(models.IncidentSeverity(*sr.severity))
		}
		detail := fmt.Sprintf("%s acknowledged but not on scene within %d minutes — supervisor alerted", sr.unit, minutes)
		if _, err := tx.Exec(ctx, "UPDATE dispatch_assignments SET on_scene_alerted_at = NOW() WHERE id = $1", sr.assignment); err != nil {
			return nil, err
		}
		a := sr.assignment
		if err := addEvent(ctx, tx, sr.incident, &a, "ON_SCENE_OVERDUE", nil, detail, nil); err != nil {
			return nil, err
		}
		records = append(records, EscalationRecord{IncidentID: sr.incident, Number: sr.number, Detail: detail})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return records, nil
}

/* -------------------------------- analytics ------------------------------- */

func (r *DispatchRepository) Stats(ctx context.Context, stationID *uuid.UUID) (*models.DispatchStats, error) {
	var s models.DispatchStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE i.status IN ('NEW', 'CLASSIFIED')),
			COUNT(*) FILTER (WHERE i.status IN ('DISPATCHED', 'ON_SCENE')),
			COUNT(*) FILTER (WHERE i.status = 'CLEARED'),
			COUNT(*) FILTER (WHERE i.status = 'CLOSED' AND i.closed_at >= date_trunc('day', NOW())),
			(SELECT COUNT(*) FROM dispatch_assignments a JOIN dispatch_incidents i2 ON i2.id = a.incident_id
			  WHERE `+ackOverdueSQL("a")+` AND ($1::uuid IS NULL OR i2.station_id = $1)),
			(SELECT COUNT(*) FROM dispatch_assignments a JOIN dispatch_incidents i2 ON i2.id = a.incident_id
			  WHERE `+onSceneOverdueSQL("a", "i2")+` AND ($1::uuid IS NULL OR i2.station_id = $1))
		FROM dispatch_incidents i
		WHERE $1::uuid IS NULL OR i.station_id = $1`, stationID).Scan(
		&s.AwaitingDispatch, &s.Active, &s.AwaitingClosure, &s.ClosedToday, &s.OverdueAcks, &s.OverdueOnScene)
	if err != nil {
		return nil, err
	}
	units, err := r.Units(ctx, stationID, nil, nil)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		switch u.Availability {
		case "AVAILABLE":
			s.UnitsAvailable++
		case "ENGAGED":
			s.UnitsEngaged++
		}
	}
	return &s, nil
}

// Analytics reports response intervals for incidents received in [from, to),
// per station and for all stations together (StationID nil). Medians and 90th
// percentiles are computed by Postgres over every sample in the period.
//
//   - call to dispatch: the incident's first assignment minus the call time
//   - dispatch to acknowledge: per assignment that was acknowledged
//   - acknowledge to scene: per assignment that reached the scene
func (r *DispatchRepository) Analytics(ctx context.Context, from, to time.Time, stationID *uuid.UUID) ([]models.DispatchAnalyticsRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH inc AS (
			SELECT i.id, i.station_id, i.received_at,
			       (SELECT MIN(a.assigned_at) FROM dispatch_assignments a WHERE a.incident_id = i.id) AS first_dispatch
			FROM dispatch_incidents i
			WHERE i.received_at >= $1 AND i.received_at < $2 AND ($3::uuid IS NULL OR i.station_id = $3)
		),
		asg AS (
			SELECT inc.station_id,
			       EXTRACT(EPOCH FROM a.acknowledged_at - a.assigned_at) AS ack_s,
			       EXTRACT(EPOCH FROM a.on_scene_at - a.acknowledged_at) AS scene_s
			FROM dispatch_assignments a JOIN inc ON inc.id = a.incident_id
		),
		c2d AS (
			SELECT station_id, COUNT(*) AS incidents,
			       COUNT(first_dispatch) AS n,
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_dispatch - received_at)) AS med,
			       percentile_cont(0.9) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_dispatch - received_at)) AS p90,
			       GROUPING(station_id) AS g
			FROM inc GROUP BY GROUPING SETS ((station_id), ())
		),
		d2a AS (
			SELECT station_id, COUNT(ack_s) AS n,
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY ack_s) AS med,
			       percentile_cont(0.9) WITHIN GROUP (ORDER BY ack_s) AS p90,
			       GROUPING(station_id) AS g
			FROM asg WHERE ack_s IS NOT NULL GROUP BY GROUPING SETS ((station_id), ())
		),
		a2s AS (
			SELECT station_id, COUNT(scene_s) AS n,
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY scene_s) AS med,
			       percentile_cont(0.9) WITHIN GROUP (ORDER BY scene_s) AS p90,
			       GROUPING(station_id) AS g
			FROM asg WHERE scene_s IS NOT NULL GROUP BY GROUPING SETS ((station_id), ())
		)
		SELECT CASE WHEN c2d.g = 1 THEN NULL ELSE c2d.station_id END,
		       CASE WHEN c2d.g = 1 THEN 'All stations' ELSE COALESCE(s.name, '') END,
		       c2d.incidents, c2d.n, c2d.med, c2d.p90,
		       COALESCE(d2a.n, 0), d2a.med, d2a.p90,
		       COALESCE(a2s.n, 0), a2s.med, a2s.p90
		FROM c2d
		LEFT JOIN d2a ON d2a.g = c2d.g AND d2a.station_id IS NOT DISTINCT FROM c2d.station_id
		LEFT JOIN a2s ON a2s.g = c2d.g AND a2s.station_id IS NOT DISTINCT FROM c2d.station_id
		LEFT JOIN stations s ON s.id = c2d.station_id
		ORDER BY c2d.g DESC, s.name`, from, to, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DispatchAnalyticsRow{}
	for rows.Next() {
		var row models.DispatchAnalyticsRow
		if err := rows.Scan(&row.StationID, &row.StationName, &row.Incidents,
			&row.CallToDispatch.Samples, &row.CallToDispatch.MedianSec, &row.CallToDispatch.P90Sec,
			&row.DispatchToAck.Samples, &row.DispatchToAck.MedianSec, &row.DispatchToAck.P90Sec,
			&row.AckToScene.Samples, &row.AckToScene.MedianSec, &row.AckToScene.P90Sec); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
