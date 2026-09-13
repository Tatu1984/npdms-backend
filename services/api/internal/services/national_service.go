package services

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type NationalService struct {
	repo      *repository.NationalRepository
	auditRepo *repository.AuditRepository
}

func NewNationalService(repo *repository.NationalRepository, auditRepo *repository.AuditRepository) *NationalService {
	return &NationalService{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// National Dashboard
func (s *NationalService) GetNationalDashboard(ctx context.Context, period string) (*models.NationalDashboard, error) {
	dashboard := &models.NationalDashboard{
		Period: period,
	}

	// Get FIR statistics
	firStats, err := s.repo.GetNationalFIRStatistics(ctx, period)
	if err == nil {
		dashboard.TotalFIRs = firStats["total"]
		dashboard.PendingFIRs = firStats["pending"]
		dashboard.ResolvedFIRs = firStats["resolved"]
		dashboard.CriticalFIRs = firStats["critical"]
	}

	// Get case statistics
	caseStats, err := s.repo.GetNationalCaseStatistics(ctx, period)
	if err == nil {
		dashboard.TotalCases = caseStats["total"]
		dashboard.UnderInvestigation = caseStats["investigating"]
		dashboard.ChargesheetFiled = caseStats["chargesheeted"]
		dashboard.Convicted = caseStats["convicted"]
		dashboard.Acquitted = caseStats["acquitted"]
	}

	// Get infrastructure counts
	infraStats, err := s.repo.GetInfrastructureStats(ctx)
	if err == nil {
		dashboard.TotalStates = infraStats["states"]
		dashboard.TotalZones = infraStats["zones"]
		dashboard.TotalRanges = infraStats["ranges"]
		dashboard.TotalDistricts = infraStats["districts"]
		dashboard.TotalStations = infraStats["stations"]
		dashboard.TotalOfficers = infraStats["officers"]
	}

	// Get state-wise statistics
	stateStats, err := s.repo.GetStateWiseStats(ctx, period)
	if err == nil {
		dashboard.StateWiseStats = stateStats
	}

	// Get crime categories
	crimesByCategory, err := s.repo.GetNationalCrimesByCategory(ctx, period)
	if err == nil {
		dashboard.CrimesByCategory = crimesByCategory
	}

	// Get alerts
	alertStats, err := s.repo.GetNationalAlertStatistics(ctx)
	if err == nil {
		dashboard.NationalAlerts = alertStats["total"]
		dashboard.CriticalAlerts = alertStats["critical"]
	}

	// Get trends
	trends, err := s.repo.GetNationalTrends(ctx, period)
	if err == nil {
		dashboard.NationalTrends = trends
	}

	return dashboard, nil
}

// National Alerts
func (s *NationalService) CreateNationalAlert(ctx context.Context, alert *models.NationalAlert) error {
	alert.ID = uuid.New()
	alert.Status = "ACTIVE"
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()
	return s.repo.CreateNationalAlert(ctx, alert)
}

func (s *NationalService) GetNationalAlert(ctx context.Context, id uuid.UUID) (*models.NationalAlert, error) {
	return s.repo.GetNationalAlert(ctx, id)
}

func (s *NationalService) ListNationalAlerts(ctx context.Context, status string) ([]models.NationalAlert, error) {
	return s.repo.ListNationalAlerts(ctx, status)
}

func (s *NationalService) UpdateNationalAlert(ctx context.Context, alert *models.NationalAlert) error {
	alert.UpdatedAt = time.Now()
	return s.repo.UpdateNationalAlert(ctx, alert)
}

func (s *NationalService) AcknowledgeNationalAlert(ctx context.Context, id uuid.UUID, stateID uuid.UUID) error {
	return s.repo.AcknowledgeNationalAlert(ctx, id, stateID)
}

// Inter-State Coordination
func (s *NationalService) CreateInterStateRequest(ctx context.Context, request *models.InterStateRequest) error {
	request.ID = uuid.New()
	request.Status = "PENDING"
	request.CreatedAt = time.Now()
	request.UpdatedAt = time.Now()
	return s.repo.CreateInterStateRequest(ctx, request)
}

func (s *NationalService) GetInterStateRequest(ctx context.Context, id uuid.UUID) (*models.InterStateRequest, error) {
	return s.repo.GetInterStateRequest(ctx, id)
}

func (s *NationalService) ListInterStateRequests(ctx context.Context, status string) ([]models.InterStateRequest, error) {
	return s.repo.ListInterStateRequests(ctx, status)
}

func (s *NationalService) ApproveInterStateRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetInterStateRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "APPROVED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateInterStateRequest(ctx, request)
}

func (s *NationalService) RejectInterStateRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetInterStateRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "REJECTED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateInterStateRequest(ctx, request)
}

// Crime Patterns
func (s *NationalService) CreateCrimePattern(ctx context.Context, pattern *models.NationalCrimePattern) error {
	pattern.ID = uuid.New()
	pattern.Status = "ACTIVE"
	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	return s.repo.CreateCrimePattern(ctx, pattern)
}

func (s *NationalService) GetCrimePattern(ctx context.Context, id uuid.UUID) (*models.NationalCrimePattern, error) {
	return s.repo.GetCrimePattern(ctx, id)
}

func (s *NationalService) ListCrimePatterns(ctx context.Context, status string) ([]models.NationalCrimePattern, error) {
	return s.repo.ListCrimePatterns(ctx, status)
}

func (s *NationalService) UpdateCrimePattern(ctx context.Context, pattern *models.NationalCrimePattern) error {
	pattern.UpdatedAt = time.Now()
	return s.repo.UpdateCrimePattern(ctx, pattern)
}

// Performance Metrics
func (s *NationalService) GetPerformanceMetrics(ctx context.Context, period string) (*models.NationalPerformanceMetrics, error) {
	return s.repo.GetPerformanceMetrics(ctx, period)
}

// State Rankings
func (s *NationalService) GetStateRankings(ctx context.Context, metric string, period string) ([]models.StateRanking, error) {
	return s.repo.GetStateRankings(ctx, metric, period)
}

// Crime Hotspots
func (s *NationalService) GetNationalCrimeHotspots(ctx context.Context, crimeType string) ([]map[string]interface{}, error) {
	return s.repo.GetNationalCrimeHotspots(ctx, crimeType)
}

// Resource Overview
func (s *NationalService) GetResourceOverview(ctx context.Context) (*models.NationalResourceOverview, error) {
	return s.repo.GetResourceOverview(ctx)
}

// Comparative Analysis
func (s *NationalService) GetStateComparison(ctx context.Context, stateIDs []uuid.UUID, period string) ([]models.StateRanking, error) {
	return s.repo.GetStateComparison(ctx, stateIDs, period)
}
