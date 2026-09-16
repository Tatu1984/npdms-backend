package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type ForensicService struct {
	forensicRepo *repository.ForensicRepository
	auditRepo    *repository.AuditRepository
}

func NewForensicService(forensicRepo *repository.ForensicRepository, auditRepo *repository.AuditRepository) *ForensicService {
	return &ForensicService{
		forensicRepo: forensicRepo,
		auditRepo:    auditRepo,
	}
}

func (s *ForensicService) List(ctx context.Context, filter repository.ForensicFilter) (*models.PaginatedResponse, error) {
	forensics, total, err := s.forensicRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       forensics,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *ForensicService) GetByID(ctx context.Context, id uuid.UUID) (*models.Forensic, error) {
	return s.forensicRepo.FindByID(ctx, id)
}

func (s *ForensicService) Create(ctx context.Context, forensic *models.Forensic) (*models.Forensic, error) {
	if forensic.Status == "" {
		forensic.Status = models.ForensicStatusPending
	}

	err := s.forensicRepo.Create(ctx, forensic)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "forensic_request_created",
		ResourceType: "forensic",
		ResourceID:   &forensic.ID,
		Success:      true,
	})

	return s.forensicRepo.FindByID(ctx, forensic.ID)
}

func (s *ForensicService) Update(ctx context.Context, forensic *models.Forensic) (*models.Forensic, error) {
	err := s.forensicRepo.Update(ctx, forensic)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "forensic_request_updated",
		ResourceType: "forensic",
		ResourceID:   &forensic.ID,
		Success:      true,
	})

	return s.forensicRepo.FindByID(ctx, forensic.ID)
}

func (s *ForensicService) CompleteRequest(ctx context.Context, id uuid.UUID, summary, findings string) (*models.Forensic, error) {
	err := s.forensicRepo.CompleteRequest(ctx, id, summary, findings)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "forensic_request_completed",
		ResourceType: "forensic",
		ResourceID:   &id,
		Success:      true,
	})

	return s.forensicRepo.FindByID(ctx, id)
}

// Owner answers which department holds a forensic request.
func (s *ForensicService) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.forensicRepo.Owner(ctx, id, viewerID)
}

func (s *ForensicService) GetStats(ctx context.Context, viewerID uuid.UUID) (map[string]interface{}, error) {
	return s.forensicRepo.GetStats(ctx, viewerID)
}
