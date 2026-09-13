package services

import (
	"context"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type PersonnelService struct {
	personnelRepo *repository.PersonnelRepository
	auditRepo     *repository.AuditRepository
}

func NewPersonnelService(personnelRepo *repository.PersonnelRepository, auditRepo *repository.AuditRepository) *PersonnelService {
	return &PersonnelService{
		personnelRepo: personnelRepo,
		auditRepo:     auditRepo,
	}
}

func (s *PersonnelService) List(ctx context.Context, filter repository.PersonnelFilter) (*models.PaginatedResponse, error) {
	personnel, total, err := s.personnelRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / filter.PageSize
	if int(total)%filter.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       personnel,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *PersonnelService) GetByID(ctx context.Context, id uuid.UUID) (*models.Personnel, error) {
	return s.personnelRepo.FindByID(ctx, id)
}

func (s *PersonnelService) Create(ctx context.Context, personnel *models.Personnel) (*models.Personnel, error) {
	if personnel.Status == "" {
		personnel.Status = models.PersonnelStatusOffDuty
	}

	err := s.personnelRepo.Create(ctx, personnel)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "personnel_created",
		ResourceType: "personnel",
		ResourceID:   &personnel.ID,
		Success:      true,
	})

	return s.personnelRepo.FindByID(ctx, personnel.ID)
}

func (s *PersonnelService) Update(ctx context.Context, personnel *models.Personnel) (*models.Personnel, error) {
	err := s.personnelRepo.Update(ctx, personnel)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "personnel_updated",
		ResourceType: "personnel",
		ResourceID:   &personnel.ID,
		Success:      true,
	})

	return s.personnelRepo.FindByID(ctx, personnel.ID)
}

func (s *PersonnelService) AssignDuty(ctx context.Context, id uuid.UUID, duty, shift string) (*models.Personnel, error) {
	err := s.personnelRepo.AssignDuty(ctx, id, duty, shift)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "personnel_duty_assigned",
		ResourceType: "personnel",
		ResourceID:   &id,
		Success:      true,
	})

	return s.personnelRepo.FindByID(ctx, id)
}

func (s *PersonnelService) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.personnelRepo.Delete(ctx, id)
	if err != nil {
		return err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "personnel_deleted",
		ResourceType: "personnel",
		ResourceID:   &id,
		Success:      true,
	})

	return nil
}
