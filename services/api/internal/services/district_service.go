package services

import (
	"context"
	"time"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"

	"github.com/google/uuid"
)

type DistrictService struct {
	repo      *repository.DistrictRepository
	auditRepo *repository.AuditRepository
}

func NewDistrictService(repo *repository.DistrictRepository, auditRepo *repository.AuditRepository) *DistrictService {
	return &DistrictService{
		repo:      repo,
		auditRepo: auditRepo,
	}
}

// District Operations
func (s *DistrictService) GetDistrict(ctx context.Context, id uuid.UUID) (*models.District, error) {
	return s.repo.GetDistrict(ctx, id)
}

func (s *DistrictService) ListDistricts(ctx context.Context, rangeID *uuid.UUID) ([]models.District, error) {
	return s.repo.ListDistricts(ctx, rangeID)
}

func (s *DistrictService) CreateDistrict(ctx context.Context, district *models.District) error {
	district.ID = uuid.New()
	district.CreatedAt = time.Now()
	district.UpdatedAt = time.Now()
	return s.repo.CreateDistrict(ctx, district)
}

func (s *DistrictService) UpdateDistrict(ctx context.Context, district *models.District) error {
	district.UpdatedAt = time.Now()
	return s.repo.UpdateDistrict(ctx, district)
}

// Station Operations
func (s *DistrictService) GetStation(ctx context.Context, id uuid.UUID) (*models.PoliceStation, error) {
	return s.repo.GetStation(ctx, id)
}

func (s *DistrictService) ListStations(ctx context.Context, districtID uuid.UUID) ([]models.PoliceStation, error) {
	return s.repo.ListStations(ctx, districtID)
}

func (s *DistrictService) CreateStation(ctx context.Context, station *models.PoliceStation) error {
	station.ID = uuid.New()
	station.CreatedAt = time.Now()
	station.UpdatedAt = time.Now()
	return s.repo.CreateStation(ctx, station)
}

func (s *DistrictService) UpdateStation(ctx context.Context, station *models.PoliceStation) error {
	station.UpdatedAt = time.Now()
	return s.repo.UpdateStation(ctx, station)
}

// Dashboard
func (s *DistrictService) GetDistrictDashboard(ctx context.Context, districtID uuid.UUID, period string) (*models.DistrictDashboard, error) {
	dashboard := &models.DistrictDashboard{
		DistrictID: districtID,
		Period:     period,
	}

	// Get district info
	district, err := s.repo.GetDistrict(ctx, districtID)
	if err != nil {
		return nil, err
	}
	dashboard.DistrictName = district.Name

	// Get FIR statistics
	firStats, err := s.repo.GetFIRStatistics(ctx, districtID, period)
	if err == nil {
		dashboard.TotalFIRs = firStats["total"]
		dashboard.PendingFIRs = firStats["pending"]
		dashboard.ResolvedFIRs = firStats["resolved"]
		dashboard.CriticalFIRs = firStats["critical"]
	}

	// Get case statistics
	caseStats, err := s.repo.GetCaseStatistics(ctx, districtID, period)
	if err == nil {
		dashboard.TotalCases = caseStats["total"]
		dashboard.UnderInvestigation = caseStats["investigating"]
		dashboard.ChargesheeetFiled = caseStats["chargesheeted"]
		dashboard.Convicted = caseStats["convicted"]
		dashboard.Acquitted = caseStats["acquitted"]
	}

	// Get personnel statistics
	personnelStats, err := s.repo.GetPersonnelStatistics(ctx, districtID)
	if err == nil {
		dashboard.TotalOfficers = personnelStats["total"]
		dashboard.OnDuty = personnelStats["on_duty"]
		dashboard.OnLeave = personnelStats["on_leave"]
		dashboard.Vacant = personnelStats["vacant"]
	}

	// Get crime categories
	crimesByCategory, err := s.repo.GetCrimesByCategory(ctx, districtID, period)
	if err == nil {
		dashboard.CrimesByCategory = crimesByCategory
	}

	// Get station-wise statistics
	stationStats, err := s.repo.GetStationWiseStats(ctx, districtID, period)
	if err == nil {
		dashboard.StationWiseFIRs = stationStats
	}

	// Get trend data
	trendData, err := s.repo.GetTrendData(ctx, districtID, period)
	if err == nil {
		dashboard.TrendData = trendData
	}

	// Get alerts
	alertStats, err := s.repo.GetAlertStatistics(ctx, districtID)
	if err == nil {
		dashboard.PendingAlerts = alertStats["pending"]
		dashboard.CriticalAlerts = alertStats["critical"]
	}

	return dashboard, nil
}

// Cross-Station Coordination
func (s *DistrictService) CreateCrossStationRequest(ctx context.Context, request *models.CrossStationRequest) error {
	request.ID = uuid.New()
	request.Status = "PENDING"
	request.CreatedAt = time.Now()
	request.UpdatedAt = time.Now()
	return s.repo.CreateCrossStationRequest(ctx, request)
}

func (s *DistrictService) GetCrossStationRequest(ctx context.Context, id uuid.UUID) (*models.CrossStationRequest, error) {
	return s.repo.GetCrossStationRequest(ctx, id)
}

func (s *DistrictService) ListCrossStationRequests(ctx context.Context, districtID uuid.UUID, status string) ([]models.CrossStationRequest, error) {
	return s.repo.ListCrossStationRequests(ctx, districtID, status)
}

func (s *DistrictService) ApproveCrossStationRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetCrossStationRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "APPROVED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateCrossStationRequest(ctx, request)
}

func (s *DistrictService) RejectCrossStationRequest(ctx context.Context, id uuid.UUID, approverID uuid.UUID, notes string) error {
	request, err := s.repo.GetCrossStationRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "REJECTED"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateCrossStationRequest(ctx, request)
}

func (s *DistrictService) CompleteCrossStationRequest(ctx context.Context, id uuid.UUID, notes string) error {
	request, err := s.repo.GetCrossStationRequest(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	request.Status = "COMPLETED"
	request.CompletedAt = &now
	request.Notes = notes
	request.UpdatedAt = now

	return s.repo.UpdateCrossStationRequest(ctx, request)
}

// Coordination Meetings
func (s *DistrictService) CreateMeeting(ctx context.Context, meeting *models.DistrictCoordinationMeeting) error {
	meeting.ID = uuid.New()
	meeting.Status = "SCHEDULED"
	meeting.CreatedAt = time.Now()
	meeting.UpdatedAt = time.Now()
	return s.repo.CreateMeeting(ctx, meeting)
}

func (s *DistrictService) GetMeeting(ctx context.Context, id uuid.UUID) (*models.DistrictCoordinationMeeting, error) {
	return s.repo.GetMeeting(ctx, id)
}

func (s *DistrictService) ListMeetings(ctx context.Context, districtID uuid.UUID, status string) ([]models.DistrictCoordinationMeeting, error) {
	return s.repo.ListMeetings(ctx, districtID, status)
}

func (s *DistrictService) UpdateMeeting(ctx context.Context, meeting *models.DistrictCoordinationMeeting) error {
	meeting.UpdatedAt = time.Now()
	return s.repo.UpdateMeeting(ctx, meeting)
}

// Resource Allocation
func (s *DistrictService) CreateResourceAllocation(ctx context.Context, allocation *models.ResourceAllocation) error {
	allocation.ID = uuid.New()
	allocation.Status = "PENDING"
	allocation.CreatedAt = time.Now()
	allocation.UpdatedAt = time.Now()
	return s.repo.CreateResourceAllocation(ctx, allocation)
}

func (s *DistrictService) GetResourceAllocation(ctx context.Context, id uuid.UUID) (*models.ResourceAllocation, error) {
	return s.repo.GetResourceAllocation(ctx, id)
}

func (s *DistrictService) ListResourceAllocations(ctx context.Context, districtID uuid.UUID, status string) ([]models.ResourceAllocation, error) {
	return s.repo.ListResourceAllocations(ctx, districtID, status)
}

func (s *DistrictService) ApproveResourceAllocation(ctx context.Context, id uuid.UUID, approverID uuid.UUID) error {
	allocation, err := s.repo.GetResourceAllocation(ctx, id)
	if err != nil {
		return err
	}

	allocation.Status = "APPROVED"
	allocation.ApprovedBy = &approverID
	allocation.UpdatedAt = time.Now()

	return s.repo.UpdateResourceAllocation(ctx, allocation)
}

func (s *DistrictService) AllocateResource(ctx context.Context, id uuid.UUID) error {
	allocation, err := s.repo.GetResourceAllocation(ctx, id)
	if err != nil {
		return err
	}

	allocation.Status = "ALLOCATED"
	allocation.UpdatedAt = time.Now()

	return s.repo.UpdateResourceAllocation(ctx, allocation)
}

func (s *DistrictService) ReturnResource(ctx context.Context, id uuid.UUID) error {
	allocation, err := s.repo.GetResourceAllocation(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	allocation.Status = "RETURNED"
	allocation.EndDate = &now
	allocation.UpdatedAt = now

	return s.repo.UpdateResourceAllocation(ctx, allocation)
}

// District Comparison/Rankings
func (s *DistrictService) GetStationRankings(ctx context.Context, districtID uuid.UUID, metric string, period string) ([]models.StationStats, error) {
	return s.repo.GetStationRankings(ctx, districtID, metric, period)
}

// Hotspot Analysis
func (s *DistrictService) GetCrimeHotspots(ctx context.Context, districtID uuid.UUID, crimeType string) ([]map[string]interface{}, error) {
	return s.repo.GetCrimeHotspots(ctx, districtID, crimeType)
}

// Pending Tasks/Actions
func (s *DistrictService) GetPendingTasks(ctx context.Context, districtID uuid.UUID) ([]map[string]interface{}, error) {
	return s.repo.GetPendingTasks(ctx, districtID)
}
