package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/fraud"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// CyberFraudService holds the Phase 05 rules. Each rule that concerns stored
// state is also enforced by a table constraint (migration 000044).
type CyberFraudService struct {
	repo      *repository.CyberFraudRepository
	auditRepo *repository.AuditRepository
}

func NewCyberFraudService(repo *repository.CyberFraudRepository, auditRepo *repository.AuditRepository) *CyberFraudService {
	return &CyberFraudService{repo: repo, auditRepo: auditRepo}
}

func (s *CyberFraudService) audit(ctx context.Context, action string, actor uuid.UUID, complaintID uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "cyber_complaint",
		ResourceID:   &complaintID,
		Description:  &description,
		Success:      true,
	})
}

// rupees renders paise in Indian digit grouping (₹1,84,000 or ₹1,84,000.50).
func rupees(paise int64) string {
	whole, frac := paise/100, paise%100
	digits := fmt.Sprintf("%d", whole)
	if len(digits) > 3 {
		head, tail := digits[:len(digits)-3], digits[len(digits)-3:]
		var groups []string
		for len(head) > 2 {
			groups = append([]string{head[len(head)-2:]}, groups...)
			head = head[:len(head)-2]
		}
		if head != "" {
			groups = append([]string{head}, groups...)
		}
		digits = strings.Join(groups, ",") + "," + tail
	}
	if frac == 0 {
		return "₹" + digits
	}
	return fmt.Sprintf("₹%s.%02d", digits, frac)
}

func trimOrNil(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func (s *CyberFraudService) validateComplaint(in *models.CyberComplaintInput) error {
	if !in.Type.Valid() {
		return invalid("unknown complaint type %q", in.Type)
	}
	if !in.Platform.Valid() {
		return invalid("unknown platform %q", in.Platform)
	}
	switch in.Priority {
	case "":
		in.Priority = "MEDIUM"
	case "LOW", "MEDIUM", "HIGH", "CRITICAL":
	default:
		return invalid("unknown priority %q", in.Priority)
	}
	if strings.TrimSpace(in.ComplainantName) == "" || strings.TrimSpace(in.IncidentDescription) == "" {
		return invalid("complainant name and incident description are required")
	}
	if in.ReportedLossPaise < 0 {
		return invalid("reported loss cannot be negative")
	}
	if in.IncidentDate.After(time.Now().Add(5 * time.Minute)) {
		return invalid("the incident date cannot be in the future")
	}
	in.ComplainantPhone = trimOrNil(in.ComplainantPhone)
	in.ComplainantEmail = trimOrNil(in.ComplainantEmail)
	in.PlatformName = trimOrNil(in.PlatformName)
	in.NCRPReference = trimOrNil(in.NCRPReference)
	in.HelplineReference = trimOrNil(in.HelplineReference)
	if in.ComplainantPhone != nil {
		if _, err := fraud.Normalize("PHONE", *in.ComplainantPhone, nil); err != nil {
			return fmt.Errorf("%w: complainant phone: %v", ErrInvalid, err)
		}
	}
	if in.ComplainantEmail != nil {
		if _, err := fraud.Normalize("EMAIL", *in.ComplainantEmail, nil); err != nil {
			return fmt.Errorf("%w: complainant email: %v", ErrInvalid, err)
		}
	}
	return nil
}

/* ------------------------------------------------------------ complaints */

func (s *CyberFraudService) List(ctx context.Context, f repository.CyberComplaintFilter) ([]models.CyberComplaint, int64, error) {
	return s.repo.ListComplaints(ctx, f)
}

// Owner answers which department registered a cyber-crime case.
func (s *CyberFraudService) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.Owner(ctx, id, viewerID)
}

func (s *CyberFraudService) Get(ctx context.Context, id uuid.UUID) (*models.CyberComplaint, error) {
	return s.repo.GetComplaint(ctx, id)
}

func (s *CyberFraudService) Dashboard(ctx context.Context) (*models.FraudDashboard, error) {
	return s.repo.Dashboard(ctx)
}

func (s *CyberFraudService) Clusters(ctx context.Context) ([]models.ComplaintCluster, error) {
	return s.repo.Clusters(ctx)
}

func (s *CyberFraudService) Network(ctx context.Context, id uuid.UUID) (*models.FraudNetwork, error) {
	return s.repo.Network(ctx, id)
}

func (s *CyberFraudService) SearchEntities(ctx context.Context, entityType, q string) ([]models.FraudEntity, error) {
	return s.repo.SearchEntities(ctx, entityType, strings.TrimSpace(q), 25)
}

func (s *CyberFraudService) Register(ctx context.Context, in models.CyberComplaintInput, actor uuid.UUID, station *uuid.UUID) (*models.CyberComplaint, error) {
	if err := s.validateComplaint(&in); err != nil {
		return nil, err
	}
	id, err := s.repo.CreateComplaint(ctx, in, station, actor)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.GetComplaint(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_complaint_registered", actor, id,
		fmt.Sprintf("Registered %s (%s), reported loss %s", c.CaseNumber, c.Type, rupees(c.ReportedLossPaise)))
	return c, nil
}

func (s *CyberFraudService) Update(ctx context.Context, id uuid.UUID, in models.CyberComplaintInput, actor uuid.UUID) (*models.CyberComplaint, error) {
	if err := s.validateComplaint(&in); err != nil {
		return nil, err
	}
	// Lowering the loss must not strand a freeze request or recovery above it.
	maxFreeze, recovered, err := s.repo.CommittedPaise(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.ReportedLossPaise < maxFreeze || in.ReportedLossPaise < recovered {
		return nil, invalid("reported loss cannot be below an open freeze request (%s) or the amount recovered (%s)",
			rupees(maxFreeze), rupees(recovered))
	}
	if err := s.repo.UpdateComplaint(ctx, id, in); err != nil {
		return nil, err
	}
	c, err := s.repo.GetComplaint(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_complaint_updated", actor, id, fmt.Sprintf("Updated %s", c.CaseNumber))
	return c, nil
}

func (s *CyberFraudService) SetStatus(ctx context.Context, id uuid.UUID, status models.CyberComplaintStatus, actor uuid.UUID) (*models.CyberComplaint, error) {
	if !status.Valid() {
		return nil, invalid("unknown status %q", status)
	}
	if err := s.repo.SetStatus(ctx, id, status); err != nil {
		return nil, err
	}
	c, err := s.repo.GetComplaint(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_complaint_status_updated", actor, id, fmt.Sprintf("%s set to %s", c.CaseNumber, status))
	return c, nil
}

/* -------------------------------------------------------------- entities */

func (s *CyberFraudService) Entities(ctx context.Context, id uuid.UUID) ([]models.ComplaintEntity, error) {
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ComplaintEntities(ctx, id)
}

// RecordEntity validates and normalises the value, adds it to the shared
// register if new, and links it to the complaint.
func (s *CyberFraudService) RecordEntity(ctx context.Context, id uuid.UUID, in models.RecordEntityRequest, actor uuid.UUID) (*models.ComplaintEntity, error) {
	if !in.Role.Valid() {
		return nil, invalid("unknown role %q", in.Role)
	}
	norm, err := fraud.Normalize(string(in.Type), in.Value, in.IFSC)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	entityID, err := s.repo.UpsertEntity(ctx, in.Type, norm.Value, norm.Display, norm.IFSC, trimOrNil(in.Provider), actor)
	if err != nil {
		return nil, err
	}
	linkID, err := s.repo.LinkEntity(ctx, id, entityID, in.Role, trimOrNil(in.Note), actor)
	if err != nil {
		return nil, err
	}
	links, err := s.repo.ComplaintEntities(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if l.ID == linkID {
			s.audit(ctx, "cyber_entity_recorded", actor, id,
				fmt.Sprintf("Recorded %s %s as %s", l.Entity.Type, l.Entity.DisplayValue, l.Role))
			return &l, nil
		}
	}
	return nil, errors.New("recorded entity link could not be read back")
}

func (s *CyberFraudService) RemoveEntity(ctx context.Context, id, linkID, actor uuid.UUID) error {
	removed, err := s.repo.UnlinkEntity(ctx, id, linkID)
	if err != nil {
		return err
	}
	s.audit(ctx, "cyber_entity_removed", actor, id, fmt.Sprintf("Removed entity link %s (%s)", removed.ID, removed.Role))
	return nil
}

/* ----------------------------------------------------------- money trail */

func (s *CyberFraudService) Transactions(ctx context.Context, id uuid.UUID) ([]models.FraudTransaction, error) {
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Transactions(ctx, id)
}

func (s *CyberFraudService) moneyEntity(ctx context.Context, complaintID, entityID uuid.UUID, side string) (*models.FraudEntity, error) {
	e, err := s.repo.EntityOnComplaint(ctx, complaintID, entityID)
	if errors.Is(err, repository.ErrEntityNotOnComplaint) {
		return nil, invalid("the %s account is not recorded on this complaint", side)
	}
	if err != nil {
		return nil, err
	}
	if !e.Type.MoneyBearing() {
		return nil, invalid("money moves only between UPI IDs, bank accounts and wallets; %s is a %s", e.DisplayValue, e.Type)
	}
	return e, nil
}

func (s *CyberFraudService) RecordTransaction(ctx context.Context, id uuid.UUID, in models.RecordTransactionRequest, actor uuid.UUID) (*models.FraudTransaction, error) {
	if in.AmountPaise <= 0 {
		return nil, invalid("the amount must be greater than zero")
	}
	if in.FromEntityID == in.ToEntityID {
		return nil, invalid("a transfer needs two different accounts")
	}
	if in.OccurredAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, invalid("a transaction cannot be in the future")
	}
	in.Reference, in.Note = trimOrNil(in.Reference), trimOrNil(in.Note)
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	from, err := s.moneyEntity(ctx, id, in.FromEntityID, "sending")
	if err != nil {
		return nil, err
	}
	to, err := s.moneyEntity(ctx, id, in.ToEntityID, "receiving")
	if err != nil {
		return nil, err
	}
	txID, err := s.repo.RecordTransaction(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_transaction_recorded", actor, id,
		fmt.Sprintf("Recorded transfer of %s from %s to %s", rupees(in.AmountPaise), from.DisplayValue, to.DisplayValue))
	list, err := s.repo.Transactions(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		if t.ID == txID {
			return &t, nil
		}
	}
	return nil, errors.New("recorded transaction could not be read back")
}

/* ------------------------------------------------------- freeze requests */

func (s *CyberFraudService) FreezeRequests(ctx context.Context, id uuid.UUID) ([]models.FreezeRequest, error) {
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.FreezeRequests(ctx, id)
}

func (s *CyberFraudService) DraftFreeze(ctx context.Context, id uuid.UUID, in models.CreateFreezeRequest, actor uuid.UUID) (*models.FreezeRequest, error) {
	if strings.TrimSpace(in.Addressee) == "" || strings.TrimSpace(in.Grounds) == "" {
		return nil, invalid("the addressee and grounds are required")
	}
	if in.AmountRequestedPaise <= 0 {
		return nil, invalid("the amount to freeze must be greater than zero")
	}
	c, err := s.repo.GetComplaint(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.AmountRequestedPaise > c.ReportedLossPaise {
		return nil, invalid("the amount to freeze (%s) exceeds the reported loss (%s)",
			rupees(in.AmountRequestedPaise), rupees(c.ReportedLossPaise))
	}
	e, err := s.moneyEntity(ctx, id, in.EntityID, "")
	if err != nil {
		return nil, err
	}
	freezeID, err := s.repo.CreateFreeze(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	f, err := s.repo.GetFreeze(ctx, id, freezeID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_freeze_drafted", actor, id,
		fmt.Sprintf("Drafted %s to %s for %s on %s", f.RequestNumber, f.Addressee, rupees(f.AmountRequestedPaise), e.DisplayValue))
	return f, nil
}

func (s *CyberFraudService) TransitionFreeze(ctx context.Context, id, freezeID uuid.UUID, in models.FreezeTransitionRequest, actor uuid.UUID) (*models.FreezeRequest, error) {
	var desc string
	switch in.Action {
	case "send":
		if in.SentVia = trimOrNil(in.SentVia); in.SentVia == nil {
			return nil, invalid("record how the request was sent")
		}
		desc = "Recorded as sent via " + *in.SentVia
	case "acknowledge":
		in.AcknowledgementRef = trimOrNil(in.AcknowledgementRef)
		desc = "Recorded acknowledgement"
		if in.AcknowledgementRef != nil {
			desc += " " + *in.AcknowledgementRef
		}
	case "frozen":
		if in.AmountFrozenPaise == nil || *in.AmountFrozenPaise <= 0 {
			return nil, invalid("record the amount frozen")
		}
		current, err := s.repo.GetFreeze(ctx, id, freezeID)
		if err != nil {
			return nil, err
		}
		if *in.AmountFrozenPaise > current.AmountRequestedPaise {
			return nil, invalid("the amount frozen (%s) exceeds the amount requested (%s)",
				rupees(*in.AmountFrozenPaise), rupees(current.AmountRequestedPaise))
		}
		desc = "Recorded " + rupees(*in.AmountFrozenPaise) + " frozen"
	case "reject":
		if in.RejectionReason = trimOrNil(in.RejectionReason); in.RejectionReason == nil {
			return nil, invalid("record the reason given for rejection")
		}
		desc = "Recorded rejection: " + *in.RejectionReason
	default:
		return nil, invalid("unknown action %q (send, acknowledge, frozen or reject)", in.Action)
	}
	if err := s.repo.TransitionFreeze(ctx, id, freezeID, in, actor); err != nil {
		return nil, err
	}
	f, err := s.repo.GetFreeze(ctx, id, freezeID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_freeze_"+in.Action, actor, id, f.RequestNumber+": "+desc)
	return f, nil
}

/* ------------------------------------------------------------ recoveries */

func (s *CyberFraudService) Recoveries(ctx context.Context, id uuid.UUID) ([]models.FraudRecovery, error) {
	if _, err := s.repo.GetComplaint(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Recoveries(ctx, id)
}

func (s *CyberFraudService) RecordRecovery(ctx context.Context, id uuid.UUID, in models.RecordRecoveryRequest, actor uuid.UUID) (*models.FraudRecovery, error) {
	if in.AmountPaise <= 0 {
		return nil, invalid("the amount recovered must be greater than zero")
	}
	if in.RecoveredOn.After(time.Now().Add(24 * time.Hour)) {
		return nil, invalid("a recovery cannot be dated in the future")
	}
	in.Reference, in.Note = trimOrNil(in.Reference), trimOrNil(in.Note)
	recID, err := s.repo.RecordRecovery(ctx, id, in, actor)
	if errors.Is(err, repository.ErrRecoveryExceedsLoss) {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if errors.Is(err, repository.ErrFreezeWrongStage) {
		return nil, invalid("a recovery can be linked only to a freeze request on this complaint that is frozen")
	}
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "cyber_recovery_recorded", actor, id, "Recorded recovery of "+rupees(in.AmountPaise))
	list, err := s.repo.Recoveries(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.ID == recID {
			return &r, nil
		}
	}
	return nil, errors.New("recorded recovery could not be read back")
}
