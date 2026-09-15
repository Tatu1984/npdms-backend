package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrFRServiceNotConnected: FR_SERVICE_URL is not set on this deployment.
var ErrFRServiceNotConnected = errors.New("face recognition service not connected: this deployment has no face recognition service configured")

// ErrFRServiceUnavailable: it is configured but did not answer properly.
var ErrFRServiceUnavailable = errors.New("face recognition service unavailable")

// FRClient calls the on-premises face recognition service (services/ml/face_recognition).
// It never falls back to anything: if the service cannot answer, the caller
// gets an error and no result.
type FRClient struct {
	baseURL string
	token   string
	http    *http.Client
	health  *http.Client

	mu        sync.Mutex
	cached    *FRHealth
	cachedErr error
	cachedAt  time.Time
}

type FRHealth struct {
	Status       string          `json:"status"`
	ModelVersion string          `json:"modelVersion"`
	EmbeddingDim int             `json:"embeddingDim"`
	Detector     json.RawMessage `json:"detector"`
	Recognizer   json.RawMessage `json:"recognizer"`
	Raw          json.RawMessage `json:"-"`
}

func NewFRClientFromEnv() *FRClient {
	timeout := 15 * time.Minute
	if v, err := strconv.Atoi(os.Getenv("FR_SERVICE_TIMEOUT_SECONDS")); err == nil && v > 0 {
		timeout = time.Duration(v) * time.Second
	}
	return &FRClient{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("FR_SERVICE_URL")), "/"),
		token:   os.Getenv("FR_SERVICE_TOKEN"),
		http:    &http.Client{Timeout: timeout},
		health:  &http.Client{Timeout: 4 * time.Second},
	}
}

func (c *FRClient) Configured() bool { return c != nil && c.baseURL != "" }

func (c *FRClient) authorise(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// Health asks the service for its model versions, cached for 20 seconds.
func (c *FRClient) Health(ctx context.Context, fresh bool) (*FRHealth, error) {
	if !c.Configured() {
		return nil, ErrFRServiceNotConnected
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !fresh && time.Since(c.cachedAt) < 20*time.Second {
		return c.cached, c.cachedErr
	}
	h, err := c.fetchHealth(ctx)
	c.cached, c.cachedErr, c.cachedAt = h, err, time.Now()
	return h, err
}

func (c *FRClient) fetchHealth(ctx context.Context) (*FRHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.health.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: could not reach %s (%v)", ErrFRServiceUnavailable, c.baseURL, rootCause(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: health check answered %d: %s", ErrFRServiceUnavailable, resp.StatusCode, serviceDetail(body))
	}
	var h FRHealth
	if err := json.Unmarshal(body, &h); err != nil || h.ModelVersion == "" {
		return nil, fmt.Errorf("%w: health check returned an unexpected body", ErrFRServiceUnavailable)
	}
	h.Raw = body
	return &h, nil
}

func rootCause(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

// serviceDetail extracts FastAPI's {"detail": "..."} message.
func serviceDetail(body []byte) string {
	var d struct {
		Detail interface{} `json:"detail"`
	}
	if json.Unmarshal(body, &d) == nil && d.Detail != nil {
		if s, ok := d.Detail.(string); ok {
			return s
		}
		b, _ := json.Marshal(d.Detail)
		return string(b)
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

type FREnrolResult struct {
	Accepted     bool            `json:"accepted"`
	Reason       string          `json:"reason"`
	Message      string          `json:"message"`
	Quality      json.RawMessage `json:"quality"`
	Face         json.RawMessage `json:"face"`
	ModelVersion string          `json:"modelVersion"`
	FacesFound   int             `json:"facesFound"`
	PhotoSHA256  string          `json:"photoSha256"`
	Embedding    []float32       `json:"embedding"`
	AlignedFace  *FRImage        `json:"alignedFace"`
}

type FRImage struct {
	JPEG   []byte `json:"jpeg"` // base64 in JSON, decoded by encoding/json
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (c *FRClient) Enrol(ctx context.Context, filename, contentType string, data []byte) (*FREnrolResult, error) {
	if !c.Configured() {
		return nil, ErrFRServiceNotConnected
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := writeFilePart(w, "file", filename, contentType, bytes.NewReader(data)); err != nil {
		return nil, err
	}
	w.Close()
	var out FREnrolResult
	if err := c.post(ctx, "/v1/enrol", w.FormDataContentType(), &buf, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type FRMatchInput struct {
	Filename     string
	ContentType  string
	Media        io.Reader
	GalleryIDs   []string
	GalleryGroup []string
	Embeddings   [][]float32
	Threshold    float64
	ModelVersion string
	SampleFps    float64
	MergeWindowS float64
}

type FRMatchCandidate struct {
	GalleryID      string                   `json:"galleryId"`
	Group          string                   `json:"group"`
	Similarity     float64                  `json:"similarity"`
	FrameIndex     int                      `json:"frameIndex"`
	FrameOffsetMs  *int                     `json:"frameOffsetMs"`
	Box            struct{ X, Y, W, H int } `json:"box"`
	DetectionScore float64                  `json:"detectionScore"`
	Quality        json.RawMessage          `json:"quality"`
	Frame          FRImage                  `json:"frame"`
	Crop           FRImage                  `json:"crop"`
}

type FRMatchResult struct {
	ModelVersion   string             `json:"modelVersion"`
	Threshold      float64            `json:"threshold"`
	Kind           string             `json:"kind"`
	SampleFps      *float64           `json:"sampleFps"`
	FramesAnalysed int                `json:"framesAnalysed"`
	FacesSeen      int                `json:"facesSeen"`
	FacesCompared  int                `json:"facesCompared"`
	GallerySize    int                `json:"gallerySize"`
	Truncated      bool               `json:"truncated"`
	DurationMs     int                `json:"durationMs"`
	Candidates     []FRMatchCandidate `json:"candidates"`
}

// Match streams the media and the gallery to the service.
func (c *FRClient) Match(ctx context.Context, in FRMatchInput) (*FRMatchResult, error) {
	if !c.Configured() {
		return nil, ErrFRServiceNotConnected
	}
	gallery := make([]byte, 0, len(in.Embeddings)*128*4)
	for _, e := range in.Embeddings {
		for _, v := range e {
			gallery = binary.LittleEndian.AppendUint32(gallery, math.Float32bits(v))
		}
	}
	ids, _ := json.Marshal(in.GalleryIDs)
	groups, _ := json.Marshal(in.GalleryGroup)

	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	go func() {
		err := func() error {
			fields := map[string]string{
				"gallery_ids": string(ids), "gallery_groups": string(groups),
				"threshold": strconv.FormatFloat(in.Threshold, 'f', 4, 64), "model_version": in.ModelVersion,
				"sample_fps": strconv.FormatFloat(in.SampleFps, 'f', 3, 64), "merge_window_s": strconv.FormatFloat(in.MergeWindowS, 'f', 2, 64),
			}
			for k, v := range fields {
				if err := w.WriteField(k, v); err != nil {
					return err
				}
			}
			if err := writeFilePart(w, "gallery", "gallery.bin", "application/octet-stream", bytes.NewReader(gallery)); err != nil {
				return err
			}
			if err := writeFilePart(w, "file", in.Filename, in.ContentType, in.Media); err != nil {
				return err
			}
			return w.Close()
		}()
		pw.CloseWithError(err)
	}()
	var out FRMatchResult
	if err := c.post(ctx, "/v1/match", w.FormDataContentType(), pr, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func writeFilePart(w *multipart.Writer, field, filename, contentType string, r io.Reader) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, strings.NewReplacer(`"`, "", "\\", "", "\r", "", "\n", "").Replace(filename)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, r)
	return err
}

func (c *FRClient) post(ctx context.Context, path, contentType string, body io.Reader, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	c.authorise(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: could not reach %s (%v)", ErrFRServiceUnavailable, c.baseURL, rootCause(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return fmt.Errorf("%w: reading the response failed (%v)", ErrFRServiceUnavailable, err)
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%w: unexpected response (%v)", ErrFRServiceUnavailable, err)
		}
		return nil
	case resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge:
		return invalid("%s", serviceDetail(raw))
	case resp.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrFRModelChanged, serviceDetail(raw))
	default:
		return fmt.Errorf("%w: answered %d: %s", ErrFRServiceUnavailable, resp.StatusCode, serviceDetail(raw))
	}
}

// ErrFRModelChanged: the service runs a different model from the one the
// gallery was enrolled with.
var ErrFRModelChanged = errors.New("face recognition model changed; re-enrol photos")
