package troupe

import (
	"context"
	"time"

	"github.com/dpopsuev/troupe/world"
)

// Admin is the privileged control plane for operators managing the World.
// Separate from Broker (agent-facing) — Admin is for humans and automation
// that supervise agent populations.
type Admin interface {
	// --- Query ---

	// Agents returns all agents matching the filter.
	Agents(ctx context.Context, filter AgentFilter) []AgentDetail

	// Inspect returns full state for a single agent.
	Inspect(ctx context.Context, id world.EntityID) (AgentDetail, error)

	// Tree returns the agent supervision hierarchy.
	Tree(ctx context.Context) []TreeNode

	// --- Lifecycle ---

	// Kill terminates an agent immediately with a reason for audit.
	Kill(ctx context.Context, id world.EntityID, reason string) error

	// Drain marks an agent as not accepting new work by setting
	// Ready{Ready: false, Reason: ReasonDrained}. Reversible via Undrain.
	Drain(ctx context.Context, id world.EntityID) error

	// Undrain re-enables work acceptance by setting
	// Ready{Ready: true, Reason: ReasonIdle}.
	Undrain(ctx context.Context, id world.EntityID) error

	// --- Policy ---

	// SetBudget updates the cost ceiling for an agent. Zero removes the limit.
	SetBudget(ctx context.Context, id world.EntityID, ceiling float64) error

	// SetQuota sets the maximum number of agents the World will admit.
	// Zero removes the limit.
	SetQuota(ctx context.Context, max int) error

	// --- Emergency ---

	// Cordon stops the World from admitting new agents or dispatching
	// new work. Running work finishes. Reason is logged to ControlLog.
	Cordon(ctx context.Context, reason string) error

	// Uncordon lifts a cordon, resuming normal admission and dispatch.
	Uncordon(ctx context.Context) error

	// KillAll terminates all agents. Nuclear option.
	KillAll(ctx context.Context, reason string) error
}

// AgentFilter selects agents for the Agents query.
type AgentFilter struct {
	Role  string `json:"role,omitempty"`
	Alive *bool  `json:"alive,omitempty"`
	Ready *bool  `json:"ready,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// AgentDetail is the full operator view of an agent.
type AgentDetail struct {
	ID       world.EntityID   `json:"id"`
	Role     string           `json:"role"`
	Model    string           `json:"model,omitempty"`
	Alive    world.AliveState `json:"alive"`
	Ready    bool             `json:"ready"`
	Reason   string           `json:"reason,omitempty"`
	Since    time.Time        `json:"since"`
	Budget   BudgetView       `json:"budget,omitempty"`
	Children []world.EntityID `json:"children,omitempty"`
	Parent   *world.EntityID  `json:"parent,omitempty"`
}

// BudgetView is the operator-facing cost snapshot for an agent.
type BudgetView struct {
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`
	Ceiling    float64 `json:"ceiling"`
}

// TreeNode is one node in the agent supervision tree.
type TreeNode struct {
	ID       world.EntityID   `json:"id"`
	Role     string           `json:"role"`
	Alive    world.AliveState `json:"alive"`
	Children []TreeNode       `json:"children,omitempty"`
}
