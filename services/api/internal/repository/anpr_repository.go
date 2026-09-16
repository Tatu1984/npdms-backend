package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
)

var (
	ErrANPRAnalysisNotFound   = errors.New("vehicle detection analysis not found")
	ErrANPRFrameNotFound      = errors.New("frame not found in this analysis")
	ErrANPRReadNotFound       = errors.New("plate read not found")
	ErrANPRHitNotFound        = errors.New("watchlist hit not found")
	ErrANPRHitReviewed        = errors.New("this watchlist hit has already been reviewed")
	ErrANPRSelfReview         = errors.New("a watchlist hit must be reviewed by an officer other than the one who submitted the footage")
	ErrWatchlistEntryNotFound = errors.New("watchlist entry not found")
	ErrWatchlistEntryRemoved  = errors.New("watchlist entry has already been removed")
	ErrWatchlistDuplicate     = errors.New("this registration number is already on the watchlist")
	ErrANPRReadAttached       = errors.New("this ANPR read is already in the incident's plate-read register")
)

type ANPRRepository struct {
	db *pgxpool.Pool
}

func NewANPRRepository(db *pgxpool.Pool) *ANPRRepository {
	return &ANPRRepository{db: db}
}

func anprPgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

/* ------------------------------------------------------------ module switch */

// The switches live in ai_module_switches, the one table the A0 gateway keeps
// for every AI module (000076 folded this module's own table into it).
func (r *ANPRRepository) Switch(ctx context.Context, module string) (*models.ModuleSwitch, error) {
	var s models.ModuleSwitch
	err := r.db.QueryRow(ctx, `
		SELECT m.module, m.enabled, COALESCE(m.reason, ''), m.updated_by, COALESCE(u.name, ''), m.updated_at
		FROM ai_module_switches m LEFT JOIN users u ON u.id = m.updated_by
		WHERE m.module = $1`, module).Scan(&s.Module, &s.Enabled, &s.Note, &s.UpdatedBy, &s.UpdatedByName, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// The migration seeds the row; its absence means the module was never installed.
		return &models.ModuleSwitch{Module: module, Enabled: false, Note: "module not installed"}, nil
	}
	return &s, err
}

func (r *ANPRRepository) SetSwitch(ctx context.Context, module string, enabled bool, note string, actor uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO ai_module_switches (module, enabled, reason, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (module) DO UPDATE SET enabled = EXCLUDED.enabled,
		       reason = EXCLUDED.reason, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		module, enabled, note, actor)
	return err
}

/* ------------------------------------------------------------------ cameras */

type ANPRCamera struct {
	ID        uuid.UUID
	Code      string
	Name      string
	Location  string
	Latitude  *float64
	Longitude *float64
	StationID uuid.UUID
	Status    string
}

func (r *ANPRRepository) Camera(ctx context.Context, id uuid.UUID) (*ANPRCamera, error) {
	var c ANPRCamera
	err := r.db.QueryRow(ctx, `SELECT id, code, name, location, latitude, longitude, station_id, status FROM cameras WHERE id = $1`, id).
		Scan(&c.ID, &c.Code, &c.Name, &c.Location, &c.Latitude, &c.Longitude, &c.StationID, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCameraNotFound
	}
	return &c, err
}

/* ---------------------------------------------------------------- analyses */

type NewANPRRead struct {
	RegistrationNumber string
	DisplayNumber      string
	RawText            string
	PlateFormat        string
	Confidence         float64
	MinCharConfidence  float64
	Characters         []models.ANPRCharacter
	Corrections        []string
	Box                [4]int
}

type NewANPRDetection struct {
	VehicleClass string
	Box          [4]int
	Confidence   float64
	Read         *NewANPRRead
}

type NewANPRFrame struct {
	Index         int
	OffsetSeconds float64
	ObjectKey     string
	SHA256        string
	Width, Height int
	Detections    []NewANPRDetection
	Unattached    []NewANPRRead
}

type NewANPRAnalysis struct {
	SourceKind         string
	CameraID           *uuid.UUID
	Purpose            string
	CapturedAt         time.Time
	MediaObjectKey     *string // nil for footage, which is not stored whole
	MediaSHA256        string
	MediaFilename      string
	MediaContentType   string
	MediaSize          int64
	StorageBackend     string
	DetectorVersion    string
	PlateReaderVersion string
	SampledFrames      int
	SampleSeconds      *float64
	ProcessingMs       int
	SubmittedBy        uuid.UUID
	Frames             []NewANPRFrame
}

// CreateAnalysis stores an analysis with every frame, detection and read in one
// transaction, then matches the reads against the watchlist and records hits
// as PENDING. It returns the analysis id.
func (r *ANPRRepository) CreateAnalysis(ctx context.Context, a NewANPRAnalysis) (uuid.UUID, error) {
	number, err := formatRecordNumber(ctx, r.db, "ANPR")
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
		INSERT INTO anpr_analyses (id, analysis_number, source_kind, camera_id, purpose, captured_at,
		    media_object_key, media_sha256, media_filename, media_content_type, media_size_bytes, storage_backend,
		    detector_version, plate_reader_version, sampled_frames, sample_seconds, processing_ms, submitted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		id, number, a.SourceKind, a.CameraID, a.Purpose, a.CapturedAt,
		a.MediaObjectKey, a.MediaSHA256, a.MediaFilename, a.MediaContentType, a.MediaSize, a.StorageBackend,
		a.DetectorVersion, a.PlateReaderVersion, a.SampledFrames, a.SampleSeconds, a.ProcessingMs, a.SubmittedBy); err != nil {
		return uuid.Nil, err
	}

	type storedRead struct {
		id     uuid.UUID
		number string
	}
	var reads []storedRead
	insertRead := func(frameID uuid.UUID, detectionID *uuid.UUID, frameTime time.Time, rd NewANPRRead) error {
		chars, err := json.Marshal(rd.Characters)
		if err != nil {
			return err
		}
		if rd.Corrections == nil {
			rd.Corrections = []string{}
		}
		corr, err := json.Marshal(rd.Corrections)
		if err != nil {
			return err
		}
		rid := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO anpr_plate_reads (id, analysis_id, frame_id, detection_id, camera_id, frame_time,
			    registration_number, display_number, raw_text, plate_format, confidence, min_char_confidence,
			    characters, corrections, box_x1, box_y1, box_x2, box_y2, model_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			rid, id, frameID, detectionID, a.CameraID, frameTime,
			rd.RegistrationNumber, rd.DisplayNumber, rd.RawText, rd.PlateFormat, rd.Confidence, rd.MinCharConfidence,
			chars, corr, rd.Box[0], rd.Box[1], rd.Box[2], rd.Box[3], a.PlateReaderVersion); err != nil {
			return err
		}
		reads = append(reads, storedRead{rid, rd.RegistrationNumber})
		return nil
	}

	for _, f := range a.Frames {
		frameID := uuid.New()
		frameTime := a.CapturedAt.Add(time.Duration(f.OffsetSeconds * float64(time.Second)))
		if _, err := tx.Exec(ctx, `
			INSERT INTO anpr_frames (id, analysis_id, frame_index, offset_seconds, frame_time, object_key, sha256, width, height)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			frameID, id, f.Index, f.OffsetSeconds, frameTime, f.ObjectKey, f.SHA256, f.Width, f.Height); err != nil {
			return uuid.Nil, err
		}
		for _, d := range f.Detections {
			did := uuid.New()
			if _, err := tx.Exec(ctx, `
				INSERT INTO vehicle_detections (id, analysis_id, frame_id, camera_id, frame_time, vehicle_class,
				    box_x1, box_y1, box_x2, box_y2, confidence, model_version)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				did, id, frameID, a.CameraID, frameTime, d.VehicleClass,
				d.Box[0], d.Box[1], d.Box[2], d.Box[3], d.Confidence, a.DetectorVersion); err != nil {
				return uuid.Nil, err
			}
			if d.Read != nil {
				if err := insertRead(frameID, &did, frameTime, *d.Read); err != nil {
					return uuid.Nil, err
				}
			}
		}
		for _, rd := range f.Unattached {
			if err := insertRead(frameID, nil, frameTime, rd); err != nil {
				return uuid.Nil, err
			}
		}
	}

	// Watchlist matching: exact normalised registration only.
	if len(reads) > 0 {
		lookouts, err := activeStolenVehicleLookouts(ctx, tx)
		if err != nil {
			return uuid.Nil, err
		}
		numbers := make([]string, 0, len(reads))
		for _, rd := range reads {
			numbers = append(numbers, rd.number)
		}
		manual := map[string]uuid.UUID{}
		rows, err := tx.Query(ctx, `
			SELECT id, registration_number FROM vehicle_watchlist
			WHERE removed_at IS NULL AND expires_at > NOW() AND registration_number = ANY($1)`, numbers)
		if err != nil {
			return uuid.Nil, err
		}
		for rows.Next() {
			var eid uuid.UUID
			var reg string
			if err := rows.Scan(&eid, &reg); err != nil {
				rows.Close()
				return uuid.Nil, err
			}
			manual[reg] = eid
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return uuid.Nil, err
		}
		insertHit := func(readID uuid.UUID, reg, source string, lookoutID, entryID *uuid.UUID) error {
			hitNumber, err := formatRecordNumber(ctx, r.db, "ANPRHIT")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO anpr_watchlist_hits (hit_number, plate_read_id, registration_number, source, lookout_id,
				    watchlist_entry_id, submitted_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
				hitNumber, readID, reg, source, lookoutID, entryID, a.SubmittedBy)
			return err
		}
		for _, rd := range reads {
			for _, l := range lookouts {
				if l.registrations[rd.number] {
					lid := l.id
					if err := insertHit(rd.id, rd.number, "LOOKOUT", &lid, nil); err != nil {
						return uuid.Nil, err
					}
				}
			}
			if eid, ok := manual[rd.number]; ok {
				if err := insertHit(rd.id, rd.number, "WATCHLIST", nil, &eid); err != nil {
					return uuid.Nil, err
				}
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

type stolenVehicleLookout struct {
	id            uuid.UUID
	number        string
	subject       string
	priority      string
	registrations map[string]bool
}

type rowQuerier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

func activeStolenVehicleLookouts(ctx context.Context, q rowQuerier) ([]stolenVehicleLookout, error) {
	rows, err := q.Query(ctx, `
		SELECT id, lookout_number, subject, details, priority FROM lookouts
		WHERE status = 'ACTIVE' AND lookout_type = 'STOLEN_VEHICLE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []stolenVehicleLookout
	for rows.Next() {
		var l stolenVehicleLookout
		var details []byte
		if err := rows.Scan(&l.id, &l.number, &l.subject, &details, &l.priority); err != nil {
			return nil, err
		}
		d := map[string]string{}
		if len(details) > 0 {
			// Details are free key/value pairs; a malformed map just yields no
			// registration from details, the subject is still read.
			_ = json.Unmarshal(details, &d)
		}
		l.registrations = LookoutRegistrations(l.subject, d)
		if len(l.registrations) > 0 {
			out = append(out, l)
		}
	}
	return out, rows.Err()
}

var registrationInText = regexp.MustCompile(`(?i)\b([A-Z]{2}[\s.-]?\d{1,2}[A-Z]?[\s.-]?[A-Z]{0,3}[\s.-]?\d{1,4}|\d{2}[\s.-]?BH[\s.-]?\d{4}[\s.-]?[A-Z]{1,2}|W[A-Z]{2}[\s.-]?\d{3,4})\b`)

// LookoutRegistrations finds the registration numbers a stolen-vehicle lookout
// names: any detail whose key mentions registration, number or plate, and any
// registration-shaped token in the subject. All are normalised with the
// Phase 06 rule.
func LookoutRegistrations(subject string, details map[string]string) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		n := normaliseRegistration(s)
		if len(n) >= 6 && len(n) <= 20 && strings.ContainsAny(n, "0123456789") {
			out[n] = true
		}
	}
	for k, v := range details {
		key := strings.ToLower(k)
		if strings.Contains(key, "registration") || strings.Contains(key, "plate") ||
			strings.Contains(key, "vehicle number") || strings.Contains(key, "vehicle no") || key == "reg no" {
			add(v)
		}
	}
	for _, m := range registrationInText.FindAllString(subject, -1) {
		add(m)
	}
	return out
}

const analysisSelect = `
	SELECT a.id, a.analysis_number, a.source_kind, a.camera_id, COALESCE(c.code, ''), COALESCE(c.name, ''),
	       COALESCE(c.location, ''), c.latitude, c.longitude, a.purpose, a.captured_at, a.media_sha256,
	       a.media_filename, a.media_content_type, a.media_size_bytes, a.detector_version, a.plate_reader_version,
	       a.sampled_frames, a.sample_seconds::float8, a.processing_ms, a.submitted_by, COALESCE(u.name, ''), a.created_at,
	       (SELECT COUNT(*) FROM anpr_frames f WHERE f.analysis_id = a.id),
	       (SELECT COUNT(*) FROM vehicle_detections d WHERE d.analysis_id = a.id),
	       (SELECT COUNT(*) FROM anpr_plate_reads p WHERE p.analysis_id = a.id),
	       (SELECT COUNT(*) FROM anpr_watchlist_hits h JOIN anpr_plate_reads p ON p.id = h.plate_read_id WHERE p.analysis_id = a.id)
	FROM anpr_analyses a
	LEFT JOIN cameras c ON c.id = a.camera_id
	LEFT JOIN users u ON u.id = a.submitted_by
`

func scanAnalysis(row pgx.Row) (*models.ANPRAnalysis, error) {
	var a models.ANPRAnalysis
	err := row.Scan(&a.ID, &a.AnalysisNumber, &a.SourceKind, &a.CameraID, &a.CameraCode, &a.CameraName,
		&a.CameraLocation, &a.Latitude, &a.Longitude, &a.Purpose, &a.CapturedAt, &a.MediaSHA256,
		&a.MediaFilename, &a.MediaContentType, &a.MediaSizeBytes, &a.DetectorVersion, &a.PlateReaderVersion,
		&a.SampledFrames, &a.SampleSeconds, &a.ProcessingMs, &a.SubmittedBy, &a.SubmittedByName, &a.CreatedAt,
		&a.FrameCount, &a.DetectionCount, &a.PlateReadCount, &a.HitCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrANPRAnalysisNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

type ANPRAnalysisFilter struct {
	SourceKind string
	CameraID   *uuid.UUID
	Page       int
	PageSize   int
}

// ListAnalyses returns submission metadata only — no plates or frames.
func (r *ANPRRepository) ListAnalyses(ctx context.Context, f ANPRAnalysisFilter) ([]models.ANPRAnalysis, int64, error) {
	where, args := []string{"1=1"}, []interface{}{}
	if f.SourceKind != "" {
		args = append(args, f.SourceKind)
		where = append(where, fmt.Sprintf("a.source_kind = $%d", len(args)))
	}
	if f.CameraID != nil {
		args = append(args, *f.CameraID)
		where = append(where, fmt.Sprintf("a.camera_id = $%d", len(args)))
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM anpr_analyses a WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, fmt.Sprintf("%s WHERE %s ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d",
		analysisSelect, clause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.ANPRAnalysis{}
	for rows.Next() {
		a, err := scanAnalysis(rows)
		if err != nil {
			return nil, 0, err
		}
		a.Purpose = "" // the purpose is visible when the analysis is opened
		out = append(out, *a)
	}
	return out, total, rows.Err()
}

func (r *ANPRRepository) Analysis(ctx context.Context, id uuid.UUID) (*models.ANPRAnalysis, error) {
	return scanAnalysis(r.db.QueryRow(ctx, analysisSelect+" WHERE a.id = $1", id))
}

// AnalysisDetail loads frames, detections, reads and hits.
func (r *ANPRRepository) AnalysisDetail(ctx context.Context, id uuid.UUID) (*models.ANPRAnalysis, error) {
	a, err := r.Analysis(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, frame_index, offset_seconds::float8, frame_time, sha256, width, height
		FROM anpr_frames WHERE analysis_id = $1 ORDER BY frame_index`, id)
	if err != nil {
		return nil, err
	}
	frames := []models.ANPRFrame{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var f models.ANPRFrame
		if err := rows.Scan(&f.ID, &f.FrameIndex, &f.OffsetSeconds, &f.FrameTime, &f.SHA256, &f.Width, &f.Height); err != nil {
			rows.Close()
			return nil, err
		}
		f.Detections, f.UnattachedReads = []models.VehicleDetection{}, []models.ANPRPlateRead{}
		index[f.ID] = len(frames)
		frames = append(frames, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	reads, _, err := r.searchReads(ctx, "p.analysis_id = $1", []interface{}{id}, 0, 0)
	if err != nil {
		return nil, err
	}
	byDetection := map[uuid.UUID]*models.ANPRPlateRead{}
	for i := range reads {
		if reads[i].DetectionID != nil {
			byDetection[*reads[i].DetectionID] = &reads[i]
		} else if fi, ok := index[reads[i].FrameID]; ok {
			frames[fi].UnattachedReads = append(frames[fi].UnattachedReads, reads[i])
		}
	}

	rows, err = r.db.Query(ctx, `
		SELECT id, frame_id, vehicle_class, box_x1, box_y1, box_x2, box_y2, confidence::float8, model_version, frame_time
		FROM vehicle_detections WHERE analysis_id = $1 ORDER BY frame_time, confidence DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d models.VehicleDetection
		if err := rows.Scan(&d.ID, &d.FrameID, &d.VehicleClass, &d.Box[0], &d.Box[1], &d.Box[2], &d.Box[3],
			&d.Confidence, &d.ModelVersion, &d.FrameTime); err != nil {
			return nil, err
		}
		d.PlateRead = byDetection[d.ID]
		if fi, ok := index[d.FrameID]; ok {
			frames[fi].Detections = append(frames[fi].Detections, d)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	a.Frames = frames
	hits, _, err := r.hits(ctx, "p.analysis_id = $1", []interface{}{id}, 0, 0)
	if err != nil {
		return nil, err
	}
	a.Hits = hits
	return a, nil
}

type ANPRFrameObject struct {
	ObjectKey   string
	SHA256      string
	ContentType string
}

func (r *ANPRRepository) FrameObject(ctx context.Context, analysisID, frameID uuid.UUID) (*ANPRFrameObject, error) {
	var o ANPRFrameObject
	err := r.db.QueryRow(ctx, `
		SELECT f.object_key, f.sha256,
		       CASE WHEN f.object_key = a.media_object_key THEN a.media_content_type ELSE 'image/jpeg' END
		FROM anpr_frames f JOIN anpr_analyses a ON a.id = f.analysis_id
		WHERE f.id = $1 AND f.analysis_id = $2`, frameID, analysisID).Scan(&o.ObjectKey, &o.SHA256, &o.ContentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrANPRFrameNotFound
	}
	return &o, err
}

// RecentlyAccessed says whether the officer submitted or opened the analysis
// under a stated purpose within the window.
func (r *ANPRRepository) RecentlyAccessed(ctx context.Context, actor, analysisID uuid.UUID, since time.Time) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM anpr_access_log WHERE actor_user_id = $1 AND analysis_id = $2 AND accessed_at >= $3)`,
		actor, analysisID, since).Scan(&ok)
	return ok, err
}

func (r *ANPRRepository) LogAccess(ctx context.Context, actor uuid.UUID, accessType, purpose string, filters interface{}, analysisID *uuid.UUID, resultCount *int, ip string) error {
	raw, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO anpr_access_log (actor_user_id, access_type, purpose, filters, analysis_id, result_count, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::inet)`, actor, accessType, purpose, raw, analysisID, resultCount, ip)
	return err
}

/* -------------------------------------------------------------- plate reads */

const readSelect = `
	SELECT p.id, p.analysis_id, a.analysis_number, p.frame_id, p.detection_id, p.camera_id, COALESCE(c.code, ''),
	       COALESCE(c.name, ''), COALESCE(c.location, ''), c.latitude, c.longitude, p.frame_time,
	       p.registration_number, p.display_number, p.raw_text, p.plate_format, p.confidence::float8,
	       p.min_char_confidence::float8, p.characters, p.corrections, p.box_x1, p.box_y1, p.box_x2, p.box_y2,
	       p.model_version, p.created_at
	FROM anpr_plate_reads p
	JOIN anpr_analyses a ON a.id = p.analysis_id
	LEFT JOIN cameras c ON c.id = p.camera_id
`

func scanRead(row pgx.Row) (*models.ANPRPlateRead, error) {
	var p models.ANPRPlateRead
	var chars, corr []byte
	if err := row.Scan(&p.ID, &p.AnalysisID, &p.AnalysisNumber, &p.FrameID, &p.DetectionID, &p.CameraID, &p.CameraCode,
		&p.CameraName, &p.CameraLocation, &p.Latitude, &p.Longitude, &p.FrameTime,
		&p.RegistrationNumber, &p.DisplayNumber, &p.RawText, &p.PlateFormat, &p.Confidence,
		&p.MinCharConfidence, &chars, &corr, &p.Box[0], &p.Box[1], &p.Box[2], &p.Box[3],
		&p.ModelVersion, &p.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(chars, &p.Characters); err != nil {
		return nil, fmt.Errorf("plate read %s characters: %w", p.ID, err)
	}
	if err := json.Unmarshal(corr, &p.Corrections); err != nil {
		return nil, fmt.Errorf("plate read %s corrections: %w", p.ID, err)
	}
	return &p, nil
}

func (r *ANPRRepository) searchReads(ctx context.Context, clause string, args []interface{}, page, size int) ([]models.ANPRPlateRead, int64, error) {
	var total int64
	query := readSelect + " WHERE " + clause + " ORDER BY p.frame_time DESC, p.confidence DESC"
	if size > 0 {
		if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM anpr_plate_reads p LEFT JOIN cameras c ON c.id = p.camera_id WHERE "+clause, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
		args = append(args, size, (page-1)*size)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.ANPRPlateRead{}
	for rows.Next() {
		p, err := scanRead(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if size <= 0 {
		total = int64(len(out))
	}
	return out, total, nil
}

// SearchReads matches a full or partial normalised plate within a time and
// place window. The caller has already validated and normalised the request.
func (r *ANPRRepository) SearchReads(ctx context.Context, req models.ANPRSearchRequest) ([]models.ANPRPlateRead, int64, error) {
	where, args := []string{"1=1"}, []interface{}{}
	add := func(clause string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if req.Plate != "" {
		add("p.registration_number LIKE $%d", "%"+req.Plate+"%")
	}
	if req.From != nil {
		add("p.frame_time >= $%d", *req.From)
	}
	if req.To != nil {
		add("p.frame_time <= $%d", *req.To)
	}
	if req.CameraID != nil {
		add("p.camera_id = $%d", *req.CameraID)
	}
	if req.Latitude != nil && req.Longitude != nil && req.RadiusM != nil {
		// Equirectangular distance is accurate to well under a percent at city scale.
		args = append(args, *req.Latitude, *req.Longitude, *req.RadiusM)
		n := len(args)
		where = append(where, fmt.Sprintf(`c.latitude IS NOT NULL AND
			6371000 * sqrt(power(radians(c.latitude - $%d), 2) +
			               power(radians(c.longitude - $%d) * cos(radians(($%d + c.latitude) / 2)), 2)) <= $%d`,
			n-2, n-1, n-2, n))
	}
	return r.searchReads(ctx, strings.Join(where, " AND "), args, req.Page, req.PageSize)
}

func (r *ANPRRepository) Read(ctx context.Context, id uuid.UUID) (*models.ANPRPlateRead, error) {
	p, err := scanRead(r.db.QueryRow(ctx, readSelect+" WHERE p.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrANPRReadNotFound
	}
	return p, err
}

/* ---------------------------------------------------------------- watchlist */

// Watchlist lists officer-added entries (all, or only open ones) followed by
// the active stolen-vehicle lookouts that are matched automatically.
func (r *ANPRRepository) Watchlist(ctx context.Context, includeClosed bool) ([]models.WatchlistEntry, error) {
	clause := "w.removed_at IS NULL"
	if includeClosed {
		clause = "TRUE"
	}
	rows, err := r.db.Query(ctx, `
		SELECT w.id, w.registration_number, w.reason, w.priority, w.fir_id, COALESCE(f.fir_number, ''), w.expires_at,
		       COALESCE(ab.name, ''), w.created_at, w.removed_at, COALESCE(rb.name, ''), w.removal_note,
		       CASE WHEN w.removed_at IS NOT NULL THEN 'REMOVED' WHEN w.expires_at <= NOW() THEN 'EXPIRED' ELSE 'ACTIVE' END
		FROM vehicle_watchlist w
		LEFT JOIN firs f ON f.id = w.fir_id
		LEFT JOIN users ab ON ab.id = w.added_by
		LEFT JOIN users rb ON rb.id = w.removed_by
		WHERE `+clause+` ORDER BY w.created_at DESC`)
	if err != nil {
		return nil, err
	}
	out := []models.WatchlistEntry{}
	for rows.Next() {
		e := models.WatchlistEntry{Source: "WATCHLIST"}
		if err := rows.Scan(&e.ID, &e.RegistrationNumber, &e.Reason, &e.Priority, &e.FIRID, &e.FIRNumber, &e.ExpiresAt,
			&e.AddedByName, &e.CreatedAt, &e.RemovedAt, &e.RemovedByName, &e.RemovalNote, &e.Status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.db.Query(ctx, `
		SELECT l.id, l.lookout_number, l.subject, l.details, l.priority, l.fir_id, COALESCE(f.fir_number, ''),
		       COALESCE(u.name, ''), l.issued_at
		FROM lookouts l LEFT JOIN firs f ON f.id = l.fir_id LEFT JOIN users u ON u.id = l.issued_by
		WHERE l.status = 'ACTIVE' AND l.lookout_type = 'STOLEN_VEHICLE' ORDER BY l.issued_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var number, subject, priority, firNumber, issuer string
		var details []byte
		var firID *uuid.UUID
		var issued time.Time
		if err := rows.Scan(&id, &number, &subject, &details, &priority, &firID, &firNumber, &issuer, &issued); err != nil {
			return nil, err
		}
		d := map[string]string{}
		_ = json.Unmarshal(details, &d)
		regs := LookoutRegistrations(subject, d)
		if len(regs) == 0 {
			// Listed so the gap is visible: this lookout cannot match any read.
			lid := id
			out = append(out, models.WatchlistEntry{ID: id, Source: "LOOKOUT", Reason: subject, Priority: priority,
				FIRID: firID, FIRNumber: firNumber, LookoutID: &lid, LookoutNumber: number, AddedByName: issuer,
				CreatedAt: issued, Status: "NO_REGISTRATION"})
			continue
		}
		for reg := range regs {
			lid := id
			out = append(out, models.WatchlistEntry{ID: id, Source: "LOOKOUT", RegistrationNumber: reg, Reason: subject,
				Priority: priority, FIRID: firID, FIRNumber: firNumber, LookoutID: &lid, LookoutNumber: number,
				AddedByName: issuer, CreatedAt: issued, Status: "ACTIVE"})
		}
	}
	return out, rows.Err()
}

func (r *ANPRRepository) AddWatchlistEntry(ctx context.Context, req models.CreateWatchlistEntryRequest, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO vehicle_watchlist (id, registration_number, reason, priority, fir_id, expires_at, added_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, req.RegistrationNumber, req.Reason, req.Priority, req.FIRID, req.ExpiresAt, actor)
	switch anprPgCode(err) {
	case "23505":
		return uuid.Nil, ErrWatchlistDuplicate
	case "23503":
		return uuid.Nil, ErrFIRNotFound
	}
	return id, err
}

func (r *ANPRRepository) WatchlistEntry(ctx context.Context, id uuid.UUID) (*models.WatchlistEntry, error) {
	list, err := r.Watchlist(ctx, true)
	if err != nil {
		return nil, err
	}
	for _, e := range list {
		if e.Source == "WATCHLIST" && e.ID == id {
			return &e, nil
		}
	}
	return nil, ErrWatchlistEntryNotFound
}

func (r *ANPRRepository) RemoveWatchlistEntry(ctx context.Context, id uuid.UUID, note string, actor uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vehicle_watchlist SET removed_at = NOW(), removed_by = $2, removal_note = $3
		WHERE id = $1 AND removed_at IS NULL`, id, actor, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := r.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM vehicle_watchlist WHERE id = $1)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrWatchlistEntryNotFound
		}
		return ErrWatchlistEntryRemoved
	}
	return nil
}

/* --------------------------------------------------------------------- hits */

const hitSelect = `
	SELECT h.id, h.hit_number, h.registration_number, h.source, h.lookout_id, COALESCE(l.lookout_number, ''),
	       COALESCE(l.subject, ''), h.watchlist_entry_id, COALESCE(w.reason, ''),
	       COALESCE(l.priority, w.priority, 'NORMAL'), h.submitted_by, COALESCE(sb.name, ''), h.status,
	       COALESCE(rb.name, ''), h.reviewed_at, h.review_note, h.alert_id, h.sighting_id, h.created_at, h.plate_read_id
	FROM anpr_watchlist_hits h
	JOIN anpr_plate_reads p ON p.id = h.plate_read_id
	LEFT JOIN lookouts l ON l.id = h.lookout_id
	LEFT JOIN vehicle_watchlist w ON w.id = h.watchlist_entry_id
	LEFT JOIN users sb ON sb.id = h.submitted_by
	LEFT JOIN users rb ON rb.id = h.reviewed_by
`

func (r *ANPRRepository) hits(ctx context.Context, clause string, args []interface{}, page, size int) ([]models.ANPRHit, int64, error) {
	var total int64
	query := hitSelect + " WHERE " + clause +
		" ORDER BY CASE h.status WHEN 'PENDING' THEN 0 ELSE 1 END, h.created_at DESC"
	if size > 0 {
		if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM anpr_watchlist_hits h JOIN anpr_plate_reads p ON p.id = h.plate_read_id WHERE "+clause, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
		args = append(args, size, (page-1)*size)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	out := []models.ANPRHit{}
	var readIDs []uuid.UUID
	for rows.Next() {
		var h models.ANPRHit
		var readID uuid.UUID
		if err := rows.Scan(&h.ID, &h.HitNumber, &h.RegistrationNumber, &h.Source, &h.LookoutID, &h.LookoutNumber,
			&h.LookoutSubject, &h.WatchlistEntryID, &h.WatchlistReason, &h.Priority, &h.SubmittedBy, &h.SubmittedByName,
			&h.Status, &h.ReviewedByName, &h.ReviewedAt, &h.ReviewNote, &h.AlertID, &h.SightingID, &h.CreatedAt, &readID); err != nil {
			rows.Close()
			return nil, 0, err
		}
		out = append(out, h)
		readIDs = append(readIDs, readID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(readIDs) > 0 {
		reads, _, err := r.searchReads(ctx, "p.id = ANY($1)", []interface{}{readIDs}, 0, 0)
		if err != nil {
			return nil, 0, err
		}
		byID := map[uuid.UUID]models.ANPRPlateRead{}
		for _, rd := range reads {
			byID[rd.ID] = rd
		}
		for i := range out {
			out[i].Read = byID[readIDs[i]]
		}
	}
	if size <= 0 {
		total = int64(len(out))
	}
	return out, total, nil
}

func (r *ANPRRepository) Hits(ctx context.Context, status string, page, size int) ([]models.ANPRHit, int64, error) {
	if status == "" {
		return r.hits(ctx, "TRUE", nil, page, size)
	}
	return r.hits(ctx, "h.status = $1", []interface{}{status}, page, size)
}

func (r *ANPRRepository) Hit(ctx context.Context, id uuid.UUID) (*models.ANPRHit, error) {
	list, _, err := r.hits(ctx, "h.id = $1", []interface{}{id}, 0, 0)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrANPRHitNotFound
	}
	return &list[0], nil
}

type ANPRHitAlert struct {
	Title       string
	Description string
	Priority    int
	StationID   *uuid.UUID
}

type ANPRHitSighting struct {
	LookoutID uuid.UUID
	Location  string
	Latitude  *float64
	Longitude *float64
	SightedAt time.Time
	Details   string
}

// ReviewHit records the decision. On confirmation it raises the alert and, when
// given, records the lookout sighting — all in one transaction, so a confirmed
// hit never lacks its alert.
func (r *ANPRRepository) ReviewHit(ctx context.Context, id uuid.UUID, decision string, note *string, actor uuid.UUID,
	alert *ANPRHitAlert, sighting *ANPRHitSighting) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var submitter uuid.UUID
	err = tx.QueryRow(ctx, "SELECT status, submitted_by FROM anpr_watchlist_hits WHERE id = $1 FOR UPDATE", id).Scan(&status, &submitter)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrANPRHitNotFound
	}
	if err != nil {
		return err
	}
	if status != "PENDING" {
		return ErrANPRHitReviewed
	}
	if submitter == actor {
		return ErrANPRSelfReview
	}

	var alertID, sightingID *uuid.UUID
	if decision == "CONFIRMED" && alert != nil {
		aid := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO alerts (id, type, scope, title, description, issued_at, expires_at, issued_by, acknowledged,
			    priority, has_image, station_id, created_at, updated_at)
			VALUES ($1, 'BOLO', 'STATION', $2, $3, NOW(), NOW() + INTERVAL '24 hours', $4, FALSE, $5, FALSE, $6, NOW(), NOW())`,
			aid, alert.Title, alert.Description, actor, alert.Priority, alert.StationID); err != nil {
			return err
		}
		alertID = &aid
	}
	if decision == "CONFIRMED" && sighting != nil {
		sid := uuid.New()
		tag, err := tx.Exec(ctx, `
			INSERT INTO lookout_sightings (id, lookout_id, reported_by, location, latitude, longitude, sighted_at, details)
			SELECT $1, l.id, $3, $4, $5, $6, $7, $8 FROM lookouts l WHERE l.id = $2 AND l.status = 'ACTIVE'`,
			sid, sighting.LookoutID, actor, sighting.Location, sighting.Latitude, sighting.Longitude, sighting.SightedAt, sighting.Details)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			sightingID = &sid
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE anpr_watchlist_hits SET status = $2, reviewed_by = $3, reviewed_at = NOW(), review_note = $4,
		       alert_id = $5, sighting_id = $6
		WHERE id = $1`, id, decision, actor, note, alertID, sightingID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

/* ---------------------------------------------------------------------- map */

func (r *ANPRRepository) Map(ctx context.Context, from, to time.Time) (*models.ANPRMap, error) {
	m := &models.ANPRMap{From: from, To: to, Cameras: []models.ANPRMapCamera{}}
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.code, c.name, c.location, c.latitude, c.longitude,
		       COUNT(p.id),
		       COUNT(h.id) FILTER (WHERE h.status = 'PENDING'),
		       COUNT(h.id) FILTER (WHERE h.status = 'CONFIRMED'),
		       MAX(p.frame_time)
		FROM anpr_plate_reads p
		JOIN cameras c ON c.id = p.camera_id
		LEFT JOIN anpr_watchlist_hits h ON h.plate_read_id = p.id
		WHERE p.frame_time BETWEEN $1 AND $2 AND c.latitude IS NOT NULL
		GROUP BY c.id ORDER BY COUNT(p.id) DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c models.ANPRMapCamera
		if err := rows.Scan(&c.CameraID, &c.Code, &c.Name, &c.Location, &c.Latitude, &c.Longitude,
			&c.Reads, &c.PendingHits, &c.ConfirmedHits, &c.LastReadAt); err != nil {
			return nil, err
		}
		m.Cameras = append(m.Cameras, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM anpr_plate_reads p LEFT JOIN cameras c ON c.id = p.camera_id
		WHERE p.frame_time BETWEEN $1 AND $2 AND c.latitude IS NULL`, from, to).Scan(&m.ReadsWithoutCamera)
	return m, err
}

/* --------------------------------------------------------------- access log */

func (r *ANPRRepository) AccessLog(ctx context.Context, page, size int) ([]models.ANPRAccessEntry, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM anpr_access_log").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT l.id, COALESCE(u.name, ''), COALESCE(u.badge_number, ''), l.access_type, l.purpose, l.filters,
		       l.analysis_id, COALESCE(a.analysis_number, ''), l.result_count, COALESCE(host(l.ip_address), ''), l.accessed_at
		FROM anpr_access_log l
		LEFT JOIN users u ON u.id = l.actor_user_id
		LEFT JOIN anpr_analyses a ON a.id = l.analysis_id
		ORDER BY l.accessed_at DESC LIMIT $1 OFFSET $2`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.ANPRAccessEntry{}
	for rows.Next() {
		var e models.ANPRAccessEntry
		var filters []byte
		if err := rows.Scan(&e.ID, &e.ActorName, &e.ActorBadge, &e.AccessType, &e.Purpose, &filters,
			&e.AnalysisID, &e.AnalysisNumber, &e.ResultCount, &e.IPAddress, &e.AccessedAt); err != nil {
			return nil, 0, err
		}
		e.Filters = map[string]interface{}{}
		if err := json.Unmarshal(filters, &e.Filters); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

/* ------------------------------------------- Phase 06 plate-read register --- */

// AttachReadToIncident writes an ANPR read into a traffic incident's
// plate-read register, keeping its model version and confidence.
func (r *ANPRRepository) AttachReadToIncident(ctx context.Context, incidentID uuid.UUID, read *models.ANPRPlateRead,
	location, cameraRef, detail string, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO traffic_incident_plate_reads (id, incident_id, registration_number, read_at, location, camera_ref,
		    source, source_detail, created_by, anpr_plate_read_id, model_version, read_confidence)
		VALUES ($1, $2, $3, $4, $5, $6, 'ANPR_SYSTEM', $7, $8, $9, $10, $11)`,
		id, incidentID, read.RegistrationNumber, read.FrameTime, location, cameraRef, detail, actor,
		read.ID, read.ModelVersion, read.Confidence)
	switch anprPgCode(err) {
	case "23505":
		return uuid.Nil, ErrANPRReadAttached
	case "23503":
		return uuid.Nil, ErrTrafficIncidentNotFound
	}
	return id, err
}
