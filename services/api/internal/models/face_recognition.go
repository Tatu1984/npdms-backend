package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Face recognition for missing persons. See migrations 000072 and 000073.

// FRDemoLabel marks everything produced under a DEMO authorisation, on every
// screen, candidate, sighting and audit entry.
const FRDemoLabel = "Demo — synthetic faces"

const (
	FRModule = "FACE_RECOGNITION"

	FRKindOrder = "ORDER"
	FRKindDemo  = "DEMO"

	FRScopeMissingPersons = "MISSING_PERSONS"

	FRModeOff  = "OFF"
	FRModeDemo = "DEMO"
	FRModeLive = "LIVE"

	FRSourceFootage  = "UPLOADED_FOOTAGE"
	FRSourceStill    = "UPLOADED_STILL"
	FRSourceSnapshot = "CAMERA_SNAPSHOT"

	FRPhotoReport    = "REPORT_PHOTO"
	FRPhotoSynthetic = "SYNTHETIC_TEST"
)

type FRAuthorisation struct {
	ID               uuid.UUID  `json:"id"`
	Kind             string     `json:"kind"`
	OrderReference   *string    `json:"orderReference"`
	IssuingAuthority *string    `json:"issuingAuthority"`
	OrderDate        *string    `json:"orderDate"`
	Scope            []string   `json:"scope"`
	ScopeNote        *string    `json:"scopeNote"`
	ValidFrom        string     `json:"validFrom"`
	ValidUntil       string     `json:"validUntil"`
	RecordedBy       uuid.UUID  `json:"recordedBy"`
	RecordedByName   string     `json:"recordedByName"`
	RecordedAt       time.Time  `json:"recordedAt"`
	RevokedAt        *time.Time `json:"revokedAt"`
	RevokedByName    *string    `json:"revokedByName"`
	RevocationReason *string    `json:"revocationReason"`
	Active           bool       `json:"active"`
	DemoLabel        string     `json:"demoLabel,omitempty"`
}

type RecordFRAuthorisationRequest struct {
	Kind             string   `json:"kind"`
	OrderReference   string   `json:"orderReference"`
	IssuingAuthority string   `json:"issuingAuthority"`
	OrderDate        string   `json:"orderDate"`
	Scope            []string `json:"scope"`
	ScopeNote        string   `json:"scopeNote"`
	ValidFrom        string   `json:"validFrom"`
	ValidUntil       string   `json:"validUntil"`
}

type RevokeFRAuthorisationRequest struct {
	Reason string `json:"reason"`
}

type FRConfig struct {
	MatchThreshold     float64 `json:"matchThreshold"`
	SampleFps          float64 `json:"sampleFps"`
	MergeWindowSeconds float64 `json:"mergeWindowSeconds"`
}

type UpdateFRSettingsRequest struct {
	Enabled        *bool    `json:"enabled"`
	MatchThreshold *float64 `json:"matchThreshold"`
	SampleFps      *float64 `json:"sampleFps"`
	Reason         string   `json:"reason"`
}

// FRServiceStatus reports whether the face recognition service can be used.
// When it is not configured or not reachable, nothing that needs it runs.
type FRServiceStatus struct {
	Configured   bool            `json:"configured"`
	Reachable    bool            `json:"reachable"`
	Message      string          `json:"message"`
	ModelVersion string          `json:"modelVersion,omitempty"`
	Models       json.RawMessage `json:"models,omitempty"`
	CheckedAt    time.Time       `json:"checkedAt"`
}

type FRStatus struct {
	SwitchOn      bool             `json:"switchOn"`
	Mode          string           `json:"mode"` // OFF, DEMO or LIVE
	ModeReason    string           `json:"modeReason"`
	DemoLabel     string           `json:"demoLabel,omitempty"`
	ActiveOrder   *FRAuthorisation `json:"activeOrder"`
	ActiveDemo    *FRAuthorisation `json:"activeDemo"`
	Config        FRConfig         `json:"config"`
	SwitchReason  *string          `json:"switchReason"`
	UpdatedAt     time.Time        `json:"updatedAt"`
	UpdatedByName *string          `json:"updatedByName"`
	Service       FRServiceStatus  `json:"service"`
}

type FaceEnrolment struct {
	ID                uuid.UUID       `json:"id"`
	ReportID          uuid.UUID       `json:"reportId"`
	PhotoID           *uuid.UUID      `json:"photoId"`
	SyntheticPhotoID  *uuid.UUID      `json:"syntheticPhotoId"`
	IsDemo            bool            `json:"isDemo"`
	AuthorisationID   uuid.UUID       `json:"authorisationId"`
	AuthorisationKind string          `json:"authorisationKind"`
	Status            string          `json:"status"`
	RejectionReason   *string         `json:"rejectionReason"`
	RejectionMessage  *string         `json:"rejectionMessage"`
	Quality           json.RawMessage `json:"quality"`
	QualityScore      *float32        `json:"qualityScore"`
	FaceBox           json.RawMessage `json:"faceBox"`
	ModelVersion      string          `json:"modelVersion"`
	PhotoSHA256       string          `json:"photoSha256"`
	HasFaceCrop       bool            `json:"hasFaceCrop"`
	Automatic         bool            `json:"automatic"`
	EnrolledBy        uuid.UUID       `json:"enrolledBy"`
	EnrolledByName    string          `json:"enrolledByName"`
	CreatedAt         time.Time       `json:"createdAt"`
	RetiredAt         *time.Time      `json:"retiredAt"`
	RetireReason      *string         `json:"retireReason"`
	CurrentModel      bool            `json:"currentModel"`
	DemoLabel         string          `json:"demoLabel,omitempty"`
}

// FRPhoto is a photo that can be enrolled: a report photo (provided by family,
// friends or officers) or, under DEMO, a synthetic test photo.
type FRPhoto struct {
	ID              uuid.UUID      `json:"id"`
	ReportID        uuid.UUID      `json:"reportId"`
	Kind            string         `json:"kind"`
	IsPrimary       bool           `json:"isPrimary"`
	Source          *string        `json:"source"`
	ProvidedByName  *string        `json:"providedByName"`
	Relationship    *string        `json:"relationship"`
	ConsentRecorded *bool          `json:"consentRecorded"`
	SyntheticSource *string        `json:"syntheticSource"`
	SHA256          string         `json:"sha256"`
	ContentType     string         `json:"contentType"`
	UploadedBy      *uuid.UUID     `json:"uploadedBy"`
	CreatedAt       time.Time      `json:"createdAt"`
	ObjectKey       string         `json:"-"`
	Enrolment       *FaceEnrolment `json:"enrolment"`
	DemoLabel       string         `json:"demoLabel,omitempty"`
}

type EnrolFacesRequest struct {
	PhotoID *uuid.UUID `json:"photoId"`
	Kind    string     `json:"kind"`
}

type RetireEnrolmentRequest struct {
	Reason string `json:"reason"`
}

type FaceMatchSearch struct {
	ID                uuid.UUID  `json:"id"`
	IsDemo            bool       `json:"isDemo"`
	AuthorisationID   uuid.UUID  `json:"authorisationId"`
	SourceMedia       string     `json:"sourceMedia"`
	CameraID          *uuid.UUID `json:"cameraId"`
	CameraName        *string    `json:"cameraName"`
	Purpose           string     `json:"purpose"`
	OriginalFilename  *string    `json:"originalFilename"`
	SizeBytes         *int64     `json:"sizeBytes"`
	MediaSHA256       string     `json:"mediaSha256"`
	MediaRetained     bool       `json:"mediaRetained"`
	RecordedAt        time.Time  `json:"recordedAt"`
	LocationText      *string    `json:"locationText"`
	Latitude          *float64   `json:"latitude"`
	Longitude         *float64   `json:"longitude"`
	ThresholdUsed     float64    `json:"thresholdUsed"`
	ModelVersion      *string    `json:"modelVersion"`
	SampleFps         *float64   `json:"sampleFps"`
	GallerySize       *int       `json:"gallerySize"`
	FramesAnalysed    *int       `json:"framesAnalysed"`
	FacesSeen         *int       `json:"facesSeen"`
	FacesCompared     *int       `json:"facesCompared"`
	CandidatesCreated int        `json:"candidatesCreated"`
	Status            string     `json:"status"`
	Error             *string    `json:"error"`
	DurationMs        *int       `json:"durationMs"`
	SubmittedBy       uuid.UUID  `json:"submittedBy"`
	SubmittedByName   string     `json:"submittedByName"`
	SubmittedAt       time.Time  `json:"submittedAt"`
	DemoLabel         string     `json:"demoLabel,omitempty"`
}

type FaceMatchCandidate struct {
	ID               uuid.UUID       `json:"id"`
	SearchID         uuid.UUID       `json:"searchId"`
	ReportID         uuid.UUID       `json:"reportId"`
	ReportNumber     string          `json:"reportNumber"`
	PersonName       string          `json:"personName"`
	ReportStatus     string          `json:"reportStatus"`
	Masked           bool            `json:"masked"`
	EnrolmentID      uuid.UUID       `json:"enrolmentId"`
	PhotoID          *uuid.UUID      `json:"photoId"`
	SyntheticPhotoID *uuid.UUID      `json:"syntheticPhotoId"`
	IsDemo           bool            `json:"isDemo"`
	CameraID         *uuid.UUID      `json:"cameraId"`
	CameraName       *string         `json:"cameraName"`
	CameraCode       *string         `json:"cameraCode"`
	SourceMedia      string          `json:"sourceMedia"`
	Purpose          string          `json:"purpose"`
	OriginalFilename *string         `json:"originalFilename"`
	MediaSHA256      string          `json:"mediaSha256"`
	SearchMediaSHA   string          `json:"searchMediaSha256"`
	FrameOffsetMs    *int            `json:"frameOffsetMs"`
	FrameTime        time.Time       `json:"frameTime"`
	LocationText     *string         `json:"locationText"`
	Latitude         *float64        `json:"latitude"`
	Longitude        *float64        `json:"longitude"`
	BoundingBox      BoundingBox     `json:"boundingBox"`
	FrameWidth       *int            `json:"frameWidth"`
	FrameHeight      *int            `json:"frameHeight"`
	DetectionScore   *float64        `json:"detectionScore"`
	Quality          json.RawMessage `json:"quality"`
	Similarity       float64         `json:"similarity"`
	ModelVersion     string          `json:"modelVersion"`
	ThresholdUsed    float64         `json:"thresholdUsed"`
	Status           string          `json:"status"`
	SubmittedBy      uuid.UUID       `json:"submittedBy"`
	SubmittedByName  string          `json:"submittedByName"`
	ReviewedBy       *uuid.UUID      `json:"reviewedBy"`
	ReviewedByName   *string         `json:"reviewedByName"`
	ReviewedAt       *time.Time      `json:"reviewedAt"`
	ReviewNote       *string         `json:"reviewNote"`
	SightingID       *uuid.UUID      `json:"sightingId"`
	CreatedAt        time.Time       `json:"createdAt"`
	DemoLabel        string          `json:"demoLabel,omitempty"`
	MediaKey         string          `json:"-"`
	CropKey          string          `json:"-"`
}

type BoundingBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type ReviewFaceMatchRequest struct {
	Note      string   `json:"note"`
	Location  string   `json:"location"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type ReviewFaceMatchResult struct {
	Candidate *FaceMatchCandidate    `json:"candidate"`
	Sighting  *MissingPersonSighting `json:"sighting,omitempty"`
}

type FaceMatchSearchResult struct {
	Search     *FaceMatchSearch     `json:"search"`
	Candidates []FaceMatchCandidate `json:"candidates"`
	Message    string               `json:"message"`
}
