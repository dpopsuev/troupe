package bugle

import (
	"time"

	"github.com/dpopsuev/bugle/world"
)

// Component type constants.
const (
	HealthType    world.ComponentType = "health"
	HierarchyType world.ComponentType = "hierarchy"
	BudgetType    world.ComponentType = "budget"
	ProgressType  world.ComponentType = "progress"
)

// AgentState represents the liveness state of an agent.
type AgentState string

const (
	Active  AgentState = "active"
	Idle    AgentState = "idle"
	Stale   AgentState = "stale"
	Errored AgentState = "errored"
	Done    AgentState = "done"
)

// Health tracks agent liveness and status.
type Health struct {
	State    AgentState `json:"state"`
	LastSeen time.Time  `json:"last_seen"`
	Error    string     `json:"error,omitempty"`
}

// ComponentType implements world.Component.
func (Health) ComponentType() world.ComponentType { return HealthType }

// Hierarchy represents a parent-child relationship (collective owns agents).
type Hierarchy struct {
	Parent world.EntityID `json:"parent"` // 0 = root / no parent
}

// ComponentType implements world.Component.
func (Hierarchy) ComponentType() world.ComponentType { return HierarchyType }

// Budget tracks cost per entity.
type Budget struct {
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`
	Ceiling    float64 `json:"ceiling"`
}

// ComponentType implements world.Component.
func (Budget) ComponentType() world.ComponentType { return BudgetType }

// Progress tracks task completion.
type Progress struct {
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// ComponentType implements world.Component.
func (Progress) ComponentType() world.ComponentType { return ProgressType }
