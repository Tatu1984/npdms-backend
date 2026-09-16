// Package ai is the one route from the platform to any model.
//
// Nothing else calls a model service directly. Every call goes through the
// gateway, which checks the module switch and the registry, calls the model,
// and records what came back as a suggestion for an officer — with its model
// version, its confidence, and the record text it relied on.
//
// Where a model's service is not configured or cannot be reached, the gateway
// says so and nothing is recorded. It never invents a result, and it never
// approves one: a suggestion leaves the gateway PENDING, always.
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/services"
)

var (
	// ErrNotConnected: no service is configured for this model on this
	// deployment. The screens say so; they do not show an empty result as if
	// the model had nothing to suggest.
	ErrNotConnected = errors.New("ai_model_not_connected")
	// ErrUnavailable: a service is configured but did not answer.
	ErrUnavailable = errors.New("ai_model_unavailable")
	// ErrNotRegistered: no registry entry by that name.
	ErrNotRegistered = errors.New("ai_model_not_registered")
	// ErrSwitchedOff: the model, or the module it belongs to, is off.
	ErrSwitchedOff = errors.New("ai_model_switched_off")
	// ErrBelowThreshold: the model answered, but under the confidence its
	// registry entry requires, so it is not put in front of an officer.
	ErrBelowThreshold = errors.New("ai_below_threshold")
)

// Client is one model service. Implementations are thin: they speak to a
// service on the premises and return what it said, unchanged.
type Client interface {
	// Configured reports whether this deployment has a service to call.
	Configured() bool
	// Health describes the service and the model it is running.
	Health(ctx context.Context) (Health, error)
	// Infer asks the model. It returns the model's own confidence and the
	// sources it was given back, never an interpretation of them.
	Infer(ctx context.Context, req Request) (Result, error)
}

// Health is what a model service reports about itself.
type Health struct {
	Service      string `json:"service"`
	ModelName    string `json:"modelName"`
	ModelVersion string `json:"modelVersion"`
	Licence      string `json:"licence,omitempty"`
	Ready        bool   `json:"ready"`
}

// Request is one question for one model.
type Request struct {
	// What the suggestion will be about.
	Type            models.AIDecisionType
	SourceType      string
	SourceID        uuid.UUID
	SourceReference string
	StationID       *uuid.UUID
	Language        string

	// The text or payload the model reads, and where it came from. The
	// sources are carried through to the suggestion so an officer can check
	// it against the record rather than take the model's word.
	Text    string
	Payload map[string]interface{}
	Sources []models.AISource
}

// Result is what a model said.
type Result struct {
	Prediction   string                           `json:"prediction"`
	Confidence   float64                          `json:"confidence"`
	Data         map[string]interface{}           `json:"data,omitempty"`
	Alternatives []services.AlternativePrediction `json:"alternatives,omitempty"`
	Language     string                           `json:"language,omitempty"`
	Sources      []models.AISource                `json:"sources,omitempty"`
}

// Suggestion is a recorded suggestion and the model that made it.
type Suggestion struct {
	Decision *models.AIDecision   `json:"decision"`
	Model    models.AIModelConfig `json:"model"`
}

// Gateway holds the model clients and the review service the suggestions land in.
type Gateway struct {
	review  *services.AIReviewService
	mu      sync.RWMutex
	clients map[string]Client
	timeout time.Duration
}

// NewGateway builds the gateway. Clients are keyed by registry model name.
func NewGateway(review *services.AIReviewService) *Gateway {
	return &Gateway{
		review:  review,
		clients: map[string]Client{},
		timeout: 30 * time.Second,
	}
}

// Register attaches a client to a model name in the registry.
func (g *Gateway) Register(modelName string, client Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clients[modelName] = client
}

// ModelNames lists the models the gateway has a client for.
func (g *Gateway) ModelNames() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	names := make([]string, 0, len(g.clients))
	for name := range g.clients {
		names = append(names, name)
	}
	return names
}

// clientFor returns the client for a registry entry, building one from the
// entry's service address if this is the first time the gateway has seen it.
//
// Without this a model registered today would need the API restarted before
// anything could call it, and the screens would say "no client is built in"
// where the truth is that its address is simply not set here.
func (g *Gateway) clientFor(entry models.AIModelConfig) (Client, bool) {
	g.mu.RLock()
	client, ok := g.clients[entry.ModelName]
	g.mu.RUnlock()
	if ok {
		return client, true
	}

	if entry.EndpointEnv == "" {
		return nil, false
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if client, ok := g.clients[entry.ModelName]; ok {
		return client, true
	}
	client = NewHTTPModel(entry.EndpointEnv)
	g.clients[entry.ModelName] = client
	return client, true
}

// Connected reports whether a model's service is configured here.
func (g *Gateway) Connected(modelName string) bool {
	g.mu.RLock()
	client, ok := g.clients[modelName]
	g.mu.RUnlock()
	return ok && client.Configured()
}

// Suggest runs one model and records what it said as a suggestion awaiting an
// officer. The order of the checks is deliberate: nothing reaches a model
// service until the registry and the switches allow it.
func (g *Gateway) Suggest(ctx context.Context, modelName string, req Request, actor uuid.UUID) (*Suggestion, error) {
	entry, err := g.review.GetModelConfig(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRegistered, modelName)
	}

	if !entry.IsEnabled {
		return nil, fmt.Errorf("%w: %s is switched off", ErrSwitchedOff, modelName)
	}
	if entry.Module != "" && !entry.ModuleOn {
		return nil, fmt.Errorf("%w: module %s is switched off", ErrSwitchedOff, entry.Module)
	}
	if entry.RetiredAt != nil {
		return nil, fmt.Errorf("%w: %s is retired", ErrSwitchedOff, modelName)
	}

	client, ok := g.clientFor(*entry)
	if !ok || !client.Configured() {
		return nil, fmt.Errorf("%w: %s", ErrNotConnected, modelName)
	}

	callCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	started := time.Now()
	result, err := client.Infer(callCtx, req)
	elapsed := time.Since(started)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrUnavailable, modelName, err)
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return nil, fmt.Errorf("%s returned a confidence of %v, which is not a fraction between 0 and 1",
			modelName, result.Confidence)
	}
	if strings.TrimSpace(result.Prediction) == "" {
		return nil, fmt.Errorf("%s returned an empty suggestion", modelName)
	}

	// A model that is not sure enough says nothing. Recording it anyway would
	// fill an officer's queue with noise the model itself does not stand by.
	if result.Confidence < entry.ConfidenceThreshold {
		return nil, fmt.Errorf("%w: %s answered at %.2f, under its %.2f threshold",
			ErrBelowThreshold, modelName, result.Confidence, entry.ConfidenceThreshold)
	}

	sources := result.Sources
	if len(sources) == 0 {
		sources = req.Sources
	}

	language := result.Language
	if language == "" {
		language = req.Language
	}

	decision, err := g.review.CreateDecision(ctx, services.CreateDecisionRequest{
		Type:             req.Type,
		SourceType:       req.SourceType,
		SourceID:         req.SourceID,
		SourceReference:  req.SourceReference,
		ModelName:        modelName,
		Prediction:       result.Prediction,
		PredictionData:   result.Data,
		Confidence:       result.Confidence,
		Language:         language,
		Sources:          sources,
		Alternatives:     result.Alternatives,
		ProcessingTimeMs: elapsed.Milliseconds(),
		StationID:        req.StationID,
	}, actor)
	if err != nil {
		return nil, err
	}

	return &Suggestion{Decision: decision, Model: *entry}, nil
}

// Status describes one registered model on this deployment: what the registry
// says, and whether its service is actually reachable from here.
type Status struct {
	models.AIModelConfig
	HasClient bool    `json:"hasClient"`
	Connected bool    `json:"connected"`
	Reachable *bool   `json:"reachable,omitempty"`
	Reports   *Health `json:"reports,omitempty"`
	Note      string  `json:"note,omitempty"`
}

// Statuses reports every registered model with the state of its service.
func (g *Gateway) Statuses(ctx context.Context) ([]Status, error) {
	entries, err := g.review.GetAllModelConfigs(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Status, 0, len(entries))
	for _, entry := range entries {
		status := Status{AIModelConfig: entry}

		client, ok := g.clientFor(entry)
		status.HasClient = ok
		switch {
		case !ok:
			status.Note = "No client is built into this API for this model."
		case !client.Configured():
			status.Note = "Not connected on this deployment: " + endpointNote(entry)
		default:
			status.Connected = true
			health, err := client.Health(ctx)
			reachable := err == nil
			status.Reachable = &reachable
			if err != nil {
				status.Note = "The service is configured but did not answer."
			} else {
				status.Reports = &health
				if health.ModelVersion != "" && entry.ModelVersion != "" && health.ModelVersion != entry.ModelVersion {
					status.Note = fmt.Sprintf("The service is running %s, the registry says %s.",
						health.ModelVersion, entry.ModelVersion)
				}
			}
		}

		out = append(out, status)
	}

	return out, nil
}

func endpointNote(entry models.AIModelConfig) string {
	if entry.EndpointEnv == "" {
		return "no service address is recorded for it."
	}
	return entry.EndpointEnv + " is not set."
}
