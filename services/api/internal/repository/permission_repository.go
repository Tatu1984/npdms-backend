package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/authz"
)

// PermissionRepository answers what an officer may do.
//
// The answer comes from the database on every request, not from a claim in the
// token. That is the whole point of it: a permission taken away from a role
// has to stop working now, not in up to an hour when the token expires. An
// inspection asks "when did that officer lose access", and "at some point
// within the hour" is not an answer.
//
// A short cache keeps that from meaning a query per request per screen — a
// dashboard issues about five — while keeping revocation prompt. The window is
// deliberately small, and any change made through the roles service clears the
// affected officer immediately, so the cache only ever covers changes made
// behind the service's back, directly in SQL.
type PermissionRepository struct {
	db *pgxpool.Pool

	mu     sync.RWMutex
	cache  map[uuid.UUID]cachedPermissions
	window time.Duration
}

type cachedPermissions struct {
	perms authz.Permissions
	until time.Time
}

// CacheWindow is how long a resolved permission set is reused.
const CacheWindow = 5 * time.Second

func NewPermissionRepository(db *pgxpool.Pool) *PermissionRepository {
	return &PermissionRepository{
		db:     db,
		cache:  make(map[uuid.UUID]cachedPermissions),
		window: CacheWindow,
	}
}

// ForUser resolves everything this officer may do.
func (r *PermissionRepository) ForUser(ctx context.Context, userID uuid.UUID) (authz.Permissions, error) {
	r.mu.RLock()
	hit, ok := r.cache[userID]
	r.mu.RUnlock()
	if ok && time.Now().Before(hit.until) {
		return hit.perms, nil
	}

	rows, err := r.db.Query(ctx,
		`SELECT permission_key FROM user_permissions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	perms := make(authz.Permissions)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		perms[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[userID] = cachedPermissions{perms: perms, until: time.Now().Add(r.window)}
	r.mu.Unlock()
	return perms, nil
}

// Forget drops an officer's cached permissions. Called whenever a role is
// changed, granted or withdrawn, so a deliberate change takes effect on the
// very next request rather than at the end of the cache window.
func (r *PermissionRepository) Forget(userID uuid.UUID) {
	r.mu.Lock()
	delete(r.cache, userID)
	r.mu.Unlock()
}

// ForgetAll drops every cached set. Called when a role's permissions change,
// since that affects every officer holding it and the holders are not known
// here without another query.
func (r *PermissionRepository) ForgetAll() {
	r.mu.Lock()
	r.cache = make(map[uuid.UUID]cachedPermissions)
	r.mu.Unlock()
}
