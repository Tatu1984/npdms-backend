package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type CaseService struct {
	caseRepo  *repository.CaseRepository
	auditRepo *repository.AuditRepository
}

func NewCaseService(caseRepo *repository.CaseRepository, auditRepo *repository.AuditRepository) *CaseService {
	return &CaseService{
		caseRepo:  caseRepo,
		auditRepo: auditRepo,
	}
}

func (s *CaseService) List(ctx context.Context, f repository.CaseFilter) (*models.PaginatedResponse, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	page, pageSize := f.Page, f.PageSize
	cases, total, err := s.caseRepo.List(ctx, f)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       cases,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *CaseService) Get(ctx context.Context, id uuid.UUID) (*models.Case, error) {
	return s.caseRepo.FindByID(ctx, id)
}

func (s *CaseService) Create(ctx context.Context, c *models.Case, userID uuid.UUID) error {
	c.Status = models.CaseStatusRegistered

	if err := s.caseRepo.Create(ctx, c); err != nil {
		return err
	}

	// Audit log
	desc := "Created Case: " + c.CaseNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "CASE",
		ResourceID:   &c.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *CaseService) Update(ctx context.Context, c *models.Case, userID uuid.UUID) error {
	if err := s.caseRepo.Update(ctx, c); err != nil {
		return err
	}

	// Audit log
	desc := "Updated Case: " + c.CaseNumber
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE",
		ResourceType: "CASE",
		ResourceID:   &c.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *CaseService) GetAccused(ctx context.Context, caseID uuid.UUID) ([]models.Accused, error) {
	return s.caseRepo.GetAccused(ctx, caseID)
}

func (s *CaseService) AddAccused(ctx context.Context, accused *models.Accused, userID uuid.UUID) error {
	if err := s.caseRepo.AddAccused(ctx, accused); err != nil {
		return err
	}

	// Audit log
	desc := "Added accused: " + accused.Name
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "ACCUSED",
		ResourceID:   &accused.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}

func (s *CaseService) GetWitnesses(ctx context.Context, caseID uuid.UUID) ([]models.Witness, error) {
	return s.caseRepo.GetWitnesses(ctx, caseID)
}

func (s *CaseService) AddWitness(ctx context.Context, witness *models.Witness, userID uuid.UUID) error {
	if err := s.caseRepo.AddWitness(ctx, witness); err != nil {
		return err
	}

	// Audit log
	desc := "Added witness: " + witness.Name
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "WITNESS",
		ResourceID:   &witness.ID,
		Description:  &desc,
		Success:      true,
	})

	return nil
}
