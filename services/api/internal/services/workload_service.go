package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// ErrForbiddenScope marks a request for stations outside the officer's reach.
var ErrForbiddenScope = errors.New("outside your scope")

// Phase 08 thresholds. Stated once, returned with the figures, shown on screen.
const (
	// BNSS s.187(3): unless the charge-sheet is filed, an accused in custody
	// becomes entitled to default bail after 60 days, or 90 days for offences
	// punishable with death, life imprisonment or at least ten years.
	CustodyPeriodShort = 60
	CustodyPeriodLong  = 90
	// Items are listed from this many days after arrest so they surface
	// before the shorter period runs out.
	CustodyWarningFrom = 45

	// SLA lists are capped in the response; the breach count is always exact.
	slaItemLimit = 25
)

type WorkloadService struct {
	repo      *repository.WorkloadRepository
	auditRepo *repository.AuditRepository
}

func NewWorkloadService(repo *repository.WorkloadRepository, auditRepo *repository.AuditRepository) *WorkloadService {
	return &WorkloadService{repo: repo, auditRepo: auditRepo}
}

// ScopeRequest is what the officer asked for, and who they are.
type ScopeRequest struct {
	Role         models.Role
	ActorStation *uuid.UUID
	StationID    string
	District     string
	From         *time.Time
	To           *time.Time
}

func canCompare(role models.Role) bool {
	return models.RoleHierarchy[role] >= models.RoleHierarchy[models.RoleDSP]
}

// resolveScope applies the access rule: an SHO sees their own station; DSP and
// above may see any station, a district, or all stations.
func (s *WorkloadService) resolveScope(ctx context.Context, req ScopeRequest) (models.WorkloadScope, error) {
	scope := models.WorkloadScope{From: req.From, To: req.To}
	if req.From != nil && req.To != nil && !req.To.After(*req.From) {
		return scope, invalid("the end of the date range must be after its start")
	}
	stations, err := s.repo.ScopeOptions(ctx)
	if err != nil {
		return scope, err
	}
	compare := canCompare(req.Role)

	switch {
	case req.StationID != "":
		if _, err := uuid.Parse(req.StationID); err != nil {
			return scope, invalid("invalid stationId")
		}
		var match *models.WorkloadScopeOption
		for i := range stations {
			if stations[i].StationID == req.StationID {
				match = &stations[i]
			}
		}
		if match == nil {
			return scope, invalid("unknown station")
		}
		if !compare && (req.ActorStation == nil || req.ActorStation.String() != req.StationID) {
			return scope, fmt.Errorf("%w: an SHO sees their own station only", ErrForbiddenScope)
		}
		scope.Level, scope.Label, scope.StationIDs = "station", match.Name, []string{match.StationID}
	case req.District != "":
		if !compare {
			return scope, fmt.Errorf("%w: district figures are for DSP and above", ErrForbiddenScope)
		}
		for _, st := range stations {
			if st.District == req.District {
				scope.StationIDs = append(scope.StationIDs, st.StationID)
			}
		}
		if len(scope.StationIDs) == 0 {
			return scope, invalid("unknown district")
		}
		scope.Level, scope.Label = "district", req.District
	case compare:
		scope.Level, scope.Label = "all", "All stations"
	default:
		if req.ActorStation == nil {
			return scope, invalid("your account is not posted to a station")
		}
		for _, st := range stations {
			if st.StationID == req.ActorStation.String() {
				scope.Level, scope.Label, scope.StationIDs = "station", st.Name, []string{st.StationID}
			}
		}
		if scope.Level == "" {
			return scope, invalid("your station is not registered")
		}
	}
	return scope, nil
}

func (s *WorkloadService) Scopes(ctx context.Context, role models.Role, actorStation *uuid.UUID) (*models.WorkloadScopes, error) {
	stations, err := s.repo.ScopeOptions(ctx)
	if err != nil {
		return nil, err
	}
	out := &models.WorkloadScopes{CanCompare: canCompare(role), Stations: []models.WorkloadScopeOption{}, Districts: []string{}}
	if actorStation != nil {
		id := actorStation.String()
		out.OwnStation = &id
	}
	seen := map[string]bool{}
	for _, st := range stations {
		if !out.CanCompare && (actorStation == nil || st.StationID != actorStation.String()) {
			continue
		}
		out.Stations = append(out.Stations, st)
		if out.CanCompare && st.District != "" && !seen[st.District] {
			seen[st.District] = true
			out.Districts = append(out.Districts, st.District)
		}
	}
	return out, nil
}

type metricDef struct {
	key, label, definition, href string
	attention                    bool
}

var summaryMetrics = []metricDef{
	{"firsRegistered", "FIRs registered, investigation not begun", "FIRs whose status is REGISTERED.", "/fir", false},
	{"firsUnderInvestigation", "FIRs under investigation", "FIRs whose status is UNDER_INVESTIGATION.", "/fir", false},
	{"casesOpen", "Cases under investigation", "Cases whose status is REGISTERED or UNDER_INVESTIGATION.", "/cases", false},
	{"casesInCourt", "Cases charge-sheeted or in court", "Cases whose status is CHARGESHEET_FILED or IN_COURT.", "/cases", false},
	{"forensicsPending", "Forensic requests pending", "Forensic requests whose status is PENDING or IN_PROGRESS.", "/forensics", false},
	{"forensicsPastExpected", "Forensic requests past the lab's expected date", "Pending forensic requests whose recorded expected date has passed.", "/forensics", true},
	{"hearingsNext7", "Court hearings in the next 7 days", "Hearings dated today or within the following six days.", "/court", false},
	{"warrantsActive", "Warrants active", "Warrants whose status is ACTIVE.", "/warrant", false},
	{"warrantsLapsed", "Active warrants past validity", "ACTIVE warrants whose valid-until date has passed.", "/warrant", true},
	{"bailPending", "Bail applications pending", "Bail applications whose status is PENDING.", "/bail", false},
	{"evidenceBroken", "Evidence failing integrity", "Evidence items whose last verification found the stored file changed.", "/custody", true},
	{"evidenceUnverified", "Evidence files not yet verified", "Evidence items with a stored file that has not been verified since upload.", "/custody", false},
	{"lookoutsActive", "Lookout notices active", "Lookout notices whose status is ACTIVE.", "/lookout", false},
	{"tasksOpen", "Investigation tasks open", "Investigation tasks not marked done.", "/investigation", false},
	{"tasksOverdue", "Investigation tasks past due", "Open investigation tasks whose due date has passed.", "/investigation", true},
	{"weaponsOverdue", "Weapons not returned by the expected time", "Weapon issuances still open after their expected return time.", "/armoury", true},
	{"rosterStrength", "Personnel on the roster", "Personnel records not SUSPENDED.", "/personnel", false},
	{"onDuty", "Personnel on duty", "Personnel records whose status is ON_DUTY.", "/personnel", false},
	{"onLeave", "Personnel on leave", "Personnel records whose status is ON_LEAVE.", "/personnel", false},
}

func (s *WorkloadService) Summary(ctx context.Context, req ScopeRequest) (*models.WorkloadSummary, error) {
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.Summary(ctx, scope.StationIDs, req.From, req.To)
	if err != nil {
		return nil, err
	}
	out := &models.WorkloadSummary{Scope: scope, GeneratedAt: time.Now()}
	for _, m := range summaryMetrics {
		out.Metrics = append(out.Metrics, models.WorkloadMetric{
			Key: m.key, Label: m.label, Count: counts[m.key], Definition: m.definition, Href: m.href,
			Attention: m.attention && counts[m.key] > 0,
		})
	}
	return out, nil
}

var backlogStages = []struct {
	stage, label, definition, ageFrom, href string
}{
	{"investigation", "Under investigation", "Cases REGISTERED or UNDER_INVESTIGATION, plus open FIRs that have no case yet.", "FIR registration (case registration where no FIR is linked)", "/cases"},
	{"forensic", "Awaiting forensic results", "Forensic requests PENDING or IN_PROGRESS.", "submission to the lab", "/forensics"},
	{"court", "Charge-sheeted or in court", "Cases CHARGESHEET_FILED or IN_COURT.", "FIR registration", "/court"},
	{"tasks", "Open investigation tasks", "Investigation tasks not marked done.", "task creation", "/investigation"},
}

// BottleneckRule is the deterministic rule the backlog view applies.
const BottleneckRule = "The bottleneck is the stage holding the most items older than 90 days in that stage. Ties go to the stage with more items older than 180 days, then to the larger open total. When no item is older than 90 days, no stage is named."

func (s *WorkloadService) Backlog(ctx context.Context, req ScopeRequest) (*models.WorkloadBacklog, error) {
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	bands, err := s.repo.Backlog(ctx, scope.StationIDs, req.From, req.To)
	if err != nil {
		return nil, err
	}
	out := &models.WorkloadBacklog{Scope: scope, Rule: BottleneckRule}
	var best *models.BacklogStage
	for _, st := range backlogStages {
		b := bands[st.stage]
		stage := models.BacklogStage{Stage: st.stage, Label: st.label, Definition: st.definition,
			AgeFrom: st.ageFrom, Href: st.href, Bands: b}
		out.Stages = append(out.Stages, stage)
		out.Totals.Days0to30 += b.Days0to30
		out.Totals.Days31to90 += b.Days31to90
		out.Totals.Days91to180 += b.Days91to180
		out.Totals.Days181Plus += b.Days181Plus
		out.Totals.Total += b.Total
	}
	for i := range out.Stages {
		st := &out.Stages[i]
		if aged(st.Bands) == 0 {
			continue
		}
		if best == nil || beats(st.Bands, best.Bands) {
			best = st
		}
	}
	if best == nil {
		out.Reason = "No stage holds items older than 90 days."
	} else {
		name := best.Stage
		out.Bottleneck = &name
		out.Reason = fmt.Sprintf("%s holds %d items older than 90 days (%d older than 180).",
			best.Label, aged(best.Bands), best.Bands.Days181Plus)
	}
	return out, nil
}

func aged(b models.AgeBands) int64 { return b.Days91to180 + b.Days181Plus }

func beats(a, b models.AgeBands) bool {
	if aged(a) != aged(b) {
		return aged(a) > aged(b)
	}
	if a.Days181Plus != b.Days181Plus {
		return a.Days181Plus > b.Days181Plus
	}
	return a.Total > b.Total
}

// Stations compares stations. DSP and above only (enforced by the route).
func (s *WorkloadService) Stations(ctx context.Context, req ScopeRequest) ([]models.StationWorkload, *models.WorkloadScope, error) {
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.repo.Stations(ctx, scope.StationIDs, req.From, req.To)
	return rows, &scope, err
}

// Officers lists assigned load per officer. Every view is audited: it exposes
// named officers' caseloads.
func (s *WorkloadService) Officers(ctx context.Context, req ScopeRequest, actor *uuid.UUID) ([]models.OfficerWorkload, *models.WorkloadScope, error) {
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.repo.Officers(ctx, scope.StationIDs)
	if err != nil {
		return nil, nil, err
	}
	desc := fmt.Sprintf("Viewed officer workload for %s (%d officers)", scope.Label, len(rows))
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: actor, Action: "workload_officers_viewed", ResourceType: "workload",
		Description: &desc, Success: true,
	})
	return rows, &scope, nil
}

func (s *WorkloadService) SLA(ctx context.Context, req ScopeRequest) (*models.WorkloadSLA, error) {
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	out := &models.WorkloadSLA{Scope: scope}

	custody, err := s.repo.CustodyWithoutChargesheet(ctx, scope.StationIDs, CustodyWarningFrom)
	if err != nil {
		return nil, err
	}
	for i := range custody {
		days := custody[i].DaysOver
		switch {
		case days > CustodyPeriodLong:
			custody[i].Band = "past-90"
		case days > CustodyPeriodShort:
			custody[i].Band = "past-60"
		default:
			custody[i].Band = "approaching-60"
		}
		custody[i].Href = "/cases/" + custody[i].ID
	}
	out.Rules = append(out.Rules, rule("custodyChargesheet", "Accused in custody without a charge-sheet",
		"Accused recorded as ARRESTED or IN_CUSTODY on a case still under investigation, listed from 45 days after arrest. Days are counted from the recorded arrest date.",
		"BNSS 2023, s.187(3) — default bail if the investigation is not completed within 60 days, or 90 days for offences punishable with death, life imprisonment or ten years or more.",
		"The period runs from the first remand, which is not recorded; the arrest date is used instead. The punishment class of the sections is not recorded either, so an item past 60 days must be checked against the 90-day period by the officer.",
		custody))

	forensic, err := s.repo.ForensicsPastExpected(ctx, scope.StationIDs)
	if err != nil {
		return nil, err
	}
	for i := range forensic {
		forensic[i].Href = "/forensics"
	}
	out.Rules = append(out.Rules, rule("forensicExpected", "Forensic results past the expected date",
		"Pending forensic requests whose expected date, as recorded when the request was made, has passed.",
		"The expected date recorded on each request. No turnaround norm is assumed.",
		"Requests without an expected date are not measured.", forensic))

	warrants, err := s.repo.WarrantsLapsed(ctx, scope.StationIDs)
	if err != nil {
		return nil, err
	}
	for i := range warrants {
		warrants[i].Href = "/warrant/" + warrants[i].ID
	}
	out.Rules = append(out.Rules, rule("warrantValidity", "Active warrants past validity",
		"Warrants still ACTIVE after the valid-until date recorded from the court's order.",
		"The validity date on the warrant.", "Warrants without a valid-until date are not measured.", warrants))

	tasks, err := s.repo.TasksOverdue(ctx, scope.StationIDs)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Href = "/investigation/" + tasks[i].ID
	}
	out.Rules = append(out.Rules, rule("taskDue", "Investigation tasks past due",
		"Open investigation tasks whose due date has passed.", "The due date set on each task.",
		"Tasks without a due date are not measured.", tasks))

	weapons, err := s.repo.WeaponsOverdue(ctx, scope.StationIDs)
	if err != nil {
		return nil, err
	}
	for i := range weapons {
		weapons[i].Href = "/armoury"
	}
	out.Rules = append(out.Rules, rule("weaponReturn", "Weapons not returned on time",
		"Weapons still issued after the expected return time recorded at issue.",
		"The expected return time on the issuance.", "Issuances without an expected return time are not measured.", weapons))

	return out, nil
}

func rule(key, label, definition, source, limits string, items []models.SLAItem) models.SLARule {
	r := models.SLARule{Key: key, Label: label, Rule: definition, Source: source, Limits: limits,
		Breaches: int64(len(items)), Items: items}
	if len(r.Items) > slaItemLimit {
		r.Items = r.Items[:slaItemLimit]
	}
	return r
}

var trendSeries = []struct{ key, label, definition string }{
	{"firsRegistered", "FIRs registered", "FIRs by the date they were registered."},
	{"casesRegistered", "Cases registered", "Cases by the date they were registered."},
	{"forensicsSubmitted", "Forensic requests submitted", "Forensic requests by submission date."},
	{"forensicsCompleted", "Forensic requests completed", "Forensic requests by completion date."},
	{"warrantsExecuted", "Warrants executed", "Warrants by execution date."},
	{"workspacesClosed", "Investigations closed", "Investigation workspaces by the date they were closed."},
	{"bailDecided", "Bail applications decided", "Bail applications by the date of the grant or rejection."},
}

// TrendsNotShown lists closures the records cannot date, so they are not trended.
var TrendsNotShown = []string{
	"FIR and case closures: the records hold a status but not the date it was reached.",
}

func (s *WorkloadService) Trends(ctx context.Context, req ScopeRequest, interval string) (*models.WorkloadTrends, error) {
	if interval == "" {
		interval = "week"
	}
	if interval != "week" && interval != "month" {
		return nil, invalid("interval must be week or month")
	}
	to := time.Now()
	if req.To != nil {
		to = *req.To
	}
	from := to.AddDate(0, 0, -7*12)
	if interval == "month" {
		from = to.AddDate(-1, 0, 0)
	}
	if req.From != nil {
		from = *req.From
	}
	maxRange := 53 * 7 * 24 * time.Hour // a year of weeks
	if interval == "month" {
		maxRange = 3 * 366 * 24 * time.Hour // three years of months
	}
	if !to.After(from) {
		return nil, invalid("the end of the date range must be after its start")
	}
	if to.Sub(from) > maxRange {
		return nil, invalid("the range is too long for %s buckets", interval)
	}
	req.From, req.To = &from, &to
	scope, err := s.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	buckets, series, err := s.repo.Trends(ctx, scope.StationIDs, from, to, interval)
	if err != nil {
		return nil, err
	}
	if buckets == nil {
		buckets = []time.Time{}
	}
	out := &models.WorkloadTrends{Scope: scope, Interval: interval, Buckets: buckets, NotShown: TrendsNotShown}
	for _, t := range trendSeries {
		out.Series = append(out.Series, models.TrendSeries{Key: t.key, Label: t.label, Definition: t.definition, Values: series[t.key]})
	}
	return out, nil
}
