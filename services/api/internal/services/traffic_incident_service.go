package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// Timeline entries must fall within this window of the incident. Wider than
// any plausible pre-collision approach or post-collision response, narrow
// enough to catch a date typed into the wrong year.
const trafficTimelineWindow = 24 * time.Hour

type TrafficIncidentService struct {
	repo      *repository.TrafficIncidentRepository
	auditRepo *repository.AuditRepository
}

func NewTrafficIncidentService(repo *repository.TrafficIncidentRepository, auditRepo *repository.AuditRepository) *TrafficIncidentService {
	return &TrafficIncidentService{repo: repo, auditRepo: auditRepo}
}

func (s *TrafficIncidentService) audit(ctx context.Context, action string, actor uuid.UUID, incidentID uuid.UUID, description string) {
	s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       &actor,
		Action:       action,
		ResourceType: "traffic_incident",
		ResourceID:   &incidentID,
		Description:  &description,
		Success:      true,
	})
}

func trafficOneOf(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

func trafficTrim(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

/* -------------------------------- incidents -------------------------------- */

func (s *TrafficIncidentService) validateIncident(in *models.TrafficIncidentInput) error {
	in.Location = strings.TrimSpace(in.Location)
	in.Description = strings.TrimSpace(in.Description)
	if in.Location == "" || in.Description == "" {
		return invalid("location and description are required")
	}
	if in.Latitude == nil || in.Longitude == nil {
		return invalid("the incident location needs coordinates")
	}
	if *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
		return invalid("coordinates are out of range")
	}
	if in.OccurredAt.IsZero() {
		return invalid("the time of the incident is required")
	}
	if in.OccurredAt.After(time.Now().Add(5 * time.Minute)) {
		return invalid("an incident cannot be in the future")
	}
	if !trafficOneOf(in.CollisionType, "HEAD_ON", "REAR_END", "SIDE_IMPACT", "SIDESWIPE", "PEDESTRIAN", "ROLLOVER", "FIXED_OBJECT", "OTHER") {
		return invalid("unknown collision type %q", in.CollisionType)
	}
	if !trafficOneOf(in.RoadCondition, "DRY", "WET", "WATERLOGGED", "POTHOLED", "UNDER_REPAIR", "OTHER") {
		return invalid("unknown road condition %q", in.RoadCondition)
	}
	if !trafficOneOf(in.Weather, "CLEAR", "RAIN", "HEAVY_RAIN", "FOG", "HAZE", "OTHER") {
		return invalid("unknown weather %q", in.Weather)
	}
	if !trafficOneOf(in.Lighting, "DAYLIGHT", "DUSK_DAWN", "DARK_LIT", "DARK_UNLIT") {
		return invalid("unknown lighting %q", in.Lighting)
	}
	return nil
}

func (s *TrafficIncidentService) List(ctx context.Context, f repository.TrafficIncidentFilter) ([]models.TrafficIncident, int64, error) {
	return s.repo.List(ctx, f)
}

func (s *TrafficIncidentService) Get(ctx context.Context, id uuid.UUID) (*models.TrafficIncident, error) {
	return s.repo.Get(ctx, id)
}

func (s *TrafficIncidentService) Stats(ctx context.Context, stationID *uuid.UUID) (*models.TrafficIncidentStats, error) {
	return s.repo.Stats(ctx, stationID)
}

func (s *TrafficIncidentService) Register(ctx context.Context, in models.TrafficIncidentInput, actor uuid.UUID, actorStation *uuid.UUID) (*models.TrafficIncident, error) {
	if err := s.validateIncident(&in); err != nil {
		return nil, err
	}
	station := in.StationID
	if station == nil {
		station = actorStation
	}
	if station == nil {
		return nil, invalid("a station is required")
	}
	id, err := s.repo.Create(ctx, in, *station, actor)
	if err != nil {
		return nil, err
	}
	inc, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_registered", actor, id,
		fmt.Sprintf("Registered %s at %s (%s)", inc.IncidentNumber, inc.Location, inc.CollisionType))
	return inc, nil
}

func (s *TrafficIncidentService) Update(ctx context.Context, id uuid.UUID, in models.TrafficIncidentInput, actor uuid.UUID) (*models.TrafficIncident, error) {
	if err := s.validateIncident(&in); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, id, in); err != nil {
		return nil, err
	}
	inc, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_updated", actor, id, fmt.Sprintf("Updated %s", inc.IncidentNumber))
	return inc, nil
}

func (s *TrafficIncidentService) within(ctx context.Context, incidentID uuid.UUID, times ...time.Time) error {
	inc, err := s.repo.Get(ctx, incidentID)
	if err != nil {
		return err
	}
	for _, t := range times {
		if t.Before(inc.OccurredAt.Add(-trafficTimelineWindow)) || t.After(inc.OccurredAt.Add(trafficTimelineWindow)) {
			return invalid("times must fall within 24 hours of the incident (%s)", inc.OccurredAt.Format(time.RFC3339))
		}
	}
	return nil
}

/* ---------------------------------- children -------------------------------- */

func (s *TrafficIncidentService) Vehicles(ctx context.Context, id uuid.UUID) ([]models.TrafficIncidentVehicle, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Vehicles(ctx, id)
}

func (s *TrafficIncidentService) AddVehicle(ctx context.Context, id uuid.UUID, in models.TrafficVehicleInput, actor uuid.UUID) (*models.TrafficIncidentVehicle, error) {
	in.RegistrationNumber = repository.NormaliseRegistration(in.RegistrationNumber)
	if len(in.RegistrationNumber) < 4 || len(in.RegistrationNumber) > 20 {
		return nil, invalid("registration number must have 4 to 20 letters and digits")
	}
	if !trafficOneOf(in.VehicleType, "TWO_WHEELER", "THREE_WHEELER", "E_RICKSHAW", "CAR", "TAXI", "BUS", "LCV", "TRUCK", "BICYCLE", "OTHER") {
		return nil, invalid("unknown vehicle type %q", in.VehicleType)
	}
	in.Description = strings.TrimSpace(in.Description)
	in.DriverName = trafficTrim(in.DriverName)
	vid, err := s.repo.AddVehicle(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_vehicle_added", actor, id, fmt.Sprintf("Vehicle %s (%s) added", in.RegistrationNumber, in.VehicleType))
	list, err := s.repo.Vehicles(ctx, id)
	return trafficFindByID(list, err, vid, func(v models.TrafficIncidentVehicle) uuid.UUID { return v.ID })
}

func (s *TrafficIncidentService) Persons(ctx context.Context, id uuid.UUID) ([]models.TrafficIncidentPerson, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Persons(ctx, id)
}

func (s *TrafficIncidentService) AddPerson(ctx context.Context, id uuid.UUID, in models.TrafficPersonInput, actor uuid.UUID) (*models.TrafficIncidentPerson, error) {
	if !trafficOneOf(in.Role, "DRIVER", "PASSENGER", "PEDESTRIAN", "CYCLIST", "OTHER") {
		return nil, invalid("unknown role %q", in.Role)
	}
	if !trafficOneOf(in.InjurySeverity, "FATAL", "GRIEVOUS", "MINOR", "NONE") {
		return nil, invalid("unknown injury severity %q", in.InjurySeverity)
	}
	if (in.Role == "DRIVER" || in.Role == "PASSENGER") && in.VehicleID == nil {
		return nil, invalid("a driver or passenger must be linked to a vehicle on this incident")
	}
	in.Name = trafficTrim(in.Name)
	in.Hospital = trafficTrim(in.Hospital)
	pid, err := s.repo.AddPerson(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	who := "unidentified person"
	if in.Name != nil {
		who = *in.Name
	}
	s.audit(ctx, "traffic_incident_person_added", actor, id, fmt.Sprintf("%s (%s, %s) added", who, in.Role, in.InjurySeverity))
	list, err := s.repo.Persons(ctx, id)
	return trafficFindByID(list, err, pid, func(p models.TrafficIncidentPerson) uuid.UUID { return p.ID })
}

func (s *TrafficIncidentService) Cameras(ctx context.Context, id uuid.UUID) ([]models.TrafficIncidentCamera, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Cameras(ctx, id)
}

func (s *TrafficIncidentService) AddCamera(ctx context.Context, id uuid.UUID, in models.TrafficCameraInput, actor uuid.UUID) (*models.TrafficIncidentCamera, error) {
	in.CameraRef = strings.TrimSpace(in.CameraRef)
	in.CameraName = strings.TrimSpace(in.CameraName)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.CameraRef == "" {
		return nil, invalid("camera reference is required")
	}
	if in.FootageFrom.IsZero() || in.FootageTo.IsZero() {
		return nil, invalid("the footage start and end times are required")
	}
	if !in.FootageTo.After(in.FootageFrom) {
		return nil, invalid("footage must end after it starts")
	}
	if in.DistanceM != nil && *in.DistanceM < 0 {
		return nil, invalid("distance cannot be negative")
	}
	if err := s.within(ctx, id, in.FootageFrom, in.FootageTo); err != nil {
		return nil, err
	}
	cid, err := s.repo.AddCamera(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_camera_linked", actor, id,
		fmt.Sprintf("Camera %s footage %s–%s linked", in.CameraRef, in.FootageFrom.Format(time.RFC3339), in.FootageTo.Format(time.RFC3339)))
	list, err := s.repo.Cameras(ctx, id)
	return trafficFindByID(list, err, cid, func(c models.TrafficIncidentCamera) uuid.UUID { return c.ID })
}

func (s *TrafficIncidentService) PlateReads(ctx context.Context, id uuid.UUID) ([]models.TrafficPlateRead, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.PlateReads(ctx, id)
}

func (s *TrafficIncidentService) AddPlateRead(ctx context.Context, id uuid.UUID, in models.TrafficPlateReadInput, actor uuid.UUID) (*models.TrafficPlateRead, error) {
	in.RegistrationNumber = repository.NormaliseRegistration(in.RegistrationNumber)
	in.Location = strings.TrimSpace(in.Location)
	in.SourceDetail = strings.TrimSpace(in.SourceDetail)
	in.CameraRef = trafficTrim(in.CameraRef)
	if len(in.RegistrationNumber) < 4 || len(in.RegistrationNumber) > 20 {
		return nil, invalid("registration number must have 4 to 20 letters and digits")
	}
	if in.Location == "" {
		return nil, invalid("location of the read is required")
	}
	if in.ReadAt.IsZero() {
		return nil, invalid("the time of the read is required")
	}
	if !trafficOneOf(in.Source, "ANPR_SYSTEM", "CCTV_REVIEW", "OFFICER") {
		return nil, invalid("unknown plate read source %q", in.Source)
	}
	if in.Source != "OFFICER" && in.CameraRef == nil {
		return nil, invalid("a read from an ANPR system or reviewed footage must name its camera")
	}
	if err := s.within(ctx, id, in.ReadAt); err != nil {
		return nil, err
	}
	rid, err := s.repo.AddPlateRead(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_plate_read_recorded", actor, id,
		fmt.Sprintf("Plate read %s at %s (%s)", in.RegistrationNumber, in.Location, in.Source))
	list, err := s.repo.PlateReads(ctx, id)
	return trafficFindByID(list, err, rid, func(p models.TrafficPlateRead) uuid.UUID { return p.ID })
}

func (s *TrafficIncidentService) SignalPhases(ctx context.Context, id uuid.UUID) ([]models.TrafficSignalPhase, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.SignalPhases(ctx, id)
}

func (s *TrafficIncidentService) AddSignalPhase(ctx context.Context, id uuid.UUID, in models.TrafficSignalPhaseInput, actor uuid.UUID) (*models.TrafficSignalPhase, error) {
	in.SignalRef = strings.TrimSpace(in.SignalRef)
	in.Approach = strings.TrimSpace(in.Approach)
	in.SourceDetail = strings.TrimSpace(in.SourceDetail)
	if in.SignalRef == "" || in.Approach == "" {
		return nil, invalid("signal reference and approach are required")
	}
	if in.PhaseFrom.IsZero() {
		return nil, invalid("the time the phase began is required")
	}
	if !trafficOneOf(in.Phase, "RED", "AMBER", "GREEN", "FLASHING", "OFF") {
		return nil, invalid("unknown signal phase %q", in.Phase)
	}
	if !trafficOneOf(in.Source, "CONTROLLER_LOG", "CCTV_REVIEW", "OFFICER_OBSERVATION", "WITNESS") {
		return nil, invalid("unknown signal phase source %q", in.Source)
	}
	if in.SourceDetail == "" {
		return nil, invalid("state where the phase came from — the log file, footage or who observed it")
	}
	times := []time.Time{in.PhaseFrom}
	if in.PhaseTo != nil {
		if !in.PhaseTo.After(in.PhaseFrom) {
			return nil, invalid("the phase must end after it starts")
		}
		times = append(times, *in.PhaseTo)
	}
	if err := s.within(ctx, id, times...); err != nil {
		return nil, err
	}
	gid, err := s.repo.AddSignalPhase(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_signal_phase_recorded", actor, id,
		fmt.Sprintf("Signal %s %s %s from %s (%s)", in.SignalRef, in.Approach, in.Phase, in.PhaseFrom.Format(time.RFC3339), in.Source))
	list, err := s.repo.SignalPhases(ctx, id)
	return trafficFindByID(list, err, gid, func(g models.TrafficSignalPhase) uuid.UUID { return g.ID })
}

func (s *TrafficIncidentService) Facts(ctx context.Context, id uuid.UUID) ([]models.TrafficFact, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Facts(ctx, id)
}

func (s *TrafficIncidentService) AddFact(ctx context.Context, id uuid.UUID, in models.TrafficFactInput, actor uuid.UUID) (*models.TrafficFact, error) {
	in.Description = strings.TrimSpace(in.Description)
	in.Source = strings.TrimSpace(in.Source)
	in.Method = trafficTrim(in.Method)
	if in.Description == "" {
		return nil, invalid("describe the fact")
	}
	if in.OccurredAt.IsZero() {
		return nil, invalid("the time of the fact is required")
	}
	if !in.Provenance.Valid() {
		return nil, invalid("provenance must be MEASURED, OBSERVED or ESTIMATED")
	}
	if in.Source == "" {
		return nil, invalid("every fact needs its source")
	}
	if in.Provenance == models.ProvenanceEstimated && in.Method == nil {
		return nil, invalid("an estimate must state the method that produced it")
	}
	if in.Quantity == nil {
		if in.Value != nil || in.ValueLow != nil || in.ValueHigh != nil || in.Unit != nil {
			return nil, invalid("a value needs a quantity (speed, distance or duration)")
		}
	} else {
		units := map[string]string{"SPEED": "km/h", "DISTANCE": "m", "DURATION": "s"}
		want, ok := units[*in.Quantity]
		if !ok {
			return nil, invalid("unknown quantity %q", *in.Quantity)
		}
		in.Unit = &want
		point := in.Value != nil
		rng := in.ValueLow != nil || in.ValueHigh != nil
		switch {
		case point && rng:
			return nil, invalid("give either a value or a range, not both")
		case !point && !rng:
			return nil, invalid("a quantity needs a value or a range")
		case rng && (in.ValueLow == nil || in.ValueHigh == nil):
			return nil, invalid("a range needs both a low and a high value")
		case rng && *in.ValueHigh < *in.ValueLow:
			return nil, invalid("the high end of the range is below the low end")
		case rng && in.Provenance != models.ProvenanceEstimated:
			return nil, invalid("a range is an estimate — record it as ESTIMATED with its method")
		}
		for _, v := range []*float64{in.Value, in.ValueLow, in.ValueHigh} {
			if v != nil && *v < 0 {
				return nil, invalid("values cannot be negative")
			}
		}
	}
	if err := s.within(ctx, id, in.OccurredAt); err != nil {
		return nil, err
	}
	fid, err := s.repo.AddFact(ctx, id, in, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_fact_recorded", actor, id, fmt.Sprintf("%s fact recorded: %s (source: %s)", in.Provenance, in.Description, in.Source))
	list, err := s.repo.Facts(ctx, id)
	return trafficFindByID(list, err, fid, func(x models.TrafficFact) uuid.UUID { return x.ID })
}

func (s *TrafficIncidentService) RemoveChild(ctx context.Context, kind string, id, childID, actor uuid.UUID) error {
	if err := s.repo.DeleteChild(ctx, kind, id, childID); err != nil {
		return err
	}
	s.audit(ctx, "traffic_incident_record_removed", actor, id, fmt.Sprintf("Removed %s record %s", kind, childID))
	return nil
}

func (s *TrafficIncidentService) PriorChallans(ctx context.Context, id uuid.UUID) ([]models.PriorChallan, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.PriorChallans(ctx, id)
}

// trafficFindByID picks the just-written row out of a freshly read list.
func trafficFindByID[T any](list []T, err error, id uuid.UUID, key func(T) uuid.UUID) (*T, error) {
	if err != nil {
		return nil, err
	}
	for i := range list {
		if key(list[i]) == id {
			return &list[i], nil
		}
	}
	return nil, repository.ErrTrafficRecordNotFound
}

/* --------------------------------- timeline -------------------------------- */

// Timeline merges facts, plate reads, signal phases and camera windows in time
// order. Each item states its provenance: a fact carries its own; a plate read
// from an ANPR system and a phase from a controller log are MEASURED; reads
// and phases taken from reviewed footage, an officer or a witness are OBSERVED.
// Nothing here is ESTIMATED unless an officer recorded it as an estimate.
func (s *TrafficIncidentService) Timeline(ctx context.Context, id uuid.UUID) ([]models.TrafficTimelineItem, error) {
	content, err := s.assemble(ctx, id)
	if err != nil {
		return nil, err
	}
	return content.Timeline, nil
}

func (s *TrafficIncidentService) assemble(ctx context.Context, id uuid.UUID) (*models.TrafficReportContent, error) {
	inc, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	c := models.TrafficReportContent{Incident: *inc}
	if c.Vehicles, err = s.repo.Vehicles(ctx, id); err != nil {
		return nil, err
	}
	if c.Persons, err = s.repo.Persons(ctx, id); err != nil {
		return nil, err
	}
	if c.Cameras, err = s.repo.Cameras(ctx, id); err != nil {
		return nil, err
	}
	if c.PlateReads, err = s.repo.PlateReads(ctx, id); err != nil {
		return nil, err
	}
	if c.SignalPhases, err = s.repo.SignalPhases(ctx, id); err != nil {
		return nil, err
	}
	facts, err := s.repo.Facts(ctx, id)
	if err != nil {
		return nil, err
	}

	items := []models.TrafficTimelineItem{}
	for i := range facts {
		f := facts[i]
		switch f.Provenance {
		case models.ProvenanceMeasured:
			c.MeasuredFacts++
		case models.ProvenanceObserved:
			c.ObservedFacts++
		case models.ProvenanceEstimated:
			c.EstimatedFacts++
		}
		items = append(items, models.TrafficTimelineItem{
			Kind: "FACT", RecordID: f.ID, At: f.OccurredAt, Summary: f.Description,
			Provenance: f.Provenance, Source: f.Source, Method: f.Method, Fact: &f,
		})
	}
	for _, p := range c.PlateReads {
		prov := models.ProvenanceObserved
		if p.Source == "ANPR_SYSTEM" {
			prov = models.ProvenanceMeasured
		}
		src := p.Source
		if p.CameraRef != nil {
			src += " · " + *p.CameraRef
		}
		if p.SourceDetail != "" {
			src += " · " + p.SourceDetail
		}
		items = append(items, models.TrafficTimelineItem{
			Kind: "PLATE_READ", RecordID: p.ID, At: p.ReadAt,
			Summary:    fmt.Sprintf("Plate %s read at %s", p.RegistrationNumber, p.Location),
			Provenance: prov, Source: src,
		})
	}
	for _, g := range c.SignalPhases {
		prov := models.ProvenanceObserved
		if g.Source == "CONTROLLER_LOG" {
			prov = models.ProvenanceMeasured
		}
		items = append(items, models.TrafficTimelineItem{
			Kind: "SIGNAL_PHASE", RecordID: g.ID, At: g.PhaseFrom,
			Summary:    fmt.Sprintf("Signal %s, %s approach: %s", g.SignalRef, g.Approach, g.Phase),
			Provenance: prov, Source: g.Source + " · " + g.SourceDetail,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	c.Timeline = items
	return &c, nil
}

// digest hashes the content with the fields that change merely because a
// report exists or moved state blanked, so only changes to the record itself
// register as drift.
func trafficDigest(c models.TrafficReportContent) (string, []byte, error) {
	c.Incident.ReportStatus = ""
	c.Incident.UpdatedAt = time.Time{}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

// Workspace assembles the incident, its attached records, timeline, prior
// challans and reports in one read.
func (s *TrafficIncidentService) Workspace(ctx context.Context, id uuid.UUID) (*models.TrafficIncidentWorkspace, error) {
	content, err := s.assemble(ctx, id)
	if err != nil {
		return nil, err
	}
	w := models.TrafficIncidentWorkspace{TrafficReportContent: *content}
	if w.PriorChallans, err = s.repo.PriorChallans(ctx, id); err != nil {
		return nil, err
	}
	if w.Reports, err = s.repo.Reports(ctx, id); err != nil {
		return nil, err
	}
	// The live digest is computed once and compared with every report's snapshot.
	current, _, err := trafficDigest(*content)
	if err != nil {
		return nil, err
	}
	for i := range w.Reports {
		if w.Reports[i].SnapshotSHA256 != nil {
			w.Reports[i].RecordChangedSinceSnapshot = current != *w.Reports[i].SnapshotSHA256
		}
	}
	return &w, nil
}

/* --------------------------------- reports --------------------------------- */

// Draft returns the report content as it would be assembled now.
func (s *TrafficIncidentService) Draft(ctx context.Context, id uuid.UUID) (*models.TrafficReportContent, error) {
	return s.assemble(ctx, id)
}

func (s *TrafficIncidentService) withDrift(ctx context.Context, rep *models.TrafficIncidentReport) (*models.TrafficIncidentReport, error) {
	if rep.SnapshotSHA256 == nil {
		return rep, nil
	}
	content, err := s.assemble(ctx, rep.IncidentID)
	if err != nil {
		return nil, err
	}
	current, _, err := trafficDigest(*content)
	if err != nil {
		return nil, err
	}
	rep.RecordChangedSinceSnapshot = current != *rep.SnapshotSHA256
	return rep, nil
}

func (s *TrafficIncidentService) Reports(ctx context.Context, id uuid.UUID) ([]models.TrafficIncidentReport, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	reports, err := s.repo.Reports(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range reports {
		if _, err := s.withDrift(ctx, &reports[i]); err != nil {
			return nil, err
		}
	}
	return reports, nil
}

func (s *TrafficIncidentService) Report(ctx context.Context, id, reportID uuid.UUID) (*models.TrafficIncidentReport, error) {
	rep, err := s.repo.Report(ctx, id, reportID)
	if err != nil {
		return nil, err
	}
	return s.withDrift(ctx, rep)
}

func (s *TrafficIncidentService) CreateReport(ctx context.Context, id uuid.UUID, in models.TrafficReportInput, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
	rid, err := s.repo.CreateReport(ctx, id, strings.TrimSpace(in.Findings), actor)
	if err != nil {
		return nil, err
	}
	rep, err := s.repo.Report(ctx, id, rid)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_report_drafted", actor, id, fmt.Sprintf("Report %s drafted", rep.ReportNumber))
	return rep, nil
}

func (s *TrafficIncidentService) UpdateReport(ctx context.Context, id, reportID uuid.UUID, in models.TrafficReportInput, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
	if err := s.repo.UpdateFindings(ctx, id, reportID, strings.TrimSpace(in.Findings)); err != nil {
		return nil, err
	}
	rep, err := s.repo.Report(ctx, id, reportID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_report_updated", actor, id, fmt.Sprintf("Report %s findings edited", rep.ReportNumber))
	return rep, nil
}

// Submit freezes the content assembled from the stored facts, with its digest.
func (s *TrafficIncidentService) Submit(ctx context.Context, id, reportID, actor uuid.UUID) (*models.TrafficIncidentReport, error) {
	rep, err := s.repo.Report(ctx, id, reportID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rep.Findings) == "" {
		return nil, invalid("write the officer's findings before submitting")
	}
	content, err := s.assemble(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(content.Timeline) == 0 {
		return nil, invalid("a report needs at least one timeline entry")
	}
	sum, raw, err := trafficDigest(*content)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Submit(ctx, id, reportID, raw, sum); err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_report_submitted", actor, id,
		fmt.Sprintf("Report %s submitted for approval (%d measured, %d observed, %d estimated facts; sha256 %s)",
			rep.ReportNumber, content.MeasuredFacts, content.ObservedFacts, content.EstimatedFacts, sum))
	return s.Report(ctx, id, reportID)
}

func (s *TrafficIncidentService) Approve(ctx context.Context, id, reportID, reviewer uuid.UUID) (*models.TrafficIncidentReport, error) {
	rep, err := s.repo.Report(ctx, id, reportID)
	if err != nil {
		return nil, err
	}
	if rep.DraftedBy == reviewer {
		return nil, repository.ErrTrafficSelfReview
	}
	if err := s.repo.Approve(ctx, id, reportID, reviewer); err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_report_approved", reviewer, id, fmt.Sprintf("Report %s approved", rep.ReportNumber))
	return s.Report(ctx, id, reportID)
}

func (s *TrafficIncidentService) Return(ctx context.Context, id, reportID, reviewer uuid.UUID, reason string) (*models.TrafficIncidentReport, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("state why the report is returned")
	}
	rep, err := s.repo.Report(ctx, id, reportID)
	if err != nil {
		return nil, err
	}
	if rep.DraftedBy == reviewer {
		return nil, repository.ErrTrafficSelfReview
	}
	if err := s.repo.Return(ctx, id, reportID, reviewer, reason); err != nil {
		return nil, err
	}
	s.audit(ctx, "traffic_incident_report_returned", reviewer, id, fmt.Sprintf("Report %s returned: %s", rep.ReportNumber, reason))
	return s.Report(ctx, id, reportID)
}
