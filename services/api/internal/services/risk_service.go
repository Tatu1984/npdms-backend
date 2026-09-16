package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// Phase 10 scores places with a documented weighted sum:
//
//	score = Σ weight(factor) × raw(factor)
//
// Raw values are counts of stored records. Weights are versioned configuration
// changed only by SP and above, with a reason. Nothing here predicts anything.

// ErrRiskForbidden marks a request outside the officer's permitted scope.
var ErrRiskForbidden = errors.New("outside your permitted scope")

// RiskFactors is the complete, ordered catalogue of scoring factors.
var RiskFactors = []models.RiskFactorDefinition{
	{Key: "fir_count", Label: "FIRs registered",
		Description: "FIRs whose incident date falls in the period and whose incident time falls in the shift window.", BeatAttributable: true},
	{Key: "serious_fir_count", Label: "High or critical priority FIRs",
		Description: "Of those FIRs, the ones recorded with HIGH or CRITICAL priority.", BeatAttributable: true},
	{Key: "night_fir_count", Label: "Night-time FIRs",
		Description: "Of those FIRs, the ones with an incident time from 20:00 to 05:59.", BeatAttributable: true},
	{Key: "alert_count", Label: "Alerts issued",
		Description: "Alerts issued for the station in the period. Alerts are issued per station and are not attributed to beats.", BeatAttributable: false},
	{Key: "fir_increase", Label: "Increase over previous period",
		Description: "FIRs in the period minus FIRs in the preceding period of equal length, when positive; otherwise 0.", BeatAttributable: true},
}

const riskFormula = "score = Σ (weight × raw value) over every factor listed; each contribution is rounded to 2 decimal places and the score is their sum"

const riskRecommendationRule = "Recommend the top N areas by score for the chosen period and shift window, excluding areas scoring 0. Ties are broken by more FIRs, then by name. The reasoning names the factor contributing most."

const riskSimulationMethod = "An area is covered when at least one unit is allocated to it. Coverage = sum of covered areas' scores ÷ sum of all areas' scores. This is arithmetic over the current scores; it does not estimate any change in incidents."

// RiskActor is the officer making a request, as established by authentication.
type RiskActor struct {
	ID      *uuid.UUID
	Role    models.Role
	Station *uuid.UUID
}

func (a RiskActor) atLeast(role models.Role) bool {
	return models.RoleHierarchy[a.Role] >= models.RoleHierarchy[role]
}

type RiskService struct {
	repo      *repository.RiskRepository
	auditRepo *repository.AuditRepository
}

func NewRiskService(repo *repository.RiskRepository, auditRepo *repository.AuditRepository) *RiskService {
	return &RiskService{repo: repo, auditRepo: auditRepo}
}

func (s *RiskService) audit(ctx context.Context, actor *uuid.UUID, action, resource string, id *uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID: actor, Action: action, ResourceType: resource, ResourceID: id,
		Description: &description, Success: true,
	})
}

// Scope resolves which station a request may see. Below DSP an officer sees
// only their own station; DSP and above may name any station or none.
func (s *RiskService) Scope(actor RiskActor, requested *uuid.UUID) (*uuid.UUID, error) {
	if actor.atLeast(models.RoleDSP) {
		return requested, nil
	}
	if actor.Station == nil {
		return nil, fmt.Errorf("%w: no station is recorded for your account", ErrRiskForbidden)
	}
	if requested != nil && *requested != *actor.Station {
		return nil, fmt.Errorf("%w: officers below DSP see only their own station", ErrRiskForbidden)
	}
	return actor.Station, nil
}

var riskShiftLabels = map[string]string{
	"all":   "All hours (FIRs without a recorded incident time are included)",
	"day":   "Day, 06:00–19:59 (FIRs without a recorded incident time are excluded)",
	"night": "Night, 20:00–05:59 (FIRs without a recorded incident time are excluded)",
}

// Window parses a period. Defaults to the 30 days ending today.
func (s *RiskService) Window(from, to, shift string) (repository.RiskWindow, models.RiskPeriod, error) {
	var w repository.RiskWindow
	if shift == "" {
		shift = "all"
	}
	if _, ok := riskShiftLabels[shift]; !ok {
		return w, models.RiskPeriod{}, invalid("shift must be all, day or night")
	}
	// A calendar date carried as midnight UTC. Truncate would cut at UTC
	// midnight and give the wrong day for part of every IST day.
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := today
	if to != "" {
		t, err := time.Parse("2006-01-02", to)
		if err != nil {
			return w, models.RiskPeriod{}, invalid("to must be a date in YYYY-MM-DD form")
		}
		end = t
	}
	start := end.AddDate(0, 0, -29)
	if from != "" {
		t, err := time.Parse("2006-01-02", from)
		if err != nil {
			return w, models.RiskPeriod{}, invalid("from must be a date in YYYY-MM-DD form")
		}
		start = t
	}
	if start.After(end) {
		return w, models.RiskPeriod{}, invalid("from must not be after to")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > 366 {
		return w, models.RiskPeriod{}, invalid("the period cannot exceed 366 days")
	}
	prevTo := start.AddDate(0, 0, -1)
	prevFrom := prevTo.AddDate(0, 0, -(days - 1))
	w = repository.RiskWindow{From: start, To: end, PreviousFrom: prevFrom, PreviousTo: prevTo, Shift: shift}
	p := models.RiskPeriod{
		From: start.Format("2006-01-02"), To: end.Format("2006-01-02"),
		PreviousFrom: prevFrom.Format("2006-01-02"), PreviousTo: prevTo.Format("2006-01-02"),
		Days: days, Shift: shift, ShiftLabel: riskShiftLabels[shift],
	}
	return w, p, nil
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func (s *RiskService) score(c repository.RiskRawCounts, level string, weights map[string]float64) models.RiskArea {
	increase := c.FIRs - c.PreviousFIRs
	if increase < 0 {
		increase = 0
	}
	raw := map[string]int64{
		"fir_count": c.FIRs, "serious_fir_count": c.Serious, "night_fir_count": c.Night,
		"alert_count": c.Alerts, "fir_increase": increase,
	}
	area := models.RiskArea{
		ID: c.AreaID, Level: level, Name: c.Name, StationID: c.StationID, StationName: c.StationName,
		Latitude: c.Latitude, Longitude: c.Longitude, RadiusMeters: c.RadiusMeters,
		FIRCount: c.FIRs, PreviousFIRCount: c.PreviousFIRs, Change: c.FIRs - c.PreviousFIRs,
		Factors: make([]models.RiskFactorScore, 0, len(RiskFactors)),
	}
	for _, f := range RiskFactors {
		attributable := level == "station" || f.BeatAttributable
		v := raw[f.Key]
		if !attributable {
			v = 0
		}
		contribution := round2(weights[f.Key] * float64(v))
		area.Factors = append(area.Factors, models.RiskFactorScore{
			Key: f.Key, Label: f.Label, Description: f.Description, Raw: v,
			Weight: weights[f.Key], Contribution: contribution, Attributable: attributable,
		})
		area.Score += contribution
	}
	area.Score = round2(area.Score)
	if level == "beat" {
		area.Coverage = &models.RiskPlacementCoverage{Placed: c.Placed, StationTotal: c.StationTotal}
	}
	return area
}

func (s *RiskService) Areas(ctx context.Context, actor RiskActor, level, from, to, shift string, station *uuid.UUID) (*models.RiskAreasResponse, error) {
	if level == "" {
		level = "station"
	}
	if level != "station" && level != "beat" {
		return nil, invalid("level must be station or beat")
	}
	scope, err := s.Scope(actor, station)
	if err != nil {
		return nil, err
	}
	w, period, err := s.Window(from, to, shift)
	if err != nil {
		return nil, err
	}
	weights, err := s.repo.CurrentWeights(ctx)
	if err != nil {
		return nil, err
	}
	var counts []repository.RiskRawCounts
	if level == "station" {
		counts, err = s.repo.StationCounts(ctx, w, scope)
	} else {
		counts, err = s.repo.BeatCounts(ctx, w, scope)
	}
	if err != nil {
		return nil, err
	}
	resp := &models.RiskAreasResponse{
		Level: level, Period: period, WeightSet: *weights, Formula: riskFormula,
		Areas: make([]models.RiskArea, 0, len(counts)),
	}
	for _, c := range counts {
		resp.Areas = append(resp.Areas, s.score(c, level, weights.Weights))
	}
	sort.SliceStable(resp.Areas, func(i, j int) bool { return lessArea(resp.Areas[i], resp.Areas[j]) })

	resp.Notes = []string{
		"Figures count stored FIRs and alerts only. They describe places and say nothing about any person.",
		"Previous period: " + period.PreviousFrom + " to " + period.PreviousTo + ", the same length immediately before.",
	}
	if level == "station" {
		resp.Notes = append(resp.Notes, "FIRs do not store incident coordinates, so station figures count every FIR registered at the station; the map shows the station's own location.")
	} else {
		resp.Notes = append(resp.Notes, "FIRs do not store incident coordinates. A beat counts only FIRs an officer has placed in it; each beat shows how many of its station's FIRs in the period are placed. Alerts are not attributed to beats.")
	}
	return resp, nil
}

// lessArea orders by score, then FIR count, then name.
func lessArea(a, b models.RiskArea) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.FIRCount != b.FIRCount {
		return a.FIRCount > b.FIRCount
	}
	return a.Name < b.Name
}

func (s *RiskService) Recommendations(ctx context.Context, actor RiskActor, level, from, to, shift string, station *uuid.UUID, top int) (*models.RiskRecommendationsResponse, error) {
	if top < 1 || top > 20 {
		return nil, invalid("top must be between 1 and 20")
	}
	areas, err := s.Areas(ctx, actor, level, from, to, shift, station)
	if err != nil {
		return nil, err
	}
	resp := &models.RiskRecommendationsResponse{
		Rule:   strings.Replace(riskRecommendationRule, "top N", fmt.Sprintf("top %d", top), 1),
		Period: areas.Period, WeightVersion: areas.WeightSet.Version,
		AreasConsidered: len(areas.Areas), Recommendations: []models.RiskRecommendation{},
	}
	for _, a := range areas.Areas {
		if len(resp.Recommendations) == top || a.Score <= 0 {
			break
		}
		topFactor := a.Factors[0]
		for _, f := range a.Factors[1:] {
			if f.Contribution > topFactor.Contribution {
				topFactor = f
			}
		}
		rank := len(resp.Recommendations) + 1
		resp.Recommendations = append(resp.Recommendations, models.RiskRecommendation{
			Rank: rank, AreaID: a.ID.String(), AreaName: a.Name, StationName: a.StationName,
			Score: a.Score, TopFactor: topFactor.Label, TopFactorValue: topFactor.Contribution,
			Reasoning: fmt.Sprintf("Ranked %d of %d areas with score %.2f. Largest contribution: %s, weight %g × %d = %.2f.",
				rank, len(areas.Areas), a.Score, topFactor.Label, topFactor.Weight, topFactor.Raw, topFactor.Contribution),
		})
	}
	return resp, nil
}

func (s *RiskService) Simulate(ctx context.Context, actor RiskActor, req models.RiskSimulationRequest) (*models.RiskSimulationResponse, error) {
	areas, err := s.Areas(ctx, actor, req.Level, req.From, req.To, req.Shift, req.StationID)
	if err != nil {
		return nil, err
	}
	units := map[string]int{}
	known := map[string]bool{}
	for _, a := range areas.Areas {
		known[a.ID.String()] = true
	}
	for _, al := range req.Allocations {
		if !known[al.AreaID] {
			return nil, invalid("area %s is not among the areas for this level and scope", al.AreaID)
		}
		if al.Units < 0 || al.Units > 100 {
			return nil, invalid("units per area must be between 0 and 100")
		}
		if _, dup := units[al.AreaID]; dup {
			return nil, invalid("area %s is allocated more than once", al.AreaID)
		}
		units[al.AreaID] = al.Units
	}
	resp := &models.RiskSimulationResponse{
		Method: riskSimulationMethod, Period: areas.Period, WeightVersion: areas.WeightSet.Version,
		Rows: make([]models.RiskSimulationRow, 0, len(areas.Areas)), UncoveredScoring: []string{},
	}
	for _, a := range areas.Areas {
		resp.TotalScore += a.Score
	}
	resp.TotalScore = round2(resp.TotalScore)
	for _, a := range areas.Areas {
		u := units[a.ID.String()]
		row := models.RiskSimulationRow{AreaID: a.ID.String(), AreaName: a.Name, Score: a.Score, Units: u, Covered: u > 0}
		if resp.TotalScore > 0 {
			row.Share = round2(a.Score / resp.TotalScore * 100)
		}
		resp.Rows = append(resp.Rows, row)
		resp.UnitsAllocated += u
		if a.Score > 0 {
			resp.AreasWithScore++
		}
		if row.Covered {
			resp.AreasCovered++
			resp.CoveredScore += a.Score
		} else if a.Score > 0 {
			resp.UncoveredScoring = append(resp.UncoveredScoring, a.Name)
		}
	}
	resp.CoveredScore = round2(resp.CoveredScore)
	if resp.TotalScore > 0 {
		resp.CoveragePercent = round2(resp.CoveredScore / resp.TotalScore * 100)
	}
	return resp, nil
}

func (s *RiskService) Factors(ctx context.Context) ([]models.RiskFactorDefinition, *models.RiskWeightSet, error) {
	ws, err := s.repo.CurrentWeights(ctx)
	return RiskFactors, ws, err
}

func (s *RiskService) WeightHistory(ctx context.Context) ([]models.RiskWeightSet, error) {
	return s.repo.WeightHistory(ctx)
}

// UpdateWeights records a new weight set. The caller's rank is enforced by the
// route; the reason and the old and new values go to the audit trail.
func (s *RiskService) UpdateWeights(ctx context.Context, actor RiskActor, req models.UpdateRiskWeightsRequest) (*models.RiskWeightSet, error) {
	reason := strings.TrimSpace(req.Reason)
	if len(reason) < 10 {
		return nil, invalid("state the reason for the change in at least 10 characters")
	}
	if len(req.Weights) != len(RiskFactors) {
		return nil, invalid("a weight is required for every factor, and only for those factors")
	}
	weights := make(map[string]float64, len(RiskFactors))
	for _, f := range RiskFactors {
		v, ok := req.Weights[f.Key]
		if !ok {
			return nil, invalid("missing weight for %s", f.Key)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return nil, invalid("weight for %s must be between 0 and 100", f.Key)
		}
		weights[f.Key] = round2(v)
	}
	current, err := s.repo.CurrentWeights(ctx)
	if err != nil {
		return nil, err
	}
	var changes []string
	for _, f := range RiskFactors {
		if current.Weights[f.Key] != weights[f.Key] {
			changes = append(changes, fmt.Sprintf("%s %g→%g", f.Key, current.Weights[f.Key], weights[f.Key]))
		}
	}
	if len(changes) == 0 {
		return nil, invalid("the weights are unchanged from version %d", current.Version)
	}
	version, err := s.repo.InsertWeights(ctx, weights, reason, actor.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "risk_weights_updated", "risk_weights", nil,
		fmt.Sprintf("Weight set v%d (from v%d): %s. Reason: %s", version, current.Version, strings.Join(changes, ", "), reason))
	return s.repo.CurrentWeights(ctx)
}

func (s *RiskService) Beats(ctx context.Context, actor RiskActor, station *uuid.UUID) ([]models.RiskBeat, error) {
	scope, err := s.Scope(actor, station)
	if err != nil {
		return nil, err
	}
	var viewer uuid.UUID
	if actor.ID != nil {
		viewer = *actor.ID
	}
	return s.repo.ListBeats(ctx, scope, viewer)
}

// Owner answers which department's ground a beat is drawn on.
func (s *RiskService) Owner(ctx context.Context, id, viewerID uuid.UUID) (bool, string, error) {
	return s.repo.Owner(ctx, id, viewerID)
}

func (s *RiskService) CreateBeat(ctx context.Context, actor RiskActor, req models.CreateRiskBeatRequest) (*models.RiskBeat, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		return nil, invalid("a beat name is required")
	}
	if req.Latitude == nil || req.Longitude == nil ||
		*req.Latitude < -90 || *req.Latitude > 90 || *req.Longitude < -180 || *req.Longitude > 180 {
		return nil, invalid("valid latitude and longitude are required")
	}
	if req.RadiusMeters < 50 || req.RadiusMeters > 10000 {
		return nil, invalid("radius must be between 50 and 10000 metres")
	}
	requested := req.StationID
	if requested == nil {
		requested = actor.Station
	}
	station, err := s.Scope(actor, requested)
	if err != nil {
		return nil, err
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	if ok, err := s.repo.StationExists(ctx, *station); err != nil {
		return nil, err
	} else if !ok {
		return nil, repository.ErrRiskStationNotFound
	}
	id, err := s.repo.CreateBeat(ctx, *station, req, actor.ID)
	if err != nil {
		return nil, err
	}
	beat, err := s.repo.GetBeat(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor.ID, "risk_beat_created", "risk_beat", &id,
		fmt.Sprintf("Beat %q at %s, centre %.5f,%.5f, radius %d m", beat.Name, beat.StationName, beat.Latitude, beat.Longitude, beat.RadiusMeters))
	return beat, nil
}

func (s *RiskService) DeleteBeat(ctx context.Context, actor RiskActor, id uuid.UUID) error {
	beat, err := s.repo.GetBeat(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.Scope(actor, &beat.StationID); err != nil {
		return err
	}
	if err := s.repo.DeleteBeat(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, actor.ID, "risk_beat_deleted", "risk_beat", &id, fmt.Sprintf("Beat %q at %s removed", beat.Name, beat.StationName))
	return nil
}

func (s *RiskService) PlaceableFIRs(ctx context.Context, actor RiskActor, station *uuid.UUID, from, to string, unplacedOnly bool) ([]models.RiskPlaceableFIR, error) {
	scope, err := s.Scope(actor, station)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, invalid("choose a station")
	}
	w, _, err := s.Window(from, to, "all")
	if err != nil {
		return nil, err
	}
	return s.repo.PlaceableFIRs(ctx, *scope, w.From, w.To, unplacedOnly)
}

func (s *RiskService) PlaceFIR(ctx context.Context, actor RiskActor, req models.PlaceRiskFIRRequest) error {
	req.Note = strings.TrimSpace(req.Note)
	firStation, firNumber, err := s.repo.FIRStation(ctx, req.FIRID)
	if err != nil {
		return err
	}
	if _, err := s.Scope(actor, &firStation); err != nil {
		return err
	}
	beat, err := s.repo.GetBeat(ctx, req.BeatID)
	if err != nil {
		return err
	}
	if beat.StationID != firStation {
		return repository.ErrRiskStationMismatch
	}
	if err := s.repo.PlaceFIR(ctx, req, actor.ID); err != nil {
		return err
	}
	s.audit(ctx, actor.ID, "risk_fir_placed", "risk_beat", &req.BeatID,
		fmt.Sprintf("FIR %s placed in beat %q", firNumber, beat.Name))
	return nil
}

func (s *RiskService) UnplaceFIR(ctx context.Context, actor RiskActor, firID uuid.UUID) error {
	firStation, firNumber, err := s.repo.FIRStation(ctx, firID)
	if err != nil {
		return err
	}
	if _, err := s.Scope(actor, &firStation); err != nil {
		return err
	}
	beatID, err := s.repo.UnplaceFIR(ctx, firID)
	if err != nil {
		return err
	}
	s.audit(ctx, actor.ID, "risk_fir_placement_removed", "risk_beat", &beatID, fmt.Sprintf("FIR %s removed from its beat", firNumber))
	return nil
}
