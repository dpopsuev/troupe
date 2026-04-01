// strategy_dialectic.go — Dialectic: thesis-antithesis ping-pong convergence.
//
// The default CollectiveStrategy. Thesis drafts, antithesis challenges,
// thesis revises. Repeat until antithesis says CONVERGED or max rounds
// exhausted. Thesis has last word — it naturally produces the synthesis.
package collective

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dpopsuev/jericho/agent"
)

// ErrTooFewAgentsDialectic is returned when dialectic has fewer than 2 agents.
var ErrTooFewAgentsDialectic = errors.New("dialectic requires at least 2 agents")

// Dialectic is the default CollectiveStrategy: thesis-antithesis ping-pong.
type Dialectic struct {
	MaxRounds       int    // default 3
	ConvergenceWord string // default "CONVERGED"
}

func (d *Dialectic) defaults() (maxRounds int, convergenceWord string) {
	maxRounds = d.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}
	convergenceWord = d.ConvergenceWord
	if convergenceWord == "" {
		convergenceWord = "CONVERGED"
	}
	return maxRounds, convergenceWord
}

// Select returns the first two agents (thesis + antithesis).
func (*Dialectic) Select(_ context.Context, agents []*agent.Solo) []*agent.Solo {
	if len(agents) < 2 { //nolint:mnd // dialectic requires at least 2
		return agents
	}
	return agents[:2]
}

// Execute runs the dialectic debate between agents[0] (thesis) and
// agents[1] (antithesis). Returns the thesis's last response as synthesis.
func (d *Dialectic) Execute(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	if len(agents) < 2 { //nolint:mnd // dialectic requires at least 2
		return "", fmt.Errorf("%w, got %d", ErrTooFewAgentsDialectic, len(agents))
	}

	maxRounds, convergenceWord := d.defaults()
	thesis, anti := agents[0], agents[1]

	thesisResp, err := thesis.Ask(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("thesis initial draft: %w", err)
	}

	for round := range maxRounds {
		antiPrompt := fmt.Sprintf(
			"Original request:\n%s\n\nThesis response (round %d):\n%s\n\n"+
				"Challenge this response. Identify flaws, missing considerations, "+
				"and alternatives. If the response adequately addresses all concerns, "+
				"respond with exactly %s.",
			prompt, round+1, thesisResp, convergenceWord,
		)
		antiResp, err := anti.Ask(ctx, antiPrompt)
		if err != nil {
			return thesisResp, nil
		}

		if strings.Contains(antiResp, convergenceWord) {
			return thesisResp, nil
		}

		revisePrompt := fmt.Sprintf(
			"Original request:\n%s\n\nYour previous response:\n%s\n\n"+
				"Critique received:\n%s\n\n"+
				"Revise your response to address valid points while defending correct positions.",
			prompt, thesisResp, antiResp,
		)
		revised, err := thesis.Ask(ctx, revisePrompt)
		if err != nil {
			return thesisResp, nil
		}
		thesisResp = revised
	}

	return thesisResp, nil
}

// Orchestrate runs the dialectic debate. Composes Select + Execute.
func (d *Dialectic) Orchestrate(ctx context.Context, prompt string, agents []*agent.Solo) (string, error) {
	selected := d.Select(ctx, agents)
	return d.Execute(ctx, prompt, selected)
}
