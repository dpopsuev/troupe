// collective.go — Collective: N agents behind one agent.Agent interface.
//
// The operator calls Ask() on a collective and gets one response. Internally,
// N agents collaborate via a pluggable CollectiveStrategy (dialectic, arbiter,
// consensus, pipeline). The strategy defines the modus operandi.
package collective

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dpopsuev/jericho"

	"github.com/dpopsuev/jericho/internal/warden"
	"github.com/dpopsuev/jericho/world"
)

// Sentinel errors for collective operations.
var (
	ErrIngressRejected  = errors.New("collective ingress rejected")
	ErrEgressRejected   = errors.New("collective egress rejected")
	ErrNoAgents         = errors.New("collective: no agents")
	ErrMaxSizeExceeded  = errors.New("collective: max size exceeded")
	ErrDisruptionBudget = errors.New("collective: disruption budget violated")
)

// CollectiveStrategy defines how agents collaborate inside a collective.
// Composed of Selector (which agents) + Executor (how they coordinate).
// Implementations should also implement Selector and Executor individually
// so consumers can reuse selection logic without triggering execution.
type CollectiveStrategy interface {
	Orchestrate(ctx context.Context, prompt string, agents []jericho.Actor) (string, error)
}

// Selector picks which agents handle a unit of work.
// Pure decision — no side effects, no execution.
type Selector interface {
	Select(ctx context.Context, agents []jericho.Actor) []jericho.Actor
}

// Executor coordinates selected agents to produce a response.
type Executor interface {
	Execute(ctx context.Context, prompt string, agents []jericho.Actor) (string, error)
}

// DebateRound records one debate round between agents.
type DebateRound struct {
	ThesisResponse     string
	AntithesisResponse string
	Converged          bool
}

// Phase represents the lifecycle state of a collective.
type Phase string

const (
	PhasePending   Phase = "pending"   // spawning agents
	PhaseRunning   Phase = "running"   // operational
	PhaseSucceeded Phase = "succeeded" // all done cleanly
	PhaseFailed    Phase = "failed"    // error during operation
)

// Collective wraps N agents behind the agent.Agent interface.
// Operators see one agent. Internally, N agents debate/collaborate.
type Collective struct {
	id           world.EntityID
	role         string
	agents       []jericho.Actor
	strategy     CollectiveStrategy
	ingress      Gatekeeper // optional bouncer (nil = pass-through)
	egress       Gatekeeper // optional reviewer (nil = pass-through)
	mu           sync.RWMutex
	rounds       []DebateRound
	phase        Phase
	maxSize      int // 0 = unlimited
	minAvailable int // 0 = no disruption budget
}

// CollectiveOption configures an Collective.
type CollectiveOption func(*Collective)

// WithIngress sets the ingress gate (bouncer).
func WithIngress(g Gatekeeper) CollectiveOption {
	return func(c *Collective) { c.ingress = g }
}

// WithEgress sets the egress gate (reviewer).
func WithEgress(g Gatekeeper) CollectiveOption {
	return func(c *Collective) { c.egress = g }
}

// WithMaxSize sets the maximum number of agents in the collective.
func WithMaxSize(n int) CollectiveOption {
	return func(c *Collective) { c.maxSize = n }
}

// WithMinAvailable sets the minimum number of agents that must remain
// available during scale-down or kill operations (disruption budget).
func WithMinAvailable(n int) CollectiveOption {
	return func(c *Collective) { c.minAvailable = n }
}

// NewCollective creates a collective from existing agent handles.
func NewCollective(id world.EntityID, role string, strategy CollectiveStrategy, agents []jericho.Actor, opts ...CollectiveOption) *Collective {
	c := &Collective{
		id:       id,
		role:     role,
		strategy: strategy,
		agents:   agents,
		phase:    PhaseRunning,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Identity ---

func (c *Collective) ID() world.EntityID { return c.id }
func (c *Collective) Role() string       { return c.role }
func (c *Collective) String() string {
	return fmt.Sprintf("%s(collective-%d, %d agents)", c.role, c.id, len(c.agents))
}

// --- Messaging ---

// Ask runs the collective strategy and returns the synthesized response.
// Applies ingress gate before entry and egress gate before exit.
func (c *Collective) Perform(ctx context.Context, content string) (string, error) {
	// Ingress gate — bouncer decides if prompt enters the room.
	if c.ingress != nil {
		ok, reason, err := c.ingress.Pass(ctx, content)
		if err != nil {
			return "", fmt.Errorf("collective %s ingress: %w", c.role, err)
		}
		if !ok {
			return "", fmt.Errorf("%w: %s %s", ErrIngressRejected, c.role, reason)
		}
	}

	// The room — agents debate/collaborate.
	result, err := c.strategy.Orchestrate(ctx, content, c.agents)
	if err != nil {
		return "", fmt.Errorf("collective %s: %w", c.role, err)
	}

	// Egress gate — reviewer decides if response exits the room.
	if c.egress != nil {
		ok, reason, err := c.egress.Pass(ctx, result)
		if err != nil {
			return "", fmt.Errorf("collective %s egress: %w", c.role, err)
		}
		if !ok {
			return "", fmt.Errorf("%w: %s %s", ErrEgressRejected, c.role, reason)
		}
	}

	return result, nil
}

// --- Lifecycle ---

// Kill stops all internal agents and transitions to Succeeded or Failed.
func (c *Collective) Kill(ctx context.Context) error {
	var firstErr error
	for _, a := range c.agents {
		if err := a.Kill(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	c.mu.Lock()
	if firstErr != nil {
		c.phase = PhaseFailed
	} else {
		c.phase = PhaseSucceeded
	}
	c.mu.Unlock()
	return firstErr
}

// Ready returns true if all agents are ready.
func (c *Collective) Ready() bool {
	for _, a := range c.agents {
		if !a.Ready() {
			return false
		}
	}
	return len(c.agents) > 0
}

// Children returns the internal agents (visible in full view).
func (c *Collective) Children() []jericho.Actor {
	return c.agents
}

// Parent returns nil — collectives are collectives have no parent.
func (c *Collective) Parent() jericho.Actor {
	return nil
}

// Progress returns the debate progress: current round / max rounds.
func (c *Collective) Progress() (world.Progress, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.rounds) == 0 {
		return world.Progress{}, false
	}
	return world.Progress{Current: len(c.rounds), Total: len(c.rounds)}, true
}

// SetProgress is a no-op for collectives (progress is driven by rounds).
func (c *Collective) SetProgress(_, _ int) {}

// --- FacadeAgent ---

// InternalAgents returns the agents hidden behind the agent.
func (c *Collective) InternalAgents() []jericho.Actor {
	return c.agents
}

// IsFacade returns true — this is a collective, not a single agent.
func (c *Collective) IsFacade() bool { return true }

// DebateRounds returns the debate history.
func (c *Collective) DebateRounds() []DebateRound {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]DebateRound, len(c.rounds))
	copy(out, c.rounds)
	return out
}

// addDebateRound appends a round to the debate history.
func (c *Collective) addDebateRound(r DebateRound) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rounds = append(c.rounds, r)
}

// Phase returns the current lifecycle phase.
func (c *Collective) Phase() Phase {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.phase
}

// Scale adjusts the number of agents to the target count.
// Spawns new agents or kills excess agents as needed.
func (c *Collective) Scale(ctx context.Context, target int, config warden.AgentConfig, broker jericho.Broker) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	current := len(c.agents)
	if target == current {
		return nil
	}

	if c.maxSize > 0 && target > c.maxSize {
		return fmt.Errorf("%w: target %d exceeds max %d", ErrMaxSizeExceeded, target, c.maxSize)
	}
	if c.minAvailable > 0 && target < c.minAvailable {
		return fmt.Errorf("%w: target %d below minimum %d", ErrDisruptionBudget, target, c.minAvailable)
	}

	if target > current {
		// Scale up — spawn new agents.
		for range target - current {
			a, err := broker.Spawn(ctx, jericho.ActorConfig{Model: config.Model, Role: config.Role})
			if err != nil {
				return fmt.Errorf("collective scale up: %w", err)
			}
			c.agents = append(c.agents, a)
		}
	} else {
		// Scale down — kill excess agents (from the end).
		for i := current - 1; i >= target; i-- {
			c.agents[i].Kill(ctx) //nolint:errcheck // best-effort during scale down
		}
		c.agents = c.agents[:target]
	}

	return nil
}

// Compile-time check: Collective implements jericho.Actor.
var _ jericho.Actor = (*Collective)(nil)
