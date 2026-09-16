package services_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
	"github.com/npdms/api/testutil"
)

// The rules this module exists to keep, exercised against a real database
// because half of each rule lives in the schema: the department follows the
// posting because a trigger says so, and an account survives deactivation
// because another trigger refuses to delete it.

type adminHarness struct {
	t       *testing.T
	ctx     context.Context
	tdb     *testutil.TestDB
	service *services.UserAdminService
	made    []uuid.UUID
}

func newAdminHarness(t *testing.T) *adminHarness {
	t.Helper()
	tdb := testutil.NewTestDB(t)

	h := &adminHarness{
		t:       t,
		ctx:     context.Background(),
		tdb:     tdb,
		service: services.NewUserAdminService(tdb.Pool, repository.NewAuditRepository(tdb.Pool)),
	}
	t.Cleanup(h.purge)
	return h
}

// purge removes the accounts the test opened.
//
// It has to take the guard off the table to do it, because the platform has no
// route to a delete: that is the rule under test, and the test would be worth
// less if there were a back door in the application for it to use. Taking the
// trigger off is an act of the database owner, outside the API entirely, and
// it is put straight back.
func (h *adminHarness) purge() {
	if len(h.made) == 0 {
		return
	}
	if _, err := h.tdb.Pool.Exec(h.ctx, `ALTER TABLE users DISABLE TRIGGER trg_users_are_never_deleted`); err != nil {
		h.t.Logf("could not lift the no-delete guard, leaving %d demo account(s) deactivated: %v", len(h.made), err)
		for _, id := range h.made {
			h.tdb.Pool.Exec(h.ctx, `UPDATE users SET is_active = FALSE, deactivated_at = NOW(),
				deactivation_reason = 'Automated test account' WHERE id = $1`, id)
		}
		return
	}
	defer h.tdb.Pool.Exec(h.ctx, `ALTER TABLE users ENABLE TRIGGER trg_users_are_never_deleted`)

	// The audit entries are left where they are. The chain is append-only and
	// the database refuses to remove a link from it, which is the whole point
	// of the chain.
	for _, id := range h.made {
		if _, err := h.tdb.Pool.Exec(h.ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
			h.t.Logf("could not remove test account %s: %v", id, err)
		}
	}
}

// userID looks up one of the demo accounts to act as. Tests never create their
// own administrator: the point is that the ranks the platform ships with can
// do this work.
func (h *adminHarness) userID(username string) uuid.UUID {
	h.t.Helper()
	var id uuid.UUID
	if err := h.tdb.Pool.QueryRow(h.ctx, `SELECT id FROM users WHERE username = $1`, username).Scan(&id); err != nil {
		h.t.Skipf("demo account %q is not in this database: %v", username, err)
	}
	return id
}

// station returns a station by its code.
func (h *adminHarness) station(code string) uuid.UUID {
	h.t.Helper()
	var id uuid.UUID
	if err := h.tdb.Pool.QueryRow(h.ctx, `SELECT id FROM stations WHERE code = $1`, code).Scan(&id); err != nil {
		h.t.Skipf("station %q is not in this database: %v", code, err)
	}
	return id
}

// newOfficer fills in a create form with a name nobody will mistake for a real
// officer, so that anything the cleanup misses is obviously a test account.
func (h *adminHarness) newOfficer(station uuid.UUID, role string) services.NewOfficer {
	suffix := strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	return services.NewOfficer{
		Username:    "zz.test." + suffix,
		Name:        "Demo Test Officer " + suffix,
		Email:       "zz.test." + suffix + "@demo.invalid",
		Phone:       "9800000000",
		BadgeNumber: "ZZTEST-" + suffix,
		Role:        role,
		StationID:   station.String(),
	}
}

func (h *adminHarness) create(actor uuid.UUID, in services.NewOfficer) (*services.IssuedPassword, error) {
	issued, err := h.service.Create(h.ctx, in, actor)
	if issued != nil && issued.Officer != nil {
		h.made = append(h.made, issued.Officer.ID)
	}
	return issued, err
}

// An administrator of one force cannot open an account in another.
//
// The database would not stop this on its own: the department follows the
// station, so a Kolkata Police Superintendent posting to a West Bengal Police
// station would simply produce a West Bengal Police officer, and every trigger
// would be satisfied. The boundary is the service's to hold, and this is the
// test that says it does.
func TestAdministratorCannotOpenAnAccountInAnotherForce(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")    // Superintendent, Kolkata Police
	westBengal := h.station("HWR") // Howrah Police Station, West Bengal Police
	before := h.tdb.Count(t, "users")

	_, err := h.create(kolkataSP, h.newOfficer(westBengal, "SI"))
	if !errors.Is(err, services.ErrPostingOutsideForce) {
		t.Fatalf("expected the posting to be refused as outside the department, got %v", err)
	}
	if after := h.tdb.Count(t, "users"); after != before {
		t.Fatalf("a refused creation still added a row: %d became %d", before, after)
	}

	// The refusal is on the record, not only in the reply.
	var refusals int
	h.tdb.Pool.QueryRow(h.ctx, `
		SELECT COUNT(*) FROM audit_logs
		 WHERE event_type = 'officer_create_refused' AND actor_user_id = $1
		   AND event_timestamp > NOW() - INTERVAL '1 minute'`, kolkataSP).Scan(&refusals)
	if refusals == 0 {
		t.Error("a refused creation left no audit entry")
	}
}

// The other direction of the same boundary: a West Bengal Police
// Superintendent cannot amend a Kolkata Police officer, and cannot even see
// them to try.
func TestAdministratorCannotAmendAnotherForcesOfficer(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	northSP := h.userID("sp.n24") // Superintendent, West Bengal Police

	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening an account at the administrator's own station: %v", err)
	}

	name := "Renamed By Another Force"
	if _, err := h.service.Amend(h.ctx, issued.Officer.ID, services.OfficerAmendment{Name: &name}, northSP); !errors.Is(err, services.ErrOfficerNotFound) {
		t.Fatalf("expected the officer to be invisible to another department, got %v", err)
	}

	after, err := h.service.Get(h.ctx, issued.Officer.ID, kolkataSP)
	if err != nil {
		t.Fatalf("reading the officer back: %v", err)
	}
	if after.Name == name {
		t.Error("an administrator of another department changed the officer's name")
	}
}

// A department is never chosen: it follows the station. A Kolkata Police
// Superintendent posting an officer to a traffic guard makes a traffic
// officer, without being asked which department to use.
func TestDepartmentFollowsThePosting(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("TG-PKS"), "SI"))
	if err != nil {
		t.Fatalf("opening an account at a traffic guard: %v", err)
	}
	if issued.Officer.ForceCode != "TRAFFIC" {
		t.Fatalf("an officer posted to a traffic guard is in %s, expected TRAFFIC", issued.Officer.ForceCode)
	}
}

// A transfer that crosses a departmental boundary moves the officer's
// department with it, in the same statement, so the row is never an officer of
// one force sitting at another force's station.
func TestTransferAcrossADepartmentMovesTheDepartment(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}
	if issued.Officer.ForceCode != "KP" {
		t.Fatalf("an officer posted to a Kolkata station is in %s, expected KP", issued.Officer.ForceCode)
	}

	traffic := h.station("TG-JDP").String()
	moved, err := h.service.Amend(h.ctx, issued.Officer.ID, services.OfficerAmendment{StationID: &traffic}, kolkataSP)
	if err != nil {
		t.Fatalf("transferring to a traffic guard: %v", err)
	}
	if moved.ForceCode != "TRAFFIC" {
		t.Fatalf("after the transfer the officer is in %s, expected TRAFFIC", moved.ForceCode)
	}

	var stored string
	h.tdb.Pool.QueryRow(h.ctx,
		`SELECT f.code FROM users u JOIN forces f ON f.id = u.force_id WHERE u.id = $1`,
		issued.Officer.ID).Scan(&stored)
	if stored != "TRAFFIC" {
		t.Fatalf("the stored department is %s, expected TRAFFIC", stored)
	}

	var transfers int
	h.tdb.Pool.QueryRow(h.ctx, `
		SELECT COUNT(*) FROM audit_logs
		 WHERE event_type = 'officer_transferred' AND resource_id = $1`, issued.Officer.ID).Scan(&transfers)
	if transfers != 1 {
		t.Errorf("a transfer left %d audit entries, expected 1", transfers)
	}
}

// A transfer to a station of a force the administrator does not administer is
// refused as a stated rule.
func TestTransferOutsideTheDepartmentIsRefused(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}

	westBengal := h.station("HWR").String()
	if _, err := h.service.Amend(h.ctx, issued.Officer.ID, services.OfficerAmendment{StationID: &westBengal}, kolkataSP); !errors.Is(err, services.ErrPostingOutsideForce) {
		t.Fatalf("expected the transfer to be refused as outside the department, got %v", err)
	}
}

// The database backs the service up. Even with the service out of the way, a
// row that puts an officer of one force at another force's station is refused,
// and the refusal names the rule rather than corrupting quietly.
func TestTheDatabaseRefusesAnImpossiblePosting(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}

	// Straight SQL: move the posting to a West Bengal Police station and leave
	// the department as Kolkata Police.
	_, err = h.tdb.Pool.Exec(h.ctx,
		`UPDATE users SET station_id = (SELECT id FROM stations WHERE code = 'HWR') WHERE id = $1`,
		issued.Officer.ID)
	if err == nil {
		t.Fatal("the database allowed a Kolkata Police officer to be posted to a West Bengal Police station")
	}
	if !strings.Contains(err.Error(), "user_force_matches_posting") &&
		!strings.Contains(err.Error(), "cannot be posted to a station of another force") {
		t.Fatalf("the refusal did not name the rule: %v", err)
	}
}

// Deactivation is a flag. The row survives, the audit trail survives, and the
// database refuses to remove the account at all.
func TestDeactivationDoesNotDelete(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}
	id := issued.Officer.ID

	closed, err := h.service.Deactivate(h.ctx, id, "Retired from the force", kolkataSP)
	if err != nil {
		t.Fatalf("deactivating: %v", err)
	}
	if closed.IsActive {
		t.Error("the account is still active after deactivation")
	}

	var active bool
	var reason, closedBy string
	if err := h.tdb.Pool.QueryRow(h.ctx, `
		SELECT u.is_active, COALESCE(u.deactivation_reason, ''), COALESCE(b.username, '')
		  FROM users u LEFT JOIN users b ON b.id = u.deactivated_by
		 WHERE u.id = $1`, id).Scan(&active, &reason, &closedBy); err != nil {
		t.Fatalf("the row did not survive deactivation: %v", err)
	}
	if active {
		t.Error("the stored row is still active")
	}
	if reason != "Retired from the force" {
		t.Errorf("the reason was not kept: %q", reason)
	}
	if closedBy != "sp" {
		t.Errorf("the officer who closed the account was not kept: %q", closedBy)
	}

	// The account can no longer be found by a sign-in, which reads only
	// active accounts.
	users := repository.NewUserRepository(h.tdb.Pool)
	if _, err := users.FindByUsername(h.ctx, issued.Officer.Username); err == nil {
		t.Error("a deactivated account can still be found by a sign-in")
	}

	// And there is no way to remove it.
	if _, err := h.tdb.Pool.Exec(h.ctx, `DELETE FROM users WHERE id = $1`, id); err == nil {
		t.Fatal("the database allowed an officer account to be deleted")
	} else if !strings.Contains(err.Error(), "deactivated, never deleted") {
		t.Fatalf("the refusal did not name the rule: %v", err)
	}

	var entries int
	h.tdb.Pool.QueryRow(h.ctx, `
		SELECT COUNT(*) FROM audit_logs WHERE resource_id = $1 AND resource_type = 'user_account'`,
		id).Scan(&entries)
	if entries < 2 {
		t.Errorf("the account has %d audit entries; expected at least the creation and the deactivation", entries)
	}

	// Reopening it is possible, because officers do come back.
	reopened, err := h.service.Reactivate(h.ctx, id, kolkataSP)
	if err != nil {
		t.Fatalf("reactivating: %v", err)
	}
	if !reopened.IsActive {
		t.Error("the account did not reopen")
	}
}

// An administrator cannot appoint above their own rank.
func TestAnAccountCannotBeOpenedAboveTheAdministratorsRank(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp") // Superintendent
	if _, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "DGP")); !errors.Is(err, services.ErrRankAboveYourOwn) {
		t.Fatalf("expected an appointment above the administrator's rank to be refused, got %v", err)
	}
}

// The password is generated, shown once, and stored only as a hash. Nothing
// the officer record carries is the password or the hash.
func TestTheFirstPasswordIsIssuedOnceAndStoredHashed(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}

	if len(issued.Password) < 16 {
		t.Errorf("the issued password is %d characters, which is too short", len(issued.Password))
	}
	if strings.HasPrefix(issued.Password, "$2") {
		t.Fatal("a hash was returned where a password should have been")
	}
	if issued.Password == "Demo@123" {
		t.Fatal("the issued password is the shared demo password")
	}

	var hash string
	if err := h.tdb.Pool.QueryRow(h.ctx, `SELECT password_hash FROM users WHERE id = $1`, issued.Officer.ID).Scan(&hash); err != nil {
		t.Fatalf("reading the stored hash: %v", err)
	}
	if hash == issued.Password {
		t.Fatal("the password was stored in clear")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(issued.Password)); err != nil {
		t.Fatalf("the stored hash does not match the issued password: %v", err)
	}
	if !issued.Officer.MustChangePassword {
		t.Error("an account opened with an administrator's password does not owe a change")
	}

	// No audit entry anywhere near this account may carry the password.
	rows, err := h.tdb.Pool.Query(h.ctx,
		`SELECT COALESCE(outcome_reason, '') || ' ' || COALESCE(resource_attributes::text, '')
		   FROM audit_logs WHERE resource_id = $1`, issued.Officer.ID)
	if err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		rows.Scan(&line)
		if strings.Contains(line, issued.Password) || strings.Contains(line, hash) {
			t.Fatal("an audit entry carries the password or its hash")
		}
	}

	// A reset issues a different password and replaces the hash.
	reset, err := h.service.ResetPassword(h.ctx, issued.Officer.ID, kolkataSP)
	if err != nil {
		t.Fatalf("resetting the password: %v", err)
	}
	if reset.Password == issued.Password {
		t.Fatal("the reset issued the same password again")
	}

	var newHash string
	h.tdb.Pool.QueryRow(h.ctx, `SELECT password_hash FROM users WHERE id = $1`, issued.Officer.ID).Scan(&newHash)
	if newHash == hash {
		t.Fatal("the reset did not change the stored hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(newHash), []byte(reset.Password)); err != nil {
		t.Fatalf("the new hash does not match the new password: %v", err)
	}
}

// The "owes a password change" mark is raised by an administrator issuing a
// password and cleared by the officer setting their own, through the ordinary
// self-service change. Neither path has to remember to do it.
func TestTheOfficersOwnChangeClearsTheMark(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	issued, err := h.create(kolkataSP, h.newOfficer(h.station("BHW"), "SI"))
	if err != nil {
		t.Fatalf("opening the account: %v", err)
	}
	if !issued.Officer.MustChangePassword {
		t.Fatal("a newly opened account does not owe a password change")
	}

	auth := services.NewAuthService(repository.NewUserRepository(h.tdb.Pool), nil, "test-secret")
	if err := auth.ChangePassword(h.ctx, issued.Officer.ID, issued.Password, "TheirOwn#Password42"); err != nil {
		t.Fatalf("the officer changing their own password: %v", err)
	}

	after, err := h.service.Get(h.ctx, issued.Officer.ID, kolkataSP)
	if err != nil {
		t.Fatalf("reading the officer back: %v", err)
	}
	if after.MustChangePassword {
		t.Error("the officer set their own password and is still told they owe a change")
	}

	// And an administrator's reset puts the mark back.
	reset, err := h.service.ResetPassword(h.ctx, issued.Officer.ID, kolkataSP)
	if err != nil {
		t.Fatalf("resetting: %v", err)
	}
	if !reset.Officer.MustChangePassword {
		t.Error("an administrator issued a password and the account does not owe a change")
	}
}

// The list an administrator sees is their own department's, and the wings of
// it — never a neighbouring force's.
func TestTheListIsTheAdministratorsOwnDepartment(t *testing.T) {
	h := newAdminHarness(t)

	kolkataSP := h.userID("sp")
	officers, err := h.service.List(h.ctx, kolkataSP, "", "")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(officers) == 0 {
		t.Fatal("a Superintendent sees no officers at all")
	}
	for _, o := range officers {
		if o.ForceCode != "KP" && o.ForceCode != "TRAFFIC" {
			t.Errorf("a Kolkata Police administrator sees %s (%s), of %s", o.Username, o.Name, o.ForceCode)
		}
	}

	// And a wing's administrator does not see the parent force. Authority runs
	// downwards, not sideways or up.
	trafficSP := h.userID("dc.traffic")
	trafficList, err := h.service.List(h.ctx, trafficSP, "", "")
	if err != nil {
		t.Fatalf("listing for the traffic administrator: %v", err)
	}
	for _, o := range trafficList {
		if o.ForceCode != "TRAFFIC" {
			t.Errorf("a traffic administrator sees %s, of %s", o.Username, o.ForceCode)
		}
	}
}

// The stations a create form may offer are only the ones a posting would be
// allowed to.
func TestPostingsOfferedAreOnesThatWouldBeAllowed(t *testing.T) {
	h := newAdminHarness(t)

	trafficSP := h.userID("dc.traffic")
	postings, err := h.service.Postings(h.ctx, trafficSP)
	if err != nil {
		t.Fatalf("reading the postings: %v", err)
	}
	if len(postings) == 0 {
		t.Fatal("the traffic administrator is offered no stations")
	}

	seenParent := false
	for _, p := range postings {
		switch p.ForceCode {
		case "TRAFFIC":
		case "KP":
			// A traffic sergeant works out of a Kolkata Police station; that
			// is the one crossing the department rule allows.
			seenParent = true
		default:
			t.Errorf("the traffic administrator is offered %s, a station of %s", p.Name, p.ForceCode)
		}
	}
	if !seenParent {
		t.Error("the traffic administrator is not offered any of the parent force's stations")
	}

	ranks, err := h.service.Ranks(h.ctx, trafficSP)
	if err != nil {
		t.Fatalf("reading the ranks: %v", err)
	}
	for _, r := range ranks {
		if r == "DGP" || r == "IG" || r == "DIG" {
			t.Errorf("a Superintendent is offered the rank %s", r)
		}
	}
}

// A guard against the module quietly acquiring a delete: if one is ever added,
// this fails.
func TestThereIsNoRouteToADelete(t *testing.T) {
	h := newAdminHarness(t)

	var guard int
	h.tdb.Pool.QueryRow(h.ctx, `
		SELECT COUNT(*) FROM pg_trigger
		 WHERE tgrelid = 'users'::regclass AND tgname = 'trg_users_are_never_deleted'
		   AND NOT tgisinternal`).Scan(&guard)
	if guard != 1 {
		t.Fatalf("the no-delete guard is not on the users table (%d)", guard)
	}

	if _, err := h.tdb.Pool.Exec(h.ctx,
		fmt.Sprintf(`DELETE FROM users WHERE username = '%s'`, "sp")); err == nil {
		t.Fatal("a demo account was deleted")
	}
}
