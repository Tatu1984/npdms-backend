package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/npdms/api/internal/models"
)

type EvidenceRepository struct {
	db *pgxpool.Pool
}

func NewEvidenceRepository(db *pgxpool.Pool) *EvidenceRepository {
	return &EvidenceRepository{db: db}
}

func (r *EvidenceRepository) List(ctx context.Context, page, pageSize int) ([]models.Evidence, int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM evidence").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `
		SELECT e.id, e.evidence_number, e.case_id, e.fir_id, e.evidence_type,
		       e.description, e.collection_location, e.collection_date, e.collected_by,
		       e.storage_location, e.container_type, e.seal_number, e.weight,
		       e.dimensions, e.condition, e.status, e.requires_forensic, e.forensic_type,
		       e.created_at, e.updated_at,
		       COALESCE(u.name, '') as collected_by_name
		FROM evidence e
		LEFT JOIN users u ON e.collected_by = u.id
		ORDER BY e.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var evidence []models.Evidence
	for rows.Next() {
		var e models.Evidence
		err := rows.Scan(
			&e.ID, &e.EvidenceNumber, &e.CaseID, &e.FIRID, &e.EvidenceType,
			&e.Description, &e.CollectionLocation, &e.CollectionDate, &e.CollectedBy,
			&e.StorageLocation, &e.ContainerType, &e.SealNumber, &e.Weight,
			&e.Dimensions, &e.Condition, &e.Status, &e.RequiresForensic, &e.ForensicType,
			&e.CreatedAt, &e.UpdatedAt, &e.CollectedByName,
		)
		if err != nil {
			return nil, 0, err
		}
		evidence = append(evidence, e)
	}

	return evidence, total, nil
}

func (r *EvidenceRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Evidence, error) {
	query := `
		SELECT e.id, e.evidence_number, e.case_id, e.fir_id, e.evidence_type,
		       e.description, e.collection_location, e.collection_date, e.collected_by,
		       e.storage_location, e.container_type, e.seal_number, e.weight,
		       e.dimensions, e.condition, e.status, e.requires_forensic, e.forensic_type,
		       e.created_at, e.updated_at,
		       COALESCE(u.name, '') as collected_by_name
		FROM evidence e
		LEFT JOIN users u ON e.collected_by = u.id
		WHERE e.id = $1
	`

	var e models.Evidence
	err := r.db.QueryRow(ctx, query, id).Scan(
		&e.ID, &e.EvidenceNumber, &e.CaseID, &e.FIRID, &e.EvidenceType,
		&e.Description, &e.CollectionLocation, &e.CollectionDate, &e.CollectedBy,
		&e.StorageLocation, &e.ContainerType, &e.SealNumber, &e.Weight,
		&e.Dimensions, &e.Condition, &e.Status, &e.RequiresForensic, &e.ForensicType,
		&e.CreatedAt, &e.UpdatedAt, &e.CollectedByName,
	)
	if err != nil {
		return nil, err
	}

	return &e, nil
}

func (r *EvidenceRepository) Create(ctx context.Context, e *models.Evidence) error {
	e.ID = uuid.New()
	e.EvidenceNumber = fmt.Sprintf("EVD-%d-%05d", time.Now().Year(), time.Now().UnixNano()%100000)

	query := `
		INSERT INTO evidence (
			id, evidence_number, case_id, fir_id, evidence_type, description,
			collection_location, collection_date, collected_by, storage_location,
			container_type, seal_number, weight, dimensions, condition,
			status, requires_forensic, forensic_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`

	_, err := r.db.Exec(ctx, query,
		e.ID, e.EvidenceNumber, e.CaseID, e.FIRID, e.EvidenceType, e.Description,
		e.CollectionLocation, e.CollectionDate, e.CollectedBy, e.StorageLocation,
		e.ContainerType, e.SealNumber, e.Weight, e.Dimensions, e.Condition,
		e.Status, e.RequiresForensic, e.ForensicType,
	)

	return err
}

func (r *EvidenceRepository) Update(ctx context.Context, e *models.Evidence) error {
	query := `
		UPDATE evidence SET
			description = $2, storage_location = $3, container_type = $4,
			seal_number = $5, condition = $6, status = $7,
			requires_forensic = $8, forensic_type = $9
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		e.ID, e.Description, e.StorageLocation, e.ContainerType,
		e.SealNumber, e.Condition, e.Status,
		e.RequiresForensic, e.ForensicType,
	)

	return err
}

func (r *EvidenceRepository) GetChainOfCustody(ctx context.Context, evidenceID uuid.UUID) ([]models.EvidenceCustody, error) {
	query := `
		SELECT ec.id, ec.evidence_id, ec.from_user, ec.to_user,
		       ec.from_location, ec.to_location, ec.purpose, ec.transfer_date,
		       ec.verified, ec.notes, ec.created_at,
		       COALESCE(fu.name, 'System') as from_user_name,
		       COALESCE(tu.name, '') as to_user_name
		FROM evidence_custody ec
		LEFT JOIN users fu ON ec.from_user = fu.id
		LEFT JOIN users tu ON ec.to_user = tu.id
		WHERE ec.evidence_id = $1
		ORDER BY ec.transfer_date
	`

	rows, err := r.db.Query(ctx, query, evidenceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var custody []models.EvidenceCustody
	for rows.Next() {
		var c models.EvidenceCustody
		err := rows.Scan(
			&c.ID, &c.EvidenceID, &c.FromUser, &c.ToUser,
			&c.FromLocation, &c.ToLocation, &c.Purpose, &c.TransferDate,
			&c.Verified, &c.Notes, &c.CreatedAt,
			&c.FromUserName, &c.ToUserName,
		)
		if err != nil {
			return nil, err
		}
		custody = append(custody, c)
	}

	return custody, nil
}

func (r *EvidenceRepository) AddCustodyTransfer(ctx context.Context, transfer *models.EvidenceCustody) error {
	transfer.ID = uuid.New()
	transfer.TransferDate = time.Now()

	query := `
		INSERT INTO evidence_custody (
			id, evidence_id, from_user, to_user, from_location, to_location,
			purpose, transfer_date, verified, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.Exec(ctx, query,
		transfer.ID, transfer.EvidenceID, transfer.FromUser, transfer.ToUser,
		transfer.FromLocation, transfer.ToLocation, transfer.Purpose,
		transfer.TransferDate, transfer.Verified, transfer.Notes,
	)

	return err
}
