package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.name, u.role,
		       u.badge_number, u.station_id, u.phone, u.is_active, u.last_login,
		       u.created_at, u.updated_at,
		       COALESCE(s.name, '') as station_name,
		       s.district_id, COALESCE(d.name, '') as district_name,
		       s.state_id, COALESCE(st.name, '') as state_name,
		       u.force_id, COALESCE(f.code, ''), COALESCE(f.name, ''), COALESCE(f.name_bn, ''),
		       COALESCE(f.short_name, ''), COALESCE(f.kind, ''), COALESCE(pf.code, ''),
		       COALESCE(pf.name, ''), COALESCE(f.headquarters, ''), COALESCE(f.remit, '')
		FROM users u
		LEFT JOIN stations s ON u.station_id = s.id
		LEFT JOIN districts d ON s.district_id = d.id
		LEFT JOIN states st ON s.state_id = st.id
		LEFT JOIN forces f ON u.force_id = f.id
		LEFT JOIN forces pf ON f.parent_id = pf.id
		WHERE u.username = $1 AND u.is_active = true
	`

	var user models.User
	var force models.Force
	err := r.db.QueryRow(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Name,
		&user.Role, &user.BadgeNumber, &user.StationID, &user.Phone, &user.IsActive,
		&user.LastLogin, &user.CreatedAt, &user.UpdatedAt, &user.StationName,
		&user.DistrictID, &user.DistrictName, &user.StateID, &user.StateName,
		&user.ForceID, &force.Code, &force.Name, &force.NameBn, &force.ShortName,
		&force.Kind, &force.ParentCode, &force.ParentName, &force.Headquarters, &force.Remit,
	)
	if err != nil {
		return nil, err
	}
	attachForce(&user, force)

	return &user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.name, u.role,
		       u.badge_number, u.station_id, u.phone, u.is_active, u.last_login,
		       u.created_at, u.updated_at,
		       COALESCE(s.name, '') as station_name,
		       s.district_id, COALESCE(d.name, '') as district_name,
		       s.state_id, COALESCE(st.name, '') as state_name,
		       u.force_id, COALESCE(f.code, ''), COALESCE(f.name, ''), COALESCE(f.name_bn, ''),
		       COALESCE(f.short_name, ''), COALESCE(f.kind, ''), COALESCE(pf.code, ''),
		       COALESCE(pf.name, ''), COALESCE(f.headquarters, ''), COALESCE(f.remit, '')
		FROM users u
		LEFT JOIN stations s ON u.station_id = s.id
		LEFT JOIN districts d ON s.district_id = d.id
		LEFT JOIN states st ON s.state_id = st.id
		LEFT JOIN forces f ON u.force_id = f.id
		LEFT JOIN forces pf ON f.parent_id = pf.id
		WHERE u.id = $1
	`

	var user models.User
	var force models.Force
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Name,
		&user.Role, &user.BadgeNumber, &user.StationID, &user.Phone, &user.IsActive,
		&user.LastLogin, &user.CreatedAt, &user.UpdatedAt, &user.StationName,
		&user.DistrictID, &user.DistrictName, &user.StateID, &user.StateName,
		&user.ForceID, &force.Code, &force.Name, &force.NameBn, &force.ShortName,
		&force.Kind, &force.ParentCode, &force.ParentName, &force.Headquarters, &force.Remit,
	)
	if err != nil {
		return nil, err
	}
	attachForce(&user, force)

	return &user, nil
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET last_login = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), id)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, passwordHash, id)
	return err
}

func (r *UserRepository) UpdateProfile(ctx context.Context, id uuid.UUID, name, email, phone string) error {
	query := `UPDATE users SET name = $2, email = $3, phone = $4, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id, name, email, phone)
	return err
}

func (r *UserRepository) GetUserWithStation(ctx context.Context, id uuid.UUID) (*models.User, string, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.name, u.role,
		       u.badge_number, u.station_id, u.phone, u.is_active, u.last_login,
		       u.created_at, u.updated_at, COALESCE(s.name, '') as station_name,
		       COALESCE(s.code, 'UNK') as station_code
		FROM users u
		LEFT JOIN stations s ON u.station_id = s.id
		WHERE u.id = $1
	`

	var user models.User
	var stationCode string
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Name,
		&user.Role, &user.BadgeNumber, &user.StationID, &user.Phone, &user.IsActive,
		&user.LastLogin, &user.CreatedAt, &user.UpdatedAt, &user.StationName, &stationCode,
	)
	if err != nil {
		return nil, "", err
	}

	return &user, stationCode, nil
}

// attachForce hangs the officer's department off the user, where there is one.
// An account with no force is one created before the departments existed; it
// is treated as Kolkata Police by the migration that added them, so an empty
// force here means the row was not read rather than that the officer has none.
func attachForce(user *models.User, force models.Force) {
	if user.ForceID == nil || force.Code == "" {
		return
	}
	force.ID = *user.ForceID
	user.Force = &force
}
