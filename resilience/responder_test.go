package resilience

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/jericho/bugle/testkit"
)

func TestCircuitBreakerResponder_Opens(t *testing.T) {
	inner := &testkit.FailingResponder{Err: errors.New("fail")}
	r := NewCircuitBreakerResponder(inner, CircuitConfig{Threshold: 2})

	// Two failures should open the circuit.
	_, _ = r.Respond(context.Background(), "p1")
	_, _ = r.Respond(context.Background(), "p2")

	if r.State() != CircuitOpen {
		t.Errorf("state = %v, want CircuitOpen", r.State())
	}

	// Third call should fail fast with ErrCircuitOpen.
	_, err := r.Respond(context.Background(), "p3")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("error = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreakerResponder_Passes(t *testing.T) {
	inner := &testkit.StaticResponder{Response: "ok"}
	r := NewCircuitBreakerResponder(inner, CircuitConfig{Threshold: 5})

	result, err := r.Respond(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
	if r.State() != CircuitClosed {
		t.Errorf("state = %v, want CircuitClosed", r.State())
	}
}

func TestRateLimitResponder_Passes(t *testing.T) {
	inner := &testkit.StaticResponder{Response: "ok"}
	r := NewRateLimitResponder(inner, RateLimitConfig{Rate: 100, Burst: 10})

	result, err := r.Respond(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}
