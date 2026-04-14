package resilience

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBudget_ImmediateSpend(t *testing.T) {
	tb := NewTokenBudget(TokenBudgetConfig{TokensPerMinute: 30000})

	// Spending less than capacity should return immediately.
	ctx := context.Background()
	if err := tb.Spend(ctx, 1000); err != nil {
		t.Fatalf("Spend(1000): %v", err)
	}
	if tb.Waits() != 0 {
		t.Fatalf("expected 0 waits, got %d", tb.Waits())
	}
}

func TestTokenBudget_ThrottlesWhenExhausted(t *testing.T) {
	// 6000 tokens/min = 100 tokens/sec.
	tb := NewTokenBudget(TokenBudgetConfig{TokensPerMinute: 6000, Burst: 1000})

	ctx := context.Background()

	// Exhaust the burst capacity.
	if err := tb.Spend(ctx, 1000); err != nil {
		t.Fatalf("Spend(1000): %v", err)
	}

	// Next spend should block until refill.
	start := time.Now()
	if err := tb.Spend(ctx, 100); err != nil {
		t.Fatalf("Spend(100): %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited ~1 second for 100 tokens at 100 tokens/sec.
	if elapsed < 500*time.Millisecond {
		t.Fatalf("expected throttle, but spent in %v", elapsed)
	}
	if tb.Waits() == 0 {
		t.Fatal("expected waits > 0")
	}
}

func TestTokenBudget_ContextCancellation(t *testing.T) {
	tb := NewTokenBudget(TokenBudgetConfig{TokensPerMinute: 100, Burst: 10})

	ctx := context.Background()
	// Exhaust budget.
	_ = tb.Spend(ctx, 10)

	// Cancel context — Spend should return error.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tb.Spend(ctx, 100)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestTokenBudget_ConcurrentSpenders(t *testing.T) {
	// 60000 tokens/min = 1000 tokens/sec. 10 concurrent spenders each spending 500.
	tb := NewTokenBudget(TokenBudgetConfig{TokensPerMinute: 60000, Burst: 5000})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var completed atomic.Int32
	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tb.Spend(ctx, 500); err != nil {
				t.Errorf("Spend: %v", err)
				return
			}
			completed.Add(1)
		}()
	}

	wg.Wait()
	if completed.Load() != 10 {
		t.Fatalf("expected 10 completions, got %d", completed.Load())
	}
}

func TestTokenBudget_Available(t *testing.T) {
	tb := NewTokenBudget(TokenBudgetConfig{TokensPerMinute: 30000, Burst: 10000})

	initial := tb.Available()
	if initial != 10000 {
		t.Fatalf("initial available = %d, want 10000", initial)
	}

	_ = tb.Spend(context.Background(), 3000)
	after := tb.Available()
	if after > 7100 || after < 6900 { // allow small refill during test
		t.Fatalf("after Spend(3000) available = %d, want ~7000", after)
	}
}

func TestTokenBudget_Defaults(t *testing.T) {
	// Zero config should use defaults.
	tb := NewTokenBudget(TokenBudgetConfig{})

	if tb.capacity != 30000 {
		t.Fatalf("default capacity = %f, want 30000", tb.capacity)
	}
}
