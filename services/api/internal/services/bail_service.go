package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type BailService struct {
	bailRepo  *repository.BailRepository
	auditRepo *repository.AuditRepository
}

func NewBailService(bailRepo *repository.BailRepository, auditRepo *repository.AuditRepository) *BailService {
	return &BailService{
		bailRepo:  bailRepo,
		auditRepo: auditRepo,
	}
}

func (s *BailService) List(ctx context.Context, filter repository.BailFilter) (*models.PaginatedResponse, error) {
	bails, total, err := s.bailRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       bails,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *BailService) GetByID(ctx context.Context, id uuid.UUID) (*models.Bail, error) {
	return s.bailRepo.FindByID(ctx, id)
}

func (s *BailService) Create(ctx context.Context, bail *models.Bail) (*models.Bail, error) {
	appNumber, err := s.bailRepo.GenerateApplicationNumber(ctx)
	if err != nil {
		return nil, err
	}
	bail.ApplicationNumber = appNumber

	if bail.Status == "" {
		bail.Status = models.BailStatusPending
	}

	err = s.bailRepo.Create(ctx, bail)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "bail_created",
		ResourceType: "bail",
		ResourceID:   &bail.ID,
		Description:  &appNumber,
		Success:      true,
	})

	return s.bailRepo.FindByID(ctx, bail.ID)
}

func (s *BailService) Update(ctx context.Context, bail *models.Bail) (*models.Bail, error) {
	err := s.bailRepo.Update(ctx, bail)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "bail_updated",
		ResourceType: "bail",
		ResourceID:   &bail.ID,
		Success:      true,
	})

	return s.bailRepo.FindByID(ctx, bail.ID)
}

func (s *BailService) UpdateStatus(ctx context.Context, id uuid.UUID, status models.BailStatus, reason *string) (*models.Bail, error) {
	err := s.bailRepo.UpdateStatus(ctx, id, status, reason)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "bail_status_updated",
		ResourceType: "bail",
		ResourceID:   &id,
		Success:      true,
	})

	return s.bailRepo.FindByID(ctx, id)
}

func (s *BailService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return s.bailRepo.GetStats(ctx)
}
