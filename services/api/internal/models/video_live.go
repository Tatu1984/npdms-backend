package models

import (
	"time"

	"github.com/google/uuid"
)

// Live CCTV streaming through the Edge Agent. See migration 000076.

// LiveStatus is what the stored playlist says about a camera's feed.
type LiveStatus string

const (
	// LiveOnline: the playlist lists segments, has no end marker, and is still
	// being rewritten.
	LiveOnline LiveStatus = "ONLINE"
	// LiveConnecting: the agent has written a playlist but no segment yet.
	LiveConnecting LiveStatus = "CONNECTING"
	// LiveStopped: the playlist ended, or the agent stopped rewriting it.
	LiveStopped LiveStatus = "STOPPED"
	// LiveOffline: nothing stored, streaming not enabled, or storage unreachable.
	LiveOffline LiveStatus = "OFFLINE"
)

// EdgeAgentConfig is shown exactly once, when streaming is enabled or the token
// rotated: the values the operator pastes into the Edge Agent.
type EdgeAgentConfig struct {
	// IngestURL is the portal base — Edge Agent → Portal Connection → Ingest URL.
	IngestURL string `json:"ingestUrl"`
	// IngestToken is the camera's upload token. Only its hash is stored.
	IngestToken string `json:"ingestToken"`
	// CameraID is the camera's ingest key — the agent's Camera ID / stream key.
	CameraID string `json:"cameraId"`
	// PublishURL is the full target the agent builds from the two above.
	PublishURL string `json:"publishUrl"`
}

// CameraWithEdgeAgent is a camera response that carries one-time Edge Agent
// settings.
type CameraWithEdgeAgent struct {
	*Camera
	EdgeAgent *EdgeAgentConfig `json:"edgeAgent,omitempty"`
}

type DisableStreamingRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// StartLiveViewRequest opens a purpose-logged live viewing session over one or
// more cameras.
type StartLiveViewRequest struct {
	Purpose   string      `json:"purpose" binding:"required"`
	CameraIDs []uuid.UUID `json:"cameraIds" binding:"required"`
}

type LiveViewSession struct {
	ID        uuid.UUID   `json:"id"`
	CameraIDs []uuid.UUID `json:"cameraIds"`
	Purpose   string      `json:"purpose"`
	StartedAt time.Time   `json:"startedAt"`
	// ExpiresAt slides forward while video is being played, up to MaxUntil.
	ExpiresAt time.Time `json:"expiresAt"`
	MaxUntil  time.Time `json:"maxUntil"`
}

// LiveMediaStatus says whether live video storage is usable, and why not.
type LiveMediaStatus struct {
	Configured bool   `json:"configured"`
	Backend    string `json:"backend"`
	Message    string `json:"message,omitempty"`
}
