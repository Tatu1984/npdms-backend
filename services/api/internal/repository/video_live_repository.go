package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/npdms/api/internal/models"
)

// Live CCTV streaming (migration 000076): the ingest credential on the camera
// register, and purpose-logged live viewing sessions.

var (
	ErrStreamingAlreadyEnabled = errors.New("live streaming is already enabled for this camera; rotate the token to issue new Edge Agent settings")
	ErrStreamingNotEnabled     = errors.New("live streaming is not enabled for this camera")
	ErrLiveSessionNotFound     = errors.New("live viewing session not found")
)

// streamingRefusal explains why a streaming change touched no row.
func (r *VideoRepository) streamingRefusal(ctx context.Context, id uuid.UUID, wantEnabled bool) error {
	var status string
	var enabled bool
	err := r.db.QueryRow(ctx, "SELECT status, ingest_enabled FROM cameras WHERE id = $1", id).Scan(&status, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCameraNotFound
	}
	if err != nil {
		return err
	}
	if status != "ACTIVE" {
		return ErrCameraDecommissioned
	}
	if wantEnabled && !enabled {
		return ErrStreamingNotEnabled
	}
	if !wantEnabled && enabled {
		return ErrStreamingAlreadyEnabled
	}
	return ErrStreamingNotEnabled
}

// EnableIngest turns streaming on with a token hash. A camera keeps the ingest
// key it had before, if any, so re-enabling does not change its Camera ID.
func (r *VideoRepository) EnableIngest(ctx context.Context, id uuid.UUID, newKey, tokenHash string, actor *uuid.UUID) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, `
		UPDATE cameras SET ingest_key = COALESCE(ingest_key, $2), ingest_token_hash = $3,
		       ingest_enabled = TRUE, ingest_enabled_at = NOW(), ingest_enabled_by = $4,
		       ingest_token_rotated_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE' AND NOT ingest_enabled
		RETURNING ingest_key
	`, id, newKey, tokenHash, actor).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", r.streamingRefusal(ctx, id, false)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// A random 96-bit key collided; the caller retries with a fresh one.
		return "", errKeyCollision
	}
	return key, err
}

var errKeyCollision = errors.New("ingest key collision")

// IsIngestKeyCollision reports the one error EnableIngest's caller retries.
func IsIngestKeyCollision(err error) bool { return errors.Is(err, errKeyCollision) }

// RotateIngestToken replaces the token hash; the old token stops working at once.
func (r *VideoRepository) RotateIngestToken(ctx context.Context, id uuid.UUID, tokenHash string) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, `
		UPDATE cameras SET ingest_token_hash = $2, ingest_token_rotated_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE' AND ingest_enabled
		RETURNING ingest_key
	`, id, tokenHash).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", r.streamingRefusal(ctx, id, true)
	}
	return key, err
}

// DisableIngest turns streaming off and revokes the token. It returns the key
// so the stored playlist can be purged, and ends open viewing sessions.
func (r *VideoRepository) DisableIngest(ctx context.Context, id uuid.UUID) (string, error) {
	var key string
	err := r.db.QueryRow(ctx, `
		UPDATE cameras SET ingest_enabled = FALSE, ingest_token_hash = NULL, updated_at = NOW()
		WHERE id = $1 AND ingest_enabled
		RETURNING ingest_key
	`, id).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		if refusal := r.streamingRefusal(ctx, id, true); errors.Is(refusal, ErrCameraNotFound) {
			return "", refusal
		}
		return "", ErrStreamingNotEnabled
	}
	return key, err
}

// IngestCredential returns the camera behind an ingest key that currently
// accepts uploads, with its token hash. Not found covers an unknown key, a
// disabled stream and a decommissioned camera alike — the agent is told only
// that it is unauthorised.
func (r *VideoRepository) IngestCredential(ctx context.Context, key string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var hash string
	err := r.db.QueryRow(ctx, `
		SELECT id, ingest_token_hash FROM cameras
		WHERE ingest_key = $1 AND ingest_enabled AND status = 'ACTIVE'
	`, key).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrCameraNotFound
	}
	return id, hash, err
}

// TouchSegment records that a segment arrived, at most once every 10 seconds
// per camera, so a camera uploading every two seconds does not rewrite its row
// every two seconds.
func (r *VideoRepository) TouchSegment(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE cameras SET last_segment_at = NOW()
		WHERE id = $1 AND (last_segment_at IS NULL OR last_segment_at < NOW() - INTERVAL '10 seconds')
	`, id)
	return err
}

// PlaybackGrant is what the playback route needs to decide one request.
type PlaybackGrant struct {
	CameraID        uuid.UUID
	MaskingRequired bool
	ViewerRole      models.Role // empty when the viewer is unknown or inactive
	SessionID       *uuid.UUID  // an open session covering this camera, if any
	LastPlayedAt    *time.Time
}

// PlaybackGrant resolves an ingest key (streaming enabled, camera active) and
// the viewer's open session for it, in one round trip.
func (r *VideoRepository) PlaybackGrant(ctx context.Context, key string, viewer uuid.UUID) (*PlaybackGrant, error) {
	var g PlaybackGrant
	var role *string
	err := r.db.QueryRow(ctx, `
		SELECT c.id, c.masking_required,
		       (SELECT u.role FROM users u WHERE u.id = $2 AND u.is_active),
		       s.id, s.last_played_at
		FROM cameras c
		LEFT JOIN LATERAL (
		    SELECT ls.id, ls.last_played_at
		    FROM live_view_sessions ls
		    JOIN live_view_session_cameras sc ON sc.session_id = ls.id
		    WHERE sc.camera_id = c.id AND ls.viewer_id = $2
		      AND ls.ended_at IS NULL AND ls.expires_at > NOW()
		    ORDER BY ls.expires_at DESC
		    LIMIT 1
		) s ON TRUE
		WHERE c.ingest_key = $1 AND c.ingest_enabled AND c.status = 'ACTIVE'
	`, key, viewer).Scan(&g.CameraID, &g.MaskingRequired, &role, &g.SessionID, &g.LastPlayedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCameraNotFound
	}
	if err != nil {
		return nil, err
	}
	if role != nil {
		g.ViewerRole = models.Role(*role)
	}
	return &g, nil
}

// ExtendLiveSession slides a session's expiry forward while video is played,
// never past its hard limit. Called on playlist reads; written at most every
// 30 seconds per session.
func (r *VideoRepository) ExtendLiveSession(ctx context.Context, id uuid.UUID, idle time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE live_view_sessions
		SET expires_at = LEAST(max_until, NOW() + make_interval(secs => $2)), last_played_at = NOW()
		WHERE id = $1 AND ended_at IS NULL
		  AND (last_played_at IS NULL OR last_played_at < NOW() - INTERVAL '30 seconds')
	`, id, idle.Seconds())
	return err
}

// LiveCameraCheck is what starting a viewing session needs to know per camera.
type LiveCameraCheck struct {
	ID               uuid.UUID
	Code             string
	Status           string
	StreamingEnabled bool
	MaskingRequired  bool
}

func (r *VideoRepository) LiveCameraChecks(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]LiveCameraCheck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, status, ingest_enabled, masking_required FROM cameras WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]LiveCameraCheck{}
	for rows.Next() {
		var c LiveCameraCheck
		if err := rows.Scan(&c.ID, &c.Code, &c.Status, &c.StreamingEnabled, &c.MaskingRequired); err != nil {
			return nil, err
		}
		out[c.ID] = c
	}
	return out, rows.Err()
}

// CreateLiveSession writes the purpose to the append-only access log and opens
// the session in one transaction: no session exists without its log entry.
func (r *VideoRepository) CreateLiveSession(ctx context.Context, viewer uuid.UUID, purpose string, cameraIDs []uuid.UUID,
	codes []string, ip string, idle, max time.Duration) (*models.LiveViewSession, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	filters, err := json.Marshal(map[string]interface{}{"cameraCodes": codes})
	if err != nil {
		return nil, err
	}
	var logID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO video_access_log (actor_user_id, access_type, purpose, filters, camera_ids, result_count, ip_address)
		VALUES ($1, 'VIEW_LIVE', $2, $3, $4, $5, NULLIF($6, '')::inet)
		RETURNING id
	`, viewer, purpose, filters, cameraIDs, len(cameraIDs), ip).Scan(&logID); err != nil {
		return nil, err
	}

	s := models.LiveViewSession{ID: uuid.New(), CameraIDs: cameraIDs, Purpose: purpose}
	if err := tx.QueryRow(ctx, `
		INSERT INTO live_view_sessions (id, viewer_id, access_log_id, expires_at, max_until)
		VALUES ($1, $2, $3, NOW() + make_interval(secs => $4), NOW() + make_interval(secs => $5))
		RETURNING started_at, expires_at, max_until
	`, s.ID, viewer, logID, idle.Seconds(), max.Seconds()).Scan(&s.StartedAt, &s.ExpiresAt, &s.MaxUntil); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO live_view_session_cameras (session_id, camera_id)
		SELECT $1, unnest($2::uuid[])
	`, s.ID, cameraIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s, nil
}

// EndLiveSession closes the viewer's own session.
func (r *VideoRepository) EndLiveSession(ctx context.Context, id, viewer uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE live_view_sessions SET ended_at = NOW(), expires_at = LEAST(expires_at, NOW())
		WHERE id = $1 AND viewer_id = $2 AND ended_at IS NULL
	`, id, viewer)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLiveSessionNotFound
	}
	return nil
}

// EndLiveSessionsForCamera closes every open session that covers a camera whose
// stream was turned off.
func (r *VideoRepository) EndLiveSessionsForCamera(ctx context.Context, cameraID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE live_view_sessions SET ended_at = NOW(), expires_at = LEAST(expires_at, NOW())
		WHERE ended_at IS NULL AND id IN (SELECT session_id FROM live_view_session_cameras WHERE camera_id = $1)
	`, cameraID)
	return err
}

// ListLiveCameras returns active cameras with streaming enabled, for the wall.
func (r *VideoRepository) ListLiveCameras(ctx context.Context, stationID *uuid.UUID, limit int) ([]models.Camera, error) {
	rows, err := r.db.Query(ctx, cameraSelect+`
		WHERE c.status = 'ACTIVE' AND c.ingest_enabled AND ($1::uuid IS NULL OR c.station_id = $1)
		ORDER BY c.code
		LIMIT $2`, stationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Camera{}
	for rows.Next() {
		c, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}
