package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/npdms/api/internal/models"
)

// fakeModel stands in for a model service. The gateway's job is to decide
// whether a model may be called at all, so the tests care about whether it was
// called, not about what a real model would say.
type fakeModel struct {
	configured bool
	called     bool
	result     Result
	err        error
}

func (f *fakeModel) Configured() bool { return f.configured }

func (f *fakeModel) Health(context.Context) (Health, error) {
	if !f.configured {
		return Health{}, ErrNotConnected
	}
	return Health{Service: "fake", ModelName: "fake", ModelVersion: "v1", Ready: true}, nil
}

func (f *fakeModel) Infer(context.Context, Request) (Result, error) {
	f.called = true
	return f.result, f.err
}

func request() Request {
	return Request{
		Type:       models.AIDecisionComplaintCategory,
		SourceType: "COMPLAINT",
		SourceID:   uuid.New(),
		Text:       "the complainant reports a stolen phone",
		Sources: []models.AISource{{
			RecordType: "COMPLAINT",
			Reference:  "MIS/2026/00001",
			Excerpt:    "the complainant reports a stolen phone",
		}},
	}
}

// A model with no service address is not connected, and the gateway must say
// so rather than return an empty suggestion.
func TestSuggestRefusesWhenNotConnected(t *testing.T) {
	model := &fakeModel{configured: false}

	if model.Configured() {
		t.Fatal("a model with no address reported itself as configured")
	}

	health, err := model.Health(context.Background())
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v (%+v)", err, health)
	}
}

// An HTTP model with no environment variable set is not connected. This is the
// state of every model on the Vercel staging API.
func TestHTTPModelWithoutAddressIsNotConnected(t *testing.T) {
	model := NewHTTPModel("NPDMS_TEST_MODEL_URL_THAT_IS_NOT_SET")

	if model.Configured() {
		t.Fatal("a model whose environment variable is unset reported itself as configured")
	}

	if _, err := model.Infer(context.Background(), request()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected from Infer, got %v", err)
	}
	if _, err := model.Health(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected from Health, got %v", err)
	}
}

// The gateway holds one client per registry model name.
func TestRegisterAndConnected(t *testing.T) {
	gateway := NewGateway(nil)

	if gateway.Connected("absent") {
		t.Fatal("a model that was never registered reported as connected")
	}

	gateway.Register("configured", &fakeModel{configured: true})
	gateway.Register("unconfigured", &fakeModel{configured: false})

	if !gateway.Connected("configured") {
		t.Fatal("a registered, configured model did not report as connected")
	}
	if gateway.Connected("unconfigured") {
		t.Fatal("a model with no address reported as connected")
	}
	if len(gateway.ModelNames()) != 2 {
		t.Fatalf("expected two registered models, got %v", gateway.ModelNames())
	}
}
