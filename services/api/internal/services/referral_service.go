package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// Referrals are how a record crosses from one force to another.
//
// The platform holds four departments and shows each its own records. A
// Kolkata Police case worked by CID is the ordinary exception, and it is an
// act rather than a setting: one force proposes, with a reason, and the other
// accepts. Until it accepts, nothing has crossed.
//
// Both halves are audited, because "why did CID have this file?" is a question
// that gets asked years later, in a courtroom.
type ReferralService struct {
	db        *pgxpool.Pool
	auditRepo *repository.AuditRepository
}

func NewReferralService(db *pgxpool.Pool, auditRepo *repository.AuditRepository) *ReferralService {
	return &ReferralService{db: db, auditRepo: auditRepo}
}

var (
	ErrReferralNotFound = errors.New("referral not found")
	ErrNotYoursToRefer  = errors.New("a record is referred by the force that holds it")
	ErrNotYoursToDecide = errors.New("a referral is decided by the force it was sent to")
	ErrReferralDecided  = errors.New("this referral has already been decided")
	ErrRecordNotVisible = errors.New("this record is not one of your force's")
)

// ProposeReferral offers a record to another force.
func (s *ReferralService) ProposeReferral(ctx context.Context, in models.NewReferral, actor uuid.UUID) (*models.Referral, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return nil, errors.New("a referral needs a reason")
	}

	// The referring officer must hold the record: a force cannot hand on what
	// it cannot see.
	visible, reference, err := s.recordIsVisibleTo(ctx, in.RecordType, in.RecordID, actor)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrRecordNotVisible
	}

	var toForce uuid.UUID
	if err := s.db.QueryRow(ctx,
		`SELECT id FROM forces WHERE code = $1 AND is_active`, in.ToForceCode).Scan(&toForce); err != nil {
		return nil, fmt.Errorf("there is no department with the code %s", in.ToForceCode)
	}

	// The number comes from the shared per-year counter, which takes a row
	// lock, rather than from MAX + 1 over the table: this platform has already
	// had to fix duplicate record numbers once, and it was found in production.
	number, err := s.nextReferralNumber(ctx)
	if err != nil {
		return nil, err
	}

	var referral models.Referral
	err = s.db.QueryRow(ctx, `
		INSERT INTO case_referrals (
			referral_number, record_type, record_id, record_reference,
			from_force_id, to_force_id, reason, authority, referred_by)
		VALUES ($1, $2, $3, NULLIF($4, ''),
		        (SELECT force_id FROM users WHERE id = $8), $5, $6, NULLIF($7, ''), $8)
		RETURNING id, referral_number, status, referred_at`,
		number, in.RecordType, in.RecordID, reference, toForce, in.Reason, in.Authority, actor,
	).Scan(&referral.ID, &referral.ReferralNumber, &referral.Status, &referral.ReferredAt)

	if err != nil {
		// The database refuses a second live referral of the same record, and
		// a referral to the referring force's own family.
		return nil, s.refusal(ctx, actor, "referral_refused", in, err)
	}

	s.audit(ctx, actor, "referral_proposed", referral.ID,
		fmt.Sprintf("Referred %s %s to %s: %s", in.RecordType, reference, in.ToForceCode, in.Reason), true, "")

	return s.Get(ctx, referral.ID, actor)
}

// Decide accepts or declines a referral. Only the receiving force may, and the
// database enforces that as well as this does.
func (s *ReferralService) Decide(ctx context.Context, id uuid.UUID, accept bool, note string, actor uuid.UUID) (*models.Referral, error) {
	status := "DECLINED"
	if accept {
		status = "ACCEPTED"
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE case_referrals
		   SET status = $2, decided_by = $3, decision_note = NULLIF($4, '')
		 WHERE id = $1 AND status = 'PROPOSED'`, id, status, actor, note)
	if err != nil {
		text := err.Error()
		switch {
		case strings.Contains(text, "decided by the force it was sent to"):
			return nil, ErrNotYoursToDecide
		case strings.Contains(text, "already been"):
			return nil, ErrReferralDecided
		}
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		// Either it does not exist, or somebody else decided it first.
		var exists bool
		s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM case_referrals WHERE id = $1)`, id).Scan(&exists)
		if exists {
			return nil, ErrReferralDecided
		}
		return nil, ErrReferralNotFound
	}

	referral, err := s.Get(ctx, id, actor)
	if err != nil {
		return nil, err
	}

	verb := "Accepted"
	if !accept {
		verb = "Declined"
	}
	s.audit(ctx, actor, "referral_"+strings.ToLower(status), id,
		fmt.Sprintf("%s the referral of %s %s from %s", verb,
			referral.RecordType, referral.RecordReference, referral.FromForceShortName), true, note)

	return referral, nil
}

// Withdraw takes back a proposal. Only the force that made it may.
func (s *ReferralService) Withdraw(ctx context.Context, id uuid.UUID, actor uuid.UUID) (*models.Referral, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE case_referrals r
		   SET status = 'WITHDRAWN'
		 WHERE r.id = $1 AND r.status = 'PROPOSED'
		   AND r.from_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $2)))`, id, actor)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotYoursToRefer
	}

	s.audit(ctx, actor, "referral_withdrawn", id, "Withdrew the referral", true, "")
	return s.Get(ctx, id, actor)
}

const referralColumns = `
	SELECT r.id, r.referral_number, r.record_type, r.record_id,
	       COALESCE(r.record_reference, ''), r.status, r.reason, COALESCE(r.authority, ''),
	       ff.code, ff.short_name, tf.code, tf.short_name,
	       r.referred_by, COALESCE(ru.name, ''), r.referred_at,
	       r.decided_by, COALESCE(du.name, ''), r.decided_at, COALESCE(r.decision_note, '')
	  FROM case_referrals r
	  JOIN forces ff ON ff.id = r.from_force_id
	  JOIN forces tf ON tf.id = r.to_force_id
	  LEFT JOIN users ru ON ru.id = r.referred_by
	  LEFT JOIN users du ON du.id = r.decided_by`

func scanReferral(row interface{ Scan(...interface{}) error }) (models.Referral, error) {
	var r models.Referral
	err := row.Scan(&r.ID, &r.ReferralNumber, &r.RecordType, &r.RecordID,
		&r.RecordReference, &r.Status, &r.Reason, &r.Authority,
		&r.FromForceCode, &r.FromForceShortName, &r.ToForceCode, &r.ToForceShortName,
		&r.ReferredBy, &r.ReferredByName, &r.ReferredAt,
		&r.DecidedBy, &r.DecidedByName, &r.DecidedAt, &r.DecisionNote)
	return r, err
}

// Get returns one referral, provided the viewer's force is on one side of it.
func (s *ReferralService) Get(ctx context.Context, id uuid.UUID, viewer uuid.UUID) (*models.Referral, error) {
	r, err := scanReferral(s.db.QueryRow(ctx, referralColumns+`
		 WHERE r.id = $1
		   AND (r.from_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $2)))
		     OR r.to_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $2))))`, id, viewer))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReferralNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// List returns the referrals this officer's force sent or received.
func (s *ReferralService) List(ctx context.Context, viewer uuid.UUID, direction, status string) ([]models.Referral, error) {
	where := ` WHERE (r.from_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $1)))
	              OR r.to_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $1))))`
	args := []interface{}{viewer}

	switch direction {
	case "incoming":
		where = ` WHERE r.to_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $1)))`
	case "outgoing":
		where = ` WHERE r.from_force_id = ANY(force_family((SELECT force_id FROM users WHERE id = $1)))`
	}

	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND r.status = $%d", len(args))
	}

	rows, err := s.db.Query(ctx, referralColumns+where+` ORDER BY r.referred_at DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Referral{}
	for rows.Next() {
		r, err := scanReferral(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// nextReferralNumber allocates REF-YYYY-NNNNN from the counter every other
// register in this platform uses.
func (s *ReferralService) nextReferralNumber(ctx context.Context) (string, error) {
	year := time.Now().Year()
	var n int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO record_counters (scope, year, last_value) VALUES ('REF', $1, 1)
		ON CONFLICT (scope, year) DO UPDATE SET last_value = record_counters.last_value + 1
		RETURNING last_value`, year).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("allocate a referral number: %w", err)
	}
	return fmt.Sprintf("REF-%d-%05d", year, n), nil
}

// recordIsVisibleTo reports whether the officer's force holds the record, and
// returns the reference an officer would recognise it by.
func (s *ReferralService) recordIsVisibleTo(ctx context.Context, recordType string, id, actor uuid.UUID) (bool, string, error) {
	var query string
	switch recordType {
	case "FIR":
		query = `SELECT f.fir_number,
		           f.station_id = ANY(force_family_stations((SELECT force_id FROM users WHERE id = $2)))
		         FROM firs f WHERE f.id = $1`
	case "CASE":
		query = `SELECT c.case_number,
		           COALESCE(fir.station_id,
		             (SELECT iou.station_id FROM users iou WHERE iou.id = c.investigating_officer))
		             = ANY(force_family_stations((SELECT force_id FROM users WHERE id = $2)))
		         FROM cases c LEFT JOIN firs fir ON fir.id = c.fir_id WHERE c.id = $1`
	case "COMPLAINT":
		query = `SELECT c.tracking_number,
		           c.station_id = ANY(force_family_stations((SELECT force_id FROM users WHERE id = $2)))
		         FROM citizen_complaints c WHERE c.id = $1`
	default:
		return false, "", fmt.Errorf("%s is not a record that can be referred", recordType)
	}

	var reference string
	var visible bool
	if err := s.db.QueryRow(ctx, query, id, actor).Scan(&reference, &visible); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", ErrRecordNotVisible
		}
		return false, "", err
	}
	return visible, reference, nil
}

func (s *ReferralService) refusal(ctx context.Context, actor uuid.UUID, action string, in models.NewReferral, err error) error {
	s.audit(ctx, actor, action, uuid.Nil,
		fmt.Sprintf("Refused a referral of %s to %s", in.RecordType, in.ToForceCode), false, err.Error())

	text := err.Error()
	if strings.Contains(text, "idx_referrals_one_live") {
		// "Still live" covers both a proposal awaiting a decision and one
		// already accepted, and reads oddly for the second. Say which.
		var status, force string
		s.db.QueryRow(ctx, `
			SELECT r.status, f.short_name
			  FROM case_referrals r JOIN forces f ON f.id = r.to_force_id
			 WHERE r.record_type = $1 AND r.record_id = $2
			   AND r.status IN ('PROPOSED', 'ACCEPTED')
			 LIMIT 1`, in.RecordType, in.RecordID).Scan(&status, &force)

		switch status {
		case "ACCEPTED":
			return fmt.Errorf("this record is already with %s, which accepted it; refer it again only after they return it", force)
		case "PROPOSED":
			return fmt.Errorf("this record has already been referred to %s and is awaiting their decision", force)
		default:
			return errors.New("this record has already been referred and that referral is still live")
		}
	}
	if strings.Contains(text, "referral_crosses_a_boundary") {
		return errors.New("a record cannot be referred to the force that already holds it")
	}
	return err
}

func (s *ReferralService) audit(ctx context.Context, actor uuid.UUID, action string, id uuid.UUID, description string, success bool, failure string) {
	entry := &repository.AuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "case_referral",
		Description:  ptr(description),
		Success:      success,
	}
	if id != uuid.Nil {
		entry.ResourceID = &id
	}
	if failure != "" {
		entry.FailureReason = ptr(failure)
	}
	s.auditRepo.Log(ctx, entry)
}
