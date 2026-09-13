package services

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type StateService struct {
	repo        *repository.StateRepository
	auditRepo   *repository.AuditRepository
}

func NewStateService(repo *repository.StateRepository, auditRepo *repository.AuditRepository) *StateService {
	return &StateService{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// State Operations
func (s *StateService) GetState(ctx context.Context, id uuid.UUID) (*models.State, error) {
	return s.repo.GetState(ctx, id)
}

func (s *StateService) GetStateByCode(ctx context.Context, code string) (*models.State, error) {
	return s.repo.GetStateByCode(ctx, code)
}

func (s *StateService) ListStates(ctx context.Context) ([]models.State, error) {
	return s.repo.ListStates(ctx)
}

func (s *StateService) CreateState(ctx context.Context, state *models.State) error {
	state.ID = uuid.New()
	state.CreatedAt = time.Now()
	state.UpdatedAt = time.Now()
	return s.repo.CreateState(ctx, state)
}

func (s *StateService) UpdateState(ctx context.Context, state *models.State) error {
	state.UpdatedAt = time.Now()
	return s.repo.UpdateState(ctx, state)
}

// Zone Operations
func (s *StateService) GetZone(ctx context.Context, id uuid.UUID) (*models.Zone, error) {
	return s.repo.GetZone(ctx, id)
}

func (s *StateService) ListZones(ctx context.Context, stateID uuid.UUID) ([]models.Zone, error) {
	return s.repo.ListZones(ctx, stateID)
}

func (s *StateService) CreateZone(ctx context.Context, zone *models.Zone) error {
	zone.ID = uuid.New()
	zone.CreatedAt = time.Now()
	zone.UpdatedAt = time.Now()
	return s.repo.CreateZone(ctx, zone)
}

func (s *StateService) UpdateZone(ctx context.Context, zone *models.Zone) error {
	zone.UpdatedAt = time.Now()
	return s.repo.UpdateZone(ctx, zone)
}

// Range Operations
func (s *StateService) GetRange(ctx context.Context, id uuid.UUID) (*models.Range, error) {
	return s.repo.GetRange(ctx, id)
}

func (s *StateService) ListRanges(ctx context.Context, zoneID uuid.UUID) ([]models.Range, error) {
	return s.repo.ListRanges(ctx, zoneID)
}

func (s *StateService) CreateRange(ctx context.Context, rangeObj *models.Range) error {
	rangeObj.ID = uuid.New()
	rangeObj.CreatedAt = time.Now()
	rangeObj.UpdatedAt = time.Now()
	return s.repo.CreateRange(ctx, rangeObj)
}

func (s *StateService) UpdateRange(ctx context.Context, rangeObj *models.Range) error {
	rangeObj.UpdatedAt = time.Now()
	return s.repo.UpdateRange(ctx, rangeObj)
}

// State Dashboard
func (s *StateService) GetStateDashboard(ctx context.Context, stateID uuid.UUID, period string) (*models.StateDashboard, error) {
	state, err := s.repo.GetState(ctx, stateID)
	if err != nil {
		return nil, err
	}

	dashboard := &models.StateDashboard{
		StateID:   stateID,
		StateName: state.Name,
		StateCode: state.Code,
		Period:    period,
	}

	// Get FIR statistics
	firStats, err := s.repo.GetStateFIRStatistics(ctx, stateID, period)
	if err == nil {
		dashboard.TotalFIRs = firStats["total"]
		dashboard.PendingFIRs = firStats["pending"]
		dashboard.ResolvedFIRs = firStats["resolved"]
		dashboard.CriticalFIRs = firStats["critical"]
	}

	// Get case statistics
	caseStats, err := s.repo.GetStateCaseStatistics(ctx, stateID, period)
	if err == nil {
		dashboard.TotalCases = caseStats["total"]
		dashboard.UnderInvestigation = caseStats["investigating"]
		dashboard.ChargesheetFiled = caseStats["chargesheeted"]
		dashboard.Convicted = caseStats["convicted"]
		dashboard.Acquitted = caseStats["acquitted"]
	}

	// Get hierarchy counts
	hierarchyStats, err := s.repo.GetHierarchyStats(ctx, stateID)
	if err == nil {
		dashboard.TotalZones = hierarchyStats["zones"]
		dashboard.TotalRanges = hierarchyStats["ranges"]
		dashboard.TotalDistricts = hierarchyStats["districts"]
		dashboard.TotalStations = hierarchyStats["stations"]
		dashboard.TotalOfficers = hierarchyStats["officers"]
	}

	// Get zone-wise statistics
	zoneStats, err := s.repo.GetZoneWiseStats(ctx, stateID, period)
	if err == nil {
		dashboard.ZoneWiseStats = zoneStats
	}

	// Get district rankings
	districtRankings, err := s.repo.GetDistrictRankings(ctx, stateID, "resolution_rate", period, 10)
	if err == nil {
		dashboard.TopDistricts = districtRankings
	}

	// Get crime categories
	crimesByCategory, err := s.repo.GetStateCrimesByCategory(ctx, stateID, period)
	if err == nil {
		dashboard.CrimesByCategory = crimesByCategory
	}

	// Get alerts
	alertStats, err := s.repo.GetStateAlertStatistics(ctx, stateID)
	if err == nil {
		dashboard.PendingAlerts = alertStats["pending"]
		dashboard.CriticalAlerts = alertStats["critical"]
	}

	return dashboard, nil
}

// Inter-District Coordination
func (s *StateService) CreateInterDistrictRequest(ctx context.Context, request *models.InterDistrictRequest) error {
	request.ID = uuid.New()
	request.Status = "PENDING"
	request.CreatedAt = time.Now()
	request.UpdatedAt = time.Now()
	return s.repo.CreateInterDistrictRequest(ctx, request)
}

func (s *StateService) GetInterDistrictRequest(ctx context.Context, id uuid.UUID) (*models.InterDistrictRequest, error) {
	return s.repo.GetInterDistrictRequest(ctx, id)
}

func (s *StateService) ListInterDistrictRequests(ctx context.Context, stateID uuid.UUID, status string) ([]models.InterDistrictRequest, error) {
	return s.repo.ListInterDistrictRequests(ctx, stateID, status)
}

func (s *StateService) ApproveInterDistrictRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetInterDistrictRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "APPROVED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateInterDistrictRequest(ctx, request)
}

func (s *StateService) RejectInterDistrictRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetInterDistrictRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "REJECTED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateInterDistrictRequest(ctx, request)
}

// State-Level Alerts
func (s *StateService) CreateStateAlert(ctx context.Context, alert *models.StateAlert) error {
	alert.ID = uuid.New()
	alert.Status = "ACTIVE"
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()
	return s.repo.CreateStateAlert(ctx, alert)
}

func (s *StateService) GetStateAlert(ctx context.Context, id uuid.UUID) (*models.StateAlert, error) {
	return s.repo.GetStateAlert(ctx, id)
}

func (s *StateService) ListStateAlerts(ctx context.Context, stateID uuid.UUID, status string) ([]models.StateAlert, error) {
	return s.repo.ListStateAlerts(ctx, stateID, status)
}

func (s *StateService) UpdateStateAlert(ctx context.Context, alert *models.StateAlert) error {
	alert.UpdatedAt = time.Now()
	return s.repo.UpdateStateAlert(ctx, alert)
}

func (s *StateService) AcknowledgeStateAlert(ctx context.Context, id uuid.UUID, districtID uuid.UUID) error {
	return s.repo.AcknowledgeStateAlert(ctx, id, districtID)
}

// Performance Metrics
func (s *StateService) GetPerformanceMetrics(ctx context.Context, stateID uuid.UUID, period string) (*models.StatePerformanceMetrics, error) {
	return s.repo.GetPerformanceMetrics(ctx, stateID, period)
}

// Crime Hotspots
func (s *StateService) GetStateCrimeHotspots(ctx context.Context, stateID uuid.UUID, crimeType string) ([]map[string]interface{}, error) {
	return s.repo.GetStateCrimeHotspots(ctx, stateID, crimeType)
}

// Resource Overview
func (s *StateService) GetResourceOverview(ctx context.Context, stateID uuid.UUID) (*models.StateResourceOverview, error) {
	return s.repo.GetResourceOverview(ctx, stateID)
}
