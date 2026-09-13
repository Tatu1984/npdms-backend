package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type CyberCrimeService struct {
	repo      *repository.CyberCrimeRepository
	auditRepo *repository.AuditRepository
}

func NewCyberCrimeService(repo *repository.CyberCrimeRepository, auditRepo *repository.AuditRepository) *CyberCrimeService {
	return &CyberCrimeService{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

func (s *CyberCrimeService) Create(ctx context.Context, crime *models.CyberCrime, userID uuid.UUID) error {
	caseNumber, err := s.repo.GenerateCaseNumber(ctx)
	if err != nil {
		return err
	}
	crime.CaseNumber = caseNumber
	crime.Status = models.CyberCrimeStatusReported

	if err := s.repo.Create(ctx, crime); err != nil {
		return err
	}

	// Audit log
	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "CREATE",
		ResourceType: "CYBER_CRIME",
		ResourceID:   &crime.ID,
		Description:  strPtr("Created cyber crime case: " + crime.CaseNumber),
		Success:      true,
	})

	return nil
}

func (s *CyberCrimeService) GetByID(ctx context.Context, id uuid.UUID) (*models.CyberCrime, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *CyberCrimeService) List(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CyberCrime, int64, error) {
	return s.repo.List(ctx, filters, page, pageSize)
}

func (s *CyberCrimeService) Update(ctx context.Context, crime *models.CyberCrime, userID uuid.UUID) error {
	if err := s.repo.Update(ctx, crime); err != nil {
		return err
	}

	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE",
		ResourceType: "CYBER_CRIME",
		ResourceID:   &crime.ID,
		Description:  strPtr("Updated cyber crime case: " + crime.CaseNumber),
		Success:      true,
	})

	return nil
}

func (s *CyberCrimeService) UpdateStatus(ctx context.Context, id uuid.UUID, status models.CyberCrimeStatus, userID uuid.UUID) error {
	crime, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	crime.Status = status
	return s.Update(ctx, crime, userID)
}

func (s *CyberCrimeService) GetStats(ctx context.Context) (*models.CyberCrimeStats, error) {
	return s.repo.GetStats(ctx)
}

func (s *CyberCrimeService) AddDigitalEvidence(ctx context.Context, evidence *models.DigitalEvidence) error {
	return s.repo.AddDigitalEvidence(ctx, evidence)
}

func (s *CyberCrimeService) GetDigitalEvidence(ctx context.Context, crimeID uuid.UUID) ([]models.DigitalEvidence, error) {
	return s.repo.GetDigitalEvidence(ctx, crimeID)
}

func (s *CyberCrimeService) AddFinancialTrail(ctx context.Context, trail *models.FinancialTrail) error {
	return s.repo.AddFinancialTrail(ctx, trail)
}

func (s *CyberCrimeService) GetFinancialTrails(ctx context.Context, crimeID uuid.UUID) ([]models.FinancialTrail, error) {
	return s.repo.GetFinancialTrails(ctx, crimeID)
}

func (s *CyberCrimeService) AddOSINTReport(ctx context.Context, report *models.OSINTReport) error {
	return s.repo.AddOSINTReport(ctx, report)
}

func (s *CyberCrimeService) GetOSINTReports(ctx context.Context, crimeID uuid.UUID) ([]models.OSINTReport, error) {
	return s.repo.GetOSINTReports(ctx, crimeID)
}

func strPtr(s string) *string {
	return &s
}
