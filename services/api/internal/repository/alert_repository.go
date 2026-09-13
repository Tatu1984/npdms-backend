package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/npdms/api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AlertRepository struct {
	db *pgxpool.Pool
}

func NewAlertRepository(db *pgxpool.Pool) *AlertRepository {
	return &AlertRepository{db: db}
}

type AlertFilter struct {
	Type          *models.AlertType
	Scope         *models.AlertScope
	Acknowledged  *bool
	Search        string
	StationID     *uuid.UUID
	Page          int
	PageSize      int
}

func (r *AlertRepository) List(ctx context.Context, filter AlertFilter) ([]models.Alert, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if filter.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.type = $%d", argIndex))
		args = append(args, *filter.Type)
		argIndex++
	}

	if filter.Scope != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.scope = $%d", argIndex))
		args = append(args, *filter.Scope)
		argIndex++
	}

	if filter.Acknowledged != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.acknowledged = $%d", argIndex))
		args = append(args, *filter.Acknowledged)
		argIndex++
	}

	if filter.StationID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.station_id = $%d", argIndex))
		args = append(args, *filter.StationID)
		argIndex++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(a.title ILIKE $%d OR a.description ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM alerts a WHERE %s", whereClause)
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT
			a.id, a.type, a.scope, a.title, a.description,
			a.issued_at, a.expires_at, a.issued_by, a.acknowledged,
			a.acknowledged_by, a.acknowledged_at, a.priority, a.has_image, a.station_id,
			a.created_at, a.updated_at,
			u1.name as issued_by_name, u2.name as acknowledged_by_name
		FROM alerts a
		LEFT JOIN users u1 ON a.issued_by = u1.id
		LEFT JOIN users u2 ON a.acknowledged_by = u2.id
		WHERE %s
		ORDER BY a.priority ASC, a.issued_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	alerts := []models.Alert{}
	for rows.Next() {
		var a models.Alert
		err := rows.Scan(
			&a.ID, &a.Type, &a.Scope, &a.Title, &a.Description,
			&a.IssuedAt, &a.ExpiresAt, &a.IssuedBy, &a.Acknowledged,
			&a.AcknowledgedBy, &a.AcknowledgedAt, &a.Priority, &a.HasImage, &a.StationID,
			&a.CreatedAt, &a.UpdatedAt,
			&a.IssuedByName, &a.AcknowledgedByName,
		)
		if err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, a)
	}

	return alerts, total, nil
}

func (r *AlertRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	query := `
		SELECT
			a.id, a.type, a.scope, a.title, a.description,
			a.issued_at, a.expires_at, a.issued_by, a.acknowledged,
			a.acknowledged_by, a.acknowledged_at, a.priority, a.has_image, a.station_id,
			a.created_at, a.updated_at,
			u1.name as issued_by_name, u2.name as acknowledged_by_name
		FROM alerts a
		LEFT JOIN users u1 ON a.issued_by = u1.id
		LEFT JOIN users u2 ON a.acknowledged_by = u2.id
		WHERE a.id = $1
	`

	var a models.Alert
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.Type, &a.Scope, &a.Title, &a.Description,
		&a.IssuedAt, &a.ExpiresAt, &a.IssuedBy, &a.Acknowledged,
		&a.AcknowledgedBy, &a.AcknowledgedAt, &a.Priority, &a.HasImage, &a.StationID,
		&a.CreatedAt, &a.UpdatedAt,
		&a.IssuedByName, &a.AcknowledgedByName,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("alert not found")
		}
		return nil, err
	}

	return &a, nil
}

func (r *AlertRepository) Create(ctx context.Context, alert *models.Alert) error {
	query := `
		INSERT INTO alerts (
			id, type, scope, title, description, issued_at, expires_at,
			issued_by, acknowledged, priority, has_image, station_id,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
	`

	alert.ID = uuid.New()
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()
	alert.IssuedAt = time.Now()

	_, err := r.db.Exec(ctx, query,
		alert.ID, alert.Type, alert.Scope, alert.Title, alert.Description, alert.IssuedAt, alert.ExpiresAt,
		alert.IssuedBy, alert.Acknowledged, alert.Priority, alert.HasImage, alert.StationID,
		alert.CreatedAt, alert.UpdatedAt,
	)

	return err
}

func (r *AlertRepository) Update(ctx context.Context, alert *models.Alert) error {
	query := `
		UPDATE alerts SET
			type = $2, scope = $3, title = $4, description = $5,
			expires_at = $6, priority = $7, has_image = $8,
			updated_at = $9
		WHERE id = $1
	`

	alert.UpdatedAt = time.Now()

	result, err := r.db.Exec(ctx, query,
		alert.ID, alert.Type, alert.Scope, alert.Title, alert.Description,
		alert.ExpiresAt, alert.Priority, alert.HasImage,
		alert.UpdatedAt,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found")
	}

	return nil
}

func (r *AlertRepository) Acknowledge(ctx context.Context, id, acknowledgedBy uuid.UUID) error {
	query := `
		UPDATE alerts SET
			acknowledged = true,
			acknowledged_by = $2,
			acknowledged_at = $3,
			updated_at = $4
		WHERE id = $1
	`

	now := time.Now()

	result, err := r.db.Exec(ctx, query, id, acknowledgedBy, now, now)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found")
	}

	return nil
}

func (r *AlertRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := "DELETE FROM alerts WHERE id = $1"
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found")
	}

	return nil
}

func (r *AlertRepository) GetActiveAlerts(ctx context.Context) ([]models.Alert, error) {
	query := `
		SELECT
			a.id, a.type, a.scope, a.title, a.description,
			a.issued_at, a.expires_at, a.issued_by, a.acknowledged,
			a.acknowledged_by, a.acknowledged_at, a.priority, a.has_image, a.station_id,
			a.created_at, a.updated_at,
			u1.name as issued_by_name, u2.name as acknowledged_by_name
		FROM alerts a
		LEFT JOIN users u1 ON a.issued_by = u1.id
		LEFT JOIN users u2 ON a.acknowledged_by = u2.id
		WHERE a.expires_at > NOW()
		ORDER BY a.priority ASC, a.issued_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []models.Alert{}
	for rows.Next() {
		var a models.Alert
		err := rows.Scan(
			&a.ID, &a.Type, &a.Scope, &a.Title, &a.Description,
			&a.IssuedAt, &a.ExpiresAt, &a.IssuedBy, &a.Acknowledged,
			&a.AcknowledgedBy, &a.AcknowledgedAt, &a.Priority, &a.HasImage, &a.StationID,
			&a.CreatedAt, &a.UpdatedAt,
			&a.IssuedByName, &a.AcknowledgedByName,
		)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}

	return alerts, nil
}

func (r *AlertRepository) GetUnacknowledgedAlerts(ctx context.Context) ([]models.Alert, error) {
	query := `
		SELECT
			a.id, a.type, a.scope, a.title, a.description,
			a.issued_at, a.expires_at, a.issued_by, a.acknowledged,
			a.acknowledged_by, a.acknowledged_at, a.priority, a.has_image, a.station_id,
			a.created_at, a.updated_at,
			u1.name as issued_by_name, u2.name as acknowledged_by_name
		FROM alerts a
		LEFT JOIN users u1 ON a.issued_by = u1.id
		LEFT JOIN users u2 ON a.acknowledged_by = u2.id
		WHERE a.acknowledged = false AND a.expires_at > NOW()
		ORDER BY a.priority ASC, a.issued_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []models.Alert{}
	for rows.Next() {
		var a models.Alert
		err := rows.Scan(
			&a.ID, &a.Type, &a.Scope, &a.Title, &a.Description,
			&a.IssuedAt, &a.ExpiresAt, &a.IssuedBy, &a.Acknowledged,
			&a.AcknowledgedBy, &a.AcknowledgedAt, &a.Priority, &a.HasImage, &a.StationID,
			&a.CreatedAt, &a.UpdatedAt,
			&a.IssuedByName, &a.AcknowledgedByName,
		)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}

	return alerts, nil
}
