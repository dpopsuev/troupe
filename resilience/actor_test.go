package resilience_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/dpopsuev/troupe/resilience"
	"github.com/dpopsuev/troupe/testkit"
)

type failNActor struct {
	failCount int
	calls     atomic.Int32
}

func (a *failNActor) Perform(_ context.Context, _ string) (string, error) {
	n := int(a.calls.Add(1))
	if n <= a.failCount {
		return "", errors.New("transient failure")
	}
	return "success", nil
}

func (a *failNActor) Ready() bool                 { return true }
func (a *failNActor) Kill(_ context.Context) error { return nil }

type alwaysFailActor struct{}

func (alwaysFailActor) Perform(_ context.Context, _ string) (string, error) {
	return "", errors.New("permanent failure")
}

func (alwaysFailActor) Ready() bool                 { return true }
func (alwaysFailActor) Kill(_ context.Context) error { return nil }

// --- RetryActor ---

func TestRetryActor_RetriesOnFailure(t *testing.T) {
	inner := &failNActor{failCount: 2}
	ra := resilience.NewRetryActor(inner, resilience.RetryConfig{MaxAttempts: 3})
	resp, err := ra.Perform(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if resp != "success" {
		t.Errorf("resp = %q, want success", resp)
	}
	if inner.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", inner.calls.Load())
	}
}

func TestRetryActor_ExhaustsAttempts(t *testing.T) {
	ra := resilience.NewRetryActor(alwaysFailActor{}, resilience.RetryConfig{MaxAttempts: 2})
	_, err := ra.Perform(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
}

func TestRetryActor_DelegatesToInner(t *testing.T) {
	actor := &testkit.MockActor{Name: "inner"}
	ra := resilience.NewRetryActor(actor, resilience.RetryConfig{MaxAttempts: 1})

	if !ra.Ready() {
		t.Error("retry actor not ready")
	}
	ra.Kill(context.Background()) //nolint:errcheck
	if ra.Ready() {
		t.Error("retry actor ready after kill")
	}
}

// --- FallbackActor ---

func TestFallbackActor_PrimarySucceeds(t *testing.T) {
	primary := &testkit.MockActor{Name: "primary"}
	fa := resilience.NewFallbackActor(primary, nil)
	resp, err := fa.Perform(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Error("empty response")
	}
}

func TestFallbackActor_FallsBack(t *testing.T) {
	backup := &testkit.MockActor{Name: "backup"}
	fa := resilience.NewFallbackActor(alwaysFailActor{}, []resilience.ActorIface{backup})
	resp, err := fa.Perform(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if resp == "" {
		t.Error("empty response from fallback")
	}
}

func TestFallbackActor_Exhausted(t *testing.T) {
	fa := resilience.NewFallbackActor(alwaysFailActor{}, []resilience.ActorIface{alwaysFailActor{}})
	_, err := fa.Perform(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
}

func TestFallbackActor_Ready(t *testing.T) {
	killed := &testkit.MockActor{Name: "dead"}
	killed.Kill(context.Background()) //nolint:errcheck
	backup := &testkit.MockActor{Name: "alive"}
	fa := resilience.NewFallbackActor(killed, []resilience.ActorIface{backup})
	if !fa.Ready() {
		t.Error("fallback should be ready if any actor is ready")
	}
}
