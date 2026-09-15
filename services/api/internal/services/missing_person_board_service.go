package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/photos"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/storage"
)

// Photographs, the city-wide board, station checks and the search map.
//
// Broadcast rule. While a report is open its purpose is to have the person
// found, so the broadcast view — the primary photograph, name, age, sex,
// physical description, last-seen place and time, and the reporting station —
// is open to every officer, including for a child. Everything else keeps the
// Phase 04 rule: a child's full record (informant, family contacts,
// circumstances, other photographs, sightings, the map) is for SI and above and
// the officers working the report. Once the report is closed the broadcast
// ends and the primary photograph falls back under the same rule.

var (
	// ErrPhotoTooLarge is answered with 413.
	ErrPhotoTooLarge = fmt.Errorf("the photograph is larger than %d MB", photos.MaxBytes>>20)
	// ErrNoStation: a station check is recorded for a station.
	ErrNoStation = errors.New("your account is not attached to a police station, so a station check cannot be recorded")
)

const thumbnailSize = 160

var validPhotoSources = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range models.PhotoSources {
		m[s] = true
	}
	return m
}()

// isOpen reports whether the search is still running.
func isOpen(p *models.MissingPerson) bool {
	return p.Status == models.MissingReported || p.Status == models.MissingSearching
}

/* ------------------------------------------------------------------ photos */

func (s *MissingPersonService) Photos(ctx context.Context, id uuid.UUID, includeRetired bool, v Viewer) ([]models.MissingPersonPhoto, error) {
	if _, err := s.authorised(ctx, id, v); err != nil {
		return nil, err
	}
	return s.repo.Photos(ctx, id, includeRetired)
}

// UploadPhoto checks, cleans and stores a photograph and records who gave it.
func (s *MissingPersonService) UploadPhoto(ctx context.Context, id uuid.UUID, body io.Reader, up models.PhotoUpload, v Viewer) (*models.MissingPersonPhoto, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	if !isOpen(p) {
		return nil, repository.ErrMissingPersonNotOpen
	}
	up.Source = strings.ToUpper(strings.TrimSpace(up.Source))
	up.ProvidedByName = strings.TrimSpace(up.ProvidedByName)
	up.Relationship = strings.TrimSpace(up.Relationship)
	up.ConsentNote, up.QualityNote = trimPtr(up.ConsentNote), trimPtr(up.QualityNote)
	switch {
	case !validPhotoSources[up.Source]:
		return nil, invalid("source must be one of %s", strings.Join(models.PhotoSources, ", "))
	case up.ProvidedByName == "":
		return nil, invalid("record who provided the photograph")
	case up.Relationship == "":
		return nil, invalid("record how the person who provided the photograph is related to the missing person")
	case up.TakenOn != nil && up.TakenOn.After(time.Now().In(istLocation())):
		return nil, invalid("the date the photograph was taken cannot be in the future")
	}

	raw, err := io.ReadAll(io.LimitReader(body, photos.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > photos.MaxBytes {
		return nil, ErrPhotoTooLarge
	}
	if len(raw) == 0 {
		return nil, invalid("the photograph file is empty")
	}
	prep, err := photos.Prepare(raw)
	if err != nil {
		switch {
		case errors.Is(err, photos.ErrUnsupportedType):
			return nil, invalid("only JPEG, PNG and WebP photographs are accepted; the file's contents are not one of these")
		case errors.Is(err, photos.ErrTooManyPixels):
			return nil, invalid("the photograph's dimensions are too large")
		default:
			return nil, invalid("the file could not be read as a photograph")
		}
	}

	photoID := uuid.New()
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[prep.ContentType]
	key := fmt.Sprintf("missing-persons/%s/%s%s", p.ID, photoID, ext)
	obj, err := s.store.Put(ctx, key, bytes.NewReader(prep.Data), prep.ContentType)
	if err != nil {
		s.auditRepo.Log(ctx, &models.SimpleAuditLog{UserID: &v.ID, Action: "missing_person_photo_upload_failed",
			ResourceType: "missing_person", ResourceID: &id, Description: strPtr("Photograph could not be stored: " + err.Error()), Success: false})
		return nil, err
	}
	var thumbKey *string
	if thumb, err := photos.Thumbnail(prep.Data, thumbnailSize); err != nil {
		log.Printf("missing person photo %s: thumbnail not drawn: %v", photoID, err)
	} else {
		k := fmt.Sprintf("missing-persons/%s/%s-thumb.jpg", p.ID, photoID)
		if _, err := s.store.Put(ctx, k, bytes.NewReader(thumb), "image/jpeg"); err != nil {
			log.Printf("missing person photo %s: thumbnail not stored: %v", photoID, err)
		} else {
			thumbKey = &k
		}
	}

	primary, err := s.repo.InsertPhoto(ctx, id, repository.NewPhoto{
		ID: photoID, StorageKey: key, StorageBackend: s.store.Backend(), SHA256: obj.SHA256, SizeBytes: obj.Size,
		ContentType: prep.ContentType, Width: prep.Width, Height: prep.Height, ThumbnailKey: thumbKey,
		LocationMetadataRemoved: prep.LocationRemoved, MetadataNote: prep.Note, Upload: up,
	}, v.ID)
	if err != nil {
		// No record points at the bytes; do not leave them behind.
		_ = s.store.Delete(ctx, key)
		if thumbKey != nil {
			_ = s.store.Delete(ctx, *thumbKey)
		}
		return nil, err
	}
	consent := "consent not recorded"
	if up.ConsentRecorded {
		consent = "consent recorded"
	}
	desc := fmt.Sprintf("%s: photograph %s added (%s, %dx%d, sha256 %s) provided by %s (%s), source %s, %s; %s",
		p.ReportNumber, photoID, prep.ContentType, prep.Width, prep.Height, obj.SHA256, up.ProvidedByName, up.Relationship, up.Source, consent, prep.Note)
	if primary {
		desc += "; set as primary"
	}
	s.audit(ctx, "missing_person_photo_uploaded", v.ID, id, desc)
	return s.repo.Photo(ctx, id, photoID)
}

// PhotoFile opens a photograph or its thumbnail for a viewer. The active
// primary photograph of an open report is broadcast to every officer; any other
// photograph follows the child-record rule. Every view of a child's photograph
// is audited, and so is every refused attempt.
func (s *MissingPersonService) PhotoFile(ctx context.Context, id, photoID uuid.UUID, thumbnail bool, v Viewer) (io.ReadCloser, *models.MissingPersonPhoto, string, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, "", err
	}
	photo, err := s.repo.Photo(ctx, id, photoID)
	if err != nil {
		return nil, nil, "", err
	}
	broadcast := isOpen(p) && photo.IsPrimary && photo.RetiredAt == nil
	variant := "photograph"
	if thumbnail {
		variant = "thumbnail"
	}
	if p.IsChild() && !broadcast && !canSeeChild(p, v) {
		s.auditRepo.Log(ctx, &models.SimpleAuditLog{UserID: &v.ID, Action: "missing_person_child_photo_view_denied",
			ResourceType: "missing_person", ResourceID: &id, Success: false,
			Description: strPtr(fmt.Sprintf("%s: %s %s refused", p.ReportNumber, variant, photoID))})
		return nil, nil, "", ErrChildRecordRestricted
	}

	key, contentType := photo.StorageKey, photo.ContentType
	if thumbnail {
		contentType = "image/jpeg"
		if photo.ThumbnailKey != nil {
			key = *photo.ThumbnailKey
		} else if key, err = s.drawThumbnail(ctx, photo); err != nil {
			return nil, nil, "", err
		}
	}
	body, _, err := s.store.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) && thumbnail {
		// The thumbnail object was lost; draw it again from the original.
		if key, err = s.drawThumbnail(ctx, photo); err == nil {
			body, _, err = s.store.Get(ctx, key)
		}
	}
	if err != nil {
		return nil, nil, "", err
	}
	if p.IsChild() {
		how := "under the child-record rule"
		if broadcast {
			how = "as the broadcast photograph of an open report"
		}
		s.audit(ctx, "missing_person_child_photo_viewed", v.ID, id,
			fmt.Sprintf("%s: %s %s viewed %s", p.ReportNumber, variant, photoID, how))
	}
	return body, photo, contentType, nil
}

func (s *MissingPersonService) drawThumbnail(ctx context.Context, photo *models.MissingPersonPhoto) (string, error) {
	body, _, err := s.store.Get(ctx, photo.StorageKey)
	if err != nil {
		return "", err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, photos.MaxBytes+1))
	if err != nil {
		return "", err
	}
	thumb, err := photos.Thumbnail(raw, thumbnailSize)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("missing-persons/%s/%s-thumb.jpg", photo.ReportID, photo.ID)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(thumb), "image/jpeg"); err != nil {
		return "", err
	}
	if err := s.repo.SetPhotoThumbnail(ctx, photo.ID, key); err != nil {
		return "", err
	}
	return key, nil
}

func (s *MissingPersonService) SetPrimaryPhoto(ctx context.Context, id, photoID uuid.UUID, v Viewer) (*models.MissingPersonPhoto, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetPrimaryPhoto(ctx, id, photoID); err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_photo_primary_set", v.ID, id, fmt.Sprintf("%s: photograph %s set as primary", p.ReportNumber, photoID))
	return s.repo.Photo(ctx, id, photoID)
}

func (s *MissingPersonService) RetirePhoto(ctx context.Context, id, photoID uuid.UUID, reason string, v Viewer) (*models.MissingPersonPhoto, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, invalid("record why the photograph is being retired")
	}
	promoted, err := s.repo.RetirePhoto(ctx, id, photoID, v.ID, reason)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("%s: photograph %s retired: %s", p.ReportNumber, photoID, reason)
	if promoted != nil {
		desc += fmt.Sprintf("; photograph %s became primary", *promoted)
	}
	s.audit(ctx, "missing_person_photo_retired", v.ID, id, desc)
	return s.repo.Photo(ctx, id, photoID)
}

/* ------------------------------------------------------------------- board */

func (s *MissingPersonService) Board(ctx context.Context, v Viewer) (*models.Board, error) {
	entries, stations, err := s.repo.Board(ctx, 300)
	if err != nil {
		return nil, err
	}
	return &models.Board{Data: entries, Stations: stations, ServerTime: time.Now(), ViewerStationID: v.Station}, nil
}

// raiseBroadcast issues the alert that tells every station a report is open.
// A child or trafficking risk goes state-wide (rail and road links leave the
// city quickly); anything else goes to the Kolkata Police district. A failure
// is logged and audited and never undoes the registration.
func (s *MissingPersonService) raiseBroadcast(ctx context.Context, p *models.MissingPerson, v Viewer) {
	scope, priority := models.AlertScopeDistrict, 2
	if p.Priority == "CRITICAL" {
		scope, priority = models.AlertScopeState, 1
	}
	title := fmt.Sprintf("Missing: %s, %d, %s — %s", p.PersonName, p.Age, strings.ToLower(p.Gender), p.ReportNumber)
	parts := []string{fmt.Sprintf("Last seen at %s on %s.", p.LastSeenLocation, p.LastSeenAt.In(istLocation()).Format("02 Jan 2006 15:04"))}
	if p.StationName != "" {
		parts = append(parts, "Reported at "+p.StationName+".")
	}
	for label, value := range map[string]*string{"Wearing": p.LastSeenWearing, "Marks": p.IdentifyingMarks} {
		if value != nil && strings.TrimSpace(*value) != "" {
			parts = append(parts, fmt.Sprintf("%s: %s.", label, strings.TrimSpace(*value)))
		}
	}
	parts = append(parts, "Every station: check and record the result on the city-wide missing-person board.")
	alertID, created, err := s.repo.RaiseBroadcastAlert(ctx, p.ID, scope, priority, title, strings.Join(parts, " "), v.ID, p.StationID)
	if err != nil {
		log.Printf("missing person broadcast alert for %s failed: %v", p.ReportNumber, err)
		s.auditRepo.Log(ctx, &models.SimpleAuditLog{UserID: &v.ID, Action: "missing_person_broadcast_alert_failed",
			ResourceType: "missing_person", ResourceID: &p.ID, Success: false,
			Description: strPtr("Broadcast alert could not be raised for " + p.ReportNumber)})
		return
	}
	if created {
		s.audit(ctx, "missing_person_broadcast_alert_raised", v.ID, p.ID,
			fmt.Sprintf("%s: %s-scope alert %s raised to all stations", p.ReportNumber, scope, alertID))
	}
}

// StationChecks lists every check recorded for a report. Which stations have
// looked is part of the broadcast, so any officer may read it.
func (s *MissingPersonService) StationChecks(ctx context.Context, id uuid.UUID) ([]models.StationCheck, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.StationChecks(ctx, id)
}

// RecordStationCheck records the viewer's station's check of an open report.
// It is open to any officer of ASI rank and above at any station — the
// broadcast asks every station to look — including for a child's report. A
// SIGHTING creates an ordinary sighting that another officer must verify.
func (s *MissingPersonService) RecordStationCheck(ctx context.Context, id uuid.UUID, req models.RecordStationCheckRequest, v Viewer) (*models.StationCheck, error) {
	if v.Station == nil {
		return nil, ErrNoStation
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isOpen(p) {
		return nil, repository.ErrMissingPersonNotOpen
	}
	outcome := strings.ToUpper(strings.TrimSpace(req.Outcome))
	details := trimPtr(&req.Details)
	if details != nil && *details == "" {
		details = nil
	}
	var sighting *models.RecordMissingSightingRequest
	switch outcome {
	case models.CheckNoMatch:
	case models.CheckPossibleMatch:
		if details == nil {
			return nil, invalid("describe the possible match: where, when and what was seen")
		}
	case models.CheckSighting:
		if req.Sighting == nil {
			return nil, invalid("a sighting check needs the sighting: place, time and source")
		}
		sighting = req.Sighting
		if err := validateSighting(sighting, p); err != nil {
			return nil, err
		}
	default:
		return nil, invalid("outcome must be NO_MATCH, POSSIBLE_MATCH or SIGHTING")
	}
	check, err := s.repo.RecordStationCheck(ctx, id, *v.Station, v.ID, outcome, details, sighting)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "missing_person_station_check_recorded", v.ID, id,
		fmt.Sprintf("%s: %s recorded %s", p.ReportNumber, check.StationName, outcome))
	if check.SightingID != nil {
		s.audit(ctx, "missing_person_sighting_recorded", v.ID, id,
			fmt.Sprintf("%s: sighting at %s (%s) from %s's station check", p.ReportNumber, sighting.Location, sighting.Source, check.StationName))
	}
	return check, nil
}

/* --------------------------------------------------------------------- map */

// SearchMap assembles every stored point for a report: last seen, sightings,
// camera matches, cameras for context, and the time-ordered path. Nothing is
// placed without stored coordinates.
func (s *MissingPersonService) SearchMap(ctx context.Context, id uuid.UUID, v Viewer) (*models.SearchMap, error) {
	p, err := s.authorised(ctx, id, v)
	if err != nil {
		return nil, err
	}
	sightings, err := s.repo.Sightings(ctx, id)
	if err != nil {
		return nil, err
	}
	cameras, err := s.repo.MapCameras(ctx)
	if err != nil {
		return nil, err
	}
	matches, err := s.repo.CameraMatches(ctx, id)
	if err != nil {
		// The face recognition layer's table is owned elsewhere; a mismatch
		// there must not take the rest of the map down.
		log.Printf("missing person map %s: camera matches unavailable: %v", p.ReportNumber, err)
		matches = models.CameraMatchLayer{Available: true, Error: "Camera matches could not be read", Data: []models.CameraMatch{}}
	}

	m := &models.SearchMap{
		LastSeen:      models.MapLastSeen{Location: p.LastSeenLocation, Latitude: p.LastSeenLatitude, Longitude: p.LastSeenLongitude, At: p.LastSeenAt},
		Sightings:     sightings,
		CameraMatches: matches,
		Cameras:       cameras,
		Path:          []models.PathPoint{},
	}
	if p.LastSeenLatitude != nil {
		m.Path = append(m.Path, models.PathPoint{Kind: "LAST_SEEN", Label: p.LastSeenLocation,
			Latitude: *p.LastSeenLatitude, Longitude: *p.LastSeenLongitude, At: p.LastSeenAt})
	}
	fromSighting := map[uuid.UUID]bool{}
	for _, x := range sightings {
		if x.Decision != nil && *x.Decision == "VERIFIED" && x.Latitude != nil && x.Longitude != nil {
			sid := x.ID
			fromSighting[sid] = true
			m.Path = append(m.Path, models.PathPoint{Kind: "VERIFIED_SIGHTING", RefID: &sid, Label: x.Location,
				Latitude: *x.Latitude, Longitude: *x.Longitude, At: x.SightedAt})
		}
	}
	for _, c := range matches.Data {
		// A confirmed match that became a sighting is already on the path.
		if c.Status != "CONFIRMED" || c.Latitude == nil || c.FrameTime == nil || (c.SightingID != nil && fromSighting[*c.SightingID]) {
			continue
		}
		cid := c.ID
		label := c.CameraName
		if label == "" {
			label = c.SourceMedia
		}
		m.Path = append(m.Path, models.PathPoint{Kind: "CONFIRMED_MATCH", RefID: &cid, Label: label,
			Latitude: *c.Latitude, Longitude: *c.Longitude, At: *c.FrameTime})
	}
	sort.SliceStable(m.Path, func(i, j int) bool { return m.Path[i].At.Before(m.Path[j].At) })
	if p.IsChild() {
		s.audit(ctx, "missing_person_child_map_viewed", v.ID, id, "Viewed search map of child record "+p.ReportNumber)
	}
	return m, nil
}
