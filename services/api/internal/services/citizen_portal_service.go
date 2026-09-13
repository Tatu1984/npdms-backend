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

// Complaint Operations
func (s *CitizenPortalService) SubmitComplaint(ctx context.Context, complaint *models.CitizenComplaint) error {
	trackingNumber, err := s.repo.GenerateTrackingNumber(ctx)
	if err != nil {
		return err
	}
	complaint.TrackingNumber = trackingNumber
	complaint.Status = models.ComplaintStatusSubmitted

	return s.repo.CreateComplaint(ctx, complaint)
}

func (s *CitizenPortalService) GetComplaintByTrackingNumber(ctx context.Context, trackingNumber string) (*models.CitizenComplaint, error) {
	return s.repo.GetComplaintByTrackingNumber(ctx, trackingNumber)
}

func (s *CitizenPortalService) GetComplaintByID(ctx context.Context, id uuid.UUID) (*models.CitizenComplaint, error) {
	return s.repo.GetComplaintByID(ctx, id)
}

func (s *CitizenPortalService) ListComplaints(ctx context.Context, filters map[string]interface{}, page, pageSize int) ([]models.CitizenComplaint, int64, error) {
	return s.repo.ListComplaints(ctx, filters, page, pageSize)
}

func (s *CitizenPortalService) UpdateComplaint(ctx context.Context, complaint *models.CitizenComplaint, userID uuid.UUID) error {
	if err := s.repo.UpdateComplaint(ctx, complaint); err != nil {
		return err
	}

	s.auditRepo.Create(ctx, &models.SimpleAuditLog{
		UserID:       &userID,
		Action:       "UPDATE",
		ResourceType: "CITIZEN_COMPLAINT",
		ResourceID:   &complaint.ID,
		Description:  strPtr("Updated complaint: " + complaint.TrackingNumber),
		Success:      true,
	})

	return nil
}

func (s *CitizenPortalService) AcknowledgeComplaint(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	complaint.Status = models.ComplaintStatusAcknowledged
	complaint.AcknowledgedAt = &now

	if err := s.repo.UpdateComplaint(ctx, complaint); err != nil {
		return err
	}

	// Add update
	update := &models.ComplaintUpdate{
		ComplaintID: id,
		Status:      models.ComplaintStatusAcknowledged,
		Message:     "Your complaint has been acknowledged and is being reviewed.",
		UpdatedBy:   &userID,
		IsPublic:    true,
	}
	s.repo.AddComplaintUpdate(ctx, update)

	return nil
}

func (s *CitizenPortalService) AssignComplaint(ctx context.Context, id uuid.UUID, assignedTo uuid.UUID, stationID *uuid.UUID, userID uuid.UUID) error {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	complaint.Status = models.ComplaintStatusAssigned
	complaint.AssignedTo = &assignedTo
	complaint.AssignedAt = &now
	if stationID != nil {
		complaint.StationID = stationID
	}

	if err := s.repo.UpdateComplaint(ctx, complaint); err != nil {
		return err
	}

	update := &models.ComplaintUpdate{
		ComplaintID: id,
		Status:      models.ComplaintStatusAssigned,
		Message:     "Your complaint has been assigned to an officer for investigation.",
		UpdatedBy:   &userID,
		IsPublic:    true,
	}
	s.repo.AddComplaintUpdate(ctx, update)

	return nil
}

func (s *CitizenPortalService) ResolveComplaint(ctx context.Context, id uuid.UUID, responseNotes string, userID uuid.UUID) error {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	complaint.Status = models.ComplaintStatusResolved
	complaint.ResolvedAt = &now
	complaint.ResponseNotes = &responseNotes

	if err := s.repo.UpdateComplaint(ctx, complaint); err != nil {
		return err
	}

	update := &models.ComplaintUpdate{
		ComplaintID: id,
		Status:      models.ComplaintStatusResolved,
		Message:     fmt.Sprintf("Your complaint has been resolved. %s", responseNotes),
		UpdatedBy:   &userID,
		IsPublic:    true,
	}
	s.repo.AddComplaintUpdate(ctx, update)

	return nil
}

func (s *CitizenPortalService) RejectComplaint(ctx context.Context, id uuid.UUID, reason string, userID uuid.UUID) error {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return err
	}

	complaint.Status = models.ComplaintStatusRejected
	complaint.RejectionReason = &reason

	if err := s.repo.UpdateComplaint(ctx, complaint); err != nil {
		return err
	}

	update := &models.ComplaintUpdate{
		ComplaintID: id,
		Status:      models.ComplaintStatusRejected,
		Message:     fmt.Sprintf("Your complaint could not be processed. Reason: %s", reason),
		UpdatedBy:   &userID,
		IsPublic:    true,
	}
	s.repo.AddComplaintUpdate(ctx, update)

	return nil
}

func (s *CitizenPortalService) ConvertToFIR(ctx context.Context, complaintID uuid.UUID, userID uuid.UUID) (uuid.UUID, string, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return uuid.Nil, "", err
	}

	// Create FIR from complaint
	fir := &models.FIR{
		ComplainantName:     *complaint.ComplainantName,
		ComplainantPhone:    complaint.ComplainantPhone,
		ComplainantAddress:  complaint.ComplainantAddress,
		IncidentDate:        *complaint.IncidentDate,
		IncidentLocation:    *complaint.IncidentLocation,
		IncidentDescription: complaint.Description,
		Status:              models.FIRStatusRegistered,
		Priority:            models.PriorityMedium,
		RegisteredBy:        &userID,
		StationID:           *complaint.StationID,
	}

	if err := s.firRepo.Create(ctx, fir); err != nil {
		return uuid.Nil, "", err
	}

	// Update complaint with FIR reference
	complaint.FIRID = &fir.ID
	s.repo.UpdateComplaint(ctx, complaint)

	// Add update
	update := &models.ComplaintUpdate{
		ComplaintID: complaintID,
		Status:      complaint.Status,
		Message:     fmt.Sprintf("Your complaint has been converted to FIR: %s", fir.FIRNumber),
		UpdatedBy:   &userID,
		IsPublic:    true,
	}
	s.repo.AddComplaintUpdate(ctx, update)

	return fir.ID, fir.FIRNumber, nil
}

func (s *CitizenPortalService) AddComplaintUpdate(ctx context.Context, update *models.ComplaintUpdate) error {
	return s.repo.AddComplaintUpdate(ctx, update)
}

func (s *CitizenPortalService) GetComplaintUpdates(ctx context.Context, complaintID uuid.UUID, publicOnly bool) ([]models.ComplaintUpdate, error) {
	return s.repo.GetComplaintUpdates(ctx, complaintID, publicOnly)
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
