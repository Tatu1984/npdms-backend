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
	ErrFRNoActiveAuthorisation = errors.New("face recognition cannot be switched on without an active authorisation covering missing persons")
	ErrFRAuthorisationNotFound = errors.New("authorisation not found")
	ErrFRAuthorisationRevoked  = errors.New("authorisation is already revoked")
	ErrFRPhotoNotFound         = errors.New("photo not found on this report")
	ErrFREnrolmentNotFound     = errors.New("enrolment not found")
	ErrFRCandidateNotFound     = errors.New("face match candidate not found")
	ErrFRCandidateReviewed     = errors.New("this candidate has already been confirmed or rejected")
	ErrFRSelfReview            = errors.New("a candidate must be confirmed or rejected by an officer other than the one who submitted the footage")
	ErrFRCameraNotFound        = errors.New("camera not found in the camera register")
	ErrFRWrongAuthorisation    = errors.New("the authorisation in force does not cover this photo")
)

type FaceRecognitionRepository struct {
	db *pgxpool.Pool
}

func NewFaceRecognitionRepository(db *pgxpool.Pool) *FaceRecognitionRepository {
	return &FaceRecognitionRepository{db: db}
}

func pgConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

/* ----------------------------------------------------------- authorisations */

const frAuthSelect = `
	SELECT a.id, a.kind, a.order_reference, a.issuing_authority, to_char(a.order_date, 'YYYY-MM-DD'),
	       a.scope, a.scope_note, to_char(a.valid_from, 'YYYY-MM-DD'), to_char(a.valid_until, 'YYYY-MM-DD'),
	       a.recorded_by, COALESCE(u.name, ''), a.recorded_at, a.revoked_at, rv.name, a.revocation_reason,
	       (a.revoked_at IS NULL AND CURRENT_DATE BETWEEN a.valid_from AND a.valid_until)
	FROM fr_authorisations a
	LEFT JOIN users u ON u.id = a.recorded_by
	LEFT JOIN users rv ON rv.id = a.revoked_by
`

func scanFRAuth(row pgx.Row) (*models.FRAuthorisation, error) {
	var a models.FRAuthorisation
	if err := row.Scan(&a.ID, &a.Kind, &a.OrderReference, &a.IssuingAuthority, &a.OrderDate, &a.Scope, &a.ScopeNote,
		&a.ValidFrom, &a.ValidUntil, &a.RecordedBy, &a.RecordedByName, &a.RecordedAt, &a.RevokedAt, &a.RevokedByName,
		&a.RevocationReason, &a.Active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFRAuthorisationNotFound
		}
		return nil, err
	}
	if a.Kind == models.FRKindDemo {
		a.DemoLabel = models.FRDemoLabel
	}
	return &a, nil
}

// ActiveAuthorisation returns the active authorisation of a kind for missing
// persons, or nil.
func (r *FaceRecognitionRepository) ActiveAuthorisation(ctx context.Context, kind string) (*models.FRAuthorisation, error) {
	a, err := scanFRAuth(r.db.QueryRow(ctx, frAuthSelect+`
		WHERE a.id = fr_active_authorisation($1, $2)`, kind, models.FRScopeMissingPersons))
	if errors.Is(err, ErrFRAuthorisationNotFound) {
		return nil, nil
	}
	return a, err
}

func (r *FaceRecognitionRepository) ListAuthorisations(ctx context.Context) ([]models.FRAuthorisation, error) {
	rows, err := r.db.Query(ctx, frAuthSelect+" ORDER BY a.recorded_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FRAuthorisation{}
	for rows.Next() {
		a, err := scanFRAuth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *FaceRecognitionRepository) GetAuthorisation(ctx context.Context, id uuid.UUID) (*models.FRAuthorisation, error) {
	return scanFRAuth(r.db.QueryRow(ctx, frAuthSelect+" WHERE a.id = $1", id))
}

func nullIfBlank(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (r *FaceRecognitionRepository) CreateAuthorisation(ctx context.Context, req models.RecordFRAuthorisationRequest, actor uuid.UUID) (*models.FRAuthorisation, error) {
	id := uuid.New()
	var orderDate, validFrom interface{}
	if req.OrderDate != "" {
		orderDate = req.OrderDate
	}
	if req.ValidFrom != "" {
		validFrom = req.ValidFrom
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO fr_authorisations (id, kind, order_reference, issuing_authority, order_date, scope, scope_note,
		                               valid_from, valid_until, recorded_by)
		VALUES ($1, $2, $3, $4, $5::date, $6, $7, COALESCE($8::date, CURRENT_DATE), $9::date, $10)`,
		id, req.Kind, nullIfBlank(req.OrderReference), nullIfBlank(req.IssuingAuthority), orderDate, req.Scope,
		nullIfBlank(req.ScopeNote), validFrom, req.ValidUntil, actor)
	if err != nil {
		return nil, err
	}
	return r.GetAuthorisation(ctx, id)
}

// RevokeAuthorisation revokes one and, if no authorisation covering missing
// persons remains active, switches face recognition off in the same step.
func (r *FaceRecognitionRepository) RevokeAuthorisation(ctx context.Context, id, actor uuid.UUID, reason string) (switchedOff bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE fr_authorisations SET revoked_at = NOW(), revoked_by = $2, revocation_reason = $3
		WHERE id = $1 AND revoked_at IS NULL`, id, actor, reason)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetAuthorisation(ctx, id); err != nil {
			return false, err
		}
		return false, ErrFRAuthorisationRevoked
	}
	tag, err = tx.Exec(ctx, `UPDATE ai_module_switches SET enabled = FALSE, updated_by = $1, updated_at = NOW(),
		reason = 'Switched off automatically: no active authorisation remains after a revocation'
		WHERE module = $2 AND enabled AND fr_active_authorisation(NULL, $3) IS NULL`,
		actor, models.FRModule, models.FRScopeMissingPersons)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, tx.Commit(ctx)
}

/* ------------------------------------------------------------ module switch */

func (r *FaceRecognitionRepository) Switch(ctx context.Context) (enabled bool, cfg models.FRConfig, reason *string, updatedAt time.Time, updatedBy *string, err error) {
	var raw []byte
	err = r.db.QueryRow(ctx, `
		SELECT s.enabled, s.config, s.reason, s.updated_at, u.name
		FROM ai_module_switches s LEFT JOIN users u ON u.id = s.updated_by
		WHERE s.module = $1`, models.FRModule).Scan(&enabled, &raw, &reason, &updatedAt, &updatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, models.FRConfig{MatchThreshold: 0.5, SampleFps: 1, MergeWindowSeconds: 5}, nil, time.Time{}, nil, nil
	}
	if err != nil {
		return
	}
	cfg = models.FRConfig{MatchThreshold: 0.5, SampleFps: 1, MergeWindowSeconds: 5}
	_ = json.Unmarshal(raw, &cfg)
	return
}

func (r *FaceRecognitionRepository) SetSwitch(ctx context.Context, enabled bool, cfg models.FRConfig, reason string, actor uuid.UUID) error {
	raw, _ := json.Marshal(cfg)
	_, err := r.db.Exec(ctx, `
		INSERT INTO ai_module_switches (module, enabled, config, reason, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (module) DO UPDATE SET enabled = EXCLUDED.enabled, config = EXCLUDED.config,
		    reason = EXCLUDED.reason, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		models.FRModule, enabled, raw, reason, actor)
	if pgConstraint(err) == "fr_switch_requires_authorisation" {
		return ErrFRNoActiveAuthorisation
	}
	return err
}

/* ------------------------------------------------------------------- photos */

const frEnrolmentSelect = `
	SELECT e.id, e.report_id, e.photo_id, e.synthetic_photo_id, e.is_demo, e.authorisation_id, a.kind, e.status,
	       e.rejection_reason, e.rejection_message, e.quality, e.quality_score, e.face_box, e.model_version,
	       e.photo_sha256, e.face_crop_key IS NOT NULL, e.automatic, e.enrolled_by, COALESCE(u.name, ''),
	       e.created_at, e.retired_at, e.retire_reason
	FROM face_enrolments e
	JOIN fr_authorisations a ON a.id = e.authorisation_id
	LEFT JOIN users u ON u.id = e.enrolled_by
`

func scanEnrolment(row pgx.Row) (*models.FaceEnrolment, error) {
	var e models.FaceEnrolment
	var quality, box []byte
	if err := row.Scan(&e.ID, &e.ReportID, &e.PhotoID, &e.SyntheticPhotoID, &e.IsDemo, &e.AuthorisationID, &e.AuthorisationKind,
		&e.Status, &e.RejectionReason, &e.RejectionMessage, &quality, &e.QualityScore, &box, &e.ModelVersion,
		&e.PhotoSHA256, &e.HasFaceCrop, &e.Automatic, &e.EnrolledBy, &e.EnrolledByName, &e.CreatedAt, &e.RetiredAt, &e.RetireReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFREnrolmentNotFound
		}
		return nil, err
	}
	e.Quality, e.FaceBox = quality, box
	if e.IsDemo {
		e.DemoLabel = models.FRDemoLabel
	}
	return &e, nil
}

// ReportPhotos lists a report's enrollable photos — report photos and
// synthetic test photos — each with its latest enrolment attempt.
func (r *FaceRecognitionRepository) ReportPhotos(ctx context.Context, reportID uuid.UUID) ([]models.FRPhoto, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.report_id, 'REPORT_PHOTO', p.is_primary, p.source, p.provided_by_name, p.relationship,
		       p.consent_recorded, NULL::text, p.sha256, p.content_type, p.uploaded_by, p.created_at, p.storage_key
		FROM missing_person_photos p WHERE p.report_id = $1 AND p.retired_at IS NULL
		UNION ALL
		SELECT s.id, s.report_id, 'SYNTHETIC_TEST', FALSE, NULL, NULL, NULL, NULL, s.synthetic_source, s.sha256,
		       s.content_type, s.uploaded_by, s.created_at, s.object_key
		FROM fr_synthetic_photos s WHERE s.report_id = $1 AND s.retired_at IS NULL
		ORDER BY 4 DESC, 13`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.FRPhoto{}
	for rows.Next() {
		var p models.FRPhoto
		var consent *bool
		if err := rows.Scan(&p.ID, &p.ReportID, &p.Kind, &p.IsPrimary, &p.Source, &p.ProvidedByName, &p.Relationship,
			&consent, &p.SyntheticSource, &p.SHA256, &p.ContentType, &p.UploadedBy, &p.CreatedAt, &p.ObjectKey); err != nil {
			return nil, err
		}
		p.ConsentRecorded = consent
		if p.Kind == models.FRPhotoSynthetic {
			p.DemoLabel = models.FRDemoLabel
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		col := "photo_id"
		if out[i].Kind == models.FRPhotoSynthetic {
			col = "synthetic_photo_id"
		}
		e, err := scanEnrolment(r.db.QueryRow(ctx, frEnrolmentSelect+" WHERE e."+col+" = $1 ORDER BY e.created_at DESC LIMIT 1", out[i].ID))
		if err == nil {
			out[i].Enrolment = e
		} else if !errors.Is(err, ErrFREnrolmentNotFound) {
			return nil, err
		}
	}
	return out, nil
}

func (r *FaceRecognitionRepository) Photo(ctx context.Context, reportID uuid.UUID, kind string, photoID uuid.UUID) (*models.FRPhoto, error) {
	photos, err := r.ReportPhotos(ctx, reportID)
	if err != nil {
		return nil, err
	}
	for i := range photos {
		if photos[i].ID == photoID && (kind == "" || photos[i].Kind == kind) {
			return &photos[i], nil
		}
	}
	return nil, ErrFRPhotoNotFound
}

func (r *FaceRecognitionRepository) CreateSyntheticPhoto(ctx context.Context, reportID uuid.UUID, key, sha, contentType string, width, height int, source string, actor uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO fr_synthetic_photos (id, report_id, object_key, sha256, content_type, width, height, synthetic_source, declared_synthetic, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, 0), NULLIF($7, 0), $8, TRUE, $9)`,
		id, reportID, key, sha, contentType, width, height, source, actor)
	return id, err
}

type NewEnrolment struct {
	ReportID         uuid.UUID
	PhotoID          *uuid.UUID
	SyntheticPhotoID *uuid.UUID
	AuthorisationID  uuid.UUID
	Accepted         bool
	RejectionReason  string
	RejectionMessage string
	Quality          json.RawMessage
	QualityScore     *float64
	FaceBox          json.RawMessage
	Embedding        []float32
	ModelVersion     string
	PhotoSHA256      string
	FaceCropKey      string
	Automatic        bool
	EnrolledBy       uuid.UUID
}

// InsertEnrolment records an attempt. An accepted enrolment replaces the
// previous one for the same photo and model: its embedding is removed.
func (r *FaceRecognitionRepository) InsertEnrolment(ctx context.Context, n NewEnrolment) (*models.FaceEnrolment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if n.Accepted {
		if _, err := tx.Exec(ctx, `
			UPDATE face_enrolments SET status = 'RETIRED', embedding = NULL, retired_at = NOW(), retired_by = $3,
			       retire_reason = 'Replaced by a new enrolment of the same photo'
			WHERE status = 'ENROLLED' AND model_version = $4
			  AND (photo_id = $1 OR synthetic_photo_id = $2)`, n.PhotoID, n.SyntheticPhotoID, n.EnrolledBy, n.ModelVersion); err != nil {
			return nil, err
		}
	}
	id := uuid.New()
	status := "REJECTED"
	var embedding interface{}
	var dim interface{}
	if n.Accepted {
		status = "ENROLLED"
		embedding = n.Embedding
		dim = len(n.Embedding)
	}
	quality := []byte(n.Quality)
	if len(quality) == 0 {
		quality = nil
	}
	box := []byte(n.FaceBox)
	if len(box) == 0 {
		box = nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO face_enrolments (id, report_id, photo_id, synthetic_photo_id, authorisation_id, status,
		       rejection_reason, rejection_message, quality, quality_score, face_box, embedding, embedding_dim,
		       model_version, photo_sha256, face_crop_key, automatic, enrolled_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		id, n.ReportID, n.PhotoID, n.SyntheticPhotoID, n.AuthorisationID, status,
		nullIfBlank(n.RejectionReason), nullIfBlank(n.RejectionMessage), quality, n.QualityScore, box, embedding, dim,
		n.ModelVersion, n.PhotoSHA256, nullIfBlank(n.FaceCropKey), n.Automatic, n.EnrolledBy)
	if err != nil {
		switch pgConstraint(err) {
		case "fe_real_photo_needs_order", "fe_synthetic_photo_needs_demo", "fe_authorisation_active":
			return nil, fmt.Errorf("%w: %v", ErrFRWrongAuthorisation, err)
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.Enrolment(ctx, id)
}

func (r *FaceRecognitionRepository) Enrolment(ctx context.Context, id uuid.UUID) (*models.FaceEnrolment, error) {
	return scanEnrolment(r.db.QueryRow(ctx, frEnrolmentSelect+" WHERE e.id = $1", id))
}

func (r *FaceRecognitionRepository) EnrolmentCropKey(ctx context.Context, id uuid.UUID) (reportID uuid.UUID, key string, err error) {
	var k *string
	err = r.db.QueryRow(ctx, `SELECT report_id, face_crop_key FROM face_enrolments WHERE id = $1`, id).Scan(&reportID, &k)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrFREnrolmentNotFound
	}
	if k != nil {
		key = *k
	}
	return
}

func (r *FaceRecognitionRepository) RetireEnrolment(ctx context.Context, reportID, id, actor uuid.UUID, reason string) (*models.FaceEnrolment, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE face_enrolments SET status = 'RETIRED', embedding = NULL, retired_at = NOW(), retired_by = $3, retire_reason = $4
		WHERE id = $1 AND report_id = $2 AND status = 'ENROLLED'`, id, reportID, actor, reason)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrFREnrolmentNotFound
	}
	return r.Enrolment(ctx, id)
}

// RetireStaleEnrolments removes the templates of reports that are no longer
// open and of photos that were retired, so a traced person's face, or a photo
// withdrawn from the report, is not kept for matching.
func (r *FaceRecognitionRepository) RetireStaleEnrolments(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE face_enrolments e SET status = 'RETIRED', embedding = NULL, retired_at = NOW(), retired_by = e.enrolled_by,
		       retire_reason = 'Report closed: template removed'
		FROM missing_person_reports m
		WHERE m.id = e.report_id AND e.status = 'ENROLLED' AND m.status NOT IN ('REPORTED', 'SEARCHING')`)
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	tag, err = r.db.Exec(ctx, `
		UPDATE face_enrolments e SET status = 'RETIRED', embedding = NULL, retired_at = NOW(), retired_by = e.enrolled_by,
		       retire_reason = 'Photo retired from the report: template removed'
		WHERE e.status = 'ENROLLED' AND (
		      EXISTS (SELECT 1 FROM missing_person_photos p WHERE p.id = e.photo_id AND p.retired_at IS NOT NULL)
		   OR EXISTS (SELECT 1 FROM fr_synthetic_photos sp WHERE sp.id = e.synthetic_photo_id AND sp.retired_at IS NOT NULL))`)
	if err != nil {
		return n, err
	}
	return n + tag.RowsAffected(), nil
}

type GalleryEntry struct {
	EnrolmentID      uuid.UUID
	ReportID         uuid.UUID
	PhotoID          *uuid.UUID
	SyntheticPhotoID *uuid.UUID
	Embedding        []float32
}

// Gallery returns every enrolled face of an open report for the model, demo
// and real kept apart.
func (r *FaceRecognitionRepository) Gallery(ctx context.Context, demo bool, modelVersion string) ([]GalleryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.report_id, e.photo_id, e.synthetic_photo_id, e.embedding
		FROM face_enrolments e JOIN missing_person_reports m ON m.id = e.report_id
		LEFT JOIN missing_person_photos p ON p.id = e.photo_id
		LEFT JOIN fr_synthetic_photos sp ON sp.id = e.synthetic_photo_id
		WHERE e.status = 'ENROLLED' AND e.is_demo = $1 AND e.model_version = $2
		  AND m.status IN ('REPORTED', 'SEARCHING')
		  AND p.retired_at IS NULL AND sp.retired_at IS NULL`, demo, modelVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GalleryEntry{}
	for rows.Next() {
		var g GalleryEntry
		if err := rows.Scan(&g.EnrolmentID, &g.ReportID, &g.PhotoID, &g.SyntheticPhotoID, &g.Embedding); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// PhotosAwaitingEnrolment: primary report photos on open reports with no
// enrolment attempt for the model yet. Used for automatic enrolment.
func (r *FaceRecognitionRepository) PhotosAwaitingEnrolment(ctx context.Context, modelVersion string, limit int) ([]models.FRPhoto, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.report_id FROM missing_person_photos p
		JOIN missing_person_reports m ON m.id = p.report_id
		WHERE p.is_primary AND p.retired_at IS NULL AND p.uploaded_by IS NOT NULL
		  AND m.status IN ('REPORTED', 'SEARCHING')
		  AND NOT EXISTS (SELECT 1 FROM face_enrolments e WHERE e.photo_id = p.id AND e.model_version = $1)
		ORDER BY p.created_at LIMIT $2`, modelVersion, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ref struct{ id, report uuid.UUID }
	refs := []ref{}
	for rows.Next() {
		var x ref
		if err := rows.Scan(&x.id, &x.report); err != nil {
			return nil, err
		}
		refs = append(refs, x)
	}
	rows.Close()
	out := []models.FRPhoto{}
	for _, x := range refs {
		p, err := r.Photo(ctx, x.report, models.FRPhotoReport, x.id)
		if err == nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

/* ----------------------------------------------------------------- cameras */

type FRCamera struct {
	ID        uuid.UUID
	Code      string
	Name      string
	Location  string
	Latitude  *float64
	Longitude *float64
	Status    string
}

func (r *FaceRecognitionRepository) Camera(ctx context.Context, id uuid.UUID) (*FRCamera, error) {
	var c FRCamera
	err := r.db.QueryRow(ctx, `SELECT id, code, name, location, latitude, longitude, status FROM cameras WHERE id = $1`, id).
		Scan(&c.ID, &c.Code, &c.Name, &c.Location, &c.Latitude, &c.Longitude, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFRCameraNotFound
	}
	return &c, err
}

/* ---------------------------------------------------------------- searches */

type NewSearch struct {
	IsDemo           bool
	AuthorisationID  uuid.UUID
	SourceMedia      string
	CameraID         *uuid.UUID
	Purpose          string
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
	MediaSHA256      string
	RecordedAt       time.Time
	LocationText     *string
	Latitude         *float64
	Longitude        *float64
	Threshold        float64
	SampleFps        *float64
	SubmittedBy      uuid.UUID
	ClientIP         string
}

// CreateSearch records the search and its purpose before any matching runs.
func (r *FaceRecognitionRepository) CreateSearch(ctx context.Context, n NewSearch) (uuid.UUID, error) {
	id := uuid.New()
	_, err := r.db.Exec(ctx, `
		INSERT INTO face_match_searches (id, is_demo, authorisation_id, source_media, camera_id, purpose, original_filename,
		       content_type, size_bytes, media_sha256, recorded_at, location_text, latitude, longitude, threshold_used,
		       sample_fps, status, submitted_by, client_ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, 'RUNNING', $17, $18)`,
		id, n.IsDemo, n.AuthorisationID, n.SourceMedia, n.CameraID, n.Purpose, nullIfBlank(n.OriginalFilename),
		nullIfBlank(n.ContentType), n.SizeBytes, n.MediaSHA256, n.RecordedAt, n.LocationText, n.Latitude, n.Longitude,
		n.Threshold, n.SampleFps, n.SubmittedBy, nullIfBlank(n.ClientIP))
	return id, err
}

func (r *FaceRecognitionRepository) FailSearch(ctx context.Context, id uuid.UUID, gallerySize int, message string, duration time.Duration) error {
	_, err := r.db.Exec(ctx, `UPDATE face_match_searches SET status = 'FAILED', error = $2, gallery_size = $3, duration_ms = $4
		WHERE id = $1`, id, message, gallerySize, int(duration.Milliseconds()))
	return err
}

type SearchOutcome struct {
	ModelVersion   string
	GallerySize    int
	FramesAnalysed int
	FacesSeen      int
	FacesCompared  int
	MediaKey       *string
	Duration       time.Duration
}

type NewCandidate struct {
	ReportID         uuid.UUID
	EnrolmentID      uuid.UUID
	PhotoID          *uuid.UUID
	SyntheticPhotoID *uuid.UUID
	MediaKey         string
	MediaSHA256      string
	CropKey          string
	FrameOffsetMs    *int
	FrameTime        time.Time
	Box              models.BoundingBox
	FrameWidth       int
	FrameHeight      int
	DetectionScore   float64
	Quality          json.RawMessage
	Similarity       float64
}

// CompleteSearch writes the candidates and closes the search in one transaction.
func (r *FaceRecognitionRepository) CompleteSearch(ctx context.Context, searchID uuid.UUID, o SearchOutcome, cands []NewCandidate) ([]uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	ids := make([]uuid.UUID, 0, len(cands))
	for _, c := range cands {
		id := uuid.New()
		quality := []byte(c.Quality)
		if len(quality) == 0 {
			quality = nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO face_match_candidates (id, search_id, report_id, enrolment_id, photo_id, synthetic_photo_id, is_demo,
			       camera_id, source_media, media_key, media_sha256, crop_key, frame_offset_ms, frame_time,
			       location_text, latitude, longitude, bbox_x, bbox_y, bbox_w, bbox_h, frame_width, frame_height,
			       detection_score, quality, similarity, model_version, threshold_used, submitted_by)
			SELECT $1, s.id, $2, $3, $4, $5, s.is_demo, s.camera_id, s.source_media, $6, $7, $8, $9, $10,
			       s.location_text, s.latitude, s.longitude, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			       s.threshold_used, s.submitted_by
			FROM face_match_searches s WHERE s.id = $21`,
			id, c.ReportID, c.EnrolmentID, c.PhotoID, c.SyntheticPhotoID, c.MediaKey, c.MediaSHA256, c.CropKey,
			c.FrameOffsetMs, c.FrameTime, c.Box.X, c.Box.Y, c.Box.W, c.Box.H, c.FrameWidth, c.FrameHeight,
			c.DetectionScore, quality, c.Similarity, o.ModelVersion, searchID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE face_match_searches SET status = 'COMPLETED', model_version = $2, gallery_size = $3, frames_analysed = $4,
		       faces_seen = $5, faces_compared = $6, candidates_created = $7, media_key = $8, duration_ms = $9
		WHERE id = $1`, searchID, o.ModelVersion, o.GallerySize, o.FramesAnalysed, o.FacesSeen, o.FacesCompared,
		len(cands), o.MediaKey, int(o.Duration.Milliseconds())); err != nil {
		return nil, err
	}
	return ids, tx.Commit(ctx)
}

const frSearchSelect = `
	SELECT s.id, s.is_demo, s.authorisation_id, s.source_media, s.camera_id, c.name, s.purpose, s.original_filename,
	       s.size_bytes, s.media_sha256, s.media_key IS NOT NULL, s.recorded_at, s.location_text, s.latitude, s.longitude,
	       s.threshold_used, s.model_version, s.sample_fps, s.gallery_size, s.frames_analysed, s.faces_seen,
	       s.faces_compared, s.candidates_created, s.status, s.error, s.duration_ms, s.submitted_by, COALESCE(u.name, ''),
	       s.submitted_at
	FROM face_match_searches s
	LEFT JOIN cameras c ON c.id = s.camera_id
	LEFT JOIN users u ON u.id = s.submitted_by
`

func scanSearch(row pgx.Row) (*models.FaceMatchSearch, error) {
	var s models.FaceMatchSearch
	var threshold float32
	var fps *float32
	if err := row.Scan(&s.ID, &s.IsDemo, &s.AuthorisationID, &s.SourceMedia, &s.CameraID, &s.CameraName, &s.Purpose,
		&s.OriginalFilename, &s.SizeBytes, &s.MediaSHA256, &s.MediaRetained, &s.RecordedAt, &s.LocationText, &s.Latitude,
		&s.Longitude, &threshold, &s.ModelVersion, &fps, &s.GallerySize, &s.FramesAnalysed, &s.FacesSeen,
		&s.FacesCompared, &s.CandidatesCreated, &s.Status, &s.Error, &s.DurationMs, &s.SubmittedBy, &s.SubmittedByName,
		&s.SubmittedAt); err != nil {
		return nil, err
	}
	s.ThresholdUsed = roundFloat(float64(threshold))
	if fps != nil {
		v := roundFloat(float64(*fps))
		s.SampleFps = &v
	}
	if s.IsDemo {
		s.DemoLabel = models.FRDemoLabel
	}
	return &s, nil
}

func roundFloat(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}

func (r *FaceRecognitionRepository) Search(ctx context.Context, id uuid.UUID) (*models.FaceMatchSearch, error) {
	return scanSearch(r.db.QueryRow(ctx, frSearchSelect+" WHERE s.id = $1", id))
}

func (r *FaceRecognitionRepository) ListSearches(ctx context.Context, page, size int) ([]models.FaceMatchSearch, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM face_match_searches`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, frSearchSelect+" ORDER BY s.submitted_at DESC LIMIT $1 OFFSET $2", size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.FaceMatchSearch{}
	for rows.Next() {
		s, err := scanSearch(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

/* -------------------------------------------------------------- candidates */

const frCandidateSelect = `
	SELECT x.id, x.search_id, x.report_id, m.report_number, m.person_name, m.status, x.enrolment_id, x.photo_id,
	       x.synthetic_photo_id, x.is_demo, x.camera_id, c.name, c.code, x.source_media, s.purpose, s.original_filename,
	       x.media_sha256, s.media_sha256, x.frame_offset_ms, x.frame_time, x.location_text, x.latitude, x.longitude,
	       x.bbox_x, x.bbox_y, x.bbox_w, x.bbox_h, x.frame_width, x.frame_height, x.detection_score, x.quality,
	       x.similarity, x.model_version, x.threshold_used, x.status, x.submitted_by, COALESCE(su.name, ''),
	       x.reviewed_by, ru.name, x.reviewed_at, x.review_note, x.sighting_id, x.created_at, x.media_key, x.crop_key,
	       m.vulnerabilities, m.assigned_to, m.registered_by, m.search_started_by
	FROM face_match_candidates x
	JOIN face_match_searches s ON s.id = x.search_id
	JOIN missing_person_reports m ON m.id = x.report_id
	LEFT JOIN cameras c ON c.id = x.camera_id
	LEFT JOIN users su ON su.id = x.submitted_by
	LEFT JOIN users ru ON ru.id = x.reviewed_by
`

// CandidateRow carries what the service needs to apply the child-record rule.
type CandidateRow struct {
	models.FaceMatchCandidate
	Vulnerabilities []string
	AssignedTo      *uuid.UUID
	RegisteredBy    *uuid.UUID
	SearchStartedBy *uuid.UUID
}

func scanCandidate(row pgx.Row) (*CandidateRow, error) {
	var c CandidateRow
	var det, sim, th *float32
	var quality []byte
	if err := row.Scan(&c.ID, &c.SearchID, &c.ReportID, &c.ReportNumber, &c.PersonName, &c.ReportStatus, &c.EnrolmentID,
		&c.PhotoID, &c.SyntheticPhotoID, &c.IsDemo, &c.CameraID, &c.CameraName, &c.CameraCode, &c.SourceMedia, &c.Purpose,
		&c.OriginalFilename, &c.MediaSHA256, &c.SearchMediaSHA, &c.FrameOffsetMs, &c.FrameTime, &c.LocationText,
		&c.Latitude, &c.Longitude, &c.BoundingBox.X, &c.BoundingBox.Y, &c.BoundingBox.W, &c.BoundingBox.H,
		&c.FrameWidth, &c.FrameHeight, &det, &quality, &sim, &c.ModelVersion, &th, &c.Status, &c.SubmittedBy,
		&c.SubmittedByName, &c.ReviewedBy, &c.ReviewedByName, &c.ReviewedAt, &c.ReviewNote, &c.SightingID, &c.CreatedAt,
		&c.MediaKey, &c.CropKey, &c.Vulnerabilities, &c.AssignedTo, &c.RegisteredBy, &c.SearchStartedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFRCandidateNotFound
		}
		return nil, err
	}
	c.Quality = quality
	if det != nil {
		v := roundFloat(float64(*det))
		c.DetectionScore = &v
	}
	if sim != nil {
		c.Similarity = roundFloat(float64(*sim))
	}
	if th != nil {
		c.ThresholdUsed = roundFloat(float64(*th))
	}
	if c.IsDemo {
		c.DemoLabel = models.FRDemoLabel
	}
	return &c, nil
}

type CandidateFilter struct {
	ReportID *uuid.UUID
	SearchID *uuid.UUID
	Status   string
	Demo     *bool
	Page     int
	PageSize int
}

func (r *FaceRecognitionRepository) ListCandidates(ctx context.Context, f CandidateFilter) ([]CandidateRow, int64, error) {
	where := []string{"TRUE"}
	args := []interface{}{}
	add := func(cond string, v interface{}) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ReportID != nil {
		add("x.report_id = $%d", *f.ReportID)
	}
	if f.SearchID != nil {
		add("x.search_id = $%d", *f.SearchID)
	}
	if f.Status != "" {
		add("x.status = $%d", f.Status)
	}
	if f.Demo != nil {
		add("x.is_demo = $%d", *f.Demo)
	}
	cond := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM face_match_candidates x WHERE "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx, frCandidateSelect+" WHERE "+cond+fmt.Sprintf(
		" ORDER BY (x.status = 'PENDING') DESC, x.created_at DESC, x.similarity DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []CandidateRow{}
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

func (r *FaceRecognitionRepository) Candidate(ctx context.Context, id uuid.UUID) (*CandidateRow, error) {
	return scanCandidate(r.db.QueryRow(ctx, frCandidateSelect+" WHERE x.id = $1", id))
}

// ConfirmCandidate creates a VERIFIED Phase 04 sighting — reported by the
// officer who submitted the footage, verified by the confirming officer — and
// marks the candidate CONFIRMED, in one transaction.
func (r *FaceRecognitionRepository) ConfirmCandidate(ctx context.Context, id, actor uuid.UUID, location string, lat, lng *float64,
	details string, note *string) (*models.MissingPersonSighting, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var reportID, submitter uuid.UUID
	var status, reportStatus string
	var frameTime, lastSeen time.Time
	err = tx.QueryRow(ctx, `
		SELECT x.report_id, x.submitted_by, x.status, m.status, x.frame_time, m.last_seen_date
		FROM face_match_candidates x JOIN missing_person_reports m ON m.id = x.report_id
		WHERE x.id = $1 FOR UPDATE OF x`, id).Scan(&reportID, &submitter, &status, &reportStatus, &frameTime, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFRCandidateNotFound
	}
	if err != nil {
		return nil, err
	}
	switch {
	case status != "PENDING":
		return nil, ErrFRCandidateReviewed
	case submitter == actor:
		return nil, ErrFRSelfReview
	case reportStatus != "REPORTED" && reportStatus != "SEARCHING":
		return nil, ErrMissingPersonNotOpen
	}
	sid := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO missing_person_sightings (id, report_id, reported_by, source, location, latitude, longitude, sighted_at,
		                                      details, decision, decided_by, decided_at, decision_note)
		VALUES ($1, $2, $3, 'CCTV_REVIEW', $4, $5, $6, $7, $8, 'VERIFIED', $9, NOW(), $10)`,
		sid, reportID, submitter, location, lat, lng, frameTime, details, actor, note); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE face_match_candidates SET status = 'CONFIRMED', reviewed_by = $2, reviewed_at = NOW(), review_note = $3, sighting_id = $4
		WHERE id = $1`, id, actor, note, sid); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return scanMissingSighting(r.db.QueryRow(ctx, missingSightingSelect+" WHERE x.id = $1", sid))
}

func (r *FaceRecognitionRepository) RejectCandidate(ctx context.Context, id, actor uuid.UUID, note string) error {
	var submitter uuid.UUID
	var status string
	err := r.db.QueryRow(ctx, `SELECT submitted_by, status FROM face_match_candidates WHERE id = $1`, id).Scan(&submitter, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFRCandidateNotFound
	}
	if err != nil {
		return err
	}
	if status != "PENDING" {
		return ErrFRCandidateReviewed
	}
	if submitter == actor {
		return ErrFRSelfReview
	}
	tag, err := r.db.Exec(ctx, `UPDATE face_match_candidates SET status = 'REJECTED', reviewed_by = $2, reviewed_at = NOW(), review_note = $3
		WHERE id = $1 AND status = 'PENDING'`, id, actor, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFRCandidateReviewed
	}
	return nil
}
