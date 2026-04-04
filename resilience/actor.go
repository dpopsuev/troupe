package resilience

import "context"

// ActorIface mirrors troupe.Actor to avoid an import cycle (resilience → troupe → acp → resilience).
// Any type implementing Perform/Ready/Kill satisfies this — including troupe.Actor.
type ActorIface interface {
	Perform(ctx context.Context, prompt string) (string, error)
	Ready() bool
	Kill(ctx context.Context) error
}

// RetryActor wraps an Actor with automatic retry on Perform failures.
type RetryActor struct {
	inner  ActorIface
	config RetryConfig
}

// NewRetryActor wraps inner with retry protection.
func NewRetryActor(inner ActorIface, cfg RetryConfig) *RetryActor {
	return &RetryActor{inner: inner, config: cfg}
}

// Perform retries the inner actor's Perform according to the retry config.
func (a *RetryActor) Perform(ctx context.Context, prompt string) (string, error) {
	var result string
	err := Retry(ctx, a.config, func() error {
		var callErr error
		result, callErr = a.inner.Perform(ctx, prompt)
		return callErr
	})
	return result, err
}

// Ready delegates to the inner actor.
func (a *RetryActor) Ready() bool { return a.inner.Ready() }

// Kill delegates to the inner actor.
func (a *RetryActor) Kill(ctx context.Context) error { return a.inner.Kill(ctx) }
