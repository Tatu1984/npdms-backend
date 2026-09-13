package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type CourtService struct {
	courtRepo *repository.CourtRepository
	auditRepo *repository.AuditRepository
}

func NewCourtService(courtRepo *repository.CourtRepository, auditRepo *repository.AuditRepository) *CourtService {
	return &CourtService{
		courtRepo: courtRepo,
		auditRepo: auditRepo,
	}
}

func (s *CourtService) ListHearings(ctx context.Context, filter repository.CourtHearingFilter) (*models.PaginatedResponse, error) {
	hearings, total, err := s.courtRepo.ListHearings(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       hearings,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *CourtService) GetHearingByID(ctx context.Context, id uuid.UUID) (*models.CourtHearing, error) {
	return s.courtRepo.FindHearingByID(ctx, id)
}

func (s *CourtService) CreateHearing(ctx context.Context, hearing *models.CourtHearing) (*models.CourtHearing, error) {
	err := s.courtRepo.CreateHearing(ctx, hearing)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "court_hearing_created",
		ResourceType: "court_hearing",
		ResourceID:   &hearing.ID,
		Success:      true,
	})

	return s.courtRepo.FindHearingByID(ctx, hearing.ID)
}

func (s *CourtService) UpdateHearing(ctx context.Context, hearing *models.CourtHearing) (*models.CourtHearing, error) {
	err := s.courtRepo.UpdateHearing(ctx, hearing)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "court_hearing_updated",
		ResourceType: "court_hearing",
		ResourceID:   &hearing.ID,
		Success:      true,
	})

	return s.courtRepo.FindHearingByID(ctx, hearing.ID)
}

func (s *CourtService) ListOrders(ctx context.Context, filter repository.CourtOrderFilter) (*models.PaginatedResponse, error) {
	orders, total, err := s.courtRepo.ListOrders(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       orders,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *CourtService) GetOrderByID(ctx context.Context, id uuid.UUID) (*models.CourtOrder, error) {
	return s.courtRepo.FindOrderByID(ctx, id)
}

func (s *CourtService) CreateOrder(ctx context.Context, order *models.CourtOrder) (*models.CourtOrder, error) {
	err := s.courtRepo.CreateOrder(ctx, order)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "court_order_created",
		ResourceType: "court_order",
		ResourceID:   &order.ID,
		Success:      true,
	})

	return s.courtRepo.FindOrderByID(ctx, order.ID)
}

func (s *CourtService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return s.courtRepo.GetStats(ctx)
}
