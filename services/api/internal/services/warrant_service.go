package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type WarrantService struct {
	warrantRepo *repository.WarrantRepository
	auditRepo   *repository.AuditRepository
}

func NewWarrantService(warrantRepo *repository.WarrantRepository, auditRepo *repository.AuditRepository) *WarrantService {
	return &WarrantService{
		warrantRepo: warrantRepo,
		auditRepo:   auditRepo,
	}
}

func (s *WarrantService) List(ctx context.Context, filter repository.WarrantFilter) (*models.PaginatedResponse, error) {
	warrants, total, err := s.warrantRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       warrants,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *WarrantService) GetByID(ctx context.Context, id uuid.UUID) (*models.Warrant, error) {
	return s.warrantRepo.FindByID(ctx, id)
}

func (s *WarrantService) Create(ctx context.Context, warrant *models.Warrant) (*models.Warrant, error) {
	// Generate warrant number
	warrantNumber, err := s.warrantRepo.GenerateWarrantNumber(ctx)
	if err != nil {
		return nil, err
	}
	warrant.WarrantNumber = warrantNumber

	// Set default status
	if warrant.Status == "" {
		warrant.Status = models.WarrantStatusActive
	}

	err = s.warrantRepo.Create(ctx, warrant)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "warrant_created",
		ResourceType: "warrant",
		ResourceID:   &warrant.ID,
		Description:  &warrantNumber,
		Success:      true,
	})

	return s.warrantRepo.FindByID(ctx, warrant.ID)
}

func (s *WarrantService) Update(ctx context.Context, warrant *models.Warrant) (*models.Warrant, error) {
	err := s.warrantRepo.Update(ctx, warrant)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "warrant_updated",
		ResourceType: "warrant",
		ResourceID:   &warrant.ID,
		Success:      true,
	})

	return s.warrantRepo.FindByID(ctx, warrant.ID)
}

func (s *WarrantService) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WarrantStatus, executedBy *uuid.UUID) (*models.Warrant, error) {
	err := s.warrantRepo.UpdateStatus(ctx, id, status, executedBy)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "warrant_status_updated",
		ResourceType: "warrant",
		ResourceID:   &id,
		Success:      true,
	})

	return s.warrantRepo.FindByID(ctx, id)
}

func (s *WarrantService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return s.warrantRepo.GetStats(ctx)
}
