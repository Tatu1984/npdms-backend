package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ErrMalkhanaForbidden marks an action outside the officer's station or rank; handlers answer 403.
var ErrMalkhanaForbidden = errors.New("not permitted")

// Actor is the authenticated officer performing a malkhana action.
type MalkhanaActor struct {
	ID        uuid.UUID
	Role      models.Role
	StationID *uuid.UUID
}

type MalkhanaService struct {
	repo      *repository.MalkhanaRepository
	alertRepo *repository.AlertRepository
	auditRepo *repository.AuditRepository
	verifyURL string
}

func NewMalkhanaService(repo *repository.MalkhanaRepository, alertRepo *repository.AlertRepository, auditRepo *repository.AuditRepository) *MalkhanaService {
	// The QR label encodes the property number and this verification address
	// only. Viewing it requires signing in; the label itself carries no case
	// details or personal data.
	base := strings.TrimRight(os.Getenv("MALKHANA_VERIFY_BASE_URL"), "/")
	if base == "" {
		base = "https://npdms.infinititechpartners.com/malkhana/verify"
	}
	return &MalkhanaService{repo: repo, alertRepo: alertRepo, auditRepo: auditRepo, verifyURL: base}
}

func (s *MalkhanaService) audit(ctx context.Context, actor uuid.UUID, action string, itemID *uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "property",
		ResourceID:   itemID,
		Description:  &description,
		Success:      true,
	})
}

// malkhanaScopeStation resolves the station an officer is working at. Officers below
// DSP act only at their own station; DSP and above may name any station.
func malkhanaScopeStation(actor MalkhanaActor, requested *uuid.UUID) (*uuid.UUID, error) {
	if models.RoleAtLeast(actor.Role, models.RoleDSP) {
		return requested, nil
	}
	if actor.StationID == nil {
		return nil, fmt.Errorf("%w: your account has no station", ErrMalkhanaForbidden)
	}
	if requested != nil && *requested != *actor.StationID {
		return nil, fmt.Errorf("%w: officers below DSP can only work with their own station's malkhana", ErrMalkhanaForbidden)
	}
	return actor.StationID, nil
}

// itemForActor loads an item and refuses officers from another station.
func (s *MalkhanaService) itemForActor(ctx context.Context, id uuid.UUID, actor MalkhanaActor) (*models.PropertyItem, error) {
	item, err := s.repo.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if !models.RoleAtLeast(actor.Role, models.RoleDSP) && (actor.StationID == nil || *actor.StationID != item.StationID) {
		// Not found rather than forbidden, so records at other stations are not confirmed to exist.
		return nil, repository.ErrPropertyNotFound
	}
	return item, nil
}

func malkhanaOneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func malkhanaRequired(fields map[string]string) error {
	var missing []string
	for name, v := range fields {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return invalid("required: %s", strings.Join(sortMissing(missing), ", "))
	}
	return nil
}

// ------------------------------------------------------------------ locations

func (s *MalkhanaService) Locations(ctx context.Context, actor MalkhanaActor, stationID *uuid.UUID) ([]models.MalkhanaLocation, error) {
	station, err := malkhanaScopeStation(actor, stationID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListLocations(ctx, station, actor.ID)
}

func (s *MalkhanaService) CreateLocation(ctx context.Context, actor MalkhanaActor, req models.CreateMalkhanaLocationRequest) (*models.MalkhanaLocation, error) {
	if err := malkhanaRequired(map[string]string{"room": req.Room, "rack": req.Rack}); err != nil {
		return nil, err
	}
	station, err := malkhanaScopeStation(actor, req.StationID)
	if err != nil {
		return nil, err
	}
	if station == nil {
		return nil, invalid("choose the station this storage location belongs to")
	}
	loc, err := s.repo.CreateLocation(ctx, *station, req, actor.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "malkhana_location_created", &loc.ID, "Storage location "+loc.Label+" at "+loc.StationName)
	return loc, nil
}

// ---------------------------------------------------------------------- items

func (s *MalkhanaService) List(ctx context.Context, actor MalkhanaActor, f repository.PropertyFilter) ([]models.PropertyItem, int64, error) {
	station, err := malkhanaScopeStation(actor, f.StationID)
	if err != nil {
		return nil, 0, err
	}
	f.StationID = station
	return s.repo.ListItems(ctx, f)
}

func (s *MalkhanaService) Get(ctx context.Context, id uuid.UUID, actor MalkhanaActor) (*models.PropertyItem, error) {
	return s.itemForActor(ctx, id, actor)
}

func (s *MalkhanaService) GetByNumber(ctx context.Context, number string, actor MalkhanaActor) (*models.PropertyItem, error) {
	item, err := s.repo.GetItemByNumber(ctx, strings.TrimSpace(number))
	if err != nil {
		return nil, err
	}
	return s.itemForActor(ctx, item.ID, actor)
}

func (s *MalkhanaService) Register(ctx context.Context, actor MalkhanaActor, stationID *uuid.UUID, req models.RegisterPropertyRequest) (*models.PropertyItem, error) {
	if err := malkhanaRequired(map[string]string{
		"category": req.Category, "description": req.Description, "unit": req.Unit,
		"place of seizure": req.SeizedPlace, "seizure memo reference": req.SeizureMemoRef, "seal number": req.SealNumber,
	}); err != nil {
		return nil, err
	}
	if !malkhanaOneOf(req.Category, models.PropertyCategories) {
		return nil, invalid("unknown category %q", req.Category)
	}
	if req.FIRID == nil && req.CaseID == nil {
		return nil, invalid("link the property to an FIR or a case")
	}
	if req.Quantity <= 0 || math.IsNaN(req.Quantity) {
		return nil, invalid("quantity must be greater than zero")
	}
	if req.WeightGrams != nil && *req.WeightGrams <= 0 {
		return nil, invalid("weight must be greater than zero when recorded")
	}
	if req.ValuePaise != nil && *req.ValuePaise < 0 {
		return nil, invalid("value cannot be negative")
	}
	if req.Category == "NARCOTICS" && req.WeightGrams == nil {
		return nil, invalid("record the weight of seized narcotics")
	}
	if req.SeizedAt == nil {
		return nil, invalid("record when the property was seized")
	}
	if req.SeizedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("the seizure time cannot be in the future")
	}
	if req.LocationID == nil {
		return nil, invalid("choose the storage location the property is deposited in")
	}
	if req.SeizedBy == nil {
		req.SeizedBy = &actor.ID
	} else if _, ok, err := s.repo.OfficerRole(ctx, *req.SeizedBy); err != nil {
		return nil, err
	} else if !ok {
		return nil, invalid("the seizing officer was not found")
	}
	station, err := malkhanaScopeStation(actor, stationID)
	if err != nil {
		return nil, err
	}
	if station == nil {
		return nil, invalid("choose the station whose malkhana receives the property")
	}
	id, err := s.repo.RegisterItem(ctx, *station, req, actor.ID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_registered", &item.ID, fmt.Sprintf("Registered %s (%s) under seal %s at %s",
		item.PropertyNumber, item.Category, item.SealNumber, item.LocationLabel))
	return item, nil
}

func (s *MalkhanaService) SealChecks(ctx context.Context, id uuid.UUID, actor MalkhanaActor) ([]models.PropertySealCheck, error) {
	if _, err := s.itemForActor(ctx, id, actor); err != nil {
		return nil, err
	}
	return s.repo.SealChecks(ctx, id)
}

func (s *MalkhanaService) Movements(ctx context.Context, id uuid.UUID, actor MalkhanaActor) ([]models.PropertyMovement, error) {
	if _, err := s.itemForActor(ctx, id, actor); err != nil {
		return nil, err
	}
	return s.repo.Movements(ctx, id)
}

func (s *MalkhanaService) Events(ctx context.Context, id uuid.UUID, actor MalkhanaActor) ([]models.PropertyEvent, error) {
	if _, err := s.itemForActor(ctx, id, actor); err != nil {
		return nil, err
	}
	return s.repo.Events(ctx, id)
}

func (s *MalkhanaService) VerifySeal(ctx context.Context, id uuid.UUID, actor MalkhanaActor, req models.SealCheckRequest) (*models.PropertyItem, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if err := malkhanaRequired(map[string]string{"seal number": req.SealNumber}); err != nil {
		return nil, err
	}
	if req.Intact == nil {
		return nil, invalid("record whether the seal is intact")
	}
	seal := strings.TrimSpace(req.SealNumber)
	intact := *req.Intact
	note := trimPtr(req.Note)
	// A seal read under a different number than recorded is not intact, whatever was ticked.
	if intact && !strings.EqualFold(seal, item.SealNumber) {
		intact = false
		n := fmt.Sprintf("Seal read as %s; recorded seal is %s", seal, item.SealNumber)
		if note != nil {
			n = *note + " — " + n
		}
		note = &n
	}
	if !intact && note == nil {
		return nil, invalid("describe the seal's condition when it is not intact")
	}
	newlyBroken, err := s.repo.RecordSealCheck(ctx, id, seal, intact, note, actor.ID)
	if err != nil {
		return nil, err
	}
	state := "intact"
	if !intact {
		state = "broken"
	}
	s.audit(ctx, actor.ID, "property_seal_verified", &id, fmt.Sprintf("Seal %s on %s verified %s", seal, item.PropertyNumber, state))
	if newlyBroken {
		s.raiseSealAlert(ctx, actor, item, *note)
	}
	return s.repo.GetItem(ctx, id)
}

// raiseSealAlert issues a station alert for a broken seal. A failure to raise
// it is logged and audited; it never undoes the recorded seal check.
func (s *MalkhanaService) raiseSealAlert(ctx context.Context, actor MalkhanaActor, item *models.PropertyItem, note string) {
	station := item.StationID
	alert := &models.Alert{
		Type:        models.AlertTypeUrgent,
		Scope:       "STATION",
		Title:       "Malkhana seal broken: " + item.PropertyNumber,
		Description: fmt.Sprintf("Seal on %s (%s) at %s recorded as not intact. %s The item cannot leave the malkhana until an SHO records a reason and reseals it.", item.PropertyNumber, item.Category, item.StationName, note),
		ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		IssuedBy:    &actor.ID,
		Priority:    1,
		StationID:   &station,
	}
	if err := s.alertRepo.Create(ctx, alert); err != nil {
		log.Printf("malkhana seal alert for %s failed: %v", item.PropertyNumber, err)
		desc := "Seal alert could not be raised for " + item.PropertyNumber
		s.auditRepo.Log(ctx, &models.SimpleAuditLog{UserID: &actor.ID, Action: "property_seal_alert_failed",
			ResourceType: "property", ResourceID: &item.ID, Description: &desc, Success: false})
		return
	}
	s.audit(ctx, actor.ID, "property_seal_alert_raised", &item.ID, "Alert raised for broken seal on "+item.PropertyNumber)
}

func (s *MalkhanaService) Reseal(ctx context.Context, id uuid.UUID, actor MalkhanaActor, req models.ResealRequest) (*models.PropertyItem, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if err := malkhanaRequired(map[string]string{"reason": req.Reason, "new seal number": req.NewSealNumber}); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(req.Reason)) < 15 {
		return nil, invalid("record the reason in at least 15 characters")
	}
	if err := s.repo.Reseal(ctx, id, strings.TrimSpace(req.Reason), strings.TrimSpace(req.NewSealNumber), actor.ID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_resealed", &id, fmt.Sprintf("%s resealed under %s: %s", item.PropertyNumber, strings.TrimSpace(req.NewSealNumber), strings.TrimSpace(req.Reason)))
	return s.repo.GetItem(ctx, id)
}

func (s *MalkhanaService) Relocate(ctx context.Context, id uuid.UUID, actor MalkhanaActor, req models.RelocateRequest) (*models.PropertyItem, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if req.LocationID == nil {
		return nil, invalid("choose the new storage location")
	}
	if err := malkhanaRequired(map[string]string{"reason": req.Reason}); err != nil {
		return nil, err
	}
	if err := s.repo.Relocate(ctx, id, *req.LocationID, strings.TrimSpace(req.Reason), actor.ID); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_relocated", &id, fmt.Sprintf("%s moved from %s to %s", item.PropertyNumber, item.LocationLabel, updated.LocationLabel))
	return updated, nil
}

// ------------------------------------------------------------------ movements

func (s *MalkhanaService) MoveOut(ctx context.Context, id uuid.UUID, actor MalkhanaActor, req models.MoveOutRequest) (*models.PropertyMovement, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if !malkhanaOneOf(req.MovementType, models.PropertyMovementTypes) {
		return nil, invalid("unknown movement type %q", req.MovementType)
	}
	if err := malkhanaRequired(map[string]string{
		"destination": req.Destination, "purpose": req.Purpose, "authority reference": req.AuthorityRef,
		"handed to": req.HandedTo, "seal number": req.SealNumber,
	}); err != nil {
		return nil, err
	}
	if req.ExpectedReturnAt == nil {
		return nil, invalid("record when the property is expected back")
	}
	if !req.ExpectedReturnAt.After(time.Now()) {
		return nil, invalid("the expected return must be in the future")
	}
	switch req.MovementType {
	case "INTER_STATION":
		if req.DestinationStationID == nil {
			return nil, invalid("choose the station the property is going to")
		}
	case "COURT_PRODUCTION":
		if item.CaseID == nil && req.CourtHearingID != nil {
			return nil, invalid("a hearing can only be linked when the property belongs to a case")
		}
	case "INTERIM_CUSTODY":
		// Interim custody to an owner is on a court's direction; the reference must be the order.
		if len(strings.TrimSpace(req.AuthorityRef)) < 5 {
			return nil, invalid("interim custody needs the court order reference")
		}
	}
	m, err := s.repo.MoveOut(ctx, id, req, actor.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_moved_out", &id, fmt.Sprintf("%s sent to %s (%s) under %s, expected back %s",
		item.PropertyNumber, m.Destination, m.MovementType, m.AuthorityRef, m.ExpectedReturnAt.Format(time.RFC3339)))
	return m, nil
}

func (s *MalkhanaService) Return(ctx context.Context, id, movementID uuid.UUID, actor MalkhanaActor, req models.ReturnMovementRequest) (*models.PropertyMovement, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if err := malkhanaRequired(map[string]string{"returned by": req.ReturnedBy, "seal number": req.SealNumber}); err != nil {
		return nil, err
	}
	if req.SealIntact == nil {
		return nil, invalid("record whether the seal came back intact")
	}
	if req.LocationID == nil {
		return nil, invalid("choose the storage location the property is returned to")
	}
	req.Note = trimPtr(req.Note)
	if !*req.SealIntact && req.Note == nil {
		return nil, invalid("describe the seal's condition when it comes back not intact")
	}
	m, broken, err := s.repo.ReturnMovement(ctx, id, movementID, req, actor.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_returned", &id, fmt.Sprintf("%s returned from %s under seal %s", item.PropertyNumber, m.Destination, strings.TrimSpace(req.SealNumber)))
	if broken {
		note := "Returned from " + m.Destination + " with the seal not intact."
		if m.ReturnNote != nil {
			note = *m.ReturnNote
		}
		s.raiseSealAlert(ctx, actor, item, note)
	}
	return m, nil
}

func (s *MalkhanaService) ForwardingLetter(ctx context.Context, id, movementID uuid.UUID, actor MalkhanaActor) (*models.ForwardingLetter, error) {
	if _, err := s.itemForActor(ctx, id, actor); err != nil {
		return nil, err
	}
	letter, err := s.repo.ForwardingLetter(ctx, id, movementID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_forwarding_letter_printed", &id, "Forwarding letter prepared for "+letter.Reference)
	return letter, nil
}

func (s *MalkhanaService) Dispose(ctx context.Context, id uuid.UUID, actor MalkhanaActor, req models.DisposeRequest) (*models.PropertyItem, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	if !malkhanaOneOf(req.DisposalType, models.PropertyDisposalTypes) {
		return nil, invalid("unknown disposal type %q", req.DisposalType)
	}
	if req.CourtOrderID == nil {
		return nil, invalid("disposal requires the court order recorded against the case")
	}
	if item.CaseID == nil {
		return nil, invalid("the property is linked only to an FIR; link it to a case before a court order can dispose of it")
	}
	if err := malkhanaRequired(map[string]string{"note": req.Note}); err != nil {
		return nil, err
	}
	if item.Category == "NARCOTICS" && req.DisposalType == "DESTROYED" {
		if req.WitnessID == nil {
			return nil, repository.ErrPropertyWitnessRank
		}
		if *req.WitnessID == actor.ID {
			return nil, invalid("the witness to narcotics destruction must be an officer other than the one recording it")
		}
	} else if req.WitnessID != nil {
		role, ok, err := s.repo.OfficerRole(ctx, *req.WitnessID)
		if err != nil {
			return nil, err
		}
		if !ok || role == "" {
			return nil, invalid("the witnessing officer was not found")
		}
	}
	if err := s.repo.Dispose(ctx, id, req, actor.ID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_disposed", &id, fmt.Sprintf("%s disposed (%s) against court order %s", item.PropertyNumber, req.DisposalType, req.CourtOrderID))
	return s.repo.GetItem(ctx, id)
}

// ---------------------------------------------------------------- label & QR

func (s *MalkhanaService) Label(ctx context.Context, id uuid.UUID, actor MalkhanaActor) (*models.PropertyLabel, error) {
	item, err := s.itemForActor(ctx, id, actor)
	if err != nil {
		return nil, err
	}
	url := s.verifyURL + "/" + item.PropertyNumber
	code, err := qr.Encode(url, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	code, err = barcode.Scale(code, 320, 320)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, code); err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "property_label_printed", &id, "Label printed for "+item.PropertyNumber)
	return &models.PropertyLabel{
		PropertyNumber: item.PropertyNumber,
		VerifyURL:      url,
		QRPNG:          "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
		StationName:    item.StationName,
		Category:       item.Category,
		SealNumber:     item.SealNumber,
		LocationLabel:  item.LocationLabel,
		CaseNumber:     item.CaseNumber,
		FIRNumber:      item.FIRNumber,
	}, nil
}

func (s *MalkhanaService) Dashboard(ctx context.Context, actor MalkhanaActor, stationID *uuid.UUID) (*models.MalkhanaDashboard, error) {
	station, err := malkhanaScopeStation(actor, stationID)
	if err != nil {
		return nil, err
	}
	return s.repo.Dashboard(ctx, station, actor.ID)
}

func sortMissing(v []string) []string {
	sort.Strings(v)
	return v
}

func (s *MalkhanaService) Stations(ctx context.Context, viewerID uuid.UUID) ([]models.MalkhanaStation, error) {
	return s.repo.Stations(ctx, viewerID)
}

// Owner answers which department's malkhana holds an item.
func (s *MalkhanaService) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.Owner(ctx, id, viewerID)
}
