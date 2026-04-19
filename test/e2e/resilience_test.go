// resilience_e2e_test.go — E2E tests for production resilience primitives.
//
// Tests: circuit breaker trips on agent crash, rate limiter throttles
// concurrent prompts, retry recovers from transient failure,
// budget enforcer blocks over-limit agents, agent lookup
// discovers and evicts stale agents.
package e2e_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/resilience"
)

// TestResilienceE2E_RetryRecoversFromTransientFailure proves retry
// with backoff succeeds after transient errors.
func TestResilienceE2E_RetryRecoversFromTransientFailure(t *testing.T) {
	var attempts atomic.Int32
	transient := errors.New("transient")

	err := resilience.Retry(context.Background(), resilience.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		Retryable:   func(e error) bool { return errors.Is(e, transient) },
	}, func() error {
		n := attempts.Add(1)
		if n < 3 {
			return transient
		}
		return nil // succeeds on 3rd attempt
	})

	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

// TestResilienceE2E_CircuitBreakerTripsOnRepeatedFailure proves the
// circuit breaker opens after threshold failures and fast-fails.
func TestResilienceE2E_CircuitBreakerTripsOnRepeatedFailure(t *testing.T) {
	var transitions []resilience.CircuitState
	cb := resilience.NewCircuitBreaker(resilience.CircuitConfig{
		Threshold: 2,
		Cooldown:  100 * time.Millisecond,
		OnChange:  func(_, to resilience.CircuitState) { transitions = append(transitions, to) },
	})

	fail := errors.New("agent crashed")

	// Two failures → circuit opens.
	cb.Call(func() error { return fail })
	cb.Call(func() error { return fail })

	if cb.State() != resilience.CircuitOpen {
		t.Fatalf("state = %v, want open", cb.State())
	}

	// Fast-fail while open.
	err := cb.Call(func() error { return nil })
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}

	// Wait for cooldown → half-open probe succeeds → closes.
	time.Sleep(150 * time.Millisecond)
	err = cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("half-open probe: %v", err)
	}
	if cb.State() != resilience.CircuitClosed {
		t.Fatalf("state = %v, want closed after recovery", cb.State())
	}

	// Should have seen: open → half-open → closed.
	if len(transitions) != 3 {
		t.Fatalf("transitions = %v, want 3", transitions)
	}
}

// TestResilienceE2E_RateLimiterThrottles proves the rate limiter
// blocks requests that exceed the configured rate.
func TestResilienceE2E_RateLimiterThrottles(t *testing.T) {
	var throttled atomic.Int32
	rl := resilience.NewRateLimiter(resilience.RateLimitConfig{
		Rate:    1000, // high rate for test speed
		Burst:   1,
		OnLimit: func() { throttled.Add(1) },
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// First should be instant (burst token).
	rl.Allow()

	// Second should throttle (burst exhausted).
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("wait: %v", err)
	}

	if throttled.Load() != 1 {
		t.Fatalf("throttled = %d, want 1", throttled.Load())
	}
}

// TestResilienceE2E_BudgetEnforcerBlocksOverLimit proves the budget
// enforcer rejects agents that exceed their cost ceiling.
func TestResilienceE2E_BudgetEnforcerBlocksOverLimit(t *testing.T) {
	tracker := billing.NewTracker()
	var hookCalled atomic.Bool
	enforcer := billing.NewBudgetEnforcer(tracker, func(id string, spent, limit float64) {
		hookCalled.Store(true)
	})

	enforcer.SetLimit("agent-1", 0.0001) // tiny limit

	// Record tokens to exceed budget.
	tracker.Record(&billing.TokenRecord{
		Node:           "agent-1",
		PromptTokens:   100000,
		ArtifactTokens: 100000,
		Timestamp:      time.Now(),
	})

	err := enforcer.Check("agent-1")
	if !errors.Is(err, billing.ErrBudgetExceeded) {
		t.Fatalf("err = %v, want ErrBudgetExceeded", err)
	}
	if !hookCalled.Load() {
		t.Fatal("onExceed hook should fire")
	}

	// Agent without limit should pass.
	if err := enforcer.Check("agent-2"); err != nil {
		t.Fatalf("unlimited agent: %v", err)
	}
}

// TestResilienceE2E_AgentLookupDiscovery proves agents can
// register, be discovered by role, heartbeat, and get evicted.
func TestResilienceE2E_AgentLookupDiscovery(t *testing.T) {
	reg := transport.NewInMemoryRegistry(50 * time.Millisecond)

	// Register agents.
	reg.Register("e1", "executor", map[string]string{"model": "claude"})
	reg.Register("e2", "executor", map[string]string{"model": "gemini"})
	reg.Register("i1", "inspector", nil)

	// Discover by role.
	executors := reg.Discover("executor")
	if len(executors) != 2 {
		t.Fatalf("executors = %d, want 2", len(executors))
	}

	// All should be healthy.
	for _, e := range executors {
		if !e.Healthy {
			t.Fatalf("agent %s should be healthy", e.ID)
		}
	}

	// Wait for staleness.
	time.Sleep(100 * time.Millisecond)

	// Should be unhealthy now.
	executors = reg.Discover("executor")
	for _, e := range executors {
		if e.Healthy {
			t.Fatalf("agent %s should be stale", e.ID)
		}
	}

	// Heartbeat revives one.
	reg.Heartbeat("e1")
	executors = reg.Discover("executor")
	for _, e := range executors {
		if e.ID == "e1" && !e.Healthy {
			t.Fatal("e1 should be healthy after heartbeat")
		}
		if e.ID == "e2" && e.Healthy {
			t.Fatal("e2 should still be stale")
		}
	}

	// Evict stale.
	evicted := reg.EvictStale()
	if evicted != 2 { // e2 + i1
		t.Fatalf("evicted = %d, want 2", evicted)
	}

	// Only e1 remains.
	all := reg.All()
	if len(all) != 1 || all[0].ID != "e1" {
		t.Fatalf("remaining = %v, want [e1]", all)
	}
}

