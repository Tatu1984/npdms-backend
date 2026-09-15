package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/npdms/api/internal/models"
)

var (
	ErrPhotoNotFound      = errors.New("photograph not found on this report")
	ErrPhotoRetired       = errors.New("this photograph has been retired")
	ErrPhotoAlreadyStored = errors.New("this exact photograph is already on the report")
)

/* ------------------------------------------------------------------ photos */

const photoSelect = `
	SELECT ph.id, ph.report_id, ph.sha256, ph.size_bytes, ph.content_type, ph.width, ph.height,
	       ph.storage_backend, ph.location_metadata_removed, ph.metadata_note,
	       ph.source, ph.provided_by_name, ph.relationship, ph.consent_recorded, ph.consent_note,
	       to_char(ph.taken_on, 'YYYY-MM-DD'), ph.is_primary, ph.quality_note,
	       ph.uploaded_by, COALESCE(ub.name, ''), ph.created_at,
	       ph.retired_at, COALESCE(rb.name, ''), ph.retire_reason,
	       ph.storage_key, ph.thumbnail_key
	FROM missing_person_photos ph
	LEFT JOIN users ub ON ub.id = ph.uploaded_by
	LEFT JOIN users rb ON rb.id = ph.retired_by
`

func scanPhoto(row pgx.Row) (*models.MissingPersonPhoto, error) {
	var p models.MissingPersonPhoto
	if err := row.Scan(&p.ID, &p.ReportID, &p.SHA256, &p.SizeBytes, &p.ContentType, &p.Width, &p.Height,
		&p.StorageBackend, &p.LocationMetadataRemoved, &p.MetadataNote,
		&p.Source, &p.ProvidedByName, &p.Relationship, &p.ConsentRecorded, &p.ConsentNote,
		&p.TakenOn, &p.IsPrimary, &p.QualityNote,
		&p.UploadedBy, &p.UploadedByName, &p.CreatedAt,
		&p.RetiredAt, &p.RetiredByName, &p.RetireReason,
		&p.StorageKey, &p.ThumbnailKey); err != nil {
		return nil, err
	}
	return &p, nil
}

// Photos lists a report's photographs: those given by family, friends and the
// informant first, then the rest; the primary first within each group.
func (r *MissingPersonRepository) Photos(ctx context.Context, reportID uuid.UUID, includeRetired bool) ([]models.MissingPersonPhoto, error) {
	q := photoSelect + " WHERE ph.report_id = $1"
	if !includeRetired {
		q += " AND ph.retired_at IS NULL"
	}
	q += `
		ORDER BY (ph.retired_at IS NOT NULL),
		         CASE ph.source WHEN 'FAMILY' THEN 0 WHEN 'FRIEND' THEN 1 WHEN 'REPORTING_PERSON' THEN 2 ELSE 3 END,
		         ph.is_primary DESC, ph.created_at`
	rows, err := r.db.Query(ctx, q, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MissingPersonPhoto{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MissingPersonRepository) Photo(ctx context.Context, reportID, photoID uuid.UUID) (*models.MissingPersonPhoto, error) {
	p, err := scanPhoto(r.db.QueryRow(ctx, photoSelect+" WHERE ph.id = $1 AND ph.report_id = $2", photoID, reportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPhotoNotFound
	}
	return p, err
}

// NewPhoto is a stored photograph ready to be recorded.
type NewPhoto struct {
	ID                      uuid.UUID
	StorageKey              string
	StorageBackend          string
	SHA256                  string
	SizeBytes               int64
	ContentType             string
	Width, Height           int
	ThumbnailKey            *string
	LocationMetadataRemoved bool
	MetadataNote            string
	Upload                  models.PhotoUpload
}

// InsertPhoto records a photograph on an open report. The first active photo
// becomes primary, and a photo marked primary takes over from the current one,
// in the same transaction under a lock on the report, so two uploads cannot
// both become primary.
func (r *MissingPersonRepository) InsertPhoto(ctx context.Context, reportID uuid.UUID, np NewPhoto, actor uuid.UUID) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM missing_person_reports WHERE id = $1 FOR UPDATE`, reportID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrMissingPersonNotFound
	}
	if err != nil {
		return false, err
	}
	if status != string(models.MissingReported) && status != string(models.MissingSearching) {
		return false, ErrMissingPersonNotOpen
	}
	var duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM missing_person_photos WHERE report_id = $1 AND sha256 = $2 AND retired_at IS NULL)`,
		reportID, np.SHA256).Scan(&duplicate); err != nil {
		return false, err
	}
	if duplicate {
		return false, ErrPhotoAlreadyStored
	}
	var hasPrimary bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM missing_person_photos WHERE report_id = $1 AND is_primary AND retired_at IS NULL)`,
		reportID).Scan(&hasPrimary); err != nil {
		return false, err
	}
	primary := np.Upload.MakePrimary || !hasPrimary
	if primary && hasPrimary {
		if _, err := tx.Exec(ctx, `UPDATE missing_person_photos SET is_primary = FALSE WHERE report_id = $1 AND is_primary`, reportID); err != nil {
			return false, err
		}
	}
	u := np.Upload
	_, err = tx.Exec(ctx, `
		INSERT INTO missing_person_photos (
			id, report_id, storage_key, storage_backend, sha256, size_bytes, content_type, width, height,
			thumbnail_key, original_filename, location_metadata_removed, metadata_note,
			source, provided_by_name, relationship, consent_recorded, consent_note, taken_on,
			is_primary, quality_note, uploaded_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
	`, np.ID, reportID, np.StorageKey, np.StorageBackend, np.SHA256, np.SizeBytes, np.ContentType, np.Width, np.Height,
		np.ThumbnailKey, nullable(u.Filename), np.LocationMetadataRemoved, np.MetadataNote,
		u.Source, u.ProvidedByName, u.Relationship, u.ConsentRecorded, u.ConsentNote, u.TakenOn,
		primary, u.QualityNote, actor)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return false, fmt.Errorf("%w: %s", ErrPhotoDetailsRejected, pgErr.ConstraintName)
		}
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE missing_person_reports SET updated_at = NOW() WHERE id = $1`, reportID); err != nil {
		return false, err
	}
	return primary, tx.Commit(ctx)
}

var ErrPhotoDetailsRejected = errors.New("photograph details rejected by the register")

func nullable(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func (r *MissingPersonRepository) SetPhotoThumbnail(ctx context.Context, photoID uuid.UUID, key string) error {
	_, err := r.db.Exec(ctx, `UPDATE missing_person_photos SET thumbnail_key = $2 WHERE id = $1`, photoID, key)
	return err
}

// SetPrimaryPhoto makes an active photo the primary.
func (r *MissingPersonRepository) SetPrimaryPhoto(ctx context.Context, reportID, photoID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM missing_person_reports WHERE id = $1 FOR UPDATE`, reportID); err != nil {
		return err
	}
	var retired bool
	err = tx.QueryRow(ctx, `SELECT retired_at IS NOT NULL FROM missing_person_photos WHERE id = $1 AND report_id = $2`, photoID, reportID).Scan(&retired)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPhotoNotFound
	}
	if err != nil {
		return err
	}
	if retired {
		return ErrPhotoRetired
	}
	if _, err := tx.Exec(ctx, `UPDATE missing_person_photos SET is_primary = FALSE WHERE report_id = $1 AND is_primary AND id <> $2`, reportID, photoID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE missing_person_photos SET is_primary = TRUE WHERE id = $1`, photoID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RetirePhoto withdraws a photo with a reason. When it was the primary, the
// next active photo — family first, then oldest — becomes primary, so a report
// with active photos always has one. The promoted photo's id is returned.
func (r *MissingPersonRepository) RetirePhoto(ctx context.Context, reportID, photoID, actor uuid.UUID, reason string) (*uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM missing_person_reports WHERE id = $1 FOR UPDATE`, reportID); err != nil {
		return nil, err
	}
	var retired, wasPrimary bool
	err = tx.QueryRow(ctx, `SELECT retired_at IS NOT NULL, is_primary FROM missing_person_photos WHERE id = $1 AND report_id = $2`,
		photoID, reportID).Scan(&retired, &wasPrimary)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPhotoNotFound
	}
	if err != nil {
		return nil, err
	}
	if retired {
		return nil, ErrPhotoRetired
	}
	if _, err := tx.Exec(ctx, `
		UPDATE missing_person_photos SET is_primary = FALSE, retired_at = NOW(), retired_by = $2, retire_reason = $3
		WHERE id = $1`, photoID, actor, reason); err != nil {
		return nil, err
	}
	var promoted *uuid.UUID
	if wasPrimary {
		var next uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT id FROM missing_person_photos WHERE report_id = $1 AND retired_at IS NULL
			ORDER BY CASE source WHEN 'FAMILY' THEN 0 WHEN 'FRIEND' THEN 1 WHEN 'REPORTING_PERSON' THEN 2 ELSE 3 END, created_at
			LIMIT 1`, reportID).Scan(&next)
		if err == nil {
			if _, err := tx.Exec(ctx, `UPDATE missing_person_photos SET is_primary = TRUE WHERE id = $1`, next); err != nil {
				return nil, err
			}
			promoted = &next
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return promoted, tx.Commit(ctx)
}

/* ---------------------------------------------------------- station checks */

const stationCheckSelect = `
	SELECT sc.id, sc.report_id, sc.station_id, COALESCE(st.name, ''), COALESCE(st.code, ''), sc.outcome, sc.details,
	       sc.sighting_id, x.decision, sc.recorded_by, COALESCE(u.name, ''), COALESCE(u.role::text, ''), sc.created_at
	FROM missing_person_station_checks sc
	LEFT JOIN stations st ON st.id = sc.station_id
	LEFT JOIN users u ON u.id = sc.recorded_by
	LEFT JOIN missing_person_sightings x ON x.id = sc.sighting_id
`

func scanStationCheck(row pgx.Row) (*models.StationCheck, error) {
	var c models.StationCheck
	if err := row.Scan(&c.ID, &c.ReportID, &c.StationID, &c.StationName, &c.StationCode, &c.Outcome, &c.Details,
		&c.SightingID, &c.SightingDecision, &c.RecordedBy, &c.RecordedByName, &c.RecordedByRank, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *MissingPersonRepository) StationChecks(ctx context.Context, reportID uuid.UUID) ([]models.StationCheck, error) {
	rows, err := r.db.Query(ctx, stationCheckSelect+" WHERE sc.report_id = $1 ORDER BY sc.created_at DESC", reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StationCheck{}
	for rows.Next() {
		c, err := scanStationCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RecordStationCheck appends a station's check. For a SIGHTING the sighting is
// recorded in the same transaction through the ordinary sighting register, so
// it awaits verification by another officer like any other.
func (r *MissingPersonRepository) RecordStationCheck(ctx context.Context, reportID, stationID, actor uuid.UUID, outcome string,
	details *string, sighting *models.RecordMissingSightingRequest) (*models.StationCheck, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM missing_person_reports WHERE id = $1 FOR SHARE`, reportID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMissingPersonNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != string(models.MissingReported) && status != string(models.MissingSearching) {
		return nil, ErrMissingPersonNotOpen
	}
	var sightingID *uuid.UUID
	if sighting != nil {
		sid := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO missing_person_sightings (id, report_id, reported_by, source, location, latitude, longitude, sighted_at, details)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, sid, reportID, actor, sighting.Source, strings.TrimSpace(sighting.Location), sighting.Latitude, sighting.Longitude,
			sighting.SightedAt, strings.TrimSpace(sighting.Details)); err != nil {
			return nil, err
		}
		sightingID = &sid
	}
	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO missing_person_station_checks (id, report_id, station_id, outcome, details, sighting_id, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, reportID, stationID, outcome, details, sightingID, actor); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrStationNotFound
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return scanStationCheck(r.db.QueryRow(ctx, stationCheckSelect+" WHERE sc.id = $1", id))
}

/* ------------------------------------------------------------------- board */

// Board returns every open report from every station, newest first, in its
// broadcast form, with each station's latest check.
func (r *MissingPersonRepository) Board(ctx context.Context, limit int) ([]models.BoardEntry, []models.BoardStation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.report_number, m.status, m.priority, m.vulnerabilities, m.person_name, m.age, m.gender,
		       m.height, m.complexion, m.identifying_marks, m.last_seen_wearing,
		       m.last_seen_location, m.last_seen_date, m.last_seen_latitude, m.last_seen_longitude,
		       m.station_id, COALESCE(s.name, ''), COALESCE(m.created_at, NOW()),
		       (SELECT ph.id FROM missing_person_photos ph WHERE ph.report_id = m.id AND ph.is_primary AND ph.retired_at IS NULL),
		       (SELECT COUNT(*) FROM missing_person_photos ph WHERE ph.report_id = m.id AND ph.retired_at IS NULL),
		       (SELECT COUNT(*) FROM missing_person_sightings x WHERE x.report_id = m.id AND x.decision = 'VERIFIED')
		FROM missing_person_reports m
		LEFT JOIN stations s ON s.id = m.station_id
		WHERE m.status IN ('REPORTED', 'SEARCHING')
		ORDER BY m.created_at DESC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	entries := []models.BoardEntry{}
	index := map[uuid.UUID]int{}
	ids := []uuid.UUID{}
	for rows.Next() {
		var e models.BoardEntry
		if err := rows.Scan(&e.ID, &e.ReportNumber, &e.Status, &e.Priority, &e.Vulnerabilities, &e.PersonName, &e.Age, &e.Gender,
			&e.Height, &e.Complexion, &e.IdentifyingMarks, &e.LastSeenWearing,
			&e.LastSeenLocation, &e.LastSeenAt, &e.LastSeenLatitude, &e.LastSeenLongitude,
			&e.StationID, &e.StationName, &e.LodgedAt, &e.PrimaryPhotoID, &e.PhotoCount, &e.VerifiedSightings); err != nil {
			return nil, nil, err
		}
		if e.Vulnerabilities == nil {
			e.Vulnerabilities = []string{}
		}
		e.Checks = []models.StationCheck{}
		index[e.ID] = len(entries)
		ids = append(ids, e.ID)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()

	if len(ids) > 0 {
		crows, err := r.db.Query(ctx, stationCheckSelect+" WHERE sc.report_id = ANY($1) ORDER BY sc.created_at DESC", ids)
		if err != nil {
			return nil, nil, err
		}
		defer crows.Close()
		seen := map[[2]uuid.UUID]bool{}
		all := []models.StationCheck{}
		for crows.Next() {
			c, err := scanStationCheck(crows)
			if err != nil {
				return nil, nil, err
			}
			all = append(all, *c)
		}
		if err := crows.Err(); err != nil {
			return nil, nil, err
		}
		for _, c := range all {
			k := [2]uuid.UUID{c.ReportID, c.StationID}
			if seen[k] {
				continue
			}
			seen[k] = true
			i := index[c.ReportID]
			entries[i].Checks = append(entries[i].Checks, c)
		}
	}

	srows, err := r.db.Query(ctx, `SELECT id, code, name FROM stations ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	defer srows.Close()
	stations := []models.BoardStation{}
	for srows.Next() {
		var s models.BoardStation
		if err := srows.Scan(&s.ID, &s.Code, &s.Name); err != nil {
			return nil, nil, err
		}
		stations = append(stations, s)
	}
	if err := srows.Err(); err != nil {
		return nil, nil, err
	}
	return entries, stations, nil
}

/* ---------------------------------------------------------------- alerts */

// RaiseBroadcastAlert issues the city-wide alert for a report, linked to it.
// An alert already active for the report is not duplicated.
func (r *MissingPersonRepository) RaiseBroadcastAlert(ctx context.Context, reportID uuid.UUID, scope models.AlertScope, priority int,
	title, description string, issuedBy uuid.UUID, stationID *uuid.UUID) (uuid.UUID, bool, error) {
	var existing uuid.UUID
	err := r.db.QueryRow(ctx, `
		SELECT id FROM alerts WHERE resource_type = 'missing_person' AND resource_id = $1 AND expires_at > NOW()::timestamp
		ORDER BY issued_at DESC LIMIT 1`, reportID).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}
	id := uuid.New()
	now := time.Now()
	if len(title) > 255 {
		title = title[:252] + "..."
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO alerts (id, type, scope, title, description, issued_at, expires_at, issued_by, acknowledged,
		                    priority, has_image, station_id, created_at, updated_at, resource_type, resource_id)
		VALUES ($1, 'BOLO', $2, $3, $4, $5, $6, $7, FALSE, $8, FALSE, $9, $5, $5, 'missing_person', $10)
	`, id, string(scope), title, description, now, now.Add(30*24*time.Hour), issuedBy, priority, stationID, reportID)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

/* --------------------------------------------------------------------- map */

func (r *MissingPersonRepository) MapCameras(ctx context.Context) ([]models.MapCamera, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.location, c.latitude, c.longitude, COALESCE(s.name, '')
		FROM cameras c LEFT JOIN stations s ON s.id = c.station_id
		WHERE c.status = 'ACTIVE' AND c.latitude IS NOT NULL
		ORDER BY c.name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MapCamera{}
	for rows.Next() {
		var c models.MapCamera
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Location, &c.Latitude, &c.Longitude, &c.StationName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CameraMatches reads PENDING and CONFIRMED face-match candidates for a report
// from the face recognition layer's table. Until that table exists the layer
// is reported unavailable rather than failing the map.
func (r *MissingPersonRepository) CameraMatches(ctx context.Context, reportID uuid.UUID) (models.CameraMatchLayer, error) {
	layer := models.CameraMatchLayer{Data: []models.CameraMatch{}}
	if err := r.db.QueryRow(ctx, `SELECT to_regclass('public.face_match_candidates') IS NOT NULL`).Scan(&layer.Available); err != nil {
		return layer, err
	}
	if !layer.Available {
		return layer, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT f.id, f.photo_id, f.camera_id, COALESCE(c.name, ''), COALESCE(f.source_media::text, ''), f.frame_time,
		       f.latitude::float8, f.longitude::float8, c.latitude, c.longitude,
		       f.similarity::float8, COALESCE(f.model_version::text, ''), f.status::text,
		       COALESCE(u.name, ''), f.reviewed_at, f.sighting_id, COALESCE((to_jsonb(f)->>'is_demo')::boolean, FALSE)
		FROM face_match_candidates f
		LEFT JOIN cameras c ON c.id = f.camera_id
		LEFT JOIN users u ON u.id = f.reviewed_by
		WHERE f.report_id = $1 AND f.status::text IN ('PENDING', 'CONFIRMED')
		ORDER BY f.frame_time NULLS LAST
		LIMIT 2000`, reportID)
	if err != nil {
		return layer, err
	}
	defer rows.Close()
	for rows.Next() {
		var m models.CameraMatch
		var camLat, camLng *float64
		if err := rows.Scan(&m.ID, &m.PhotoID, &m.CameraID, &m.CameraName, &m.SourceMedia, &m.FrameTime,
			&m.Latitude, &m.Longitude, &camLat, &camLng,
			&m.Similarity, &m.ModelVersion, &m.Status, &m.ReviewedByName, &m.ReviewedAt, &m.SightingID, &m.IsDemo); err != nil {
			return layer, err
		}
		switch {
		case m.Latitude != nil && m.Longitude != nil:
			m.PointFrom = "CANDIDATE"
		case camLat != nil && camLng != nil:
			m.Latitude, m.Longitude, m.PointFrom = camLat, camLng, "CAMERA"
		default:
			m.Latitude, m.Longitude, m.PointFrom = nil, nil, "NONE"
		}
		layer.Data = append(layer.Data, m)
	}
	return layer, rows.Err()
}
