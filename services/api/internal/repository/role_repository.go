package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RoleRepository reads and writes roles, what they grant, and who holds them.
//
// The rules about what may be changed are the database's — a rank role cannot
// be renamed, deleted, or have its grants edited, and those are triggers from
// migration 000088. This layer turns the database's refusal into an error the
// handler can put in front of an administrator.
type RoleRepository struct {
	db *pgxpool.Pool
}

func NewRoleRepository(db *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{db: db}
}

var ErrRoleNotFound = errors.New("no such role")

type Role struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	NameBn        *string    `json:"nameBn,omitempty"`
	Description   string     `json:"description"`
	ForceID       *uuid.UUID `json:"forceId,omitempty"`
	ForceName     *string    `json:"forceName,omitempty"`
	IsRankDefault bool       `json:"isRankDefault"`
	Rank          *string    `json:"rank,omitempty"`
	Permissions   []string   `json:"permissions,omitempty"`
	GrantCount    int        `json:"grantCount"`
	HolderCount   int        `json:"holderCount"`
}

type Permission struct {
	Key            string   `json:"key"`
	Module         string   `json:"module"`
	Action         string   `json:"action"`
	Description    string   `json:"description"`
	DefaultMinRank *string  `json:"defaultMinRank,omitempty"`
	Routes         []string `json:"routes"`
}

// Catalogue lists every capability the API can be asked for, in the order an
// administrator reads them: by module, then by name.
func (r *RoleRepository) Catalogue(ctx context.Context) ([]Permission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT key, module, action, description, default_min_rank::TEXT, routes
		FROM permissions ORDER BY module, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Permission{}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Key, &p.Module, &p.Action, &p.Description,
			&p.DefaultMinRank, &p.Routes); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const roleColumns = `
	ro.id, ro.code, ro.name, ro.name_bn, ro.description, ro.force_id,
	f.name, ro.is_rank_default, ro.rank::TEXT,
	(SELECT COUNT(*) FROM role_permissions rp WHERE rp.role_id = ro.id),
	(SELECT COUNT(*) FROM user_roles ur WHERE ur.role_id = ro.id)`

func scanRole(row pgx.Row) (*Role, error) {
	var ro Role
	err := row.Scan(&ro.ID, &ro.Code, &ro.Name, &ro.NameBn, &ro.Description,
		&ro.ForceID, &ro.ForceName, &ro.IsRankDefault, &ro.Rank,
		&ro.GrantCount, &ro.HolderCount)
	if err != nil {
		return nil, err
	}
	return &ro, nil
}

// List returns the roles, rank roles first in seniority order and then the
// roles an administrator made, alphabetically.
func (r *RoleRepository) List(ctx context.Context) ([]Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+roleColumns+`
		FROM roles ro
		LEFT JOIN forces f ON f.id = ro.force_id
		ORDER BY ro.is_rank_default DESC,
		         CASE WHEN ro.is_rank_default THEN rank_level(ro.rank) END,
		         ro.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Role{}
	for rows.Next() {
		ro, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ro)
	}
	return out, rows.Err()
}

// Get returns one role and everything it grants.
func (r *RoleRepository) Get(ctx context.Context, id uuid.UUID) (*Role, error) {
	ro, err := scanRole(r.db.QueryRow(ctx, `
		SELECT `+roleColumns+`
		FROM roles ro LEFT JOIN forces f ON f.id = ro.force_id
		WHERE ro.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT permission_key FROM role_permissions WHERE role_id = $1 ORDER BY permission_key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ro.Permissions = []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		ro.Permissions = append(ro.Permissions, key)
	}
	return ro, rows.Err()
}

func (r *RoleRepository) Create(ctx context.Context, code, name string, nameBn *string,
	description string, forceID *uuid.UUID, createdBy uuid.UUID) (*Role, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO roles (code, name, name_bn, description, force_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		code, name, nameBn, description, forceID, createdBy).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *RoleRepository) Update(ctx context.Context, id uuid.UUID, name string,
	nameBn *string, description string) (*Role, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE roles SET name = $2, name_bn = $3, description = $4, updated_at = NOW()
		WHERE id = $1`, id, name, nameBn, description)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrRoleNotFound
	}
	return r.Get(ctx, id)
}

func (r *RoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// SetPermissions replaces what a role grants, in one transaction, so a role is
// never briefly holding half of its permissions while an administrator's save
// is in flight.
func (r *RoleRepository) SetPermissions(ctx context.Context, id uuid.UUID,
	keys []string, grantedBy uuid.UUID) (*Role, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM roles WHERE id = $1`, id).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, id); err != nil {
		return nil, err
	}
	if len(keys) > 0 {
		// A key that is not in the catalogue fails on the foreign key rather
		// than being stored as a permission nothing will ever check.
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_key, granted_by)
			SELECT $1, k, $3 FROM unnest($2::TEXT[]) k`, id, keys, grantedBy); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

// RolesOf lists the roles an officer holds.
func (r *RoleRepository) RolesOf(ctx context.Context, userID uuid.UUID) ([]Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+roleColumns+`
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id
		LEFT JOIN forces f ON f.id = ro.force_id
		WHERE ur.user_id = $1
		ORDER BY ro.is_rank_default DESC, ro.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Role{}
	for rows.Next() {
		ro, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ro)
	}
	return out, rows.Err()
}

func (r *RoleRepository) Assign(ctx context.Context, userID, roleID, assignedBy uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, assigned_by)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, userID, roleID, assignedBy)
	return err
}

func (r *RoleRepository) Unassign(ctx context.Context, userID, roleID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, userID, roleID)
	return err
}

// IsRankDefault says whether a role mirrors a rank, so the service can refuse
// before the database does and give a better sentence than a trigger can.
func (r *RoleRepository) IsRankDefault(ctx context.Context, id uuid.UUID) (bool, string, error) {
	var isDefault bool
	var name string
	err := r.db.QueryRow(ctx,
		`SELECT is_rank_default, name FROM roles WHERE id = $1`, id).Scan(&isDefault, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", ErrRoleNotFound
	}
	return isDefault, name, err
}
