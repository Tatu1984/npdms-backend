package services

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// DispatchService runs Phase 07: intake, operator classification, assignment
// of fleet vehicles and on-duty officers, the unit's own progress, the stated
// escalation rules, closure and response analytics.
type DispatchService struct {
	repo      *repository.DispatchRepository
	auditRepo *repository.AuditRepository
}

func NewDispatchService(repo *repository.DispatchRepository, auditRepo *repository.AuditRepository) *DispatchService {
	return &DispatchService{repo: repo, auditRepo: auditRepo}
}

var callerPhonePattern = regexp.MustCompile(`^\+?[0-9]{6,15}$`)

func (s *DispatchService) audit(ctx context.Context, action string, actor *uuid.UUID, incidentID uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: "dispatch_incident",
		ResourceID:   &incidentID,
		Description:  &description,
		Success:      true,
	})
}

// ApplyEscalations raises whatever the stated rules require now and audits
// each escalation. It runs on a timer and before every read, so an overdue
// unit is escalated whether or not anyone is looking at the board.
func (s *DispatchService) ApplyEscalations(ctx context.Context) {
	records, err := s.repo.ApplyEscalations(ctx)
	if err != nil {
		log.Printf("dispatch escalation pass failed: %v", err)
		return
	}
	for _, r := range records {
		s.audit(ctx, "dispatch_escalated", nil, r.IncidentID, r.Number+": "+r.Detail)
	}
}

// RunEscalations applies the rules every interval until ctx ends.
func (s *DispatchService) RunEscalations(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ApplyEscalations(ctx)
		}
	}
}

func (s *DispatchService) List(ctx context.Context, f repository.IncidentFilter) ([]models.DispatchIncident, int64, error) {
	s.ApplyEscalations(ctx)
	return s.repo.ListIncidents(ctx, f)
}

func (s *DispatchService) Get(ctx context.Context, id uuid.UUID) (*models.DispatchIncident, error) {
	s.ApplyEscalations(ctx)
	return s.repo.GetIncident(ctx, id)
}

func (s *DispatchService) Events(ctx context.Context, id uuid.UUID) ([]models.DispatchEvent, error) {
	if _, err := s.repo.GetIncident(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Events(ctx, id)
}

func (s *DispatchService) Stats(ctx context.Context, stationID *uuid.UUID) (*models.DispatchStats, error) {
	s.ApplyEscalations(ctx)
	return s.repo.Stats(ctx, stationID)
}

// Units lists dispatchable units; with an incident, ranked by straight-line
// distance from it.
func (s *DispatchService) Units(ctx context.Context, stationID *uuid.UUID, incidentID *uuid.UUID) ([]models.DispatchUnit, error) {
	var lat, lng *float64
	if incidentID != nil {
		inc, err := s.repo.GetIncident(ctx, *incidentID)
		if err != nil {
			return nil, err
		}
		lat, lng = inc.Latitude, inc.Longitude
	}
	return s.repo.Units(ctx, stationID, lat, lng)
}

func validCoordinates(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return invalid("latitude and longitude must be given together")
	}
	if lat != nil && (*lat < -90 || *lat > 90 || *lng < -180 || *lng > 180) {
		return invalid("coordinates are out of range")
	}
	return nil
}

func (s *DispatchService) Intake(ctx context.Context, req models.CreateIncidentRequest, actor uuid.UUID, actorStation *uuid.UUID) (*models.DispatchIncident, error) {
	if !req.Source.Valid() {
		return nil, invalid("unknown source %q", req.Source)
	}
	if strings.TrimSpace(req.Description) == "" || strings.TrimSpace(req.LocationText) == "" {
		return nil, invalid("what was reported and where are both required")
	}
	if err := validCoordinates(req.Latitude, req.Longitude); err != nil {
		return nil, err
	}
	if req.CallerPhone != nil {
		phone := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(*req.CallerPhone), " ", ""), "-", "")
		if phone == "" {
			req.CallerPhone = nil
		} else if !callerPhonePattern.MatchString(phone) {
			return nil, invalid("caller phone must be 6 to 15 digits")
		} else {
			req.CallerPhone = &phone
		}
	}
	if req.CallerName != nil && strings.TrimSpace(*req.CallerName) == "" {
		req.CallerName = nil
	}
	received := time.Now()
	if req.ReceivedAt != nil {
		if req.ReceivedAt.After(time.Now().Add(5 * time.Minute)) {
			return nil, invalid("the call time cannot be in the future")
		}
		received = *req.ReceivedAt
	}
	station := req.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	id, err := s.repo.CreateIncident(ctx, req, *station, received, actor)
	if err != nil {
		if err == repository.ErrStationNotFound {
			return nil, invalid("unknown station")
		}
		return nil, err
	}
	inc, err := s.repo.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "dispatch_incident_recorded", &actor, id,
		fmt.Sprintf("Logged %s via %s at %s", inc.IncidentNumber, inc.Source, inc.LocationText))
	return inc, nil
}

func (s *DispatchService) Classify(ctx context.Context, id uuid.UUID, req models.ClassifyIncidentRequest, actor uuid.UUID) (*models.DispatchIncident, error) {
	if strings.TrimSpace(req.IncidentType) == "" {
		return nil, invalid("incident type is required")
	}
	if len(strings.TrimSpace(req.IncidentType)) > 40 {
		return nil, invalid("incident type must be 40 characters or fewer")
	}
	if !req.Severity.Valid() {
		return nil, invalid("severity must be one of CRITICAL, HIGH, MEDIUM or LOW")
	}
	if err := s.repo.Classify(ctx, id, req.IncidentType, req.Severity, actor); err != nil {
		return nil, err
	}
	inc, err := s.repo.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "dispatch_incident_classified", &actor, id,
		fmt.Sprintf("%s classified as %s, %s", inc.IncidentNumber, req.IncidentType, req.Severity))
	return inc, nil
}

func (s *DispatchService) Assign(ctx context.Context, id uuid.UUID, req models.AssignUnitRequest, actor uuid.UUID) (*models.DispatchAssignment, error) {
	if req.Kind != models.UnitVehicle && req.Kind != models.UnitOfficer {
		return nil, invalid("unit kind must be VEHICLE or OFFICER")
	}
	a, err := s.repo.Assign(ctx, id, req, actor)
	if err != nil {
		return nil, err
	}
	unit := a.VehicleNumber
	if unit == "" {
		unit = a.OfficerName
	}
	s.audit(ctx, "dispatch_unit_assigned", &actor, id, fmt.Sprintf("%s assigned", unit))
	return a, nil
}

// Progress records the unit's own step. The assigned officer records it, or
// an ASI or above records it on their behalf (a radio report).
func (s *DispatchService) Progress(ctx context.Context, assignmentID uuid.UUID, to models.AssignmentStatus, actor uuid.UUID, actorRole models.Role) (*models.DispatchAssignment, error) {
	mayActForOthers := models.RoleHierarchy[actorRole] >= models.RoleHierarchy[models.RoleASI]
	a, err := s.repo.Progress(ctx, assignmentID, to, actor, mayActForOthers)
	if err != nil {
		return nil, err
	}
	unit := a.VehicleNumber
	if unit == "" {
		unit = a.OfficerName
	}
	s.audit(ctx, "dispatch_unit_"+strings.ToLower(string(to)), &actor, a.IncidentID, fmt.Sprintf("%s %s", unit, strings.ToLower(string(to))))
	return a, nil
}

func (s *DispatchService) CancelAssignment(ctx context.Context, assignmentID uuid.UUID, req models.CancelAssignmentRequest, actor uuid.UUID) (*models.DispatchAssignment, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, invalid("record why the unit is being stood down")
	}
	a, err := s.repo.CancelAssignment(ctx, assignmentID, reason, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "dispatch_unit_stood_down", &actor, a.IncidentID, "Unit stood down: "+reason)
	return a, nil
}

func (s *DispatchService) Escalate(ctx context.Context, id uuid.UUID, req models.EscalateIncidentRequest, actor uuid.UUID) (*models.DispatchIncident, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, invalid("record why the incident is being escalated")
	}
	if err := s.repo.Escalate(ctx, id, reason, actor); err != nil {
		return nil, err
	}
	inc, err := s.repo.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "dispatch_escalated", &actor, id, fmt.Sprintf("%s escalated to level %d: %s", inc.IncidentNumber, inc.EscalationLevel, reason))
	return inc, nil
}

func (s *DispatchService) Close(ctx context.Context, id uuid.UUID, req models.CloseIncidentRequest, actor uuid.UUID) (*models.DispatchIncident, error) {
	if !req.Outcome.Valid() {
		return nil, invalid("unknown outcome %q", req.Outcome)
	}
	if req.Outcome == "FIR_REGISTERED" && req.FIRID == nil {
		return nil, invalid("link the FIR that was registered")
	}
	if req.Outcome != "FIR_REGISTERED" && strings.TrimSpace(req.Note) == "" {
		return nil, invalid("record how the incident ended")
	}
	if err := s.repo.Close(ctx, id, req, actor); err != nil {
		return nil, err
	}
	inc, err := s.repo.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "dispatch_incident_closed", &actor, id, fmt.Sprintf("%s closed: %s", inc.IncidentNumber, req.Outcome))
	return inc, nil
}

func (s *DispatchService) Analytics(ctx context.Context, from, to time.Time, stationID *uuid.UUID) ([]models.DispatchAnalyticsRow, error) {
	if !to.After(from) {
		return nil, invalid("the end of the period must be after its start")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return nil, invalid("the period can be at most one year")
	}
	return s.repo.Analytics(ctx, from, to, stationID)
}
