package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type AlertService struct {
	alertRepo *repository.AlertRepository
	auditRepo *repository.AuditRepository
}

func NewAlertService(alertRepo *repository.AlertRepository, auditRepo *repository.AuditRepository) *AlertService {
	return &AlertService{
		alertRepo: alertRepo,
		auditRepo: auditRepo,
	}
}

func (s *AlertService) List(ctx context.Context, filter repository.AlertFilter) (*models.PaginatedResponse, error) {
	alerts, total, err := s.alertRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       alerts,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *AlertService) GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	return s.alertRepo.FindByID(ctx, id)
}

// MinimumRankForScope is how far an alert may reach, by the rank of the
// officer issuing it.
//
// An alert is a broadcast: every officer inside its scope is expected to act
// on it. A station-house officer could issue a NATIONAL one, which is the
// whole country told to look for a vehicle on one SHO's say-so. Rank is
// exactly the right instrument here — this is seniority, not a job — which is
// why it stays rank-based while day-to-day access moved to permissions.
var MinimumRankForScope = map[models.AlertScope]models.Role{
	models.AlertScopeStation:  models.RoleSI,
	models.AlertScopeDistrict: models.RoleSHO,
	models.AlertScopeState:    models.RoleDIG,
	models.AlertScopeNational: models.RoleIG,
}

// ErrScopeAboveRank is returned when an officer reaches further than their
// rank allows.
var ErrScopeAboveRank = errors.New("that scope is above your rank")

// CheckScope refuses an alert that reaches further than the issuing officer's
// rank permits, naming the rank that could issue it.
func CheckScope(scope models.AlertScope, rank models.Role) error {
	required, known := MinimumRankForScope[scope]
	if !known {
		return fmt.Errorf("%q is not an alert scope", scope)
	}
	if models.RoleHierarchy[rank] < models.RoleHierarchy[required] {
		return fmt.Errorf("%w: a %s alert is issued by %s rank and above",
			ErrScopeAboveRank, scope, required)
	}
	return nil
}

func (s *AlertService) Create(ctx context.Context, alert *models.Alert) (*models.Alert, error) {
	// A new alert is unacknowledged, and has no image because there is no
	// image storage. Both were previously taken from the request body, so an
	// alert could be created already acknowledged.
	alert.Acknowledged = false
	alert.AcknowledgedBy = nil
	alert.AcknowledgedAt = nil
	alert.HasImage = false

	err := s.alertRepo.Create(ctx, alert)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       alert.IssuedBy,
		Action:       "alert_created",
		ResourceType: "alert",
		ResourceID:   &alert.ID,
		Success:      true,
	})

	return s.alertRepo.FindByID(ctx, alert.ID)
}

func (s *AlertService) Update(ctx context.Context, alert *models.Alert, actor uuid.UUID) (*models.Alert, error) {
	alert.HasImage = false
	err := s.alertRepo.Update(ctx, alert)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "alert_updated",
		ResourceType: "alert",
		ResourceID:   &alert.ID,
		Success:      true,
	})

	return s.alertRepo.FindByID(ctx, alert.ID)
}

func (s *AlertService) Acknowledge(ctx context.Context, id, acknowledgedBy uuid.UUID) (*models.Alert, error) {
	err := s.alertRepo.Acknowledge(ctx, id, acknowledgedBy)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &acknowledgedBy,
		Action:       "alert_acknowledged",
		ResourceType: "alert",
		ResourceID:   &id,
		Success:      true,
	})

	return s.alertRepo.FindByID(ctx, id)
}

func (s *AlertService) Delete(ctx context.Context, id, actor uuid.UUID) error {
	err := s.alertRepo.Delete(ctx, id)
	if err != nil {
		return err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "alert_deleted",
		ResourceType: "alert",
		ResourceID:   &id,
		Success:      true,
	})

	return nil
}

func (s *AlertService) GetActiveAlerts(ctx context.Context) ([]models.Alert, error) {
	return s.alertRepo.GetActiveAlerts(ctx)
}

func (s *AlertService) GetUnacknowledgedAlerts(ctx context.Context) ([]models.Alert, error) {
	return s.alertRepo.GetUnacknowledgedAlerts(ctx)
}
