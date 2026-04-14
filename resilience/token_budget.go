package resilience

import (
	"context"
	"sync"
	"time"
)

// TokenBudgetConfig configures a token-weighted rate limiter.
type TokenBudgetConfig struct {
	TokensPerMinute int // refill rate (e.g., 30000 for Anthropic)
	Burst           int // max burst capacity (0 = same as TokensPerMinute)
}

// TokenBudget implements token-weighted rate limiting.
// Unlike RateLimiter (1 token per request), TokenBudget gates on
// estimated input tokens per API call — matching actual provider
// rate limits (e.g., Anthropic's 30K tokens/min).
//
// Usage: call Spend(ctx, estimatedTokens) before each API call.
// It blocks until the budget has enough capacity.
type TokenBudget struct {
	mu        sync.Mutex
	available float64   // current token budget
	capacity  float64   // max burst capacity
	refillPer float64   // tokens per nanosecond
	lastRefil time.Time // last refill timestamp
	waits     int64     // total times a caller was throttled
}

// NewTokenBudget creates a token-weighted rate limiter.
func NewTokenBudget(cfg TokenBudgetConfig) *TokenBudget {
	if cfg.TokensPerMinute <= 0 {
		cfg.TokensPerMinute = 30000 //nolint:mnd // Anthropic default
	}
	capacity := float64(cfg.Burst)
	if capacity <= 0 {
		capacity = float64(cfg.TokensPerMinute)
	}
	return &TokenBudget{
		available: capacity, // start full
		capacity:  capacity,
		refillPer: float64(cfg.TokensPerMinute) / float64(time.Minute),
		lastRefil: time.Now(),
	}
}

// Spend blocks until the budget can accommodate n tokens.
// Returns immediately if enough tokens are available.
// Returns ctx.Err() if the context is canceled while waiting.
func (tb *TokenBudget) Spend(ctx context.Context, tokens int) error {
	for {
		tb.mu.Lock()
		tb.refill()

		if tb.available >= float64(tokens) {
			tb.available -= float64(tokens)
			tb.mu.Unlock()
			return nil
		}

		// Calculate wait time until enough tokens are available.
		deficit := float64(tokens) - tb.available
		waitDur := time.Duration(deficit / tb.refillPer)
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond //nolint:mnd // minimum granularity
		}
		tb.waits++
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
			// Refill happened, retry.
		}
	}
}

// refill adds tokens based on elapsed time since last refill.
// Must be called with mu held.
func (tb *TokenBudget) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefil)
	tb.lastRefil = now

	tb.available += float64(elapsed) * tb.refillPer
	if tb.available > tb.capacity {
		tb.available = tb.capacity
	}
}

// Available returns the current token budget (approximate, for monitoring).
func (tb *TokenBudget) Available() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return int(tb.available)
}

// Waits returns the total number of times a caller was throttled.
func (tb *TokenBudget) Waits() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.waits
}
