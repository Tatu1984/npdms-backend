package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ErrTrackingMismatch is returned for an unknown tracking number and for a
// known one with the wrong phone or access code alike, so a caller cannot
// learn which tracking numbers exist.
var ErrTrackingMismatch = errors.New("no complaint matches this tracking number and phone number or access code")

// ErrForbiddenAction marks a rank the action needs; handlers answer 403.
var ErrForbiddenAction = errors.New("insufficient rank for this action")

type ComplaintService struct {
	repo      *repository.ComplaintRepository
	auditRepo *repository.AuditRepository
}

func NewComplaintService(repo *repository.ComplaintRepository, auditRepo *repository.AuditRepository) *ComplaintService {
	return &ComplaintService{repo: repo, auditRepo: auditRepo}
}

// Citizen-visible messages. Stored bilingually because the citizen chooses
// no language when tracking, and never name an officer or another complaint.
const (
	msgReceived     = "Complaint received. — অভিযোগ গৃহীত হয়েছে।"
	msgAcknowledged = "Your complaint has been reviewed and categorised. — আপনার অভিযোগ পর্যালোচনা ও শ্রেণিবদ্ধ করা হয়েছে।"
	msgRouted       = "Your complaint has been sent to the police station responsible. — আপনার অভিযোগ দায়িত্বপ্রাপ্ত থানায় পাঠানো হয়েছে।"
	msgInProgress   = "Action on your complaint is in progress. — আপনার অভিযোগ নিয়ে কাজ চলছে।"
	msgResponse     = "A response to your complaint has been issued. — আপনার অভিযোগের উত্তর দেওয়া হয়েছে।"
	msgResolved     = "Your complaint has been resolved. — আপনার অভিযোগের নিষ্পত্তি হয়েছে।"
	msgClosed       = "Your complaint has been closed. — আপনার অভিযোগ বন্ধ করা হয়েছে।"
	msgReopened     = "Your complaint has been reopened for further action. — আরও ব্যবস্থার জন্য আপনার অভিযোগ আবার খোলা হয়েছে।"
	msgRejected     = "Your complaint could not be taken up. The reason is shown below. — আপনার অভিযোগ গ্রহণ করা যায়নি। কারণ নিচে দেওয়া আছে।"
	msgMerged       = "Your complaint is being handled together with a related complaint. — আপনার অভিযোগটি একটি সম্পর্কিত অভিযোগের সঙ্গে একসাথে দেখা হচ্ছে।"
)

// Allowed officer-driven status changes. Acknowledgement happens on
// categorisation and assignment on routing, so neither is set directly.
var complaintTransitions = map[models.ComplaintStatus][]models.ComplaintStatus{
	models.ComplaintStatusSubmitted:    {models.ComplaintStatusRejected},
	models.ComplaintStatusAcknowledged: {models.ComplaintStatusInProgress, models.ComplaintStatusRejected},
	models.ComplaintStatusAssigned:     {models.ComplaintStatusInProgress, models.ComplaintStatusResolved, models.ComplaintStatusRejected},
	models.ComplaintStatusInProgress:   {models.ComplaintStatusResolved},
	models.ComplaintStatusResolved:     {models.ComplaintStatusClosed, models.ComplaintStatusInProgress},
}

func transitionAllowed(from, to models.ComplaintStatus) bool {
	for _, s := range complaintTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

func atLeast(role models.Role, floor models.Role) bool {
	return models.RoleHierarchy[role] >= models.RoleHierarchy[floor]
}

var phoneDigits = regexp.MustCompile(`\D`)

// normalizeIndianMobile accepts +91, 0 or no prefix and returns the ten-digit
// number, or "" when it is not a valid Indian mobile number.
// NormalizeIndianMobile is normalizeIndianMobile for other packages.
func NormalizeIndianMobile(s string) string { return normalizeIndianMobile(s) }

func normalizeIndianMobile(s string) string {
	d := phoneDigits.ReplaceAllString(s, "")
	if len(d) == 12 && strings.HasPrefix(d, "91") {
		d = d[2:]
	} else if len(d) == 11 && strings.HasPrefix(d, "0") {
		d = d[1:]
	}
	if len(d) != 10 || d[0] < '6' {
		return ""
	}
	return d
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func hashAccessCode(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

// newAccessCode returns ten characters from an alphabet without look-alike
// characters (no 0/O, 1/I/L), from a cryptographic source.
func newAccessCode() (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 10)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

func (s *ComplaintService) audit(ctx context.Context, action string, actor *uuid.UUID, id *uuid.UUID, success bool, desc string, ip, ua *string) {
	entry := &models.SimpleAuditLog{
		UserID: actor, Action: action, ResourceType: "citizen_complaint", ResourceID: id,
		Description: &desc, Success: success, IPAddress: ip, UserAgent: ua,
	}
	if !success {
		entry.FailureReason = &desc
	}
	s.auditRepo.Log(ctx, entry)
}

// validateIntake checks and normalises a complaint however it arrived.
func validateIntake(req *models.ComplaintIntakeRequest) error {
	req.Subject = strings.TrimSpace(req.Subject)
	req.Description = strings.TrimSpace(req.Description)
	req.SourceReference = trimPtr(req.SourceReference)
	req.ComplainantName = trimPtr(req.ComplainantName)
	req.ComplainantEmail = trimPtr(req.ComplainantEmail)
	req.ComplainantAddress = trimPtr(req.ComplainantAddress)
	req.IncidentLocation = trimPtr(req.IncidentLocation)

	if !req.Category.Valid() {
		return invalid("choose a complaint category")
	}
	if n := utf8.RuneCountInString(req.Subject); n < 3 || n > 200 {
		return invalid("subject must be between 3 and 200 characters")
	}
	if n := utf8.RuneCountInString(req.Description); n < 10 || n > 5000 {
		return invalid("description must be between 10 and 5000 characters")
	}
	if req.IncidentLocation != nil && utf8.RuneCountInString(*req.IncidentLocation) > 500 {
		return invalid("incident location must be at most 500 characters")
	}
	if req.ComplainantAddress != nil && utf8.RuneCountInString(*req.ComplainantAddress) > 500 {
		return invalid("address must be at most 500 characters")
	}
	if req.IsAnonymous {
		// Nothing identifying is kept for an anonymous complaint.
		req.ComplainantName, req.ComplainantPhone, req.ComplainantEmail, req.ComplainantAddress = nil, nil, nil, nil
		return nil
	}
	if req.ComplainantName == nil || utf8.RuneCountInString(*req.ComplainantName) > 120 {
		return invalid("complainant name is required (at most 120 characters) unless the complaint is anonymous")
	}
	if req.ComplainantPhone == nil {
		return invalid("a mobile number is required unless the complaint is anonymous")
	}
	phone := normalizeIndianMobile(*req.ComplainantPhone)
	if phone == "" {
		return invalid("enter a valid 10-digit Indian mobile number")
	}
	req.ComplainantPhone = &phone
	if req.ComplainantEmail != nil {
		if _, err := mail.ParseAddress(*req.ComplainantEmail); err != nil || len(*req.ComplainantEmail) > 200 {
			return invalid("enter a valid email address or leave it blank")
		}
	}
	return nil
}

// SubmitPublic records a complaint made through the public web portal.
func (s *ComplaintService) SubmitPublic(ctx context.Context, req models.ComplaintIntakeRequest, ip, ua string) (*models.PublicSubmitResult, error) {
	req.Channel = models.ChannelWeb
	req.SourceReference = nil
	if err := validateIntake(&req); err != nil {
		return nil, err
	}
	result := &models.PublicSubmitResult{}
	var codeHash *string
	if req.IsAnonymous {
		code, err := newAccessCode()
		if err != nil {
			return nil, err
		}
		h := hashAccessCode(code)
		codeHash = &h
		result.AccessCode = &code
	}
	id, number, err := s.repo.Create(ctx, repository.NewComplaint{
		Request: req, AccessCodeHash: codeHash,
		Script:        repository.DetectScript(req.Subject, req.Description),
		PublicMessage: msgReceived,
	})
	if err != nil {
		return nil, err
	}
	result.TrackingNumber = number
	s.audit(ctx, "complaint_submitted_web", nil, &id, true, "Public web complaint "+number, &ip, &ua)
	return result, nil
}

// Record takes a complaint from the counter or enters one that arrived on
// another channel.
func (s *ComplaintService) Record(ctx context.Context, req models.ComplaintIntakeRequest, actor uuid.UUID, actorStation *uuid.UUID) (*models.Complaint, *models.PublicSubmitResult, error) {
	if !req.Channel.Valid() || req.Channel == models.ChannelWeb {
		return nil, nil, invalid("choose the channel the complaint arrived through; web complaints are submitted by citizens on the portal")
	}
	if err := validateIntake(&req); err != nil {
		return nil, nil, err
	}
	if req.Channel.NeedsSourceReference() && req.SourceReference == nil {
		return nil, nil, invalid("record the %s reference the complaint arrived under", strings.ToLower(strings.ReplaceAll(string(req.Channel), "_", " ")))
	}
	if req.SourceReference != nil && utf8.RuneCountInString(*req.SourceReference) > 200 {
		return nil, nil, invalid("source reference must be at most 200 characters")
	}
	result := &models.PublicSubmitResult{}
	var codeHash *string
	if req.IsAnonymous {
		code, err := newAccessCode()
		if err != nil {
			return nil, nil, err
		}
		h := hashAccessCode(code)
		codeHash = &h
		result.AccessCode = &code
	}
	// A counter complaint belongs to the station that took it; complaints
	// entered from remote channels wait to be routed.
	var station *uuid.UUID
	if req.Channel == models.ChannelCounter {
		station = actorStation
	}
	id, number, err := s.repo.Create(ctx, repository.NewComplaint{
		Request: req, RecordedBy: &actor, StationID: station, AccessCodeHash: codeHash,
		Script:        repository.DetectScript(req.Subject, req.Description),
		PublicMessage: msgReceived,
	})
	if err != nil {
		return nil, nil, err
	}
	result.TrackingNumber = number
	s.audit(ctx, "complaint_recorded", &actor, &id, true, fmt.Sprintf("Recorded %s complaint %s", req.Channel, number), nil, nil)
	c, err := s.repo.Get(ctx, id)
	return c, result, err
}

func (s *ComplaintService) List(ctx context.Context, f repository.ComplaintFilter) ([]models.Complaint, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *ComplaintService) Stats(ctx context.Context, station *uuid.UUID) (*models.ComplaintStats, error) {
	return s.repo.Stats(ctx, station)
}

func (s *ComplaintService) RoutingTargets(ctx context.Context) ([]models.RoutingTarget, error) {
	return s.repo.RoutingTargets(ctx)
}

// ComplaintDetail is the officer view of one complaint.
type ComplaintDetail struct {
	*models.Complaint
	History   []models.ComplaintHistoryEntry `json:"history"`
	Routings  []models.ComplaintRouting      `json:"routings"`
	Responses []models.ComplaintResponse     `json:"responses"`
}

// Get returns the full record. Viewing a complainant's details is audited.
func (s *ComplaintService) Get(ctx context.Context, id uuid.UUID, actor uuid.UUID) (*ComplaintDetail, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &ComplaintDetail{Complaint: c}
	if d.History, err = s.repo.History(ctx, id, false); err != nil {
		return nil, err
	}
	if d.Routings, err = s.repo.Routings(ctx, id); err != nil {
		return nil, err
	}
	if d.Responses, err = s.repo.Responses(ctx, id, false); err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_viewed", &actor, &id, true, "Viewed complaint "+c.TrackingNumber, nil, nil)
	return d, nil
}

func (s *ComplaintService) DuplicateCandidates(ctx context.Context, id uuid.UUID) ([]models.DuplicateCandidate, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.DuplicateCandidates(ctx, id)
}

func closedFor(action string) func(models.ComplaintStatus, *uuid.UUID) error {
	return func(current models.ComplaintStatus, _ *uuid.UUID) error {
		if current == models.ComplaintStatusClosed || current == models.ComplaintStatusRejected {
			return fmt.Errorf("%w: a %s complaint cannot be %s", repository.ErrComplaintClosed, strings.ToLower(string(current)), action)
		}
		return nil
	}
}

func (s *ComplaintService) Categorise(ctx context.Context, id uuid.UUID, req models.CategoriseComplaintRequest, actor uuid.UUID) (*models.Complaint, error) {
	if !req.Category.Valid() {
		return nil, invalid("unknown category %q", req.Category)
	}
	switch req.Priority {
	case "LOW", "NORMAL", "HIGH", "URGENT":
	default:
		return nil, invalid("priority must be LOW, NORMAL, HIGH or URGENT")
	}
	err := s.repo.Apply(ctx, id, repository.Change{
		Validate: closedFor("recategorised"),
		Describe: func(current models.ComplaintStatus) (models.ComplaintStatus, string) {
			if current == models.ComplaintStatusSubmitted {
				return models.ComplaintStatusAcknowledged, msgAcknowledged
			}
			return current, ""
		},
		SetSQL: `category = $2::complaint_category, priority = $3, categorised_by = $4, categorised_at = NOW(),
		         status = CASE WHEN status = 'SUBMITTED' THEN 'ACKNOWLEDGED'::complaint_status ELSE status END,
		         acknowledged_at = COALESCE(acknowledged_at, NOW())`,
		SetArgs:      []interface{}{string(req.Category), req.Priority, actor},
		InternalNote: fmt.Sprintf("Categorised as %s, priority %s", req.Category, req.Priority),
		Actor:        actor,
	})
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_categorised", &actor, &id, true, fmt.Sprintf("Category %s, priority %s", req.Category, req.Priority), nil, nil)
	return s.repo.Get(ctx, id)
}

func (s *ComplaintService) Route(ctx context.Context, id uuid.UUID, req models.RouteComplaintRequest, actor uuid.UUID) (*models.Complaint, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.Unit = trimPtr(req.Unit)
	if utf8.RuneCountInString(req.Reason) < 5 {
		return nil, invalid("state the reason for routing (at least 5 characters)")
	}
	if req.Unit != nil && utf8.RuneCountInString(*req.Unit) > 120 {
		return nil, invalid("unit must be at most 120 characters")
	}
	err := s.repo.Route(ctx, id, req, actor, func(current models.ComplaintStatus) (*models.ComplaintStatus, error) {
		if err := closedFor("routed")(current, nil); err != nil {
			return nil, err
		}
		if current == models.ComplaintStatusResolved {
			return nil, fmt.Errorf("%w: reopen the complaint before routing it", repository.ErrComplaintClosed)
		}
		if current == models.ComplaintStatusSubmitted || current == models.ComplaintStatusAcknowledged {
			next := models.ComplaintStatusAssigned
			return &next, nil
		}
		return nil, nil
	}, msgRouted)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_routed", &actor, &id, true, "Routed: "+req.Reason, nil, nil)
	return s.repo.Get(ctx, id)
}

func (s *ComplaintService) SetStatus(ctx context.Context, id uuid.UUID, req models.ComplaintStatusRequest, actor uuid.UUID, role models.Role) (*models.Complaint, error) {
	req.Reason = trimPtr(req.Reason)
	req.Note = trimPtr(req.Note)
	target := req.Status

	if target == models.ComplaintStatusRejected {
		if !atLeast(role, models.RoleSI) {
			return nil, fmt.Errorf("%w: rejecting a complaint needs the rank of SI or above", ErrForbiddenAction)
		}
		if req.Reason == nil || utf8.RuneCountInString(*req.Reason) < 10 {
			return nil, invalid("state the reason the citizen will be shown (at least 10 characters)")
		}
	}
	if target == models.ComplaintStatusResolved {
		c, err := s.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if c.ApprovedResponses == 0 {
			return nil, invalid("a complaint is resolved only after an approved response has been issued to the citizen")
		}
	}

	public := map[models.ComplaintStatus]string{
		models.ComplaintStatusInProgress: msgInProgress,
		models.ComplaintStatusResolved:   msgResolved,
		models.ComplaintStatusClosed:     msgClosed,
		models.ComplaintStatusRejected:   msgRejected,
	}[target]

	setSQL := "status = $2::complaint_status"
	args := []interface{}{string(target)}
	switch target {
	case models.ComplaintStatusResolved:
		setSQL += ", resolved_at = NOW()"
	case models.ComplaintStatusRejected:
		setSQL += ", rejection_reason = $3"
		args = append(args, *req.Reason)
	case models.ComplaintStatusInProgress:
		setSQL += ", resolved_at = NULL"
	}

	note := ""
	if req.Note != nil {
		note = *req.Note
	}
	err := s.repo.Apply(ctx, id, repository.Change{
		Validate: func(current models.ComplaintStatus, _ *uuid.UUID) error {
			if !transitionAllowed(current, target) {
				return fmt.Errorf("%w: cannot move a complaint from %s to %s", ErrInvalid, current, target)
			}
			return nil
		},
		Describe: func(current models.ComplaintStatus) (models.ComplaintStatus, string) {
			if current == models.ComplaintStatusResolved && target == models.ComplaintStatusInProgress {
				return target, msgReopened
			}
			return target, public
		},
		SetSQL: setSQL, SetArgs: args, InternalNote: note, Actor: actor,
	})
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_status_changed", &actor, &id, true, "Status "+string(target), nil, nil)
	return s.repo.Get(ctx, id)
}

func (s *ComplaintService) AddNote(ctx context.Context, id uuid.UUID, note string, actor uuid.UUID) error {
	note = strings.TrimSpace(note)
	if n := utf8.RuneCountInString(note); n < 3 || n > 2000 {
		return invalid("a note must be between 3 and 2000 characters")
	}
	if err := s.repo.Apply(ctx, id, repository.Change{InternalNote: note, Actor: actor}); err != nil {
		return err
	}
	s.audit(ctx, "complaint_note_added", &actor, &id, true, "Internal note added", nil, nil)
	return nil
}

func (s *ComplaintService) LinkDuplicate(ctx context.Context, id uuid.UUID, req models.LinkDuplicateRequest, actor uuid.UUID) (*models.Complaint, error) {
	req.Note = strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(req.Note) < 5 {
		return nil, invalid("state why these complaints are the same (at least 5 characters)")
	}
	if err := s.repo.LinkDuplicate(ctx, id, req.OriginalID, req.Note, actor, msgMerged); err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_duplicate_linked", &actor, &id, true, "Linked as duplicate of "+req.OriginalID.String()+": "+req.Note, nil, nil)
	return s.repo.Get(ctx, id)
}

func (s *ComplaintService) LinkFIR(ctx context.Context, id uuid.UUID, firID uuid.UUID, actor uuid.UUID) (*models.Complaint, error) {
	exists, err := s.repo.FIRExists(ctx, firID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, repository.ErrFIRNotFound
	}
	err = s.repo.Apply(ctx, id, repository.Change{
		Validate: func(current models.ComplaintStatus, _ *uuid.UUID) error {
			if current == models.ComplaintStatusRejected {
				return fmt.Errorf("%w: a rejected complaint cannot be linked to an FIR", repository.ErrComplaintClosed)
			}
			return nil
		},
		SetSQL: "fir_id = $2", SetArgs: []interface{}{firID},
		InternalNote: "Linked to FIR", Actor: actor,
	})
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_fir_linked", &actor, &id, true, "Linked FIR "+firID.String(), nil, nil)
	return s.repo.Get(ctx, id)
}

func (s *ComplaintService) DraftResponse(ctx context.Context, id uuid.UUID, body string, actor uuid.UUID) (*models.ComplaintResponse, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 10 || n > 4000 {
		return nil, invalid("a response must be between 10 and 4000 characters")
	}
	rid, err := s.repo.DraftResponse(ctx, id, body, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "complaint_response_drafted", &actor, &id, true, "Response drafted", nil, nil)
	list, err := s.repo.Responses(ctx, id, false)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == rid {
			return &list[i], nil
		}
	}
	return nil, repository.ErrResponseNotFound
}

func (s *ComplaintService) ReviewResponse(ctx context.Context, id, responseID uuid.UUID, req models.ReviewResponseRequest, actor uuid.UUID) (*models.ComplaintResponse, error) {
	req.Note = trimPtr(req.Note)
	if !req.Approve && (req.Note == nil || utf8.RuneCountInString(*req.Note) < 5) {
		return nil, invalid("state what must change before this response can be approved")
	}
	r, err := s.repo.ReviewResponse(ctx, id, responseID, req.Approve, req.Note, actor, msgResponse)
	if err != nil {
		return nil, err
	}
	action := "complaint_response_rejected"
	if req.Approve {
		action = "complaint_response_approved"
	}
	s.audit(ctx, action, &actor, &id, true, "Response "+strings.ToLower(r.Status), nil, nil)
	return r, nil
}

// Track returns the citizen-safe view when the tracking number and the
// second factor match. Every attempt is audited with its address.
func (s *ComplaintService) Track(ctx context.Context, req models.PublicTrackRequest, ip, ua string) (*models.PublicComplaintView, error) {
	number := strings.ToUpper(strings.TrimSpace(req.TrackingNumber))
	if number == "" || len(number) > 40 {
		return nil, ErrTrackingMismatch
	}
	fail := func(id *uuid.UUID) (*models.PublicComplaintView, error) {
		s.audit(ctx, "complaint_track_failed", nil, id, false, "Tracking attempt did not match for "+number, &ip, &ua)
		return nil, ErrTrackingMismatch
	}
	id, phone, codeHash, anonymous, err := s.repo.TrackingCredentials(ctx, number)
	if errors.Is(err, repository.ErrComplaintNotFound) {
		return fail(nil)
	}
	if err != nil {
		return nil, err
	}
	matched := false
	if anonymous {
		if req.AccessCode != nil && codeHash != nil {
			got := hashAccessCode(*req.AccessCode)
			matched = subtle.ConstantTimeCompare([]byte(got), []byte(*codeHash)) == 1
		}
	} else if req.Phone != nil && phone != nil {
		got := normalizeIndianMobile(*req.Phone)
		matched = got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(*phone)) == 1
	}
	if !matched {
		return fail(&id)
	}

	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	history, err := s.repo.History(ctx, id, true)
	if err != nil {
		return nil, err
	}
	responses, err := s.repo.Responses(ctx, id, true)
	if err != nil {
		return nil, err
	}
	view := &models.PublicComplaintView{
		TrackingNumber: c.TrackingNumber, Category: c.Category, Status: c.Status, Subject: c.Subject,
		SubmittedAt: c.SubmittedAt, LastUpdatedAt: c.UpdatedAt, StationName: c.StationName,
		HandledWithRelated: c.DuplicateOf != nil,
		History:            []models.PublicHistoryEntry{}, Responses: []models.PublicResponseEntry{},
	}
	if c.Status == models.ComplaintStatusRejected {
		view.RejectionReason = c.RejectionReason
	}
	for _, h := range history {
		view.History = append(view.History, models.PublicHistoryEntry{Status: h.Status, Message: h.Message, At: h.CreatedAt})
	}
	for _, r := range responses {
		if r.ReviewedAt != nil {
			view.Responses = append(view.Responses, models.PublicResponseEntry{Body: r.Body, IssuedAt: *r.ReviewedAt})
		}
	}
	s.audit(ctx, "complaint_tracked_view", nil, &id, true, "Citizen viewed tracking for "+number, &ip, &ua)
	return view, nil
}
