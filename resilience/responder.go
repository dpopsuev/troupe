package resilience

import (
	"context"

	"github.com/dpopsuev/jericho/bugle"
)

// CircuitBreakerResponder wraps a bugle.Responder with circuit breaker protection.
type CircuitBreakerResponder struct {
	inner   bugle.Responder
	breaker *CircuitBreaker
}

// NewCircuitBreakerResponder wraps inner with circuit breaker protection.
func NewCircuitBreakerResponder(inner bugle.Responder, cfg CircuitConfig) *CircuitBreakerResponder {
	return &CircuitBreakerResponder{
		inner:   inner,
		breaker: NewCircuitBreaker(cfg),
	}
}

// Respond delegates to the inner responder if the circuit allows it.
func (r *CircuitBreakerResponder) Respond(ctx context.Context, prompt string) (string, error) {
	var result string
	err := r.breaker.Call(func() error {
		var callErr error
		result, callErr = r.inner.Respond(ctx, prompt)
		return callErr
	})
	return result, err
}

// State returns the current circuit state.
func (r *CircuitBreakerResponder) State() CircuitState { return r.breaker.State() }

// Inner returns the wrapped responder.
func (r *CircuitBreakerResponder) Inner() bugle.Responder { return r.inner }

// RateLimitResponder wraps a bugle.Responder with token bucket rate limiting.
type RateLimitResponder struct {
	inner   bugle.Responder
	limiter *RateLimiter
}

// NewRateLimitResponder wraps inner with rate limiting.
func NewRateLimitResponder(inner bugle.Responder, cfg RateLimitConfig) *RateLimitResponder {
	return &RateLimitResponder{
		inner:   inner,
		limiter: NewRateLimiter(cfg),
	}
}

// Respond waits for a rate limit token, then delegates.
func (r *RateLimitResponder) Respond(ctx context.Context, prompt string) (string, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return r.inner.Respond(ctx, prompt)
}

// Waits returns the total number of times a call was delayed.
func (r *RateLimitResponder) Waits() int64 { return r.limiter.Waits() }

// Inner returns the wrapped responder.
func (r *RateLimitResponder) Inner() bugle.Responder { return r.inner }
