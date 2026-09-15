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

// ErrChildRecordRestricted is returned when an officer below the rank allowed
// to handle a child's identifying details, and not assigned to the report,
// asks for it.
var ErrChildRecordRestricted = errors.New("this report concerns a child; it is restricted to SI and above and the officers working it")

// childRecordMinimumRole is the lowest rank that may see a child's identifying
// details without being assigned to the report.
const childRecordMinimumRole = models.RoleSI

// baseChecklist is the first-24-hours SOP applied to every search. Hours are
// measured from the moment the search starts.
var baseChecklist = []repository.ChecklistTemplateItem{
	{Code: "REGISTER_ENTRY", Label: "Entry made in the missing person register", Hours: 1},
	{Code: "INFORMANT_STATEMENT", Label: "Statement of the informant recorded", Hours: 2},
	{Code: "PHOTO_DESCRIPTION_CIRCULATED", Label: "Photograph and description circulated to divisions and control room", Hours: 4},
	{Code: "TRANSPORT_HUBS_ALERTED", Label: "Railway stations, bus termini and ferry ghats alerted", Hours: 6},
	{Code: "HOSPITALS_SHELTERS_CHECKED", Label: "Hospitals, shelters and unidentified-body records checked", Hours: 12},
	{Code: "NATIONAL_PORTALS_UPDATED", Label: "Details entered on the national missing-person portals (done outside this platform, which is not integrated with them)", Hours: 24},
}

// childChecklist is added when the person is a child.
var childChecklist = []repository.ChecklistTemplateItem{
	{Code: "FIR_REGISTERED_CHILD", Label: "FIR registered for the missing child", Hours: 2},
	{Code: "CHILD_WELFARE_INFORMED", Label: "Special Juvenile Police Unit and Child Welfare Committee informed", Hours: 24},
}

var (
	validSightingSources = map[string]bool{"OFFICER_OBSERVATION": true, "PUBLIC_TIP": true, "CCTV_REVIEW": true, "OTHER": true}
	validChannels        = map[string]bool{"IN_PERSON": true, "PHONE": true, "SMS": true, "WHATSAPP": true, "EMAIL": true, "LETTER": true}
	validDirections      = map[string]bool{"OUTBOUND": true, "INBOUND": true}
	validGenders         = map[string]bool{"MALE": true, "FEMALE": true, "TRANSGENDER": true, "UNKNOWN": true}
)

type MissingPersonService struct {
	repo      *repository.MissingPersonRepository
	lookouts  *LookoutService
	auditRepo *repository.AuditRepository
}

func NewMissingPersonService(repo *repository.MissingPersonRepository, lookouts *LookoutService, auditRepo *repository.AuditRepository) *MissingPersonService {
	return &MissingPersonService{repo: repo, lookouts: lookouts, auditRepo: auditRepo}
}

// Viewer is the authenticated officer making a request.
type Viewer struct {
	ID      uuid.UUID
	Role    models.Role
	Station *uuid.UUID
}

func (s *MissingPersonService) audit(ctx context.Context, action string, actor uuid.UUID, id uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: &actor, Action: action, ResourceType: "missing_person", ResourceID: &id,
		Description: &description, Success: true,
	})
}

// priorityFor derives priority from the flags: a child or trafficking risk is
// critical, any other vulnerability is high. Mirrors the table constraint.
func priorityFor(vulnerabilities []string) string {
	priority := "NORMAL"
	for _, v := range vulnerabilities {
		if v == models.VulnerabilityChild || v == models.VulnerabilityTraffickingRisk {
			return "CRITICAL"
		}
		priority = "HIGH"
	}
	return priority
}

// normaliseVulnerabilities validates and de-duplicates flags, and flags every
// person under 18 as a child.
func normaliseVulnerabilities(age int, flags []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, v := range models.Vulnerabilities {
		allowed[v] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, f := range flags {
		f = strings.ToUpper(strings.TrimSpace(f))
		if !allowed[f] {
			return nil, invalid("unknown vulnerability %q", f)
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	if age < 18 && !seen[models.VulnerabilityChild] {
		out = append([]string{models.VulnerabilityChild}, out...)
	}
	return out, nil
}

func checklistFor(vulnerabilities []string) []repository.ChecklistTemplateItem {
	items := append([]repository.ChecklistTemplateItem{}, baseChecklist...)
	for _, v := range vulnerabilities {
		if v == models.VulnerabilityChild {
			items = append(items, childChecklist...)
		}
	}
	return items
}

// canSeeChild: SI and above, or the officer assigned to the report, who
// registered it, or who took it up.
func canSeeChild(p *models.MissingPerson, v Viewer) bool {
	if models.RoleHierarchy[v.Role] >= models.RoleHierarchy[childRecordMinimumRole] {
		return true
	}
	for _, involved := range []*uuid.UUID{p.AssignedTo, p.RegisteredBy, p.SearchStartedBy} {
		if involved != nil && *involved == v.ID {
			return true
		}
	}
	return false
}

// mask blanks a child's identifying details for a viewer who may not see them.
func mask(p *models.MissingPerson) {
	initials := []string{}
	for _, part := range strings.Fields(p.PersonName) {
		initials = append(initials, string([]rune(part)[0])+".")
	}
	p.PersonName = strings.Join(initials, " ")
	p.ReporterName, p.ReporterPhone = "", ""
	p.IdentifyingMarks, p.Height, p.Complexion, p.LastSeenWearing, p.Circumstances = nil, nil, nil, nil, nil
	p.Masked = true
}

func (s *MissingPersonService) List(ctx context.Context, f repository.MissingPersonFilter, v Viewer) ([]models.MissingPerson, int64, error) {
	list, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	for i := range list {
		if list[i].IsChild() && !canSeeChild(&list[i], v) {
			mask(&list[i])
		}
	}
	return list, total, nil
}

func (s *MissingPersonService) Stats(ctx context.Context) (*models.MissingPersonStats, error) {
	return s.repo.Stats(ctx)
}

// authorised loads a report and applies the child-record rule for access to
// the record and everything under it.
func (s *MissingPersonService) authorised(ctx context.Context, id uuid.UUID, v Viewer) (*models.MissingPerson, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.IsChild() && !canSeeChild(p, v) {
		return nil, ErrChildRecordRestricted
	}
	return p, nil
}

// Get returns a report. Every view of a child's record is audited.
func (s *MissingPersonService) Get(ctx context.Context, id uuid.UUID, v Viewer) (*models.MissingPerson, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		if errors.Is(err, ErrChildRecordRestricted) {
			s.auditRepo.Log(ctx, &models.SimpleAuditLog{
				UserID: &v.ID, Action: "missing_person_child_view_denied", ResourceType: "missing_person",
				ResourceID: &id, Success: false,
			})
		}
		return nil, err
	}
	if p.IsChild() {
		s.audit(ctx, "missing_person_child_record_viewed", v.ID, id, "Viewed child record "+p.ReportNumber)
	}
	return p, nil
}

func (s *MissingPersonService) Register(ctx context.Context, req models.RegisterMissingPersonRequest, v Viewer) (*models.MissingPerson, error) {
	for field, value := range map[string]string{
		"person name": req.PersonName, "last seen location": req.LastSeenLocation,
		"reporter name": req.ReporterName, "reporter phone": req.ReporterPhone, "reporter relation": req.ReporterRelation,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, invalid("%s is required", field)
		}
	}
	if req.Age == nil || *req.Age < 0 || *req.Age > 130 {
		return nil, invalid("age must be between 0 and 130")
	}
	req.Gender = strings.ToUpper(strings.TrimSpace(req.Gender))
	if !validGenders[req.Gender] {
		return nil, invalid("gender must be MALE, FEMALE, TRANSGENDER or UNKNOWN")
	}
	if req.LastSeenAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("last seen time cannot be in the future")
	}
	flags, err := normaliseVulnerabilities(*req.Age, req.Vulnerabilities)
	if err != nil {
		return nil, err
	}
	req.Vulnerabilities = flags
	station := req.StationID
	if station == nil {
		station = v.Station
	}
	id, err := s.repo.Register(ctx, req, priorityFor(flags), station, v.ID, checklistFor(flags))
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_registered", v.ID, id,
		fmt.Sprintf("Registered %s, priority %s, flags %v; search started", p.ReportNumber, p.Priority, p.Vulnerabilities))
	return p, nil
}

func (s *MissingPersonService) StartSearch(ctx context.Context, id uuid.UUID, v Viewer) (*models.MissingPerson, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	if err := s.repo.StartSearch(ctx, id, v.ID, v.Station, checklistFor(p.Vulnerabilities)); err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_search_accepted", v.ID, id, "Took up citizen report "+p.ReportNumber+"; search started")
	return s.repo.Get(ctx, id)
}

func (s *MissingPersonService) Update(ctx context.Context, id uuid.UUID, req models.UpdateMissingPersonRequest, v Viewer) (*models.MissingPerson, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	flags := p.Vulnerabilities
	if req.Vulnerabilities != nil {
		requested := *req.Vulnerabilities
		if p.Age < 18 {
			hasChild := false
			for _, f := range requested {
				hasChild = hasChild || strings.EqualFold(strings.TrimSpace(f), models.VulnerabilityChild)
			}
			if !hasChild {
				return nil, invalid("a person under 18 is always flagged as a child")
			}
		}
		if flags, err = normaliseVulnerabilities(p.Age, requested); err != nil {
			return nil, err
		}
	}
	// A child flag added later brings the child-specific checklist items.
	var extra []repository.ChecklistTemplateItem
	wasChild := p.IsChild()
	for _, f := range flags {
		if f == models.VulnerabilityChild && !wasChild {
			extra = childChecklist
		}
	}
	if err := s.repo.Update(ctx, id, req, flags, priorityFor(flags), extra); err != nil {
		return nil, err
	}
	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	desc := "Updated " + p.ReportNumber
	if updated.Priority != p.Priority {
		desc += fmt.Sprintf("; priority %s → %s", p.Priority, updated.Priority)
	}
	s.audit(ctx, "missing_person_updated", v.ID, id, desc)
	return updated, nil
}

func (s *MissingPersonService) Checklist(ctx context.Context, id uuid.UUID, v Viewer) ([]models.MissingPersonChecklistItem, error) {
	if _, err := s.authorised(ctx, id, v); err != nil {
		return nil, err
	}
	return s.repo.Checklist(ctx, id)
}

func (s *MissingPersonService) CompleteChecklistItem(ctx context.Context, id uuid.UUID, code string, req models.CompleteChecklistItemRequest, v Viewer) (*models.MissingPersonChecklistItem, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	var note *string
	if n := strings.TrimSpace(req.Note); n != "" {
		note = &n
	}
	item, err := s.repo.CompleteChecklistItem(ctx, id, code, v.ID, note)
	if err != nil {
		return nil, err
	}
	late := ""
	if item.CompletedAt != nil && item.CompletedAt.After(item.DueAt) {
		late = fmt.Sprintf(" (%.0f minutes after due)", item.CompletedAt.Sub(item.DueAt).Minutes())
	}
	s.audit(ctx, "missing_person_checklist_item_done", v.ID, id,
		fmt.Sprintf("%s: %s%s", p.ReportNumber, item.Label, late))
	return item, nil
}

func (s *MissingPersonService) Sightings(ctx context.Context, id uuid.UUID, v Viewer) ([]models.MissingPersonSighting, error) {
	if _, err := s.authorised(ctx, id, v); err != nil {
		return nil, err
	}
	return s.repo.Sightings(ctx, id)
}

func (s *MissingPersonService) RecordSighting(ctx context.Context, id uuid.UUID, req models.RecordMissingSightingRequest, v Viewer) (*models.MissingPersonSighting, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	req.Source = strings.ToUpper(strings.TrimSpace(req.Source))
	if !validSightingSources[req.Source] {
		return nil, invalid("unknown sighting source %q", req.Source)
	}
	if strings.TrimSpace(req.Location) == "" {
		return nil, invalid("location is required")
	}
	if (req.Latitude == nil) != (req.Longitude == nil) {
		return nil, invalid("latitude and longitude must be given together")
	}
	if req.Latitude != nil && (*req.Latitude < -90 || *req.Latitude > 90 || *req.Longitude < -180 || *req.Longitude > 180) {
		return nil, invalid("coordinates are out of range")
	}
	if req.SightedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a sighting cannot be in the future")
	}
	if req.SightedAt.Before(p.LastSeenAt) {
		return nil, invalid("a sighting cannot be earlier than when the person was last seen")
	}
	sighting, err := s.repo.RecordSighting(ctx, id, req, v.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_sighting_recorded", v.ID, id, fmt.Sprintf("%s: sighting at %s (%s)", p.ReportNumber, sighting.Location, sighting.Source))
	return sighting, nil
}

func (s *MissingPersonService) DecideSighting(ctx context.Context, id, sightingID uuid.UUID, verify bool, req models.DecideSightingRequest, v Viewer) (*models.MissingPersonSighting, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	decision := "VERIFIED"
	action := "missing_person_sighting_verified"
	var note *string
	if n := strings.TrimSpace(req.Note); n != "" {
		note = &n
	}
	if !verify {
		decision, action = "REJECTED", "missing_person_sighting_rejected"
		if note == nil {
			return nil, invalid("record why the sighting is rejected")
		}
	}
	sighting, err := s.repo.DecideSighting(ctx, id, sightingID, v.ID, decision, note)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, action, v.ID, id, fmt.Sprintf("%s: sighting at %s reported by %s marked %s",
		p.ReportNumber, sighting.Location, sighting.ReportedByName, decision))
	return sighting, nil
}

func (s *MissingPersonService) Movement(ctx context.Context, id uuid.UUID, v Viewer) ([]models.MovementPoint, error) {
	if _, err := s.authorised(ctx, id, v); err != nil {
		return nil, err
	}
	return s.repo.Movement(ctx, id)
}

func (s *MissingPersonService) FamilyContacts(ctx context.Context, id uuid.UUID, v Viewer) ([]models.FamilyContact, error) {
	if _, err := s.authorised(ctx, id, v); err != nil {
		return nil, err
	}
	return s.repo.FamilyContacts(ctx, id)
}

func (s *MissingPersonService) RecordFamilyContact(ctx context.Context, id uuid.UUID, req models.RecordFamilyContactRequest, v Viewer) (*models.FamilyContact, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	req.Direction = strings.ToUpper(strings.TrimSpace(req.Direction))
	req.Channel = strings.ToUpper(strings.TrimSpace(req.Channel))
	if !validDirections[req.Direction] {
		return nil, invalid("direction must be OUTBOUND or INBOUND")
	}
	if !validChannels[req.Channel] {
		return nil, invalid("unknown contact channel %q", req.Channel)
	}
	if strings.TrimSpace(req.ContactName) == "" || strings.TrimSpace(req.Summary) == "" {
		return nil, invalid("contact name and summary are required")
	}
	if req.ContactedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a contact cannot be in the future")
	}
	contact, err := s.repo.RecordFamilyContact(ctx, id, req, v.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_family_contact_recorded", v.ID, id,
		fmt.Sprintf("%s: %s %s contact with %s", p.ReportNumber, strings.ToLower(req.Direction), strings.ToLower(req.Channel), contact.ContactName))
	return contact, nil
}

// Close records the outcome. A linked lookout still active is resolved with
// the same note, so the public notice does not outlive the search.
func (s *MissingPersonService) Close(ctx context.Context, id uuid.UUID, req models.CloseMissingPersonRequest, v Viewer) (*models.MissingPerson, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	req.Outcome = strings.ToUpper(strings.TrimSpace(req.Outcome))
	var status models.MissingPersonStatus
	switch req.Outcome {
	case "TRACED", "RETURNED":
		status = models.MissingFound
	case "DECEASED", "OTHER":
		status = models.MissingClosed
	default:
		return nil, invalid("outcome must be TRACED, RETURNED, DECEASED or OTHER")
	}
	if strings.TrimSpace(req.Note) == "" {
		return nil, invalid("record how the report was closed")
	}
	if err := s.repo.Close(ctx, id, req, status, v.ID); err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_closed", v.ID, id, fmt.Sprintf("%s closed as %s: %s", p.ReportNumber, req.Outcome, req.Note))

	if p.LookoutID != nil {
		resolution := models.LookoutClosed
		if status == models.MissingFound {
			resolution = models.LookoutLocated
		}
		_, err := s.lookouts.Resolve(ctx, *p.LookoutID, models.ResolveLookoutRequest{
			Status: resolution, Note: fmt.Sprintf("Missing person report %s closed as %s: %s", p.ReportNumber, req.Outcome, req.Note),
		}, v.ID)
		if err != nil && !errors.Is(err, repository.ErrLookoutNotActive) {
			return nil, fmt.Errorf("report closed but the linked lookout could not be resolved: %w", err)
		}
	}
	return s.repo.Get(ctx, id)
}

// IssueLookout publishes a MISSING notice on the lookout register from the
// report, rather than keeping a second notice list, and links it.
func (s *MissingPersonService) IssueLookout(ctx context.Context, id uuid.UUID, v Viewer) (*models.MissingPerson, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	if p.Status != models.MissingSearching {
		if p.Status == models.MissingReported {
			return nil, invalid("take up the report and start the search before issuing a lookout")
		}
		return nil, repository.ErrMissingPersonNotOpen
	}
	if p.LookoutID != nil {
		return nil, repository.ErrLookoutAlreadyLinked
	}
	details := map[string]string{
		"Report":    p.ReportNumber,
		"Age":       fmt.Sprintf("%d", p.Age),
		"Gender":    p.Gender,
		"Last seen": fmt.Sprintf("%s, %s", p.LastSeenLocation, p.LastSeenAt.In(istLocation()).Format("02 Jan 2006 15:04")),
	}
	for key, value := range map[string]*string{"Wearing": p.LastSeenWearing, "Identifying marks": p.IdentifyingMarks, "Height": p.Height} {
		if value != nil && strings.TrimSpace(*value) != "" {
			details[key] = *value
		}
	}
	lookoutPriority := "HIGH"
	if p.Priority == "CRITICAL" {
		lookoutPriority = "CRITICAL"
	}
	lookout, err := s.lookouts.Issue(ctx, models.CreateLookoutRequest{
		Type:        models.LookoutMissing,
		Subject:     p.PersonName,
		Description: fmt.Sprintf("Missing since %s from %s.", p.LastSeenAt.In(istLocation()).Format("02 Jan 2006 15:04"), p.LastSeenLocation),
		Details:     details,
		Priority:    lookoutPriority,
		FIRID:       p.FIRID,
		StationID:   p.StationID,
	}, v.ID, v.Station)
	if err != nil {
		return nil, err
	}
	if err := s.repo.LinkLookout(ctx, id, lookout.ID); err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_lookout_linked", v.ID, id, fmt.Sprintf("%s: lookout %s issued and linked", p.ReportNumber, lookout.LookoutNumber))
	return s.repo.Get(ctx, id)
}

func istLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
		return loc
	}
	return time.FixedZone("IST", 5*3600+1800)
}
