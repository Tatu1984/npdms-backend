package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type CitizenPortalService struct {
	repo      *repository.CitizenPortalRepository
	firRepo   *repository.FIRRepository
	auditRepo *repository.AuditRepository
}

func NewCitizenPortalService(repo *repository.CitizenPortalRepository, firRepo *repository.FIRRepository, auditRepo *repository.AuditRepository) *CitizenPortalService {
	return &CitizenPortalService{
		repo:      repo,
		firRepo:   firRepo,
		auditRepo: auditRepo,
	}
}

// FIR Status
func (s *CitizenPortalService) GetPublicFIRStatus(ctx context.Context, firNumber, phone string) (*models.FIRStatusResponse, error) {
	return s.repo.GetPublicFIRStatus(ctx, firNumber, phone)
}

// Grievance Operations
func (s *CitizenPortalService) SubmitGrievance(ctx context.Context, grievance *models.Grievance) error {
	grievanceNumber, err := s.repo.GenerateGrievanceNumber(ctx)
	if err != nil {
		return err
	}
	grievance.GrievanceNumber = grievanceNumber
	grievance.Status = "SUBMITTED"

	return s.repo.CreateGrievance(ctx, grievance)
}

func (s *CitizenPortalService) GetGrievance(ctx context.Context, id uuid.UUID) (*models.Grievance, error) {
	return s.repo.GetGrievance(ctx, id)
}

// Missing Person Reports
func (s *CitizenPortalService) SubmitMissingPersonReport(ctx context.Context, report *models.MissingPersonReport) error {
	reportNumber, err := s.repo.GenerateMissingReportNumber(ctx)
	if err != nil {
		return err
	}
	report.ReportNumber = reportNumber
	report.Status = "REPORTED"

	return s.repo.CreateMissingPersonReport(ctx, report)
}

func (s *CitizenPortalService) GetMissingPersonReport(ctx context.Context, reportNumber string) (*models.MissingPersonReport, error) {
	return s.repo.GetMissingPersonReport(ctx, reportNumber)
}

// FIR Copy Requests
func (s *CitizenPortalService) SubmitFIRCopyRequest(ctx context.Context, request *models.PublicFIRRequest) error {
	request.RequestNumber = fmt.Sprintf("FCR/%d/%06d", time.Now().Year(), time.Now().UnixNano()%1000000)
	request.Status = "SUBMITTED"

	return s.repo.CreateFIRCopyRequest(ctx, request)
}

// Statistics
func (s *CitizenPortalService) GetPortalStats(ctx context.Context) (*models.CitizenPortalStats, error) {
	return s.repo.GetPortalStats(ctx)
}
