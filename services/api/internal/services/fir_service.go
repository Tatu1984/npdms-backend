package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type FIRService struct {
	firRepo   *repository.FIRRepository
	auditRepo *repository.AuditRepository
}

func NewFIRService(firRepo *repository.FIRRepository, auditRepo *repository.AuditRepository) *FIRService {
	return &FIRService{
		firRepo:   firRepo,
		auditRepo: auditRepo,
	}
}

func (s *FIRService) List(ctx context.Context, filter repository.FIRFilter) (*models.PaginatedResponse, error) {
	firs, total, err := s.firRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize > 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       firs,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *FIRService) Get(ctx context.Context, id uuid.UUID) (*models.FIR, error) {
	return s.firRepo.FindByID(ctx, id)
}

func (s *FIRService) Create(ctx context.Context, fir *models.FIR, userID uuid.UUID, stationCode string) error {
	// Generate FIR number
	firNumber, err := s.firRepo.GenerateFIRNumber(ctx, stationCode)
	if err != nil {
		return err
	}

	fir.FIRNumber = firNumber
	fir.RegisteredBy = &userID
	fir.Status = models.FIRStatusRegistered

	if err := s.firRepo.Create(ctx, fir); err != nil {
		return err
	}

	// Audit log
	desc := "Created FIR: " + fir.FIRNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "FIR",
		ResourceID:   &fir.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *FIRService) Update(ctx context.Context, fir *models.FIR, userID uuid.UUID) error {
	if err := s.firRepo.Update(ctx, fir); err != nil {
		return err
	}

	// Audit log
	desc := "Updated FIR: " + fir.FIRNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE",
		ResourceType: "FIR",
		ResourceID:   &fir.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *FIRService) UpdateStatus(ctx context.Context, id uuid.UUID, status models.FIRStatus, userID uuid.UUID) error {
	if err := s.firRepo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}

	// Audit log
	desc := "Updated FIR status to: " + string(status)
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE_STATUS",
		ResourceType: "FIR",
		ResourceID:   &id,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *FIRService) GetStats(ctx context.Context, stationID *uuid.UUID) (map[string]int64, error) {
	return s.firRepo.GetStats(ctx, stationID)
}

func (s *FIRService) GetTimeline(ctx context.Context, firID uuid.UUID) ([]models.TimelineEntry, error) {
	return s.firRepo.GetTimeline(ctx, firID)
}
