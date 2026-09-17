package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SuretyRepository records who stands surety on a bail application.
//
// The `bail_sureties` table has existed since the bail module was built and no
// route ever reached it. A bail order that names no surety is an incomplete
// record of the order: the surety is the person the court holds answerable if
// the accused does not appear, and verifying them is a station's work.
type SuretyRepository struct {
	db *pgxpool.Pool
}

func NewSuretyRepository(db *pgxpool.Pool) *SuretyRepository {
	return &SuretyRepository{db: db}
}

var ErrSuretyNotFound = errors.New("no such surety")

type Surety struct {
	ID             uuid.UUID  `json:"id"`
	BailID         uuid.UUID  `json:"bailId"`
	Name           string     `json:"name"`
	Relation       *string    `json:"relation,omitempty"`
	Phone          *string    `json:"phone,omitempty"`
	Address        *string    `json:"address,omitempty"`
	IDType         *string    `json:"idType,omitempty"`
	IDNumber       *string    `json:"idNumber,omitempty"`
	Verified       bool       `json:"verified"`
	VerifiedBy     *uuid.UUID `json:"verifiedBy,omitempty"`
	VerifiedByName string     `json:"verifiedByName,omitempty"`
	VerifiedAt     *time.Time `json:"verifiedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

const suretyColumns = `
	s.id, s.bail_id, s.name, s.relation, s.phone, s.address,
	s.id_type, s.id_number, s.verified, s.verified_by,
	COALESCE(u.name, ''), s.verified_at, s.created_at`

func scanSurety(row pgx.Row) (*Surety, error) {
	var s Surety
	err := row.Scan(&s.ID, &s.BailID, &s.Name, &s.Relation, &s.Phone, &s.Address,
		&s.IDType, &s.IDNumber, &s.Verified, &s.VerifiedBy,
		&s.VerifiedByName, &s.VerifiedAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ForBail lists the sureties standing on one application.
func (r *SuretyRepository) ForBail(ctx context.Context, bailID uuid.UUID) ([]Surety, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+suretyColumns+`
		FROM bail_sureties s
		LEFT JOIN users u ON u.id = s.verified_by
		WHERE s.bail_id = $1
		ORDER BY s.created_at`, bailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Surety{}
	for rows.Next() {
		s, err := scanSurety(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *SuretyRepository) Add(ctx context.Context, s *Surety) (*Surety, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO bail_sureties
			(bail_id, name, relation, phone, address, id_type, id_number)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		s.BailID, s.Name, s.Relation, s.Phone, s.Address, s.IDType, s.IDNumber).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *SuretyRepository) Get(ctx context.Context, id uuid.UUID) (*Surety, error) {
	s, err := scanSurety(r.db.QueryRow(ctx, `
		SELECT `+suretyColumns+`
		FROM bail_sureties s
		LEFT JOIN users u ON u.id = s.verified_by
		WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSuretyNotFound
	}
	return s, err
}

// Verify records that an officer has checked the surety is who they say and
// stands good for the amount. Withdrawing verification clears the attribution,
// because an unverified surety was not verified by anybody.
func (r *SuretyRepository) Verify(ctx context.Context, id uuid.UUID, verified bool,
	by uuid.UUID) (*Surety, error) {
	var tag interface{ RowsAffected() int64 }
	var err error
	if verified {
		tag, err = r.db.Exec(ctx, `
			UPDATE bail_sureties SET verified = TRUE, verified_by = $2, verified_at = NOW()
			WHERE id = $1`, id, by)
	} else {
		tag, err = r.db.Exec(ctx, `
			UPDATE bail_sureties SET verified = FALSE, verified_by = NULL, verified_at = NULL
			WHERE id = $1`, id)
	}
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrSuretyNotFound
	}
	return r.Get(ctx, id)
}

func (r *SuretyRepository) Remove(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM bail_sureties WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSuretyNotFound
	}
	return nil
}

// BailOwner answers which department holds the application a surety stands on,
// so the boundary covers sureties through the bail record rather than needing
// a placement of its own.
func (r *SuretyRepository) BailOwner(ctx context.Context, bailID, viewerID uuid.UUID) (bool, string, error) {
	return RecordOwner(ctx, r.db, "BAIL", bailID, viewerID)
}
