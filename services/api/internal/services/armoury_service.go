package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ErrInvalid marks a request the caller must correct; handlers answer 400.
var ErrInvalid = errors.New("invalid request")

func invalid(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

type ArmouryService struct {
	repo      *repository.ArmouryRepository
	auditRepo *repository.AuditRepository
}

func NewArmouryService(repo *repository.ArmouryRepository, auditRepo *repository.AuditRepository) *ArmouryService {
	return &ArmouryService{repo: repo, auditRepo: auditRepo}
}

func (s *ArmouryService) audit(ctx context.Context, action string, actor *uuid.UUID, weaponID uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: "weapon",
		ResourceID:   &weaponID,
		Description:  &description,
		Success:      true,
	})
}

func (s *ArmouryService) List(ctx context.Context, f repository.WeaponFilter) ([]models.Weapon, int64, error) {
	return s.repo.ListWeapons(ctx, f)
}

func (s *ArmouryService) Get(ctx context.Context, id uuid.UUID) (*models.Weapon, error) {
	return s.repo.GetWeapon(ctx, id)
}

func (s *ArmouryService) Stats(ctx context.Context, stationID *uuid.UUID) (*models.WeaponStats, error) {
	return s.repo.Stats(ctx, stationID)
}

func (s *ArmouryService) Issuances(ctx context.Context, f repository.IssuanceFilter) ([]models.WeaponIssuance, int64, error) {
	return s.repo.ListIssuances(ctx, f)
}

// Register adds a weapon to a station's armoury — the officer's own station
// unless another is named.
func (s *ArmouryService) Register(ctx context.Context, req models.CreateWeaponRequest, actor *uuid.UUID, actorStation *uuid.UUID) (*models.Weapon, error) {
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Make) == "" || strings.TrimSpace(req.SerialNumber) == "" {
		return nil, invalid("type, make and serial number are required")
	}
	station := req.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	id, err := s.repo.CreateWeapon(ctx, req, *station, actor)
	if err != nil {
		return nil, err
	}
	w, err := s.repo.GetWeapon(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "weapon_registered", actor, id, fmt.Sprintf("Registered %s %s (serial %s)", w.WeaponNumber, w.Make, w.SerialNumber))
	return w, nil
}

func (s *ArmouryService) SetState(ctx context.Context, id uuid.UUID, req models.SetWeaponStateRequest, actor *uuid.UUID) (*models.Weapon, error) {
	switch req.Status {
	case models.WeaponInArmoury, models.WeaponMaintenance, models.WeaponCondemned:
	case models.WeaponIssued:
		return nil, invalid("use the issue action to issue a weapon")
	default:
		return nil, invalid("unknown status %q", req.Status)
	}
	if !req.Condition.Valid() {
		return nil, invalid("unknown condition %q", req.Condition)
	}
	if req.Status == models.WeaponInArmoury && req.Condition != models.WeaponServiceable {
		return nil, invalid("a weapon returns to the armoury only when serviceable")
	}
	if req.Status != models.WeaponInArmoury && (req.MaintenanceNote == nil || strings.TrimSpace(*req.MaintenanceNote) == "") {
		return nil, invalid("record the reason for maintenance or condemnation")
	}
	if err := s.repo.SetState(ctx, id, req); err != nil {
		return nil, err
	}
	note := ""
	if req.MaintenanceNote != nil {
		note = ": " + *req.MaintenanceNote
	}
	s.audit(ctx, "weapon_state_updated", actor, id, fmt.Sprintf("Set to %s, %s%s", req.Status, req.Condition, note))
	return s.repo.GetWeapon(ctx, id)
}

func (s *ArmouryService) Issue(ctx context.Context, id uuid.UUID, req models.IssueWeaponRequest, actor uuid.UUID) (*models.WeaponIssuance, error) {
	if strings.TrimSpace(req.Purpose) == "" {
		return nil, invalid("purpose is required")
	}
	if req.RoundsIssued < 0 {
		return nil, invalid("rounds issued cannot be negative")
	}
	issue, err := s.repo.Issue(ctx, id, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "weapon_issued", &actor, id, fmt.Sprintf("Issued %s to %s (%s) with %d rounds: %s",
		issue.WeaponNumber, issue.IssuedToName, issue.IssuedToBadge, issue.RoundsIssued, issue.Purpose))
	return issue, nil
}

func (s *ArmouryService) Return(ctx context.Context, id uuid.UUID, req models.ReturnWeaponRequest, actor uuid.UUID) (*models.WeaponIssuance, error) {
	if req.RoundsReturned < 0 {
		return nil, invalid("rounds returned cannot be negative")
	}
	if !req.Condition.Valid() {
		return nil, invalid("unknown condition %q", req.Condition)
	}
	if req.Condition != models.WeaponServiceable && (req.Note == nil || strings.TrimSpace(*req.Note) == "") {
		return nil, invalid("describe the damage when a weapon returns unserviceable or needing repair")
	}
	issue, err := s.repo.Return(ctx, id, req, actor)
	if errors.Is(err, repository.ErrInvalidReturn) {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Returned %s by %s, %d of %d rounds, %s",
		issue.WeaponNumber, issue.IssuedToName, *issue.RoundsReturned, issue.RoundsIssued, req.Condition)
	if short := issue.RoundsIssued - *issue.RoundsReturned; short > 0 {
		desc += fmt.Sprintf(" — %d rounds not returned", short)
	}
	s.audit(ctx, "weapon_returned", &actor, id, desc)
	return issue, nil
}
