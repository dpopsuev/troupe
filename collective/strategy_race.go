// strategy_race.go — Race: all agents compete, first response wins.
package collective

import (
	"context"
	"fmt"

	"github.com/dpopsuev/jericho/agent"
)

// Race fans out to all agents concurrently. The first successful response
// wins. Remaining agents are canceled via context.
type Race struct{}

// Select returns all agents — Race fans out to everyone.
func (Race) Select(_ context.Context, agents []*agent.Solo) []*agent.Solo {
	return agents
}

// Execute sends the prompt to all agents in parallel, returns the first response.
func (Race) Execute(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	if len(agents) == 0 {
		return "", ErrNoAgents
	}

	type result struct {
		resp string
		err  error
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, len(agents))

	for _, a := range agents {
		go func(ag *agent.Solo) {
			resp, err := ag.Ask(raceCtx, prompt)
			ch <- result{resp, err}
		}(a)
	}

	var lastErr error
	for range agents {
		r := <-ch
		if r.err == nil {
			cancel() // cancel remaining
			return r.resp, nil
		}
		lastErr = r.err
	}

	return "", fmt.Errorf("race: all %d agents failed, last: %w", len(agents), lastErr)
}

// Orchestrate sends the prompt to all agents in parallel, returns the first response.
func (r Race) Orchestrate(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	selected := r.Select(ctx, agents)
	return r.Execute(ctx, prompt, selected)
}
