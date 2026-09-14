package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type LookoutService struct {
	repo      *repository.LookoutRepository
	auditRepo *repository.AuditRepository
}

func NewLookoutService(repo *repository.LookoutRepository, auditRepo *repository.AuditRepository) *LookoutService {
	return &LookoutService{repo: repo, auditRepo: auditRepo}
}

func (s *LookoutService) audit(ctx context.Context, action string, actor uuid.UUID, lookoutID uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "lookout",
		ResourceID:   &lookoutID,
		Description:  &description,
		Success:      true,
	})
}

func (s *LookoutService) List(ctx context.Context, f repository.LookoutFilter) ([]models.Lookout, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *LookoutService) Get(ctx context.Context, id uuid.UUID) (*models.Lookout, error) {
	return s.repo.Get(ctx, id)
}

func (s *LookoutService) Stats(ctx context.Context) (*models.LookoutStats, error) {
	return s.repo.Stats(ctx)
}

func (s *LookoutService) Sightings(ctx context.Context, id uuid.UUID) ([]models.LookoutSighting, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Sightings(ctx, id)
}

func (s *LookoutService) Issue(ctx context.Context, req models.CreateLookoutRequest, actor uuid.UUID, actorStation *uuid.UUID) (*models.Lookout, error) {
	if !req.Type.Valid() {
		return nil, invalid("unknown lookout type %q", req.Type)
	}
	if strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Description) == "" {
		return nil, invalid("subject and description are required")
	}
	switch req.Priority {
	case "":
		req.Priority = "NORMAL"
	case "LOW", "NORMAL", "HIGH", "CRITICAL":
	default:
		return nil, invalid("unknown priority %q", req.Priority)
	}
	station := req.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	id, err := s.repo.Create(ctx, req, *station, actor)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "lookout_issued", actor, id, fmt.Sprintf("Issued %s %s: %s", l.LookoutNumber, l.Type, l.Subject))
	return l, nil
}

func (s *LookoutService) ReportSighting(ctx context.Context, id uuid.UUID, req models.ReportSightingRequest, actor uuid.UUID) (*models.LookoutSighting, error) {
	if strings.TrimSpace(req.Location) == "" {
		return nil, invalid("location is required")
	}
	if (req.Latitude == nil) != (req.Longitude == nil) {
		return nil, invalid("latitude and longitude must be given together")
	}
	if req.Latitude != nil && (*req.Latitude < -90 || *req.Latitude > 90 || *req.Longitude < -180 || *req.Longitude > 180) {
		return nil, invalid("coordinates are out of range")
	}
	// A few minutes of clock skew between terminals is tolerated.
	if req.SightedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a sighting cannot be in the future")
	}
	sighting, err := s.repo.ReportSighting(ctx, id, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "lookout_sighting_recorded", actor, id, fmt.Sprintf("Sighting reported at %s", sighting.Location))
	return sighting, nil
}

func (s *LookoutService) VerifySighting(ctx context.Context, id, sightingID, actor uuid.UUID) (*models.LookoutSighting, error) {
	sighting, err := s.repo.VerifySighting(ctx, id, sightingID, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "lookout_sighting_verified", actor, id,
		fmt.Sprintf("Verified sighting at %s reported by %s", sighting.Location, sighting.ReportedByName))
	return sighting, nil
}

func (s *LookoutService) Resolve(ctx context.Context, id uuid.UUID, req models.ResolveLookoutRequest, actor uuid.UUID) (*models.Lookout, error) {
	if req.Status != models.LookoutLocated && req.Status != models.LookoutClosed {
		return nil, invalid("a lookout is resolved as LOCATED or CLOSED")
	}
	if strings.TrimSpace(req.Note) == "" {
		return nil, invalid("record how the lookout was resolved")
	}
	if err := s.repo.Resolve(ctx, id, req.Status, strings.TrimSpace(req.Note), actor); err != nil {
		return nil, err
	}
	l, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "lookout_resolved", actor, id, fmt.Sprintf("%s marked %s: %s", l.LookoutNumber, req.Status, req.Note))
	return l, nil
}
