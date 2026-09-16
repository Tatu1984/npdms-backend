package services

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// Officer account administration.
//
// Until this module there was no way to create an officer. Every account in
// the platform was inserted by hand, which meant that joining a force,
// transferring between stations and leaving the service were all database
// work. This is the register of who may sign in, kept the way every other
// register here is kept: an act by a named officer, refused with a reason, and
// written to the audit trail.
//
// Two rules run through all of it.
//
// A department is never chosen. It follows the posting, because an officer who
// could pick their own department could pick which force's records they read.
// The database holds that rule in trg_user_force_matches_posting; this service
// works out the same answer through force_for_posting so that the two agree
// rather than race.
//
// An account is never deleted. An officer who has left still recorded the FIRs
// they recorded and still signed the custody entries they signed. Deactivation
// is a flag, the references survive, and the database refuses a DELETE.
type UserAdminService struct {
	db        *pgxpool.Pool
	auditRepo *repository.AuditRepository
}

func NewUserAdminService(db *pgxpool.Pool, auditRepo *repository.AuditRepository) *UserAdminService {
	return &UserAdminService{db: db, auditRepo: auditRepo}
}

var (
	ErrOfficerNotFound     = errors.New("no such officer in your department")
	ErrNotYourOfficer      = errors.New("an officer is administered by their own department")
	ErrPostingOutsideForce = errors.New("an officer is posted within your own department")
	ErrRankAboveYourOwn    = errors.New("an account cannot be created above your own rank")
	ErrUsernameTaken       = errors.New("that username is already in use")
	ErrBadgeTaken          = errors.New("that badge number is already in use")
	ErrEmailTaken          = errors.New("that email address is already in use")
	ErrAlreadyInactive     = errors.New("this account is already deactivated")
	ErrAlreadyActive       = errors.New("this account is already active")
	ErrUnknownRank         = errors.New("that is not a rank this platform knows")
	ErrPostingNotFound     = errors.New("an officer needs a posting")
)

// Officer is an account as an administrator sees it. There is no password
// field of any kind: a hash is never returned, and a generated password is
// returned once, by the call that generated it, in its own reply.
type Officer struct {
	ID                 uuid.UUID  `json:"id"`
	Username           string     `json:"username"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Phone              string     `json:"phone,omitempty"`
	BadgeNumber        string     `json:"badgeNumber,omitempty"`
	Role               string     `json:"role"`
	StationID          *uuid.UUID `json:"stationId,omitempty"`
	StationName        string     `json:"stationName,omitempty"`
	StationCode        string     `json:"stationCode,omitempty"`
	ForceCode          string     `json:"forceCode"`
	ForceShortName     string     `json:"forceShortName"`
	IsActive           bool       `json:"isActive"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLogin          *time.Time `json:"lastLogin,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	DeactivatedAt      *time.Time `json:"deactivatedAt,omitempty"`
	DeactivatedByName  string     `json:"deactivatedByName,omitempty"`
	DeactivationReason string     `json:"deactivationReason,omitempty"`
}

// NewOfficer is what an administrator fills in. Note what is absent: the
// department, which follows the station, and the password, which is generated.
type NewOfficer struct {
	Username    string `json:"username" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Email       string `json:"email" binding:"required"`
	Phone       string `json:"phone"`
	BadgeNumber string `json:"badgeNumber"`
	Role        string `json:"role" binding:"required"`
	StationID   string `json:"stationId" binding:"required"`
}

// OfficerAmendment carries only the fields the administrator changed. A nil
// field is left alone, so amending a phone number cannot quietly move an
// officer's posting.
type OfficerAmendment struct {
	Name        *string `json:"name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	BadgeNumber *string `json:"badgeNumber"`
	Role        *string `json:"role"`
	StationID   *string `json:"stationId"`
}

// IssuedPassword is the one moment a password is readable. It is returned by
// the call that created it and is never stored, logged or returned again.
type IssuedPassword struct {
	Officer  *Officer `json:"officer"`
	Password string   `json:"password"`
	Notice   string   `json:"notice"`
}

// Posting is a station an administrator may post an officer to.
type Posting struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	ForceCode      string    `json:"forceCode"`
	ForceShortName string    `json:"forceShortName"`
}

// administrator is the acting officer, read fresh rather than taken from the
// token: a rank or a posting may have changed since they signed in.
type administrator struct {
	ID        uuid.UUID
	Name      string
	Role      models.Role
	ForceID   uuid.UUID
	ForceCode string
}

func (s *UserAdminService) actor(ctx context.Context, id uuid.UUID) (*administrator, error) {
	var a administrator
	a.ID = id
	err := s.db.QueryRow(ctx, `
		SELECT u.name, u.role, u.force_id, f.code
		  FROM users u JOIN forces f ON f.id = u.force_id
		 WHERE u.id = $1 AND u.is_active`, id).Scan(&a.Name, &a.Role, &a.ForceID, &a.ForceCode)
	if err != nil {
		return nil, errors.New("the acting officer's account could not be read")
	}
	return &a, nil
}

// administersClause matches the officers an administrator may see and act on:
// their own department, and any wing of it. A Kolkata Police administrator
// administers Kolkata Traffic Police; a traffic administrator does not
// administer Kolkata Police. Authority runs downwards.
const administersClause = `(u.force_id = %[1]s
	OR (SELECT f2.parent_id FROM forces f2 WHERE f2.id = u.force_id) = %[1]s)`

const officerColumns = `
	SELECT u.id, u.username, u.name, u.email, COALESCE(u.phone, ''), COALESCE(u.badge_number, ''),
	       u.role::text, u.station_id, COALESCE(s.name, ''), COALESCE(s.code, ''),
	       f.code, f.short_name, u.is_active, u.must_change_password,
	       u.last_login, u.created_at, u.deactivated_at, COALESCE(du.name, ''),
	       COALESCE(u.deactivation_reason, '')
	  FROM users u
	  JOIN forces f ON f.id = u.force_id
	  LEFT JOIN stations s ON s.id = u.station_id
	  LEFT JOIN users du ON du.id = u.deactivated_by`

func scanOfficer(row interface{ Scan(...interface{}) error }) (Officer, error) {
	var o Officer
	err := row.Scan(&o.ID, &o.Username, &o.Name, &o.Email, &o.Phone, &o.BadgeNumber,
		&o.Role, &o.StationID, &o.StationName, &o.StationCode,
		&o.ForceCode, &o.ForceShortName, &o.IsActive, &o.MustChangePassword,
		&o.LastLogin, &o.CreatedAt, &o.DeactivatedAt, &o.DeactivatedByName,
		&o.DeactivationReason)
	return o, err
}

// List returns the officers of the administrator's own department.
func (s *UserAdminService) List(ctx context.Context, viewer uuid.UUID, search, status string) ([]Officer, error) {
	a, err := s.actor(ctx, viewer)
	if err != nil {
		return nil, err
	}

	args := []interface{}{a.ForceID}
	where := " WHERE " + fmt.Sprintf(administersClause, "$1")

	switch status {
	case "active":
		where += " AND u.is_active"
	case "inactive":
		where += " AND NOT u.is_active"
	}

	if term := strings.TrimSpace(search); term != "" {
		args = append(args, "%"+strings.ToLower(term)+"%")
		where += fmt.Sprintf(` AND (LOWER(u.name) LIKE $%[1]d OR LOWER(u.username) LIKE $%[1]d
			OR LOWER(COALESCE(u.badge_number, '')) LIKE $%[1]d OR LOWER(COALESCE(s.name, '')) LIKE $%[1]d)`, len(args))
	}

	rows, err := s.db.Query(ctx, officerColumns+where+` ORDER BY u.is_active DESC, u.name LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Officer{}
	for rows.Next() {
		o, err := scanOfficer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Get returns one officer, provided they are one of the administrator's.
func (s *UserAdminService) Get(ctx context.Context, id, viewer uuid.UUID) (*Officer, error) {
	a, err := s.actor(ctx, viewer)
	if err != nil {
		return nil, err
	}
	return s.get(ctx, id, a.ForceID)
}

func (s *UserAdminService) get(ctx context.Context, id, actorForce uuid.UUID) (*Officer, error) {
	o, err := scanOfficer(s.db.QueryRow(ctx,
		officerColumns+" WHERE u.id = $2 AND "+fmt.Sprintf(administersClause, "$1"), actorForce, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOfficerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Postings lists the stations this administrator may post an officer to: their
// own department's stations, their wings' stations, and — for a wing — the
// parent force's stations, which is where a traffic sergeant actually works.
func (s *UserAdminService) Postings(ctx context.Context, viewer uuid.UUID) ([]Posting, error) {
	a, err := s.actor(ctx, viewer)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.code, s.name, f.code, f.short_name
		  FROM stations s JOIN forces f ON f.id = s.force_id
		 WHERE s.force_id = $1
		    OR f.parent_id = $1
		    OR s.force_id = (SELECT af.parent_id FROM forces af WHERE af.id = $1)
		 ORDER BY f.short_name, s.name`, a.ForceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Posting{}
	for rows.Next() {
		var p Posting
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.ForceCode, &p.ForceShortName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Ranks lists the ranks this administrator may issue, which stops at their own.
// An SP does not appoint a DIG.
func (s *UserAdminService) Ranks(ctx context.Context, viewer uuid.UUID) ([]string, error) {
	a, err := s.actor(ctx, viewer)
	if err != nil {
		return nil, err
	}

	ceiling := models.RoleHierarchy[a.Role]
	order := []models.Role{
		models.RoleConstable, models.RoleHeadConstable, models.RoleASI, models.RoleSI,
		models.RoleInspector, models.RoleSHO, models.RoleDSP, models.RoleSP,
		models.RoleDIG, models.RoleIG, models.RoleSecretary, models.RoleDGP,
	}

	out := []string{}
	for _, r := range order {
		if models.RoleHierarchy[r] <= ceiling {
			out = append(out, string(r))
		}
	}
	return out, nil
}

// Create opens an account and issues its first password.
//
// The password is generated here rather than accepted from the request. An
// administrator who chooses the password knows it, tends to choose the same
// one twice, and has no reason to be able to sign in as the officer. It is
// shown once, and the account is marked as owing a change so that the officer
// replaces a password two people have seen with one only they have.
func (s *UserAdminService) Create(ctx context.Context, in NewOfficer, actorID uuid.UUID) (*IssuedPassword, error) {
	a, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}

	username := strings.ToLower(strings.TrimSpace(in.Username))
	name := strings.TrimSpace(in.Name)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	role := strings.ToUpper(strings.TrimSpace(in.Role))

	if username == "" || name == "" || email == "" {
		return nil, errors.New("an account needs a username, a name and an email address")
	}
	if _, known := models.RoleHierarchy[models.Role(role)]; !known {
		return nil, ErrUnknownRank
	}
	if models.RoleHierarchy[models.Role(role)] > models.RoleHierarchy[a.Role] {
		return nil, ErrRankAboveYourOwn
	}

	station, err := uuid.Parse(strings.TrimSpace(in.StationID))
	if err != nil {
		return nil, ErrPostingNotFound
	}
	if err := s.postingIsYours(ctx, station, a); err != nil {
		s.refused(ctx, a, "officer_create_refused", uuid.Nil,
			fmt.Sprintf("Refused to open an account for %s", username), err)
		return nil, err
	}

	password, err := generatePassword()
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	// The department is not in the INSERT as a value the caller supplied: it is
	// computed by the database from the station, and the 000082 trigger then
	// checks the answer. Two independent statements of the same rule.
	var id uuid.UUID
	err = s.db.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, name, role, badge_number,
		                   station_id, phone, force_id, is_active, must_change_password)
		VALUES ($1, $2, $3, $4, $5::user_role, NULLIF($6, ''), $7, NULLIF($8, ''),
		        force_for_posting($7, $9), TRUE, TRUE)
		RETURNING id`,
		username, email, hash, name, role, strings.TrimSpace(in.BadgeNumber),
		station, strings.TrimSpace(in.Phone), a.ForceID).Scan(&id)
	if err != nil {
		stated := s.stateTheRule(err)
		s.refused(ctx, a, "officer_create_refused", uuid.Nil,
			fmt.Sprintf("Refused to open an account for %s", username), stated)
		return nil, stated
	}

	officer, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}

	s.audit(ctx, a, "officer_created", id,
		fmt.Sprintf("Opened an account for %s (%s), %s at %s, %s",
			officer.Name, officer.Username, officer.Role, officer.StationName, officer.ForceShortName),
		true, "")

	return &IssuedPassword{
		Officer:  officer,
		Password: password,
		Notice:   "This password is shown once. Hand it to the officer, who must change it when they first sign in.",
	}, nil
}

// Amend changes an officer's details and, where the posting changes, transfers
// them. A transfer to a station of another department moves the officer's
// department with it; a transfer the database will not allow is refused with
// the rule rather than a server error.
func (s *UserAdminService) Amend(ctx context.Context, id uuid.UUID, in OfficerAmendment, actorID uuid.UUID) (*Officer, error) {
	a, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}

	before, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}

	sets := []string{}
	args := []interface{}{id}
	add := func(fragment string, value interface{}) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf(fragment, len(args)))
	}

	changed := []string{}
	if in.Name != nil && strings.TrimSpace(*in.Name) != before.Name {
		add("name = $%d", strings.TrimSpace(*in.Name))
		changed = append(changed, "name")
	}
	if in.Email != nil && strings.ToLower(strings.TrimSpace(*in.Email)) != before.Email {
		add("email = $%d", strings.ToLower(strings.TrimSpace(*in.Email)))
		changed = append(changed, "email")
	}
	if in.Phone != nil && strings.TrimSpace(*in.Phone) != before.Phone {
		add("phone = NULLIF($%d, '')", strings.TrimSpace(*in.Phone))
		changed = append(changed, "phone")
	}
	if in.BadgeNumber != nil && strings.TrimSpace(*in.BadgeNumber) != before.BadgeNumber {
		add("badge_number = NULLIF($%d, '')", strings.TrimSpace(*in.BadgeNumber))
		changed = append(changed, "badge number")
	}
	if in.Role != nil {
		role := strings.ToUpper(strings.TrimSpace(*in.Role))
		if _, known := models.RoleHierarchy[models.Role(role)]; !known {
			return nil, ErrUnknownRank
		}
		if models.RoleHierarchy[models.Role(role)] > models.RoleHierarchy[a.Role] {
			return nil, ErrRankAboveYourOwn
		}
		if role != before.Role {
			add("role = $%d::user_role", role)
			changed = append(changed, "rank")
		}
	}

	transfer := ""
	if in.StationID != nil && strings.TrimSpace(*in.StationID) != "" {
		station, err := uuid.Parse(strings.TrimSpace(*in.StationID))
		if err != nil {
			return nil, ErrPostingNotFound
		}
		if before.StationID == nil || *before.StationID != station {
			if err := s.postingIsYours(ctx, station, a); err != nil {
				s.refused(ctx, a, "officer_transfer_refused", id,
					fmt.Sprintf("Refused to transfer %s", before.Username), err)
				return nil, err
			}
			// The department moves with the posting, in the same statement, so
			// the row is never momentarily an officer of one force sitting at
			// another force's station.
			add("station_id = $%d", station)
			args = append(args, a.ForceID)
			sets = append(sets, fmt.Sprintf("force_id = force_for_posting($%d, $%d)", len(args)-1, len(args)))
			changed = append(changed, "posting")
			transfer = station.String()
		}
	}

	if len(sets) == 0 {
		return before, nil
	}
	sets = append(sets, "updated_at = NOW()")

	if _, err := s.db.Exec(ctx, "UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = $1", args...); err != nil {
		stated := s.stateTheRule(err)
		s.refused(ctx, a, "officer_amend_refused", id,
			fmt.Sprintf("Refused an amendment to %s", before.Username), stated)
		return nil, stated
	}

	after, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		// A transfer can move an officer into a wing this administrator still
		// administers; if it somehow did not, say so plainly.
		return nil, err
	}

	action, description := "officer_amended", fmt.Sprintf("Amended %s (%s): %s",
		after.Name, after.Username, strings.Join(changed, ", "))
	if transfer != "" {
		action = "officer_transferred"
		description = fmt.Sprintf("Transferred %s (%s) from %s, %s to %s, %s",
			after.Name, after.Username, orNone(before.StationName), before.ForceShortName,
			orNone(after.StationName), after.ForceShortName)
		if len(changed) > 1 {
			description += "; also amended " + strings.Join(without(changed, "posting"), ", ")
		}
	}
	s.audit(ctx, a, action, id, description, true, "")

	return after, nil
}

// Deactivate closes an account. It is a flag and nothing else: the officer's
// FIRs, cases, custody entries and audit lines all still name them.
func (s *UserAdminService) Deactivate(ctx context.Context, id uuid.UUID, reason string, actorID uuid.UUID) (*Officer, error) {
	a, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if id == actorID {
		return nil, errors.New("an officer cannot deactivate their own account")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("closing an account needs a reason")
	}

	officer, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}
	if !officer.IsActive {
		return nil, ErrAlreadyInactive
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE users
		   SET is_active = FALSE, deactivated_at = NOW(), deactivated_by = $2,
		       deactivation_reason = $3, updated_at = NOW()
		 WHERE id = $1`, id, actorID, strings.TrimSpace(reason)); err != nil {
		return nil, err
	}

	s.audit(ctx, a, "officer_deactivated", id,
		fmt.Sprintf("Deactivated %s (%s): %s", officer.Name, officer.Username, strings.TrimSpace(reason)), true, "")

	return s.get(ctx, id, a.ForceID)
}

// Reactivate reopens a closed account, for the officer who comes back.
func (s *UserAdminService) Reactivate(ctx context.Context, id uuid.UUID, actorID uuid.UUID) (*Officer, error) {
	a, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}

	officer, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}
	if officer.IsActive {
		return nil, ErrAlreadyActive
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE users
		   SET is_active = TRUE, deactivated_at = NULL, deactivated_by = NULL,
		       deactivation_reason = NULL, updated_at = NOW()
		 WHERE id = $1`, id); err != nil {
		return nil, err
	}

	s.audit(ctx, a, "officer_reactivated", id,
		fmt.Sprintf("Reactivated %s (%s)", officer.Name, officer.Username), true, "")

	return s.get(ctx, id, a.ForceID)
}

// ResetPassword issues a new password for an officer who has lost theirs. The
// new one is shown once and the account owes a change, exactly as on creation.
func (s *UserAdminService) ResetPassword(ctx context.Context, id uuid.UUID, actorID uuid.UUID) (*IssuedPassword, error) {
	a, err := s.actor(ctx, actorID)
	if err != nil {
		return nil, err
	}

	officer, err := s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}

	password, err := generatePassword()
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	// Two statements, in one transaction. A change of password_hash clears the
	// "owes a change" mark by trigger — that is how the officer's own change
	// through /me/password clears it — so an administrator's reset raises the
	// mark again afterwards, in a statement that leaves password_hash alone.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, hash); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET must_change_password = TRUE WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// The description names the officer and the actor. It never names the
	// password, and the hash is not written anywhere but the column.
	s.audit(ctx, a, "officer_password_reset", id,
		fmt.Sprintf("Issued a new password for %s (%s)", officer.Name, officer.Username), true, "")

	officer, err = s.get(ctx, id, a.ForceID)
	if err != nil {
		return nil, err
	}
	return &IssuedPassword{
		Officer:  officer,
		Password: password,
		Notice:   "This password is shown once. Hand it to the officer, who must change it when they next sign in.",
	}, nil
}

// postingIsYours checks that the station is one this administrator may post to
// before the database is asked. The database would refuse a posting that
// breaks the department rule; it would not refuse a Kolkata Police
// administrator opening an account at a West Bengal Police station, because
// the department would simply follow the station. That boundary is this
// service's to hold, and it is held here.
func (s *UserAdminService) postingIsYours(ctx context.Context, station uuid.UUID, a *administrator) error {
	// COALESCE, because a force with no parent makes the comparison NULL and
	// `FALSE OR NULL` is NULL, not false: without it a refusal arrives as a
	// scan error instead of as a rule.
	var allowed bool
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(s.force_id = $2, FALSE)
		    OR COALESCE((SELECT f.parent_id FROM forces f WHERE f.id = s.force_id) = $2, FALSE)
		    OR COALESCE(s.force_id = (SELECT af.parent_id FROM forces af WHERE af.id = $2), FALSE)
		  FROM stations s WHERE s.id = $1`, station, a.ForceID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("there is no such station")
	}
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPostingOutsideForce
	}
	return nil
}

// stateTheRule turns a database refusal into the rule that caused it. A
// refusal is a rule, and an officer is told which rule rather than shown a
// server error.
func (s *UserAdminService) stateTheRule(err error) error {
	text := err.Error()
	switch {
	case strings.Contains(text, "user_force_matches_posting"):
		return ErrPostingOutsideForce
	case strings.Contains(text, "users_username_key"):
		return ErrUsernameTaken
	case strings.Contains(text, "users_badge_number_key"):
		return ErrBadgeTaken
	case strings.Contains(text, "users_email_key"):
		return ErrEmailTaken
	case strings.Contains(text, "users_are_never_deleted"):
		return errors.New("an officer account is deactivated, never deleted")
	case strings.Contains(text, "invalid input value for enum user_role"):
		return ErrUnknownRank
	}
	return err
}

func (s *UserAdminService) audit(ctx context.Context, a *administrator, action string, id uuid.UUID, description string, success bool, failure string) {
	actor := a.ID
	entry := &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "user_account",
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

// refused records an attempt the rules stopped. An account that appears with
// no audit entry is what an inspection looks for; so is a refusal nobody can
// find afterwards.
func (s *UserAdminService) refused(ctx context.Context, a *administrator, action string, id uuid.UUID, description string, err error) {
	s.audit(ctx, a, action, id, description, false, err.Error())
}

// generatePassword makes a first password strong enough that nobody is tempted
// to keep it: twenty characters from crypto/rand, with at least one of each
// class, and no characters that are read wrongly off a screen (O/0, l/1/I).
func generatePassword() (string, error) {
	const (
		upper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		lower  = "abcdefghijkmnopqrstuvwxyz"
		digits = "23456789"
		marks  = "!@#$%^&*?-+"
	)
	all := upper + lower + digits + marks

	out := make([]byte, 0, 20)
	for _, set := range []string{upper, lower, digits, marks} {
		c, err := pick(set)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	for len(out) < 20 {
		c, err := pick(all)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}

	// Shuffle, so the first four characters are not always one of each class.
	for i := len(out) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := n.Int64()
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

func pick(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, err
	}
	return set[n.Int64()], nil
}

func orNone(s string) string {
	if s == "" {
		return "no station"
	}
	return s
}

func without(values []string, drop string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}
