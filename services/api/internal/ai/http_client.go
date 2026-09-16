package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/npdms/api/internal/models"
)

// HTTPModel talks to a model service over HTTP on the premises.
//
// The contract is the same for every model, so a new one needs a registry
// entry and an address, not new code in the API:
//
//	GET  {base}/health   -> {"service","modelName","modelVersion","licence","ready"}
//	POST {base}/v1/infer -> {"prediction","confidence","data","alternatives","language","sources"}
//
// The address comes from an environment variable named in the registry. Where
// that variable is unset, the model is simply not connected on this
// deployment — the Vercel staging API, for instance, runs no model services at
// all, and every AI screen there says so.
type HTTPModel struct {
	baseURL string
	token   string
	client  *http.Client

	mu         sync.Mutex
	health     Health
	healthAt   time.Time
	healthGood bool
}

// NewHTTPModel reads the service address from the named environment variable.
// The token variable is the same name with _TOKEN appended.
func NewHTTPModel(endpointEnv string) *HTTPModel {
	timeout := 30 * time.Second
	if raw := os.Getenv(endpointEnv + "_TIMEOUT_SECONDS"); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 {
			timeout = seconds
		}
	}

	return &HTTPModel{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv(endpointEnv)), "/"),
		token:   strings.TrimSpace(os.Getenv(endpointEnv + "_TOKEN")),
		client:  &http.Client{Timeout: timeout},
	}
}

// Configured reports whether an address was given for this model.
func (m *HTTPModel) Configured() bool {
	return m.baseURL != ""
}

func (m *HTTPModel) authorise(req *http.Request) {
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
}

// Health asks the service what it is running. The answer is cached briefly so
// a screen that lists several models does not hammer them.
func (m *HTTPModel) Health(ctx context.Context) (Health, error) {
	if !m.Configured() {
		return Health{}, ErrNotConnected
	}

	m.mu.Lock()
	if m.healthGood && time.Since(m.healthAt) < 30*time.Second {
		cached := m.health
		m.mu.Unlock()
		return cached, nil
	}
	m.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/health", nil)
	if err != nil {
		return Health{}, err
	}
	m.authorise(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return Health{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Health{}, fmt.Errorf("model service health returned %d", resp.StatusCode)
	}

	var health Health
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return Health{}, err
	}

	m.mu.Lock()
	m.health, m.healthAt, m.healthGood = health, time.Now(), true
	m.mu.Unlock()

	return health, nil
}

type inferPayload struct {
	Type       string                 `json:"type"`
	SourceType string                 `json:"sourceType"`
	SourceID   string                 `json:"sourceId"`
	Reference  string                 `json:"reference,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Sources    []models.AISource      `json:"sources,omitempty"`
}

// Infer asks the model and returns what it said, unchanged.
func (m *HTTPModel) Infer(ctx context.Context, req Request) (Result, error) {
	if !m.Configured() {
		return Result{}, ErrNotConnected
	}

	body, err := json.Marshal(inferPayload{
		Type:       string(req.Type),
		SourceType: req.SourceType,
		SourceID:   req.SourceID.String(),
		Reference:  req.SourceReference,
		Language:   req.Language,
		Text:       req.Text,
		Payload:    req.Payload,
		Sources:    req.Sources,
	})
	if err != nil {
		return Result{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/infer", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	m.authorise(httpReq)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Carry the service's own words back: a model that refuses a request
		// usually says why, and that reason belongs in the audit entry.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return Result{}, fmt.Errorf("model service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Result{}, err
	}

	return result, nil
}
