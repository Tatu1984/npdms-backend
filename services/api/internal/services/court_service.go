package services

import (
	"context"
	"fmt"
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

// UpdateOrder corrects a recorded order. What a court directed is corrected,
// never deleted, and the correction names who made it.
func (s *CourtService) UpdateOrder(ctx context.Context, order *models.CourtOrder,
	by uuid.UUID) (*models.CourtOrder, error) {
	if err := s.courtRepo.UpdateOrder(ctx, order, by); err != nil {
		return nil, err
	}
	actor := by
	detail := "Amended court order " + order.OrderDate.Format("2006-01-02")
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "court_order_amended",
		ResourceType: "court_order",
		ResourceID:   &order.ID,
		Description:  &detail,
		Success:      true,
	})
	return s.courtRepo.FindOrderByID(ctx, order.ID)
}

// RecordCompliance settles what happened after the direction.
func (s *CourtService) RecordCompliance(ctx context.Context, id uuid.UUID,
	status models.CourtOrderCompliance, note *string, by uuid.UUID) (*models.CourtOrder, error) {
	if !status.Valid() {
		return nil, fmt.Errorf("%q is not a compliance state", status)
	}
	if err := s.courtRepo.RecordCompliance(ctx, id, status, note, by); err != nil {
		return nil, err
	}
	actor := by
	detail := "Court order recorded as " + string(status)
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "court_order_compliance_recorded",
		ResourceType: "court_order",
		ResourceID:   &id,
		Description:  &detail,
		Success:      true,
	})
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

// HearingOwner and OrderOwner answer which department the case behind a court
// paper belongs to.
func (s *CourtService) HearingOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.courtRepo.HearingOwner(ctx, id, viewerID)
}

func (s *CourtService) OrderOwner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.courtRepo.OrderOwner(ctx, id, viewerID)
}

func (s *CourtService) GetStats(ctx context.Context, viewerID uuid.UUID) (map[string]interface{}, error) {
	return s.courtRepo.GetStats(ctx, viewerID)
}
