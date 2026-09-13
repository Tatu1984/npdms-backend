package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

type UploadRepository struct {
	db *pgxpool.Pool
}

func NewUploadRepository(db *pgxpool.Pool) *UploadRepository {
	return &UploadRepository{db: db}
}

func (r *UploadRepository) Create(ctx context.Context, upload *models.Upload) error {
	query := `
		INSERT INTO uploads (
			object_key, original_filename, file_size, content_type, category,
			bucket_name, entity_type, entity_id, uploaded_by, public_url
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, uploaded_at, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		upload.ObjectKey,
		upload.OriginalFilename,
		upload.FileSize,
		upload.ContentType,
		upload.Category,
		upload.BucketName,
		upload.EntityType,
		upload.EntityID,
		upload.UploadedBy,
		upload.PublicURL,
	).Scan(&upload.ID, &upload.UploadedAt, &upload.CreatedAt, &upload.UpdatedAt)
}

func (r *UploadRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Upload, error) {
	query := `
		SELECT id, object_key, original_filename, file_size, content_type, category,
			bucket_name, entity_type, entity_id, uploaded_by, uploaded_at, public_url,
			is_deleted, deleted_at, deleted_by, created_at, updated_at
		FROM uploads
		WHERE id = $1 AND is_deleted = false`

	var upload models.Upload
	err := r.db.QueryRow(ctx, query, id).Scan(
		&upload.ID, &upload.ObjectKey, &upload.OriginalFilename, &upload.FileSize,
		&upload.ContentType, &upload.Category, &upload.BucketName, &upload.EntityType,
		&upload.EntityID, &upload.UploadedBy, &upload.UploadedAt, &upload.PublicURL,
		&upload.IsDeleted, &upload.DeletedAt, &upload.DeletedBy, &upload.CreatedAt, &upload.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &upload, nil
}

func (r *UploadRepository) GetByObjectKey(ctx context.Context, objectKey string) (*models.Upload, error) {
	query := `
		SELECT id, object_key, original_filename, file_size, content_type, category,
			bucket_name, entity_type, entity_id, uploaded_by, uploaded_at, public_url,
			is_deleted, deleted_at, deleted_by, created_at, updated_at
		FROM uploads
		WHERE object_key = $1 AND is_deleted = false`

	var upload models.Upload
	err := r.db.QueryRow(ctx, query, objectKey).Scan(
		&upload.ID, &upload.ObjectKey, &upload.OriginalFilename, &upload.FileSize,
		&upload.ContentType, &upload.Category, &upload.BucketName, &upload.EntityType,
		&upload.EntityID, &upload.UploadedBy, &upload.UploadedAt, &upload.PublicURL,
		&upload.IsDeleted, &upload.DeletedAt, &upload.DeletedBy, &upload.CreatedAt, &upload.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &upload, nil
}

func (r *UploadRepository) List(ctx context.Context, params models.UploadListParams) ([]models.Upload, int, error) {
	baseQuery := `FROM uploads WHERE is_deleted = false`
	args := []interface{}{}
	argCount := 0

	if params.EntityType != nil && params.EntityID != nil {
		argCount++
		baseQuery += fmt.Sprintf(" AND entity_type = $%d", argCount)
		args = append(args, *params.EntityType)
		argCount++
		baseQuery += fmt.Sprintf(" AND entity_id = $%d", argCount)
		args = append(args, *params.EntityID)
	}

	if params.Category != nil {
		argCount++
		baseQuery += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, *params.Category)
	}

	if params.UploadedBy != nil {
		argCount++
		baseQuery += fmt.Sprintf(" AND uploaded_by = $%d", argCount)
		args = append(args, *params.UploadedBy)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get uploads with pagination
	query := `SELECT id, object_key, original_filename, file_size, content_type, category,
		bucket_name, entity_type, entity_id, uploaded_by, uploaded_at, public_url,
		is_deleted, deleted_at, deleted_by, created_at, updated_at ` + baseQuery +
		" ORDER BY created_at DESC"

	if params.Limit > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, params.Limit)
	}
	if params.Offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, params.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var uploads []models.Upload
	for rows.Next() {
		var upload models.Upload
		err := rows.Scan(
			&upload.ID, &upload.ObjectKey, &upload.OriginalFilename, &upload.FileSize,
			&upload.ContentType, &upload.Category, &upload.BucketName, &upload.EntityType,
			&upload.EntityID, &upload.UploadedBy, &upload.UploadedAt, &upload.PublicURL,
			&upload.IsDeleted, &upload.DeletedAt, &upload.DeletedBy, &upload.CreatedAt, &upload.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		uploads = append(uploads, upload)
	}

	return uploads, total, nil
}

func (r *UploadRepository) ListByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]models.Upload, error) {
	query := `
		SELECT id, object_key, original_filename, file_size, content_type, category,
			bucket_name, entity_type, entity_id, uploaded_by, uploaded_at, public_url,
			is_deleted, deleted_at, deleted_by, created_at, updated_at
		FROM uploads
		WHERE entity_type = $1 AND entity_id = $2 AND is_deleted = false
		ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []models.Upload
	for rows.Next() {
		var upload models.Upload
		err := rows.Scan(
			&upload.ID, &upload.ObjectKey, &upload.OriginalFilename, &upload.FileSize,
			&upload.ContentType, &upload.Category, &upload.BucketName, &upload.EntityType,
			&upload.EntityID, &upload.UploadedBy, &upload.UploadedAt, &upload.PublicURL,
			&upload.IsDeleted, &upload.DeletedAt, &upload.DeletedBy, &upload.CreatedAt, &upload.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}

	return uploads, nil
}

func (r *UploadRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	query := `
		UPDATE uploads
		SET is_deleted = true, deleted_at = NOW(), deleted_by = $2
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, deletedBy)
	return err
}

func (r *UploadRepository) LinkToEntity(ctx context.Context, id uuid.UUID, entityType string, entityID uuid.UUID) error {
	query := `
		UPDATE uploads
		SET entity_type = $2, entity_id = $3
		WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id, entityType, entityID)
	return err
}
