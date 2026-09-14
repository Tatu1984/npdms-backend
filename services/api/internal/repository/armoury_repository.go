package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrWeaponNotFound     = errors.New("weapon not found")
	ErrWeaponNotAvailable = errors.New("weapon is not in the armoury")
	ErrWeaponNotIssued    = errors.New("weapon is not issued")
	ErrDuplicateSerial    = errors.New("a weapon with this serial number is already registered")
	ErrOfficerNotFound    = errors.New("officer not found")
)

type ArmouryRepository struct {
	db *pgxpool.Pool
}

func NewArmouryRepository(db *pgxpool.Pool) *ArmouryRepository {
	return &ArmouryRepository{db: db}
}

const issuanceSelect = `
	SELECT i.id, i.weapon_id, w.weapon_number,
	       i.issued_to, COALESCE(ut.name, ''), COALESCE(ut.badge_number, ''),
	       i.issued_by, COALESCE(ub.name, ''),
	       i.purpose, i.rounds_issued, i.expected_return, i.issued_at,
	       i.returned_at, i.received_by, COALESCE(ur.name, ''),
	       i.rounds_returned, i.return_condition, i.return_note,
	       (i.returned_at IS NULL AND i.expected_return IS NOT NULL AND i.expected_return < NOW())
	FROM weapon_issuances i
	JOIN weapons w ON w.id = i.weapon_id
	LEFT JOIN users ut ON ut.id = i.issued_to
	LEFT JOIN users ub ON ub.id = i.issued_by
	LEFT JOIN users ur ON ur.id = i.received_by
`

func scanIssuance(row pgx.Row) (*models.WeaponIssuance, error) {
	var i models.WeaponIssuance
	err := row.Scan(
		&i.ID, &i.WeaponID, &i.WeaponNumber,
		&i.IssuedTo, &i.IssuedToName, &i.IssuedToBadge,
		&i.IssuedBy, &i.IssuedByName,
		&i.Purpose, &i.RoundsIssued, &i.ExpectedReturn, &i.IssuedAt,
		&i.ReturnedAt, &i.ReceivedBy, &i.ReceivedByName,
		&i.RoundsReturned, &i.ReturnCondition, &i.ReturnNote,
		&i.Overdue,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

const weaponSelect = `
	SELECT w.id, w.weapon_number, w.weapon_type, w.make, w.serial_number,
	       w.station_id, COALESCE(s.name, ''), w.status, w.condition, w.maintenance_note,
	       w.created_at, w.updated_at,
	       open.id
	FROM weapons w
	LEFT JOIN stations s ON s.id = w.station_id
	LEFT JOIN weapon_issuances open ON open.weapon_id = w.id AND open.returned_at IS NULL
`

func (r *ArmouryRepository) scanWeapon(ctx context.Context, row pgx.Row) (*models.Weapon, error) {
	var w models.Weapon
	var openIssue *uuid.UUID
	err := row.Scan(
		&w.ID, &w.WeaponNumber, &w.Type, &w.Make, &w.SerialNumber,
		&w.StationID, &w.StationName, &w.Status, &w.Condition, &w.MaintenanceNote,
		&w.CreatedAt, &w.UpdatedAt, &openIssue,
	)
	if err != nil {
		return nil, err
	}
	if openIssue != nil {
		if w.CurrentIssue, err = r.issuance(ctx, *openIssue); err != nil {
			return nil, err
		}
	}
	return &w, nil
}

func (r *ArmouryRepository) issuance(ctx context.Context, id uuid.UUID) (*models.WeaponIssuance, error) {
	return scanIssuance(r.db.QueryRow(ctx, issuanceSelect+" WHERE i.id = $1", id))
}

type WeaponFilter struct {
	Search    string
	Status    string
	StationID *uuid.UUID
	Overdue   bool
	Page      int
	PageSize  int
}

func (r *ArmouryRepository) ListWeapons(ctx context.Context, f WeaponFilter) ([]models.Weapon, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(w.weapon_number ILIKE $%d OR w.serial_number ILIKE $%d OR w.make ILIKE $%d OR w.weapon_type ILIKE $%d)", n, n, n, n))
	}
	if f.Status != "" {
		add("w.status = $%d", f.Status)
	}
	if f.StationID != nil {
		add("w.station_id = $%d", *f.StationID)
	}
	if f.Overdue {
		where = append(where, "open.expected_return < NOW()")
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM weapons w
		LEFT JOIN weapon_issuances open ON open.weapon_id = w.id AND open.returned_at IS NULL
		WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, weaponSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY w.weapon_number LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	// Collect ids first: nested queries cannot run while rows is open on the same connection.
	type pending struct {
		w    models.Weapon
		open *uuid.UUID
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(
			&p.w.ID, &p.w.WeaponNumber, &p.w.Type, &p.w.Make, &p.w.SerialNumber,
			&p.w.StationID, &p.w.StationName, &p.w.Status, &p.w.Condition, &p.w.MaintenanceNote,
			&p.w.CreatedAt, &p.w.UpdatedAt, &p.open,
		); err != nil {
			rows.Close()
			return nil, 0, err
		}
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	weapons := make([]models.Weapon, 0, len(list))
	for _, p := range list {
		if p.open != nil {
			if p.w.CurrentIssue, err = r.issuance(ctx, *p.open); err != nil {
				return nil, 0, err
			}
		}
		weapons = append(weapons, p.w)
	}
	return weapons, total, nil
}

func (r *ArmouryRepository) GetWeapon(ctx context.Context, id uuid.UUID) (*models.Weapon, error) {
	w, err := r.scanWeapon(ctx, r.db.QueryRow(ctx, weaponSelect+" WHERE w.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWeaponNotFound
	}
	return w, err
}

func (r *ArmouryRepository) CreateWeapon(ctx context.Context, req models.CreateWeaponRequest, stationID uuid.UUID, actor *uuid.UUID) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "WPN")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	_, err = r.db.Exec(ctx, `
		INSERT INTO weapons (id, weapon_number, weapon_type, make, serial_number, station_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, number, strings.TrimSpace(req.Type), strings.TrimSpace(req.Make),
		strings.TrimSpace(req.SerialNumber), stationID, actor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return uuid.Nil, ErrDuplicateSerial
	}
	return id, err
}

// SetState changes status and condition for a weapon that is not issued.
func (r *ArmouryRepository) SetState(ctx context.Context, id uuid.UUID, req models.SetWeaponStateRequest) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE weapons SET status = $2, condition = $3, maintenance_note = $4, updated_at = NOW()
		WHERE id = $1 AND status <> 'ISSUED'
	`, id, req.Status, req.Condition, req.MaintenanceNote)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetWeapon(ctx, id); err != nil {
			return err
		}
		return ErrWeaponNotAvailable
	}
	return nil
}

// Issue opens an issuance and marks the weapon issued in one transaction. The
// status guard and the unique open-issuance index both stop a double issue.
func (r *ArmouryRepository) Issue(ctx context.Context, weaponID uuid.UUID, req models.IssueWeaponRequest, issuedBy uuid.UUID) (*models.WeaponIssuance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var active bool
	if err := tx.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", req.IssuedTo).Scan(&active); err != nil || !active {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOfficerNotFound
		}
		return nil, err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE weapons SET status = 'ISSUED', updated_at = NOW()
		WHERE id = $1 AND status = 'IN_ARMOURY' AND condition = 'SERVICEABLE'
	`, weaponID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetWeapon(ctx, weaponID); err != nil {
			return nil, err
		}
		return nil, ErrWeaponNotAvailable
	}

	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO weapon_issuances (id, weapon_id, issued_to, issued_by, purpose, rounds_issued, expected_return)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, weaponID, req.IssuedTo, issuedBy, strings.TrimSpace(req.Purpose), req.RoundsIssued, req.ExpectedReturn); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrWeaponNotAvailable
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.issuance(ctx, id)
}

// Return closes the open issuance and puts the weapon back, or into
// maintenance when it comes back unserviceable or needing repair.
func (r *ArmouryRepository) Return(ctx context.Context, weaponID uuid.UUID, req models.ReturnWeaponRequest, receivedBy uuid.UUID) (*models.WeaponIssuance, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var issueID uuid.UUID
	var roundsIssued int
	err = tx.QueryRow(ctx, `
		SELECT id, rounds_issued FROM weapon_issuances
		WHERE weapon_id = $1 AND returned_at IS NULL FOR UPDATE
	`, weaponID).Scan(&issueID, &roundsIssued)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := r.GetWeapon(ctx, weaponID); err != nil {
			return nil, err
		}
		return nil, ErrWeaponNotIssued
	}
	if err != nil {
		return nil, err
	}
	if req.RoundsReturned > roundsIssued {
		return nil, fmt.Errorf("%w: %d rounds returned but %d were issued", ErrInvalidReturn, req.RoundsReturned, roundsIssued)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE weapon_issuances
		SET returned_at = NOW(), received_by = $2, rounds_returned = $3, return_condition = $4, return_note = $5
		WHERE id = $1
	`, issueID, receivedBy, req.RoundsReturned, req.Condition, req.Note); err != nil {
		return nil, err
	}

	status := models.WeaponInArmoury
	if req.Condition != models.WeaponServiceable {
		status = models.WeaponMaintenance
	}
	if _, err := tx.Exec(ctx, `
		UPDATE weapons SET status = $2::varchar, condition = $3,
		       maintenance_note = CASE WHEN $2::varchar = 'MAINTENANCE' THEN $4 ELSE maintenance_note END,
		       updated_at = NOW()
		WHERE id = $1
	`, weaponID, status, req.Condition, req.Note); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.issuance(ctx, issueID)
}

var ErrInvalidReturn = errors.New("invalid return")

type IssuanceFilter struct {
	WeaponID  *uuid.UUID
	OfficerID *uuid.UUID
	OpenOnly  bool
	Page      int
	PageSize  int
}

func (r *ArmouryRepository) ListIssuances(ctx context.Context, f IssuanceFilter) ([]models.WeaponIssuance, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if f.WeaponID != nil {
		args = append(args, *f.WeaponID)
		where = append(where, fmt.Sprintf("i.weapon_id = $%d", len(args)))
	}
	if f.OfficerID != nil {
		args = append(args, *f.OfficerID)
		where = append(where, fmt.Sprintf("i.issued_to = $%d", len(args)))
	}
	if f.OpenOnly {
		where = append(where, "i.returned_at IS NULL")
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM weapon_issuances i WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, issuanceSelect+" WHERE "+clause+
		fmt.Sprintf(" ORDER BY COALESCE(i.returned_at, i.issued_at) DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.WeaponIssuance{}
	for rows.Next() {
		i, err := scanIssuance(rows)
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

func (r *ArmouryRepository) Stats(ctx context.Context, stationID *uuid.UUID) (*models.WeaponStats, error) {
	var s models.WeaponStats
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE w.status = 'IN_ARMOURY'),
			COUNT(*) FILTER (WHERE w.status = 'ISSUED'),
			COUNT(*) FILTER (WHERE w.status = 'MAINTENANCE'),
			COUNT(*) FILTER (WHERE w.status = 'CONDEMNED'),
			COUNT(*) FILTER (WHERE open.expected_return < NOW()),
			COALESCE(SUM(open.rounds_issued), 0),
			COALESCE((SELECT SUM(i.rounds_issued - i.rounds_returned) FROM weapon_issuances i
			          JOIN weapons w2 ON w2.id = i.weapon_id
			          WHERE i.returned_at IS NOT NULL AND ($1::uuid IS NULL OR w2.station_id = $1)), 0)
		FROM weapons w
		LEFT JOIN weapon_issuances open ON open.weapon_id = w.id AND open.returned_at IS NULL
		WHERE $1::uuid IS NULL OR w.station_id = $1
	`, stationID).Scan(&s.Total, &s.InArmoury, &s.Issued, &s.Maintenance, &s.Condemned,
		&s.Overdue, &s.RoundsOut, &s.RoundsShortfall)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
