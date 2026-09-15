package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrANPRNotConnected: the detection service URL is unset or the service
	// does not answer. Nothing is analysed and nothing is invented.
	ErrANPRNotConnected = errors.New("vehicle detection service not connected")
	// ErrANPRServiceRejected: the service answered but refused the media.
	ErrANPRServiceRejected = errors.New("the vehicle detection service could not analyse this file")
)

// ANPRClient talks to services/ml/vehicle_detection on the edge server. Data
// never leaves the premises: the URL is expected to be a local address.
type ANPRClient struct {
	baseURL string
	health  *http.Client
	analyse *http.Client
}

func NewANPRClient(baseURL string) *ANPRClient {
	return &ANPRClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		health:  &http.Client{Timeout: 3 * time.Second},
		// Footage is analysed frame by frame on CPU; a minute of samples can take a while.
		analyse: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *ANPRClient) Configured() bool { return c.baseURL != "" }

func (c *ANPRClient) host() string {
	if u, err := url.Parse(c.baseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return c.baseURL
}

type anprHealth struct {
	Status                    string          `json:"status"`
	Versions                  json.RawMessage `json:"versions"`
	Models                    json.RawMessage `json:"models"`
	VehicleClasses            []string        `json:"vehicleClasses"`
	UnsupportedVehicleClasses []string        `json:"unsupportedVehicleClasses"`
	Colour                    string          `json:"colour"`
}

// Health returns the service's own report. An error means not connected.
func (c *ANPRClient) Health(ctx context.Context) (*anprHealth, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("%w: no service address is configured on this server (ML_VEHICLE_DETECTION_URL)", ErrANPRNotConnected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrANPRNotConnected, err)
	}
	resp, err := c.health.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s did not answer", ErrANPRNotConnected, c.host())
	}
	defer resp.Body.Close()
	var h anprHealth
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&h); err != nil {
		return nil, fmt.Errorf("%w: %s answered with something other than a health report", ErrANPRNotConnected, c.host())
	}
	if h.Status != "ok" {
		return &h, fmt.Errorf("%w: the service is running but its models are not ready", ErrANPRNotConnected)
	}
	return &h, nil
}

type anprMLCharacter struct {
	Char       string  `json:"char"`
	Confidence float64 `json:"confidence"`
	Corrected  bool    `json:"corrected"`
	Raw        string  `json:"raw"`
}

type anprMLPlateRead struct {
	RawText string `json:"rawText"`
	Plate   struct {
		Normalised  string   `json:"normalised"`
		Display     string   `json:"display"`
		Format      string   `json:"format"`
		Valid       bool     `json:"valid"`
		Corrections []string `json:"corrections"`
	} `json:"plate"`
	Confidence        float64           `json:"confidence"`
	MinCharConfidence float64           `json:"minCharConfidence"`
	Characters        []anprMLCharacter `json:"characters"`
	Box               []float64         `json:"box"`
}

type anprMLDetection struct {
	VehicleClass       string           `json:"vehicleClass"`
	Confidence         float64          `json:"confidence"`
	Box                []float64        `json:"box"`
	PlateReadAttempted bool             `json:"plateReadAttempted"`
	PlateRead          *anprMLPlateRead `json:"plateRead"`
}

type anprMLFrame struct {
	Index                int               `json:"index"`
	OffsetSeconds        float64           `json:"offsetSeconds"`
	Width                int               `json:"width"`
	Height               int               `json:"height"`
	Detections           []anprMLDetection `json:"detections"`
	UnattachedPlateReads []anprMLPlateRead `json:"unattachedPlateReads"`
	JPEGBase64           string            `json:"jpegBase64"`
	TimingMs             struct {
		Total float64 `json:"total"`
	} `json:"timingMs"`
}

type anprMLResult struct {
	Kind     string `json:"kind"`
	Versions struct {
		Pipeline    string `json:"pipeline"`
		Detector    string `json:"detector"`
		PlateReader string `json:"plateReader"`
	} `json:"versions"`
	SampledFrames int           `json:"sampledFrames"`
	ProcessingMs  float64       `json:"processingMs"`
	Frames        []anprMLFrame `json:"frames"`
}

// Analyse sends a still (video=false) or a clip to the service.
func (c *ANPRClient) Analyse(ctx context.Context, filename string, body io.Reader, video bool, sampleSeconds float64) (*anprMLResult, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("%w: no service address is configured on this server (ML_VEHICLE_DETECTION_URL)", ErrANPRNotConnected)
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, body)
		}
		if err == nil && video {
			err = mw.WriteField("sample_seconds", fmt.Sprintf("%.2f", sampleSeconds))
		}
		if err == nil {
			err = mw.WriteField("max_frames", "60")
		}
		if err == nil {
			err = mw.Close()
		}
		pw.CloseWithError(err)
	}()
	path := "/v1/analyse/image"
	if video {
		path = "/v1/analyse/video"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, pr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.analyse.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s did not complete the analysis (%v)", ErrANPRNotConnected, c.host(), errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("%w: the service is running but its models are not ready", ErrANPRNotConnected)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&e)
		if e.Detail == "" {
			e.Detail = resp.Status
		}
		return nil, fmt.Errorf("%w: %s", ErrANPRServiceRejected, e.Detail)
	}
	var out anprMLResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("reading the vehicle detection result: %w", err)
	}
	if out.Versions.Detector == "" || out.Versions.PlateReader == "" {
		return nil, fmt.Errorf("%w: the result does not state its model versions", ErrANPRServiceRejected)
	}
	return &out, nil
}
