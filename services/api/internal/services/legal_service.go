package services

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// LegalService serves the statute library (Acts, sections, the IPC-BNS
// correspondence) and the Kolkata gazetteer used for incident locations.
// Published law text and imported map places are read-only; officers of SP
// rank and above add custom entries, and every change is audited with its reason.
type LegalService struct {
	legal     *repository.LegalRepository
	gazetteer *repository.GazetteerRepository
	auditRepo *repository.AuditRepository
}

func NewLegalService(legal *repository.LegalRepository, gazetteer *repository.GazetteerRepository, auditRepo *repository.AuditRepository) *LegalService {
	return &LegalService{legal: legal, gazetteer: gazetteer, auditRepo: auditRepo}
}

func (s *LegalService) audit(ctx context.Context, action, resourceType string, actor uuid.UUID, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   &id,
		Description:  &description,
		Success:      true,
	})
}

var (
	actCodePattern       = regexp.MustCompile(`^[A-Z0-9_]{2,16}$`)
	pinCodePattern       = regexp.MustCompile(`^7[0-4][0-9]{4}$`)
	sectionNumberPattern = regexp.MustCompile(`^([0-9]{1,4})(-?[A-Z]{1,4})?((?:\([0-9A-Za-z]+\))*)$`)
)

// requireReason trims a mandatory reason for a change to reference data.
func requireReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 {
		return "", invalid("give a reason of at least 5 characters; it is kept in the audit log")
	}
	if len(reason) > 1000 {
		return "", invalid("keep the reason under 1000 characters")
	}
	return reason, nil
}

// normaliseSectionNumber turns " 66 d" or "303 (2)" into "66D" and "303(2)".
func normaliseSectionNumber(n string) (string, error) {
	n = strings.ReplaceAll(strings.TrimSpace(n), " ", "")
	if i := strings.Index(n, "("); i >= 0 {
		n = strings.ToUpper(n[:i]) + n[i:]
	} else {
		n = strings.ToUpper(n)
	}
	if !sectionNumberPattern.MatchString(n) || len(n) > 24 {
		return "", invalid("write the section number as digits with an optional letter and sub-sections, for example 303, 66D or 318(4)")
	}
	return n, nil
}

/* --------------------------------------------------------------- reading --- */

func (s *LegalService) ListActs(ctx context.Context, includeRetired bool) ([]models.LegalAct, error) {
	return s.legal.ListActs(ctx, includeRetired)
}

func (s *LegalService) GetAct(ctx context.Context, id uuid.UUID) (*models.LegalAct, error) {
	return s.legal.GetAct(ctx, id)
}

func (s *LegalService) ListSections(ctx context.Context, actID uuid.UUID, page, size int, includeRetired bool) ([]models.LegalSection, int64, error) {
	if _, err := s.legal.GetAct(ctx, actID); err != nil {
		return nil, 0, err
	}
	return s.legal.ListSections(ctx, actID, page, size, includeRetired)
}

func (s *LegalService) GetSection(ctx context.Context, id uuid.UUID) (*models.LegalSection, error) {
	return s.legal.GetSection(ctx, id)
}

func (s *LegalService) SearchSections(ctx context.Context, q repository.SectionQuery) ([]models.LegalSection, error) {
	if len(q.Q) > 120 {
		return nil, invalid("search text is too long")
	}
	return s.legal.SearchSections(ctx, q)
}

// Correspondence looks up the published IPC-BNS table from either side.
func (s *LegalService) Correspondence(ctx context.Context, ipc, bns string) ([]models.LegalCorrespondence, error) {
	ipc = strings.ToUpper(strings.TrimSpace(ipc))
	bns = strings.TrimSpace(bns)
	if i := strings.Index(bns, "("); i > 0 {
		bns = bns[:i]
	}
	if ipc == "" && bns == "" {
		return nil, invalid("give an IPC or a BNS section number")
	}
	return s.legal.Correspondence(ctx, ipc, bns)
}

/* ------------------------------------------------------------------ acts --- */

func validateActFields(citation, shortName, name, source, completeness *string, year *int, sourceURL **string, actNumber **string) error {
	*citation = strings.TrimSpace(*citation)
	*shortName = strings.TrimSpace(*shortName)
	*name = strings.TrimSpace(*name)
	*source = strings.TrimSpace(*source)
	*completeness = strings.TrimSpace(*completeness)
	*sourceURL = trimPtr(*sourceURL)
	*actNumber = trimPtr(*actNumber)
	switch {
	case *citation == "" || len(*citation) > 40:
		return invalid("give the citation used on an FIR, such as \"WB Excise Act\" (up to 40 characters)")
	case *shortName == "" || len(*shortName) > 120:
		return invalid("give a short name (up to 120 characters)")
	case *name == "" || len(*name) > 255:
		return invalid("give the full title of the Act (up to 255 characters)")
	case *source == "":
		return invalid("name the official source of the text, such as India Code or the Gazette notification")
	case *completeness != "complete" && *completeness != "selected":
		return invalid("say whether the library holds the complete Act or selected sections")
	case year != nil && (*year < 1800 || *year > 2100):
		return invalid("the year must be between 1800 and 2100")
	case *sourceURL != nil && !strings.HasPrefix(**sourceURL, "https://") && !strings.HasPrefix(**sourceURL, "http://"):
		return invalid("the source link must start with https://")
	}
	return nil
}

func (s *LegalService) CreateAct(ctx context.Context, req models.CreateLegalActRequest, actor uuid.UUID) (*models.LegalAct, error) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if !actCodePattern.MatchString(req.Code) {
		return nil, invalid("the code must be 2 to 16 capital letters, digits or underscores, such as WBEXCISE")
	}
	if err := validateActFields(&req.Citation, &req.ShortName, &req.Name, &req.Source, &req.Completeness, req.Year, &req.SourceURL, &req.ActNumber); err != nil {
		return nil, err
	}
	reason, err := requireReason(req.Reason)
	if err != nil {
		return nil, err
	}
	id, err := s.legal.CreateAct(ctx, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_act_added", "legal_act", actor, id, fmt.Sprintf("Added Act %s (%s): %s", req.Code, req.Name, reason))
	return s.legal.GetAct(ctx, id)
}

func (s *LegalService) UpdateAct(ctx context.Context, id uuid.UUID, req models.UpdateLegalActRequest, actor uuid.UUID) (*models.LegalAct, error) {
	act, err := s.legal.GetAct(ctx, id)
	if err != nil {
		return nil, err
	}
	if act.IsBuiltin {
		return nil, repository.ErrLegalBuiltinReadOnly
	}
	if err := validateActFields(&req.Citation, &req.ShortName, &req.Name, &req.Source, &req.Completeness, req.Year, &req.SourceURL, &req.ActNumber); err != nil {
		return nil, err
	}
	reason, err := requireReason(req.Reason)
	if err != nil {
		return nil, err
	}
	if err := s.legal.UpdateAct(ctx, id, req); err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_act_corrected", "legal_act", actor, id, fmt.Sprintf("Corrected Act %s: %s", act.Code, reason))
	return s.legal.GetAct(ctx, id)
}

func (s *LegalService) RetireAct(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) (*models.LegalAct, error) {
	act, err := s.legal.GetAct(ctx, id)
	if err != nil {
		return nil, err
	}
	if act.IsBuiltin {
		return nil, repository.ErrLegalBuiltinReadOnly
	}
	if reason, err = requireReason(reason); err != nil {
		return nil, err
	}
	if err := s.legal.RetireAct(ctx, id, reason); err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_act_retired", "legal_act", actor, id, fmt.Sprintf("Retired Act %s: %s", act.Code, reason))
	return s.legal.GetAct(ctx, id)
}

/* -------------------------------------------------------------- sections --- */

func validateSection(req *models.LegalSectionRequest) (string, error) {
	n, err := normaliseSectionNumber(req.Number)
	if err != nil {
		return "", err
	}
	req.Number = n
	req.Heading = strings.TrimSpace(req.Heading)
	if req.Heading == "" || len(req.Heading) > 500 {
		return "", invalid("give the section heading as published (up to 500 characters)")
	}
	req.Description = trimPtr(req.Description)
	return requireReason(req.Reason)
}

func (s *LegalService) AddSection(ctx context.Context, actID uuid.UUID, req models.LegalSectionRequest, actor uuid.UUID) (*models.LegalSection, error) {
	reason, err := validateSection(&req)
	if err != nil {
		return nil, err
	}
	act, err := s.legal.GetAct(ctx, actID)
	if err != nil {
		return nil, err
	}
	id, err := s.legal.AddSection(ctx, actID, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_section_added", "legal_section", actor, id,
		fmt.Sprintf("Added %s %s (%s): %s", act.Citation, req.Number, req.Heading, reason))
	return s.legal.GetSection(ctx, id)
}

func (s *LegalService) UpdateSection(ctx context.Context, id uuid.UUID, req models.LegalSectionRequest, actor uuid.UUID) (*models.LegalSection, error) {
	current, err := s.legal.GetSection(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.IsBuiltin {
		return nil, repository.ErrLegalBuiltinReadOnly
	}
	reason, err := validateSection(&req)
	if err != nil {
		return nil, err
	}
	if err := s.legal.UpdateSection(ctx, id, req); err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_section_corrected", "legal_section", actor, id,
		fmt.Sprintf("Corrected %s (was %q, now %s %q): %s", current.Cite, current.Heading, req.Number, req.Heading, reason))
	return s.legal.GetSection(ctx, id)
}

func (s *LegalService) RetireSection(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) (*models.LegalSection, error) {
	current, err := s.legal.GetSection(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.IsBuiltin {
		return nil, repository.ErrLegalBuiltinReadOnly
	}
	if reason, err = requireReason(reason); err != nil {
		return nil, err
	}
	if err := s.legal.RetireSection(ctx, id, reason); err != nil {
		return nil, err
	}
	s.audit(ctx, "legal_section_retired", "legal_section", actor, id, fmt.Sprintf("Retired %s: %s", current.Cite, reason))
	return s.legal.GetSection(ctx, id)
}

/* ------------------------------------------------------------- gazetteer --- */

// Incident points must fall inside West Bengal; the firs and gazetteer_places
// CHECK constraints use the same box.
const (
	minIncidentLat = 21.4
	maxIncidentLat = 27.3
	minIncidentLng = 85.8
	maxIncidentLng = 89.9
)

// ValidateIncidentPoint accepts both coordinates or neither, inside West Bengal.
func ValidateIncidentPoint(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return invalid("give both latitude and longitude for the incident location, or neither")
	}
	if lat == nil {
		return nil
	}
	if math.IsNaN(*lat) || math.IsNaN(*lng) || *lat < minIncidentLat || *lat > maxIncidentLat || *lng < minIncidentLng || *lng > maxIncidentLng {
		return invalid("the incident location must be inside West Bengal; move the pin onto the map of Kolkata")
	}
	return nil
}

var gazetteerKinds = map[string]bool{
	"locality": true, "road": true, "landmark": true, "police_station": true,
	"rail_station": true, "metro_station": true, "pin_code": true,
}

func (s *LegalService) SearchPlaces(ctx context.Context, q string, limit int) ([]models.GazetteerPlace, error) {
	if len(q) > 120 {
		return nil, invalid("search text is too long")
	}
	return s.gazetteer.Search(ctx, q, limit)
}

func (s *LegalService) NearestPlaces(ctx context.Context, lat, lng float64, limit int) ([]models.GazetteerPlace, error) {
	if err := ValidateIncidentPoint(&lat, &lng); err != nil {
		return nil, err
	}
	return s.gazetteer.Nearest(ctx, lat, lng, limit)
}

func (s *LegalService) ListOfficerPlaces(ctx context.Context, status string, page, size int) ([]models.GazetteerPlace, int64, error) {
	switch status {
	case "", "active", "retired":
	default:
		return nil, 0, invalid("status must be active or retired")
	}
	return s.gazetteer.ListOfficerPlaces(ctx, status, page, size)
}

func (s *LegalService) AddPlace(ctx context.Context, req models.AddPlaceRequest, actor uuid.UUID) (*models.GazetteerPlace, error) {
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	req.NameBn = trimPtr(req.NameBn)
	req.Pin = trimPtr(req.Pin)
	switch {
	case !gazetteerKinds[req.Kind]:
		return nil, invalid("choose what kind of place this is")
	case len([]rune(req.Name)) < 2 || len(req.Name) > 200:
		return nil, invalid("give the place name (2 to 200 characters)")
	case req.Pin != nil && !pinCodePattern.MatchString(*req.Pin):
		return nil, invalid("a West Bengal PIN code has six digits starting 70 to 74")
	case req.Latitude == nil || req.Longitude == nil:
		return nil, invalid("place the pin on the map to set the location")
	}
	if err := ValidateIncidentPoint(req.Latitude, req.Longitude); err != nil {
		return nil, err
	}
	reason, err := requireReason(req.Reason)
	if err != nil {
		return nil, err
	}
	req.Reason = reason
	id, err := s.gazetteer.Add(ctx, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "gazetteer_place_added", "gazetteer_place", actor, id,
		fmt.Sprintf("Added %s %q at %.6f, %.6f: %s", req.Kind, req.Name, *req.Latitude, *req.Longitude, reason))
	return s.gazetteer.Get(ctx, id)
}

func (s *LegalService) RetirePlace(ctx context.Context, id uuid.UUID, reason string, actor uuid.UUID) (*models.GazetteerPlace, error) {
	reason, err := requireReason(reason)
	if err != nil {
		return nil, err
	}
	if err := s.gazetteer.Retire(ctx, id, reason, actor); err != nil {
		return nil, err
	}
	p, err := s.gazetteer.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "gazetteer_place_retired", "gazetteer_place", actor, id, fmt.Sprintf("Retired %s %q: %s", p.Kind, p.Name, reason))
	return p, nil
}
