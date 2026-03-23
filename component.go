package bugle

import (
	"fmt"
	"time"
)

// Component type constants.
const (
	ColorIdentityType ComponentType = "color_identity"
	HealthType        ComponentType = "health"
	HierarchyType     ComponentType = "hierarchy"
	BudgetType        ComponentType = "budget"
	ProgressType      ComponentType = "progress"
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

// ColorIdentity is the visual identity for humans.
// Format: "Denim Writer of Indigo Refactor" (Colour Role of Shade Collective).
type ColorIdentity struct {
	Shade      string `json:"shade"`      // group family: "Indigo", "Crimson"
	Colour     string `json:"colour"`     // individual: "Denim", "Scarlet"
	Role       string `json:"role"`       // function: "Writer", "Reviewer"
	Collective string `json:"collective"` // formation: "Refactor", "Triage"
	Hex        string `json:"hex"`        // CSS hex: "#6F8FAF"
}

func (ColorIdentity) componentType() ComponentType { return ColorIdentityType }

// Title returns the heraldic name: "Denim Writer of Indigo Refactor".
func (c ColorIdentity) Title() string { //nolint:gocritic // value receiver needed for ECS Get[T]
	return fmt.Sprintf("%s %s of %s %s", c.Colour, c.Role, c.Shade, c.Collective)
}

// Label returns the compact log format: "[Indigo·Denim|Writer]".
func (c ColorIdentity) Label() string { //nolint:gocritic // value receiver needed for ECS Get[T]
	return fmt.Sprintf("[%s·%s|%s]", c.Shade, c.Colour, c.Role)
}

// Short returns just the colour name: "Denim".
func (c ColorIdentity) Short() string { return c.Colour } //nolint:gocritic // value receiver

// Health tracks agent liveness and status.
type Health struct {
	State    AgentState `json:"state"`
	LastSeen time.Time  `json:"last_seen"`
	Error    string     `json:"error,omitempty"`
}

func (Health) componentType() ComponentType { return HealthType }

// Hierarchy represents a parent-child relationship (collective owns agents).
type Hierarchy struct {
	Parent EntityID `json:"parent"` // 0 = root / no parent
}

func (Hierarchy) componentType() ComponentType { return HierarchyType }

// Budget tracks cost per entity.
type Budget struct {
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`
	Ceiling    float64 `json:"ceiling"`
}

func (Budget) componentType() ComponentType { return BudgetType }

// Progress tracks task completion.
type Progress struct {
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

func (Progress) componentType() ComponentType { return ProgressType }
