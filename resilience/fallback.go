package resilience

import (
	"context"
	"fmt"
)

// FallbackActor tries the primary actor, then falls back to alternates in order.
type FallbackActor struct {
	primary   ActorIface
	fallbacks []ActorIface
}

// NewFallbackActor wraps a primary actor with fallback alternates.
func NewFallbackActor(primary ActorIface, fallbacks []ActorIface) *FallbackActor {
	return &FallbackActor{primary: primary, fallbacks: fallbacks}
}

// Perform tries the primary, then each fallback until one succeeds.
func (a *FallbackActor) Perform(ctx context.Context, prompt string) (string, error) {
	resp, err := a.primary.Perform(ctx, prompt)
	if err == nil {
		return resp, nil
	}
	for _, fb := range a.fallbacks {
		resp, err = fb.Perform(ctx, prompt)
		if err == nil {
			return resp, nil
		}
	}
	return "", fmt.Errorf("all fallbacks exhausted: %w", err)
}

// Ready returns true if the primary or any fallback is ready.
func (a *FallbackActor) Ready() bool {
	if a.primary.Ready() {
		return true
	}
	for _, fb := range a.fallbacks {
		if fb.Ready() {
			return true
		}
	}
	return false
}

// Kill stops the primary and all fallbacks.
func (a *FallbackActor) Kill(ctx context.Context) error {
	err := a.primary.Kill(ctx)
	for _, fb := range a.fallbacks {
		if fbErr := fb.Kill(ctx); fbErr != nil && err == nil {
			err = fbErr
		}
	}
	return err
}
