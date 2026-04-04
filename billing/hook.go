package billing

import (
	"context"

	"github.com/dpopsuev/troupe"
)

// BudgetHook implements troupe.SpawnHook to enforce budget limits before spawning.
type BudgetHook struct {
	enforcer *BudgetEnforcer
}

// NewBudgetHook creates a hook that checks budget before each spawn.
func NewBudgetHook(e *BudgetEnforcer) *BudgetHook {
	return &BudgetHook{enforcer: e}
}

// Name returns the hook identifier.
func (h *BudgetHook) Name() string { return "budget" }

// PreSpawn checks the budget enforcer for the actor's role.
func (h *BudgetHook) PreSpawn(_ context.Context, config troupe.ActorConfig) error {
	return h.enforcer.Check(config.Role)
}

// PostSpawn is a no-op observer.
func (h *BudgetHook) PostSpawn(_ context.Context, _ troupe.ActorConfig, _ troupe.Actor, _ error) {}

var _ troupe.SpawnHook = (*BudgetHook)(nil)
