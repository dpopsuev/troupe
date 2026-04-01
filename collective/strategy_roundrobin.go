// strategy_roundrobin.go — RoundRobin: stateless load distribution with health filtering.
package collective

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/dpopsuev/jericho/agent"
)

// ErrNoHealthyAgents is returned when all agents are not ready.
var ErrNoHealthyAgents = errors.New("roundrobin: no healthy agents available")

// RoundRobin picks one healthy agent per request via atomic round-robin index.
// Agents where IsReady() returns false are skipped.
type RoundRobin struct {
	idx atomic.Uint64
}

// Select picks the next healthy agent via round-robin index.
func (r *RoundRobin) Select(_ context.Context, agents []*agent.Solo) []*agent.Solo {
	if len(agents) == 0 {
		return nil
	}

	start := r.idx.Add(1) - 1
	n := uint64(len(agents))

	for i := range uint64(len(agents)) {
		candidate := agents[(start+i)%n]
		if candidate.IsReady() {
			return []*agent.Solo{candidate}
		}
	}

	return nil
}

// Execute forwards the prompt to the selected agent.
func (*RoundRobin) Execute(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	if len(agents) == 0 {
		return "", ErrNoHealthyAgents
	}
	return agents[0].Ask(ctx, prompt)
}

// Orchestrate picks the next healthy agent and forwards the prompt.
func (r *RoundRobin) Orchestrate(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	selected := r.Select(ctx, agents)
	if len(selected) == 0 {
		return "", ErrNoHealthyAgents
	}
	return r.Execute(ctx, prompt, selected)
}
