package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type EvidenceService struct {
	evidenceRepo *repository.EvidenceRepository
	auditRepo    *repository.AuditRepository
}

func NewEvidenceService(evidenceRepo *repository.EvidenceRepository, auditRepo *repository.AuditRepository) *EvidenceService {
	return &EvidenceService{
		evidenceRepo: evidenceRepo,
		auditRepo:    auditRepo,
	}
}

func (s *EvidenceService) List(ctx context.Context, page, pageSize int) (*models.PaginatedResponse, error) {
	evidence, total, err := s.evidenceRepo.List(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       evidence,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *EvidenceService) Get(ctx context.Context, id uuid.UUID) (*models.Evidence, error) {
	return s.evidenceRepo.FindByID(ctx, id)
}

func (s *EvidenceService) Create(ctx context.Context, e *models.Evidence, userID uuid.UUID) error {
	e.CollectedBy = &userID
	e.Status = models.EvidenceStatusCollected

	if err := s.evidenceRepo.Create(ctx, e); err != nil {
		return err
	}

	// Create initial custody record
	s.evidenceRepo.AddCustodyTransfer(ctx, &models.EvidenceCustody{
		EvidenceID:   e.ID,
		ToUser:       &userID,
		ToLocation:   e.StorageLocation,
		Purpose:      stringPtr("Initial collection"),
		Verified:     true,
	})

	// Audit log
	desc := "Created Evidence: " + e.EvidenceNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "EVIDENCE",
		ResourceID:   &e.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *EvidenceService) Update(ctx context.Context, e *models.Evidence, userID uuid.UUID) error {
	if err := s.evidenceRepo.Update(ctx, e); err != nil {
		return err
	}

	// Audit log
	desc := "Updated Evidence: " + e.EvidenceNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE",
		ResourceType: "EVIDENCE",
		ResourceID:   &e.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *EvidenceService) GetChainOfCustody(ctx context.Context, evidenceID uuid.UUID) ([]models.EvidenceCustody, error) {
	return s.evidenceRepo.GetChainOfCustody(ctx, evidenceID)
}

func (s *EvidenceService) Transfer(ctx context.Context, transfer *models.EvidenceCustody, userID uuid.UUID) error {
	transfer.FromUser = &userID

	if err := s.evidenceRepo.AddCustodyTransfer(ctx, transfer); err != nil {
		return err
	}

	// Audit log
	desc := "Evidence custody transfer"
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "TRANSFER",
		ResourceType: "EVIDENCE",
		ResourceID:   &transfer.EvidenceID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func stringPtr(s string) *string {
	return &s
}
