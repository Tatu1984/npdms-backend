package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrPropertyNotFound        = errors.New("property not found")
	ErrPropertyLocationMissing = errors.New("storage location not found at this station")
	ErrPropertyLocationExists  = errors.New("this room, rack and shelf is already registered at the station")
	ErrPropertyNotInMalkhana   = errors.New("the property is not in the malkhana")
	ErrPropertySealBroken      = errors.New("the property's seal is recorded as broken; an SHO must record a reason and reseal it before it can move")
	ErrPropertySealNotBroken   = errors.New("the property's seal is not recorded as broken")
	ErrPropertyOpenMovement    = errors.New("the property already has a movement that has not been returned")
	ErrPropertyNoOpenMovement  = errors.New("the property has no movement awaiting return")
	ErrPropertyMovementMissing = errors.New("movement not found for this property")
	ErrPropertyDisposed        = errors.New("the property has been disposed of and its record is closed")
	ErrPropertyEvidenceLinked  = errors.New("that evidence item is already linked to another property record")
	ErrPropertyRecordMissing   = errors.New("the FIR, case or evidence item was not found")
	ErrPropertyOrderMismatch   = errors.New("the court order must be recorded against the same case as the property")
	ErrPropertyWitnessRank     = errors.New("narcotics destruction must be witnessed by an officer of DSP rank or above")
	ErrPropertyHearingMismatch = errors.New("the hearing must belong to the property's case")
)

type MalkhanaRepository struct {
	db *pgxpool.Pool
}

func NewMalkhanaRepository(db *pgxpool.Pool) *MalkhanaRepository {
	return &MalkhanaRepository{db: db}
}

// ------------------------------------------------------------------ locations

const malkhanaLocationSelect = `
	SELECT l.id, l.station_id, COALESCE(s.name, ''), l.room, l.rack, l.shelf, l.notes, l.active,
	       (SELECT COUNT(*) FROM property_items p WHERE p.location_id = l.id AND p.status = 'IN_MALKHANA'),
	       l.created_at
	FROM malkhana_locations l
	LEFT JOIN stations s ON s.id = l.station_id
`

func scanMalkhanaLocation(row pgx.Row) (*models.MalkhanaLocation, error) {
	var l models.MalkhanaLocation
	if err := row.Scan(&l.ID, &l.StationID, &l.StationName, &l.Room, &l.Rack, &l.Shelf, &l.Notes, &l.Active,
		&l.ItemsHeld, &l.CreatedAt); err != nil {
		return nil, err
	}
	l.Label = malkhanaLocationLabel(l.Room, l.Rack, l.Shelf)
	return &l, nil
}

func malkhanaLocationLabel(room, rack, shelf string) string {
	label := room + " / " + rack
	if strings.TrimSpace(shelf) != "" {
		label += " / " + shelf
	}
	return label
}

func (r *MalkhanaRepository) ListLocations(ctx context.Context, stationID *uuid.UUID) ([]models.MalkhanaLocation, error) {
	rows, err := r.db.Query(ctx, malkhanaLocationSelect+`
		WHERE ($1::uuid IS NULL OR l.station_id = $1)
		ORDER BY s.name, l.room, l.rack, l.shelf`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MalkhanaLocation{}
	for rows.Next() {
		l, err := scanMalkhanaLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (r *MalkhanaRepository) CreateLocation(ctx context.Context, stationID uuid.UUID, req models.CreateMalkhanaLocationRequest, actor uuid.UUID) (*models.MalkhanaLocation, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO malkhana_locations (id, station_id, room, rack, shelf, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, stationID, strings.TrimSpace(req.Room), strings.TrimSpace(req.Rack), strings.TrimSpace(req.Shelf), req.Notes, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, ErrPropertyLocationExists
	}
	if err != nil {
		return nil, err
	}
	return scanMalkhanaLocation(r.db.QueryRow(ctx, malkhanaLocationSelect+" WHERE l.id = $1", id))
}

// locationAtStation confirms a location exists, is active, and belongs to the station.
func malkhanaLocationAtStation(ctx context.Context, q pgx.Tx, locationID, stationID uuid.UUID) error {
	var ok bool
	err := q.QueryRow(ctx,
		"SELECT TRUE FROM malkhana_locations WHERE id = $1 AND station_id = $2 AND active", locationID, stationID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPropertyLocationMissing
	}
	return err
}

// ---------------------------------------------------------------------- items

const propertyItemSelect = `
	SELECT p.id, p.property_number, p.station_id, COALESCE(s.name, ''), COALESCE(s.code, ''),
	       p.fir_id, COALESCE(f.fir_number, ''), p.case_id, COALESCE(c.case_number, ''),
	       p.evidence_id, COALESCE(e.evidence_number, ''),
	       p.category, p.description, p.quantity::float8, p.unit, p.weight_grams::float8, p.value_paise,
	       p.seized_at, p.seized_place, p.seized_by, COALESCE(sb.name, ''), p.seizure_memo_ref,
	       p.status, p.location_id, COALESCE(l.room, ''), COALESCE(l.rack, ''), COALESCE(l.shelf, ''),
	       p.seal_number, p.seal_state, p.seal_broken_at,
	       p.deposited_at, COALESCE(db.name, ''),
	       p.disposal_type, p.disposal_order_id, COALESCE(w.name, ''), p.disposal_note, p.disposed_at, COALESCE(ds.name, ''),
	       GREATEST(0, EXTRACT(DAY FROM NOW() - p.deposited_at))::int,
	       (p.status <> 'DISPOSED' AND p.deposited_at < NOW() - make_interval(days => $REVIEW)),
	       EXISTS (SELECT 1 FROM property_movements m WHERE m.item_id = p.id AND m.returned_at IS NULL AND m.expected_return_at < NOW()),
	       (SELECT m.id FROM property_movements m WHERE m.item_id = p.id AND m.returned_at IS NULL),
	       p.created_at, p.updated_at
	FROM property_items p
	LEFT JOIN stations s ON s.id = p.station_id
	LEFT JOIN firs f ON f.id = p.fir_id
	LEFT JOIN cases c ON c.id = p.case_id
	LEFT JOIN evidence e ON e.id = p.evidence_id
	LEFT JOIN users sb ON sb.id = p.seized_by
	LEFT JOIN users db ON db.id = p.deposited_by
	LEFT JOIN users w ON w.id = p.disposal_witness_id
	LEFT JOIN users ds ON ds.id = p.disposed_by
	LEFT JOIN malkhana_locations l ON l.id = p.location_id
`

var propertyItemSelectSQL = strings.Replace(propertyItemSelect, "$REVIEW", fmt.Sprint(models.PropertyReviewPeriodDays), 1)

func scanPropertyItem(row pgx.Row) (*models.PropertyItem, *uuid.UUID, error) {
	var p models.PropertyItem
	var room, rack, shelf string
	var openMovement *uuid.UUID
	err := row.Scan(
		&p.ID, &p.PropertyNumber, &p.StationID, &p.StationName, &p.StationCode,
		&p.FIRID, &p.FIRNumber, &p.CaseID, &p.CaseNumber,
		&p.EvidenceID, &p.EvidenceNumber,
		&p.Category, &p.Description, &p.Quantity, &p.Unit, &p.WeightGrams, &p.ValuePaise,
		&p.SeizedAt, &p.SeizedPlace, &p.SeizedBy, &p.SeizedByName, &p.SeizureMemoRef,
		&p.Status, &p.LocationID, &room, &rack, &shelf,
		&p.SealNumber, &p.SealState, &p.SealBrokenAt,
		&p.DepositedAt, &p.DepositedByName,
		&p.DisposalType, &p.DisposalOrderID, &p.DisposalWitness, &p.DisposalNote, &p.DisposedAt, &p.DisposedByName,
		&p.HeldDays, &p.ReviewDue, &p.MovementOverdue, &openMovement,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}
	if room != "" {
		p.LocationLabel = malkhanaLocationLabel(room, rack, shelf)
	}
	p.ReviewPeriodDays = models.PropertyReviewPeriodDays
	return &p, openMovement, nil
}

type PropertyFilter struct {
	Search    string
	Status    string
	Category  string
	StationID *uuid.UUID
	CaseID    *uuid.UUID
	FIRID     *uuid.UUID
	Attention string // SEAL_BROKEN, OVERDUE, REVIEW_DUE
	Page      int
	PageSize  int
}

func (r *MalkhanaRepository) ListItems(ctx context.Context, f PropertyFilter) ([]models.PropertyItem, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(`(p.property_number ILIKE $%d OR p.description ILIKE $%d OR p.seal_number ILIKE $%d
			OR p.seizure_memo_ref ILIKE $%d OR f.fir_number ILIKE $%d OR c.case_number ILIKE $%d)`, n, n, n, n, n, n))
	}
	if f.Status != "" {
		add("p.status = $%d", f.Status)
	}
	if f.Category != "" {
		add("p.category = $%d", f.Category)
	}
	if f.StationID != nil {
		add("p.station_id = $%d", *f.StationID)
	}
	if f.CaseID != nil {
		add("p.case_id = $%d", *f.CaseID)
	}
	if f.FIRID != nil {
		add("p.fir_id = $%d", *f.FIRID)
	}
	switch f.Attention {
	case "SEAL_BROKEN":
		where = append(where, "p.seal_state = 'BROKEN' AND p.status <> 'DISPOSED'")
	case "OVERDUE":
		where = append(where, "EXISTS (SELECT 1 FROM property_movements m WHERE m.item_id = p.id AND m.returned_at IS NULL AND m.expected_return_at < NOW())")
	case "REVIEW_DUE":
		where = append(where, fmt.Sprintf("p.status <> 'DISPOSED' AND p.deposited_at < NOW() - make_interval(days => %d)", models.PropertyReviewPeriodDays))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM property_items p
		LEFT JOIN firs f ON f.id = p.fir_id
		LEFT JOIN cases c ON c.id = p.case_id
		WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, propertyItemSelectSQL+" WHERE "+clause+fmt.Sprintf(`
		ORDER BY (p.seal_state = 'BROKEN' AND p.status <> 'DISPOSED') DESC, p.deposited_at DESC
		LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	type pending struct {
		item models.PropertyItem
		open *uuid.UUID
	}
	var list []pending
	for rows.Next() {
		p, open, err := scanPropertyItem(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		list = append(list, pending{*p, open})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	out := make([]models.PropertyItem, 0, len(list))
	for _, p := range list {
		if p.open != nil {
			m, err := r.movement(ctx, *p.open)
			if err != nil {
				return nil, 0, err
			}
			p.item.OpenMovement = m
		}
		out = append(out, p.item)
	}
	return out, total, nil
}

func (r *MalkhanaRepository) GetItem(ctx context.Context, id uuid.UUID) (*models.PropertyItem, error) {
	p, open, err := scanPropertyItem(r.db.QueryRow(ctx, propertyItemSelectSQL+" WHERE p.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPropertyNotFound
	}
	if err != nil {
		return nil, err
	}
	if open != nil {
		if p.OpenMovement, err = r.movement(ctx, *open); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (r *MalkhanaRepository) GetItemByNumber(ctx context.Context, number string) (*models.PropertyItem, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, "SELECT id FROM property_items WHERE property_number = $1", number).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPropertyNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetItem(ctx, id)
}

// RegisterItem records the seizure, the deposit and the first seal check in one transaction.
func (r *MalkhanaRepository) RegisterItem(ctx context.Context, stationID uuid.UUID, req models.RegisterPropertyRequest, actor uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "MLK")
	if err != nil {
		return uuid.Nil, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	if err := malkhanaLocationAtStation(ctx, tx, *req.LocationID, stationID); err != nil {
		return uuid.Nil, err
	}
	// A case must agree with the FIR it was registered from, when both are given.
	if req.CaseID != nil && req.FIRID != nil {
		var caseFIR *uuid.UUID
		if err := tx.QueryRow(ctx, "SELECT fir_id FROM cases WHERE id = $1", *req.CaseID).Scan(&caseFIR); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, ErrPropertyRecordMissing
			}
			return uuid.Nil, err
		}
		if caseFIR != nil && *caseFIR != *req.FIRID {
			return uuid.Nil, fmt.Errorf("%w: the case was registered from a different FIR", ErrPropertyRecordMissing)
		}
	}

	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO property_items (id, property_number, station_id, fir_id, case_id, evidence_id, category, description,
			quantity, unit, weight_grams, value_paise, seized_at, seized_place, seized_by, seizure_memo_ref,
			location_id, seal_number, deposited_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		id, number, stationID, req.FIRID, req.CaseID, req.EvidenceID, req.Category, strings.TrimSpace(req.Description),
		req.Quantity, strings.TrimSpace(req.Unit), req.WeightGrams, req.ValuePaise, *req.SeizedAt,
		strings.TrimSpace(req.SeizedPlace), *req.SeizedBy, strings.TrimSpace(req.SeizureMemoRef),
		*req.LocationID, strings.TrimSpace(req.SealNumber), actor)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "evidence"):
				return uuid.Nil, ErrPropertyEvidenceLinked
			case pgErr.Code == "23503":
				return uuid.Nil, ErrPropertyRecordMissing
			}
		}
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_seal_checks (item_id, checked_by, seal_number, seal_state, context, note)
		VALUES ($1, $2, $3, 'INTACT', 'DEPOSIT', 'Sealed at deposit')`, id, actor, strings.TrimSpace(req.SealNumber)); err != nil {
		return uuid.Nil, err
	}
	if err := addPropertyEvent(ctx, tx, id, "DEPOSITED", &actor,
		fmt.Sprintf("Deposited %s under seal %s", number, strings.TrimSpace(req.SealNumber)),
		map[string]interface{}{"locationId": req.LocationID, "seizureMemoRef": req.SeizureMemoRef}); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

func addPropertyEvent(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, eventType string, actor *uuid.UUID, summary string, details map[string]interface{}) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO property_events (item_id, event_type, actor_id, summary, details)
		VALUES ($1, $2, $3, $4, $5)`, itemID, eventType, actor, summary, raw)
	return err
}

// lockItem reads an item's state under a row lock for the rest of the transaction.
func lockPropertyItem(ctx context.Context, tx pgx.Tx, id uuid.UUID) (status, sealState, category string, stationID uuid.UUID, caseID *uuid.UUID, err error) {
	err = tx.QueryRow(ctx,
		"SELECT status, seal_state, category, station_id, case_id FROM property_items WHERE id = $1 FOR UPDATE", id).
		Scan(&status, &sealState, &category, &stationID, &caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrPropertyNotFound
	}
	return
}

// RecordSealCheck records a verification. A broken seal marks the item and returns true.
func (r *MalkhanaRepository) RecordSealCheck(ctx context.Context, id uuid.UUID, sealNumber string, intact bool, note *string, actor uuid.UUID) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	status, sealState, _, _, _, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if status == "DISPOSED" {
		return false, ErrPropertyDisposed
	}
	if status != "IN_MALKHANA" {
		return false, ErrPropertyNotInMalkhana
	}
	state := "INTACT"
	if !intact {
		state = "BROKEN"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_seal_checks (item_id, checked_by, seal_number, seal_state, context, note)
		VALUES ($1, $2, $3, $4, 'VERIFICATION', $5)`, id, actor, sealNumber, state, note); err != nil {
		return false, err
	}
	newlyBroken := !intact && sealState == "INTACT"
	if newlyBroken {
		if _, err := tx.Exec(ctx, `
			UPDATE property_items SET seal_state = 'BROKEN', seal_broken_at = NOW(), updated_at = NOW() WHERE id = $1`, id); err != nil {
			return false, err
		}
	}
	summary := "Seal " + sealNumber + " verified intact"
	if !intact {
		summary = "Seal " + sealNumber + " found broken"
	}
	if err := addPropertyEvent(ctx, tx, id, "SEAL_VERIFIED", &actor, summary, map[string]interface{}{"intact": intact, "note": note}); err != nil {
		return false, err
	}
	return newlyBroken, tx.Commit(ctx)
}

// Reseal clears a broken seal with the SHO's recorded reason and a new seal number.
func (r *MalkhanaRepository) Reseal(ctx context.Context, id uuid.UUID, reason, newSeal string, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, sealState, _, _, _, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return err
	}
	if status == "DISPOSED" {
		return ErrPropertyDisposed
	}
	if sealState != "BROKEN" {
		return ErrPropertySealNotBroken
	}
	if _, err := tx.Exec(ctx, `
		UPDATE property_items SET seal_state = 'INTACT', seal_broken_at = NULL, seal_number = $2, updated_at = NOW()
		WHERE id = $1`, id, newSeal); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_seal_checks (item_id, checked_by, seal_number, seal_state, context, note)
		VALUES ($1, $2, $3, 'INTACT', 'RESEAL', $4)`, id, actor, newSeal, reason); err != nil {
		return err
	}
	if err := addPropertyEvent(ctx, tx, id, "RESEALED", &actor, "Resealed under "+newSeal+": "+reason,
		map[string]interface{}{"reason": reason, "newSealNumber": newSeal}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *MalkhanaRepository) Relocate(ctx context.Context, id, locationID uuid.UUID, reason string, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, _, _, stationID, _, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return err
	}
	if status != "IN_MALKHANA" {
		return ErrPropertyNotInMalkhana
	}
	if err := malkhanaLocationAtStation(ctx, tx, locationID, stationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE property_items SET location_id = $2, updated_at = NOW() WHERE id = $1", id, locationID); err != nil {
		return err
	}
	if err := addPropertyEvent(ctx, tx, id, "RELOCATED", &actor, "Moved within the malkhana: "+reason,
		map[string]interface{}{"locationId": locationID, "reason": reason}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ------------------------------------------------------------------ movements

const propertyMovementSelect = `
	SELECT m.id, m.item_id, p.property_number, m.movement_type, m.destination, m.destination_station_id,
	       COALESCE(ds.name, ''), m.court_hearing_id, h.hearing_date::timestamptz, m.purpose, m.authority_ref, m.handed_to,
	       m.expected_return_at, m.moved_out_at, COALESCE(mo.name, ''), m.seal_number_out,
	       m.returned_at, COALESCE(rb.name, ''), m.returned_by_name, m.seal_number_back, m.seal_intact_back, m.return_note,
	       (m.returned_at IS NULL AND m.expected_return_at < NOW())
	FROM property_movements m
	JOIN property_items p ON p.id = m.item_id
	LEFT JOIN stations ds ON ds.id = m.destination_station_id
	LEFT JOIN court_hearings h ON h.id = m.court_hearing_id
	LEFT JOIN users mo ON mo.id = m.moved_out_by
	LEFT JOIN users rb ON rb.id = m.received_back_by
`

func scanPropertyMovement(row pgx.Row) (*models.PropertyMovement, error) {
	var m models.PropertyMovement
	err := row.Scan(&m.ID, &m.ItemID, &m.PropertyNumber, &m.MovementType, &m.Destination, &m.DestinationStationID,
		&m.DestinationStation, &m.CourtHearingID, &m.HearingDate, &m.Purpose, &m.AuthorityRef, &m.HandedTo,
		&m.ExpectedReturnAt, &m.MovedOutAt, &m.MovedOutByName, &m.SealNumberOut,
		&m.ReturnedAt, &m.ReceivedBackByName, &m.ReturnedByName, &m.SealNumberBack, &m.SealIntactBack, &m.ReturnNote,
		&m.Overdue)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MalkhanaRepository) movement(ctx context.Context, id uuid.UUID) (*models.PropertyMovement, error) {
	return scanPropertyMovement(r.db.QueryRow(ctx, propertyMovementSelect+" WHERE m.id = $1", id))
}

// Movement returns a movement only when it belongs to the item in the path.
func (r *MalkhanaRepository) Movement(ctx context.Context, itemID, movementID uuid.UUID) (*models.PropertyMovement, error) {
	m, err := scanPropertyMovement(r.db.QueryRow(ctx, propertyMovementSelect+" WHERE m.id = $1 AND m.item_id = $2", movementID, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPropertyMovementMissing
	}
	return m, err
}

func (r *MalkhanaRepository) Movements(ctx context.Context, itemID uuid.UUID) ([]models.PropertyMovement, error) {
	rows, err := r.db.Query(ctx, propertyMovementSelect+" WHERE m.item_id = $1 ORDER BY m.moved_out_at DESC", itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PropertyMovement{}
	for rows.Next() {
		m, err := scanPropertyMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// MoveOut opens a movement. The item is locked, so concurrent movements are
// serialised; the unique open-movement index and the table trigger hold the
// same rules if anything bypasses this path.
func (r *MalkhanaRepository) MoveOut(ctx context.Context, id uuid.UUID, req models.MoveOutRequest, actor uuid.UUID) (*models.PropertyMovement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	status, sealState, _, stationID, caseID, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	switch {
	case status == "DISPOSED":
		return nil, ErrPropertyDisposed
	case status == "MOVED_OUT":
		return nil, ErrPropertyOpenMovement
	case sealState != "INTACT":
		return nil, ErrPropertySealBroken
	}
	if req.CourtHearingID != nil {
		var hearingCase *uuid.UUID
		err := tx.QueryRow(ctx, "SELECT case_id FROM court_hearings WHERE id = $1", *req.CourtHearingID).Scan(&hearingCase)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (caseID == nil || hearingCase == nil || *hearingCase != *caseID)) {
			return nil, ErrPropertyHearingMismatch
		}
		if err != nil {
			return nil, err
		}
	}
	if req.DestinationStationID != nil && *req.DestinationStationID == stationID {
		return nil, fmt.Errorf("%w: an inter-station movement must go to a different station", ErrPropertyRecordMissing)
	}

	var sealNow string
	if err := tx.QueryRow(ctx, "SELECT seal_number FROM property_items WHERE id = $1", id).Scan(&sealNow); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(req.SealNumber), sealNow) {
		return nil, fmt.Errorf("%w: seal %s does not match the recorded seal %s", ErrPropertySealMismatch, strings.TrimSpace(req.SealNumber), sealNow)
	}

	mid := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_movements (id, item_id, movement_type, destination, destination_station_id, court_hearing_id,
			purpose, authority_ref, handed_to, expected_return_at, moved_out_by, seal_number_out)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		mid, id, req.MovementType, strings.TrimSpace(req.Destination), req.DestinationStationID, req.CourtHearingID,
		strings.TrimSpace(req.Purpose), strings.TrimSpace(req.AuthorityRef), strings.TrimSpace(req.HandedTo),
		*req.ExpectedReturnAt, actor, sealNow); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrPropertyOpenMovement
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE property_items SET status = 'MOVED_OUT', location_id = NULL, updated_at = NOW() WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_seal_checks (item_id, checked_by, seal_number, seal_state, context, note)
		VALUES ($1, $2, $3, 'INTACT', 'MOVEMENT_OUT', $4)`, id, actor, sealNow, "Handed to "+strings.TrimSpace(req.HandedTo)); err != nil {
		return nil, err
	}
	if err := addPropertyEvent(ctx, tx, id, "MOVED_OUT", &actor,
		fmt.Sprintf("Sent to %s for %s under %s", strings.TrimSpace(req.Destination), strings.TrimSpace(req.Purpose), strings.TrimSpace(req.AuthorityRef)),
		map[string]interface{}{"movementId": mid, "movementType": req.MovementType, "expectedReturnAt": req.ExpectedReturnAt}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.movement(ctx, mid)
}

var ErrPropertySealMismatch = errors.New("seal number does not match")

// ReturnMovement closes the open movement and puts the item back in a storage
// location. A seal that comes back broken, or under a different number, marks
// the item's seal broken and returns true.
func (r *MalkhanaRepository) ReturnMovement(ctx context.Context, id, movementID uuid.UUID, req models.ReturnMovementRequest, actor uuid.UUID) (*models.PropertyMovement, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	status, _, _, stationID, _, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	if status == "DISPOSED" {
		return nil, false, ErrPropertyDisposed
	}
	var sealOut string
	var returned *time.Time
	err = tx.QueryRow(ctx, "SELECT seal_number_out, returned_at FROM property_movements WHERE id = $1 AND item_id = $2 FOR UPDATE",
		movementID, id).Scan(&sealOut, &returned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrPropertyMovementMissing
	}
	if err != nil {
		return nil, false, err
	}
	if returned != nil || status != "MOVED_OUT" {
		return nil, false, ErrPropertyNoOpenMovement
	}
	if err := malkhanaLocationAtStation(ctx, tx, *req.LocationID, stationID); err != nil {
		return nil, false, err
	}
	sealBack := strings.TrimSpace(req.SealNumber)
	intact := *req.SealIntact && strings.EqualFold(sealBack, sealOut)
	note := req.Note
	if *req.SealIntact && !strings.EqualFold(sealBack, sealOut) {
		n := fmt.Sprintf("Returned under seal %s, sent out under %s", sealBack, sealOut)
		if note != nil && strings.TrimSpace(*note) != "" {
			n = strings.TrimSpace(*note) + " — " + n
		}
		note = &n
	}
	if _, err := tx.Exec(ctx, `
		UPDATE property_movements SET returned_at = NOW(), received_back_by = $3, returned_by_name = $4,
			seal_number_back = $5, seal_intact_back = $6, return_note = $7, return_location_id = $8
		WHERE id = $1 AND item_id = $2`,
		movementID, id, actor, strings.TrimSpace(req.ReturnedBy), sealBack, intact, note, *req.LocationID); err != nil {
		return nil, false, err
	}
	state := "INTACT"
	if !intact {
		state = "BROKEN"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE property_items SET status = 'IN_MALKHANA', location_id = $2, seal_number = $3,
			seal_state = $4::varchar, seal_broken_at = CASE WHEN $4::varchar = 'BROKEN' THEN NOW() ELSE NULL END, updated_at = NOW()
		WHERE id = $1`, id, *req.LocationID, sealBack, state); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO property_seal_checks (item_id, checked_by, seal_number, seal_state, context, note)
		VALUES ($1, $2, $3, $4, 'MOVEMENT_BACK', $5)`, id, actor, sealBack, state, note); err != nil {
		return nil, false, err
	}
	summary := "Returned to the malkhana under seal " + sealBack
	if !intact {
		summary += " — seal not intact"
	}
	if err := addPropertyEvent(ctx, tx, id, "RETURNED", &actor, summary,
		map[string]interface{}{"movementId": movementID, "sealIntact": intact, "returnedBy": req.ReturnedBy}); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	m, err := r.movement(ctx, movementID)
	return m, !intact, err
}

// Dispose closes the record against a court order for the same case.
func (r *MalkhanaRepository) Dispose(ctx context.Context, id uuid.UUID, req models.DisposeRequest, actor uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, _, category, _, caseID, err := lockPropertyItem(ctx, tx, id)
	if err != nil {
		return err
	}
	switch status {
	case "DISPOSED":
		return ErrPropertyDisposed
	case "MOVED_OUT":
		return fmt.Errorf("%w: return it before recording disposal", ErrPropertyNotInMalkhana)
	}
	var orderCase uuid.UUID
	var orderType, court string
	err = tx.QueryRow(ctx, "SELECT case_id, order_type, court FROM court_orders WHERE id = $1", *req.CourtOrderID).Scan(&orderCase, &orderType, &court)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (caseID == nil || orderCase != *caseID)) {
		return ErrPropertyOrderMismatch
	}
	if err != nil {
		return err
	}
	if category == "NARCOTICS" && req.DisposalType == "DESTROYED" {
		var role string
		err := tx.QueryRow(ctx, "SELECT role::text FROM users WHERE id = $1 AND is_active", *req.WitnessID).Scan(&role)
		if err != nil || !models.RoleAtLeast(models.Role(role), models.RoleDSP) {
			return ErrPropertyWitnessRank
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE property_items SET status = 'DISPOSED', location_id = NULL, disposal_type = $2, disposal_order_id = $3,
			disposal_witness_id = $4, disposal_note = $5, disposed_at = NOW(), disposed_by = $6, updated_at = NOW()
		WHERE id = $1`, id, req.DisposalType, *req.CourtOrderID, req.WitnessID, strings.TrimSpace(req.Note), actor); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return fmt.Errorf("%w: %s", ErrPropertyOrderMismatch, pgErr.Message)
		}
		return err
	}
	if err := addPropertyEvent(ctx, tx, id, "DISPOSED", &actor,
		fmt.Sprintf("Disposed (%s) under %s order of %s", req.DisposalType, orderType, court),
		map[string]interface{}{"courtOrderId": req.CourtOrderID, "witnessId": req.WitnessID, "note": req.Note}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ------------------------------------------------------------ history/checks

func (r *MalkhanaRepository) SealChecks(ctx context.Context, itemID uuid.UUID) ([]models.PropertySealCheck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT k.id, k.item_id, COALESCE(u.name, ''), k.checked_at, k.seal_number, k.seal_state, k.context, k.note
		FROM property_seal_checks k LEFT JOIN users u ON u.id = k.checked_by
		WHERE k.item_id = $1 ORDER BY k.checked_at DESC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PropertySealCheck{}
	for rows.Next() {
		var k models.PropertySealCheck
		if err := rows.Scan(&k.ID, &k.ItemID, &k.CheckedByName, &k.CheckedAt, &k.SealNumber, &k.SealState, &k.Context, &k.Note); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *MalkhanaRepository) Events(ctx context.Context, itemID uuid.UUID) ([]models.PropertyEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.event_type, COALESCE(u.name, 'System'), e.occurred_at, e.summary, e.details
		FROM property_events e LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.item_id = $1 ORDER BY e.occurred_at, e.id`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PropertyEvent{}
	for rows.Next() {
		var e models.PropertyEvent
		var raw []byte
		if err := rows.Scan(&e.ID, &e.EventType, &e.ActorName, &e.OccurredAt, &e.Summary, &raw); err != nil {
			return nil, err
		}
		e.Details = map[string]interface{}{}
		_ = json.Unmarshal(raw, &e.Details)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------- forwarding letter

// ForwardingLetter gathers every item sent to the same destination under the
// same authority reference on the same day as the given movement.
func (r *MalkhanaRepository) ForwardingLetter(ctx context.Context, itemID, movementID uuid.UUID) (*models.ForwardingLetter, error) {
	m, err := r.Movement(ctx, itemID, movementID)
	if err != nil {
		return nil, err
	}
	var l models.ForwardingLetter
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(s.name, ''), COALESCE(s.code, ''), COALESCE(c.case_number, ''), COALESCE(f.fir_number, ''),
		       COALESCE(u.name, ''), COALESCE(u.role::text, '')
		FROM property_movements m
		JOIN property_items p ON p.id = m.item_id
		LEFT JOIN stations s ON s.id = p.station_id
		LEFT JOIN cases c ON c.id = p.case_id
		LEFT JOIN firs f ON f.id = p.fir_id
		LEFT JOIN users u ON u.id = m.moved_out_by
		WHERE m.id = $1`, movementID).Scan(&l.StationName, &l.StationCode, &l.CaseNumber, &l.FIRNumber, &l.SignedByName, &l.SignedByRank)
	if err != nil {
		return nil, err
	}
	l.Reference, l.Date, l.Laboratory, l.Purpose, l.HandedTo = m.AuthorityRef, m.MovedOutAt, m.Destination, m.Purpose, m.HandedTo

	rows, err := r.db.Query(ctx, `
		SELECT p.property_number, p.description, p.category, p.quantity::float8, p.unit, p.weight_grams::float8,
		       m.seal_number_out, p.seized_at, p.seizure_memo_ref
		FROM property_movements m
		JOIN property_items p ON p.id = m.item_id
		WHERE m.authority_ref = $1 AND m.destination = $2 AND m.moved_out_at::date = $3::date
		  AND p.station_id = (SELECT station_id FROM property_items WHERE id = $4)
		ORDER BY p.property_number`, m.AuthorityRef, m.Destination, m.MovedOutAt, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	l.Items = []models.ForwardingLetterItem{}
	for rows.Next() {
		var it models.ForwardingLetterItem
		if err := rows.Scan(&it.PropertyNumber, &it.Description, &it.Category, &it.Quantity, &it.Unit, &it.WeightGrams,
			&it.SealNumber, &it.SeizedAt, &it.SeizureMemoRef); err != nil {
			return nil, err
		}
		l.Items = append(l.Items, it)
	}
	return &l, rows.Err()
}

// ------------------------------------------------------------------ dashboard

func (r *MalkhanaRepository) Dashboard(ctx context.Context, stationID *uuid.UUID) (*models.MalkhanaDashboard, error) {
	d := models.MalkhanaDashboard{ReviewPeriodDays: models.PropertyReviewPeriodDays}
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'IN_MALKHANA'),
		       COUNT(*) FILTER (WHERE status = 'MOVED_OUT'),
		       COUNT(*) FILTER (WHERE status = 'DISPOSED'),
		       COUNT(*) FILTER (WHERE seal_state = 'BROKEN' AND status <> 'DISPOSED'),
		       COUNT(*) FILTER (WHERE status <> 'DISPOSED' AND deposited_at < NOW() - make_interval(days => $2)),
		       COALESCE(SUM(value_paise) FILTER (WHERE status <> 'DISPOSED'), 0),
		       COUNT(*) FILTER (WHERE status <> 'DISPOSED' AND value_paise IS NULL),
		       (SELECT COUNT(*) FROM property_movements m JOIN property_items p2 ON p2.id = m.item_id
		        WHERE m.returned_at IS NULL AND m.expected_return_at < NOW() AND ($1::uuid IS NULL OR p2.station_id = $1))
		FROM property_items WHERE $1::uuid IS NULL OR station_id = $1`, stationID, models.PropertyReviewPeriodDays).
		Scan(&d.Total, &d.InMalkhana, &d.MovedOut, &d.Disposed, &d.SealBroken, &d.ReviewDue, &d.ValueHeldPaise, &d.ValueUnrecorded, &d.OverdueMovements)
	if err != nil {
		return nil, err
	}
	group := func(query string) ([]models.MalkhanaCount, error) {
		rows, err := r.db.Query(ctx, query, stationID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []models.MalkhanaCount{}
		for rows.Next() {
			var c models.MalkhanaCount
			if err := rows.Scan(&c.Key, &c.Label, &c.Count, &c.ValuePaise); err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
	if d.ByCategory, err = group(`
		SELECT category, category, COUNT(*), COALESCE(SUM(value_paise), 0)
		FROM property_items WHERE status <> 'DISPOSED' AND ($1::uuid IS NULL OR station_id = $1)
		GROUP BY category ORDER BY COUNT(*) DESC`); err != nil {
		return nil, err
	}
	if d.ByLocation, err = group(`
		SELECT l.id::text, COALESCE(s.code, '') || ' · ' || l.room || ' / ' || l.rack || CASE WHEN l.shelf <> '' THEN ' / ' || l.shelf ELSE '' END,
		       COUNT(p.id), COALESCE(SUM(p.value_paise), 0)
		FROM malkhana_locations l
		LEFT JOIN stations s ON s.id = l.station_id
		LEFT JOIN property_items p ON p.location_id = l.id AND p.status = 'IN_MALKHANA'
		WHERE $1::uuid IS NULL OR l.station_id = $1
		GROUP BY l.id, s.code, l.room, l.rack, l.shelf ORDER BY COUNT(p.id) DESC, 2`); err != nil {
		return nil, err
	}
	if d.ByMovementType, err = group(`
		SELECT m.movement_type, m.movement_type, COUNT(*), COALESCE(SUM(p.value_paise), 0)
		FROM property_movements m JOIN property_items p ON p.id = m.item_id
		WHERE m.returned_at IS NULL AND ($1::uuid IS NULL OR p.station_id = $1)
		GROUP BY m.movement_type ORDER BY COUNT(*) DESC`); err != nil {
		return nil, err
	}
	return &d, nil
}

// OfficerRole reports an active officer's rank for rule checks in the service.
func (r *MalkhanaRepository) OfficerRole(ctx context.Context, id uuid.UUID) (models.Role, bool, error) {
	var role string
	err := r.db.QueryRow(ctx, "SELECT role::text FROM users WHERE id = $1 AND is_active", id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return models.Role(role), err == nil, err
}

// Stations lists stations for choosing an inter-station destination.
func (r *MalkhanaRepository) Stations(ctx context.Context) ([]models.MalkhanaStation, error) {
	rows, err := r.db.Query(ctx, "SELECT id, name, COALESCE(code, '') FROM stations ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MalkhanaStation{}
	for rows.Next() {
		var s models.MalkhanaStation
		if err := rows.Scan(&s.ID, &s.Name, &s.Code); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
