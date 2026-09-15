package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type TrafficChallanService struct {
	challanRepo *repository.TrafficChallanRepository
	auditRepo   *repository.AuditRepository
}

func NewTrafficChallanService(challanRepo *repository.TrafficChallanRepository, auditRepo *repository.AuditRepository) *TrafficChallanService {
	return &TrafficChallanService{
		challanRepo: challanRepo,
		auditRepo:   auditRepo,
	}
}

// GetViolationTypes returns all active violation types
func (s *TrafficChallanService) GetViolationTypes(ctx context.Context) ([]models.TrafficViolationType, error) {
	return s.challanRepo.GetViolationTypes(ctx)
}

// GetViolationTypeByID returns a specific violation type
func (s *TrafficChallanService) GetViolationTypeByID(ctx context.Context, id uuid.UUID) (*models.TrafficViolationType, error) {
	vt, err := s.challanRepo.GetViolationTypeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if vt == nil {
		return nil, errors.New("violation type not found")
	}
	return vt, nil
}

// CreateChallan creates a new traffic challan
func (s *TrafficChallanService) CreateChallan(ctx context.Context, req models.CreateChallanRequest, officerID uuid.UUID, stationID uuid.UUID, officerName, officerBadge string) (*models.TrafficChallan, error) {
	// Get violation type to determine fine amount
	violationType, err := s.challanRepo.GetViolationTypeByID(ctx, req.ViolationTypeID)
	if err != nil {
		return nil, err
	}
	if violationType == nil {
		return nil, errors.New("invalid violation type")
	}

	// Generate challan number
	challanNumber, err := s.challanRepo.GenerateChallanNumber(ctx, "TRF")
	if err != nil {
		return nil, err
	}

	// Calculate payment due date (15 days from now)
	paymentDueDate := time.Now().AddDate(0, 0, 15)

	challan := &models.TrafficChallan{
		ID:                   uuid.New(),
		ChallanNumber:        challanNumber,
		ViolationTypeID:      req.ViolationTypeID,
		ViolationDate:        req.ViolationDate,
		ViolationLocation:    req.ViolationLocation,
		ViolationLatitude:    req.ViolationLatitude,
		ViolationLongitude:   req.ViolationLongitude,
		ViolationDescription: req.ViolationDescription,
		VehicleNumber:        req.VehicleNumber,
		VehicleType:          req.VehicleType,
		VehicleMake:          req.VehicleMake,
		VehicleModel:         req.VehicleModel,
		VehicleColor:         req.VehicleColor,
		OwnerName:            req.OwnerName,
		OwnerPhone:           req.OwnerPhone,
		DriverName:           req.DriverName,
		DriverLicenseNumber:  req.DriverLicenseNumber,
		DriverPhone:          req.DriverPhone,
		IssuingOfficerID:     &officerID,
		IssuingStationID:     stationID,
		IssuingOfficerName:   &officerName,
		IssuingOfficerBadge:  &officerBadge,
		OriginalFineAmount:   violationType.FineAmount,
		DiscountAmount:       0,
		PenaltyAmount:        0,
		FinalAmount:          violationType.FineAmount,
		PaymentDueDate:       paymentDueDate,
		Status:               string(models.ChallanStatusIssued),
		EvidencePhotos:       req.EvidencePhotos,
		SpeedReading:         req.SpeedReading,
		BreathAnalyzerReading: req.BreathAnalyzerReading,
		LicenseSeized:        violationType.LicenseSuspensionDays > 0,
		VehicleSeized:        violationType.VehicleSeizure,
		SyncStatus:           "PENDING",
	}

	err = s.challanRepo.CreateChallan(ctx, challan)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "challan_created",
		ResourceType: "traffic_challan",
		ResourceID:   &challan.ID,
		Description:  &challanNumber,
		Success:      true,
	})

	return s.challanRepo.GetChallanByID(ctx, challan.ID)
}

// GetChallanByID returns a challan by ID
func (s *TrafficChallanService) GetChallanByID(ctx context.Context, id uuid.UUID) (*models.TrafficChallan, error) {
	challan, err := s.challanRepo.GetChallanByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if challan == nil {
		return nil, errors.New("challan not found")
	}
	return challan, nil
}

// GetChallanByNumber returns a challan by challan number
func (s *TrafficChallanService) GetChallanByNumber(ctx context.Context, number string) (*models.TrafficChallan, error) {
	challan, err := s.challanRepo.GetChallanByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if challan == nil {
		return nil, errors.New("challan not found")
	}
	return challan, nil
}

// SearchChallans searches challans with filters
func (s *TrafficChallanService) SearchChallans(ctx context.Context, params models.ChallanSearchParams) (*models.PaginatedResponse, error) {
	challans, total, err := s.challanRepo.SearchChallans(ctx, params)
	if err != nil {
		return nil, err
	}

	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	totalPages := int(total) / params.PageSize
	if int(total)%params.PageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       challans,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// UpdateChallanStatus updates the status of a challan
// challanTransitions lists the statuses a challan may move to from each
// status. PAID, CANCELLED and COMPOUNDED are final.
var challanTransitions = map[string][]string{
	"ISSUED":         {"PAID", "COMPOUNDED", "CANCELLED", "COURT_REFERRED", "DEFAULTED"},
	"PENDING":        {"PAID", "COMPOUNDED", "CANCELLED", "COURT_REFERRED", "DEFAULTED"},
	"DEFAULTED":      {"PAID", "COMPOUNDED", "COURT_REFERRED"},
	"DISPUTED":       {"ISSUED", "CANCELLED", "COURT_REFERRED"},
	"COURT_REFERRED": {"PAID", "COMPOUNDED", "CANCELLED"},
}

// ErrChallanTransition marks a status change the challan's current status does not allow.
var ErrChallanTransition = errors.New("status change not allowed")

// ChangeChallanStatus applies one officer action. Payment and compounding need
// the receipt or order reference; cancelling, referring to court, marking a
// default and rejecting a dispute need a reason. The action is audited with
// the officer and the reason.
func (s *TrafficChallanService) ChangeChallanStatus(ctx context.Context, id uuid.UUID, to string, reference, note *string, actor uuid.UUID) (*models.TrafficChallan, error) {
	challan, err := s.challanRepo.GetChallanByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if challan == nil {
		return nil, errors.New("challan not found")
	}
	allowed := false
	for _, next := range challanTransitions[challan.Status] {
		if next == to {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("%w: a %s challan cannot be marked %s", ErrChallanTransition, challan.Status, to)
	}
	blank := func(p *string) bool { return p == nil || strings.TrimSpace(*p) == "" }
	if (to == "PAID" || to == "COMPOUNDED") && blank(reference) {
		return nil, fmt.Errorf("%w: record the receipt or compounding order reference", ErrChallanTransition)
	}
	if to != "PAID" && to != "COMPOUNDED" && blank(note) {
		return nil, fmt.Errorf("%w: record the reason for this change", ErrChallanTransition)
	}
	ok, err := s.challanRepo.TransitionChallanStatus(ctx, id, challan.Status, to, reference)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: the challan changed while you were working on it; reload and try again", ErrChallanTransition)
	}
	desc := fmt.Sprintf("%s: %s → %s", challan.ChallanNumber, challan.Status, to)
	if !blank(reference) {
		desc += " (reference " + strings.TrimSpace(*reference) + ")"
	}
	if !blank(note) {
		desc += ": " + strings.TrimSpace(*note)
	}
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "challan_status_updated",
		ResourceType: "traffic_challan",
		ResourceID:   &id,
		Description:  &desc,
		Success:      true,
	})
	return s.challanRepo.GetChallanByID(ctx, id)
}

// OfficerIdentity returns the name and badge an issuing officer is recorded under.
func (s *TrafficChallanService) OfficerIdentity(ctx context.Context, userID uuid.UUID) (string, string) {
	return s.challanRepo.OfficerIdentity(ctx, userID)
}

func (s *TrafficChallanService) UpdateChallanStatus(ctx context.Context, id uuid.UUID, status string) (*models.TrafficChallan, error) {
	err := s.challanRepo.UpdateChallanStatus(ctx, id, status)
	if err != nil {
		return nil, err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "challan_status_updated",
		ResourceType: "traffic_challan",
		ResourceID:   &id,
		Success:      true,
	})

	return s.challanRepo.GetChallanByID(ctx, id)
}

// InitiatePayment initiates a payment for a challan
func (s *TrafficChallanService) InitiatePayment(ctx context.Context, req models.InitiatePaymentRequest) (*models.ChallanPayment, error) {
	challan, err := s.challanRepo.GetChallanByID(ctx, req.ChallanID)
	if err != nil {
		return nil, err
	}
	if challan == nil {
		return nil, errors.New("challan not found")
	}

	if challan.Status == string(models.ChallanStatusPaid) {
		return nil, errors.New("challan already paid")
	}

	payment := &models.ChallanPayment{
		ID:             uuid.New(),
		ChallanID:      req.ChallanID,
		TransactionID:  uuid.New().String(),
		PaymentGateway: req.PaymentGateway,
		PaymentMethod:  req.PaymentMethod,
		Amount:         challan.FinalAmount,
		GatewayFee:     0,
		TotalAmount:    challan.FinalAmount,
		Status:         string(models.PaymentStatusInitiated),
		InitiatedAt:    time.Now(),
	}

	return payment, nil
}

// RecordPayment records a payment for a challan
func (s *TrafficChallanService) RecordPayment(ctx context.Context, payment *models.ChallanPayment) error {
	err := s.challanRepo.RecordPayment(ctx, payment)
	if err != nil {
		return err
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		Action:       "challan_payment_recorded",
		ResourceType: "traffic_challan",
		ResourceID:   &payment.ChallanID,
		Description:  &payment.TransactionID,
		Success:      true,
	})

	return nil
}

// GetDefaulters returns vehicles with pending challans
func (s *TrafficChallanService) GetDefaulters(ctx context.Context, page, pageSize int) (*models.PaginatedResponse, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	defaulters, total, err := s.challanRepo.GetDefaulters(ctx, pageSize, offset)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}

	return &models.PaginatedResponse{
		Data:       defaulters,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetHotspots returns challan hotspots
func (s *TrafficChallanService) GetHotspots(ctx context.Context) ([]models.ChallanHotspot, error) {
	return s.challanRepo.GetHotspots(ctx)
}

// GetStats returns challan statistics
func (s *TrafficChallanService) GetStats(ctx context.Context, stationID *uuid.UUID, fromDate, toDate *time.Time) (*models.ChallanStats, error) {
	from := time.Now().AddDate(0, -1, 0) // Default to last 30 days
	to := time.Now()

	if fromDate != nil {
		from = *fromDate
	}
	if toDate != nil {
		to = *toDate
	}

	return s.challanRepo.GetStats(ctx, stationID, from, to)
}

// GetChallansByVehicle returns all challans for a vehicle
func (s *TrafficChallanService) GetChallansByVehicle(ctx context.Context, vehicleNumber string) ([]models.TrafficChallan, error) {
	return s.challanRepo.GetChallansByVehicle(ctx, vehicleNumber)
}

// FileChallanDispute files a dispute for a challan
func (s *TrafficChallanService) FileChallanDispute(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) (*models.TrafficChallan, error) {
	challan, err := s.challanRepo.GetChallanByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if challan == nil {
		return nil, errors.New("challan not found")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: record the grounds of the dispute", ErrChallanTransition)
	}
	switch challan.Status {
	case "ISSUED", "PENDING", "DEFAULTED":
	default:
		return nil, fmt.Errorf("%w: a %s challan cannot be disputed", ErrChallanTransition, challan.Status)
	}
	ok, err := s.challanRepo.RecordDispute(ctx, id, challan.Status, reason)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: the challan changed while you were working on it; reload and try again", ErrChallanTransition)
	}

	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       "challan_disputed",
		ResourceType: "traffic_challan",
		ResourceID:   &id,
		Description:  &reason,
		Success:      true,
	})

	return s.challanRepo.GetChallanByID(ctx, id)
}
