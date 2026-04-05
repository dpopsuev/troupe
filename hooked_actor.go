package troupe

import "context"

// hookedActor wraps an Actor with PerformHook interception.
type hookedActor struct {
	inner Actor
	hooks []PerformHook
}

func newHookedActor(inner Actor, hooks []PerformHook) *hookedActor {
	return &hookedActor{inner: inner, hooks: hooks}
}

func (a *hookedActor) Perform(ctx context.Context, prompt string) (string, error) {
	for _, h := range a.hooks {
		if err := h.PrePerform(ctx, prompt); err != nil {
			return "", err
		}
	}
	resp, err := a.inner.Perform(ctx, prompt)
	for _, h := range a.hooks {
		h.PostPerform(ctx, prompt, resp, err)
	}
	return resp, err
}

func (a *hookedActor) Ready() bool                    { return a.inner.Ready() }
func (a *hookedActor) Kill(ctx context.Context) error { return a.inner.Kill(ctx) }

var _ Actor = (*hookedActor)(nil)
