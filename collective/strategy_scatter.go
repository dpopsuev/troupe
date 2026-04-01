// strategy_scatter.go — Scatter: all agents process, collect all responses.
package collective

import (
	"context"
	"strings"
	"sync"

	"github.com/dpopsuev/jericho/agent"
)

// Scatter fans out to all agents and collects ALL responses.
// Responses are joined with Separator (default newline).
type Scatter struct {
	Separator string // default "\n"
}

// Select returns all agents — Scatter fans out to everyone.
func (*Scatter) Select(_ context.Context, agents []*agent.Solo) []*agent.Solo {
	return agents
}

// Execute sends the prompt to all agents concurrently and joins responses.
func (s *Scatter) Execute(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	if len(agents) == 0 {
		return "", ErrNoAgents
	}

	sep := s.Separator
	if sep == "" {
		sep = "\n"
	}

	type indexed struct {
		idx  int
		resp string
		err  error
	}

	var wg sync.WaitGroup
	results := make([]indexed, len(agents))

	for i, a := range agents {
		wg.Add(1)
		go func(idx int, ag *agent.Solo) {
			defer wg.Done()
			resp, err := ag.Ask(ctx, prompt)
			results[idx] = indexed{idx, resp, err}
		}(i, a)
	}
	wg.Wait()

	var parts []string
	for _, r := range results {
		if r.err == nil {
			parts = append(parts, r.resp)
		}
	}

	if len(parts) == 0 {
		return "", results[0].err
	}

	return strings.Join(parts, sep), nil
}

// Orchestrate sends the prompt to all agents concurrently and joins responses.
func (s *Scatter) Orchestrate(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	selected := s.Select(ctx, agents)
	return s.Execute(ctx, prompt, selected)
}
