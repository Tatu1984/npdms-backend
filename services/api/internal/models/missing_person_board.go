package models

import (
	"time"

	"github.com/google/uuid"
)

// Photographs, the city-wide board, station checks and the search map for
// Phase 04. See migrations 000070 and 000071.

var PhotoSources = []string{"FAMILY", "FRIEND", "REPORTING_PERSON", "OFFICER", "CCTV_STILL", "OTHER"}

type MissingPersonPhoto struct {
	ID                      uuid.UUID  `json:"id"`
	ReportID                uuid.UUID  `json:"reportId"`
	SHA256                  string     `json:"sha256"`
	SizeBytes               int64      `json:"sizeBytes"`
	ContentType             string     `json:"contentType"`
	Width                   int        `json:"width"`
	Height                  int        `json:"height"`
	StorageBackend          string     `json:"storageBackend"`
	LocationMetadataRemoved bool       `json:"locationMetadataRemoved"`
	MetadataNote            string     `json:"metadataNote"`
	Source                  string     `json:"source"`
	ProvidedByName          string     `json:"providedByName"`
	Relationship            string     `json:"relationship"`
	ConsentRecorded         bool       `json:"consentRecorded"`
	ConsentNote             *string    `json:"consentNote"`
	TakenOn                 *string    `json:"takenOn"`
	IsPrimary               bool       `json:"isPrimary"`
	QualityNote             *string    `json:"qualityNote"`
	UploadedBy              uuid.UUID  `json:"uploadedBy"`
	UploadedByName          string     `json:"uploadedByName"`
	CreatedAt               time.Time  `json:"createdAt"`
	RetiredAt               *time.Time `json:"retiredAt"`
	RetiredByName           string     `json:"retiredByName"`
	RetireReason            *string    `json:"retireReason"`
	// Not serialised: where the bytes are.
	StorageKey   string  `json:"-"`
	ThumbnailKey *string `json:"-"`
}

// PhotoUpload is a checked photograph with who provided it.
type PhotoUpload struct {
	Source          string
	ProvidedByName  string
	Relationship    string
	ConsentRecorded bool
	ConsentNote     *string
	TakenOn         *time.Time
	QualityNote     *string
	MakePrimary     bool
	Filename        string
}

type RetirePhotoRequest struct {
	Reason string `json:"reason"`
}

// Station check outcomes.
const (
	CheckNoMatch       = "NO_MATCH"
	CheckPossibleMatch = "POSSIBLE_MATCH"
	CheckSighting      = "SIGHTING"
)

type StationCheck struct {
	ID               uuid.UUID  `json:"id"`
	ReportID         uuid.UUID  `json:"reportId"`
	StationID        uuid.UUID  `json:"stationId"`
	StationName      string     `json:"stationName"`
	StationCode      string     `json:"stationCode"`
	Outcome          string     `json:"outcome"`
	Details          *string    `json:"details"`
	SightingID       *uuid.UUID `json:"sightingId"`
	SightingDecision *string    `json:"sightingDecision"`
	RecordedBy       uuid.UUID  `json:"recordedBy"`
	RecordedByName   string     `json:"recordedByName"`
	RecordedByRank   string     `json:"recordedByRank"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type RecordStationCheckRequest struct {
	Outcome  string                        `json:"outcome"`
	Details  string                        `json:"details"`
	Sighting *RecordMissingSightingRequest `json:"sighting"`
}

type BoardStation struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// BoardEntry is the broadcast view of an open report: what every officer may
// see so that any station can look for the person. Family contacts, the
// informant, circumstances and notes are not part of it.
type BoardEntry struct {
	ID                uuid.UUID           `json:"id"`
	ReportNumber      string              `json:"reportNumber"`
	Status            MissingPersonStatus `json:"status"`
	Priority          string              `json:"priority"`
	Vulnerabilities   []string            `json:"vulnerabilities"`
	PersonName        string              `json:"personName"`
	Age               int                 `json:"age"`
	Gender            string              `json:"gender"`
	Height            *string             `json:"height"`
	Complexion        *string             `json:"complexion"`
	IdentifyingMarks  *string             `json:"identifyingMarks"`
	LastSeenWearing   *string             `json:"lastSeenWearing"`
	LastSeenLocation  string              `json:"lastSeenLocation"`
	LastSeenAt        time.Time           `json:"lastSeenAt"`
	LastSeenLatitude  *float64            `json:"lastSeenLatitude"`
	LastSeenLongitude *float64            `json:"lastSeenLongitude"`
	StationID         *uuid.UUID          `json:"stationId"`
	StationName       string              `json:"stationName"`
	LodgedAt          time.Time           `json:"lodgedAt"`
	PrimaryPhotoID    *uuid.UUID          `json:"primaryPhotoId"`
	PhotoCount        int                 `json:"photoCount"`
	VerifiedSightings int                 `json:"verifiedSightings"`
	// Each station's latest check.
	Checks []StationCheck `json:"checks"`
}

type Board struct {
	Data       []BoardEntry   `json:"data"`
	Stations   []BoardStation `json:"stations"`
	ServerTime time.Time      `json:"serverTime"`
	// The viewer's own station, so the screen can offer "record our check".
	ViewerStationID *uuid.UUID `json:"viewerStationId"`
}

/* --------------------------------------------------------------------- map */

type MapLastSeen struct {
	Location  string    `json:"location"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	At        time.Time `json:"at"`
}

type MapCamera struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Location    string    `json:"location"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	StationName string    `json:"stationName"`
}

// CameraMatch is a face-match candidate from the face recognition layer
// (table face_match_candidates, migrations 000072+). Coordinates are the
// candidate's own, or else the registered camera's; never estimated.
type CameraMatch struct {
	ID             uuid.UUID  `json:"id"`
	PhotoID        *uuid.UUID `json:"photoId"`
	CameraID       *uuid.UUID `json:"cameraId"`
	CameraName     string     `json:"cameraName"`
	SourceMedia    string     `json:"sourceMedia"`
	FrameTime      *time.Time `json:"frameTime"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	PointFrom      string     `json:"pointFrom"` // CANDIDATE, CAMERA or NONE
	Similarity     *float64   `json:"similarity"`
	ModelVersion   string     `json:"modelVersion"`
	Status         string     `json:"status"`
	ReviewedByName string     `json:"reviewedByName"`
	ReviewedAt     *time.Time `json:"reviewedAt"`
	SightingID     *uuid.UUID `json:"sightingId"`
	// IsDemo marks a match made under a DEMO authorisation from synthetic
	// test faces; the map labels it.
	IsDemo bool `json:"isDemo"`
}

type CameraMatchLayer struct {
	// Available is false until the face recognition tables exist.
	Available bool          `json:"available"`
	Error     string        `json:"error,omitempty"`
	Data      []CameraMatch `json:"data"`
}

// PathPoint is one stop on the time-ordered route: last seen, then verified
// sightings and confirmed camera matches with stored coordinates.
type PathPoint struct {
	Kind      string     `json:"kind"` // LAST_SEEN, VERIFIED_SIGHTING, CONFIRMED_MATCH
	RefID     *uuid.UUID `json:"refId"`
	Label     string     `json:"label"`
	Latitude  float64    `json:"latitude"`
	Longitude float64    `json:"longitude"`
	At        time.Time  `json:"at"`
}

type SearchMap struct {
	LastSeen      MapLastSeen             `json:"lastSeen"`
	Sightings     []MissingPersonSighting `json:"sightings"`
	CameraMatches CameraMatchLayer        `json:"cameraMatches"`
	Cameras       []MapCamera             `json:"cameras"`
	Path          []PathPoint             `json:"path"`
}

type SetLastSeenPointRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}
