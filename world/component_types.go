package world

import "time"

// Component type constants for core components.
const (
	AliveType       ComponentType = "alive"
	ReadyType       ComponentType = "ready"
	BudgetType      ComponentType = "budget"
	ProgressType    ComponentType = "progress"
	DisplayType     ComponentType = "display"
	SpanContextType ComponentType = "span_context"
	ToolCardType    ComponentType = "tool_card"
	AnnotationType  ComponentType = "annotation"
	NamespaceType   ComponentType = "namespace"
)

// Alive tracks whether the agent process exists (liveness probe).
// Attached at fork, state set to Done at kill, detached at despawn.
type Alive struct {
	State    AliveState `json:"state"`
	Since    time.Time  `json:"since"`
	ExitedAt time.Time  `json:"exited_at,omitempty"`
}

// ComponentType implements Component.
func (Alive) ComponentType() ComponentType { return AliveType }

// AliveState is the liveness state of an agent process.
type AliveState string

const (
	AliveStarting   AliveState = "starting"   // process spawned, not yet ready
	AliveRunning    AliveState = "running"    // process exists and has been ready at least once
	AliveTerminated AliveState = "terminated" // process exited
)

// ReadyReason describes why an agent is not ready.
type ReadyReason string

const (
	ReasonIdle        ReadyReason = "idle"
	ReasonStale       ReadyReason = "stale"
	ReasonErrored     ReadyReason = "errored"
	ReasonDrained     ReadyReason = "drained"
	ReasonTerminating ReadyReason = "terminating"
	ReasonTerminated  ReadyReason = "terminated"
)

// Ready tracks whether the agent can accept work (readiness probe).
// Independent of Alive — an agent can be alive but not ready
// (starting up, overloaded, errored).
type Ready struct {
	Ready    bool        `json:"ready"`
	LastSeen time.Time   `json:"last_seen"`
	Reason   ReadyReason `json:"reason,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// ComponentType implements Component.
func (Ready) ComponentType() ComponentType { return ReadyType }

// Budget tracks cost per entity.
type Budget struct {
	TokensUsed int     `json:"tokens_used"`
	Cost       float64 `json:"cost"`
	Ceiling    float64 `json:"ceiling"`
}

// ComponentType implements Component.
func (Budget) ComponentType() ComponentType { return BudgetType }

// Progress tracks task completion.
type Progress struct {
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// ComponentType implements Component.
func (Progress) ComponentType() ComponentType { return ProgressType }

// Display holds human-facing presentation data for an agent.
// Color is a hex string (e.g., "#50C878"). Consumers use this
// for terminal ANSI, web CSS, or IDE badges — the server never
// renders it.
type Display struct {
	Name  string `json:"name"`            // human-friendly name
	Color string `json:"color,omitempty"` // hex color (e.g., "#DC143C")
	Icon  string `json:"icon,omitempty"`  // optional emoji or icon name
}

// ComponentType implements Component.
func (Display) ComponentType() ComponentType { return DisplayType }

// SpanContext carries trace correlation for distributed tracing.
// Attached to agents at spawn — child entities inherit parent's TraceID.
// Consumers query EventLog.ByTraceID(span.TraceID) for the full timeline.
type SpanContext struct {
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// ComponentType implements Component.
func (SpanContext) ComponentType() ComponentType { return SpanContextType }

// Annotation holds operator-supplied metadata for an agent.
type Annotation struct {
	Data map[string]string `json:"data,omitempty"`
}

// ComponentType implements Component.
func (Annotation) ComponentType() ComponentType { return AnnotationType }

// Namespace scopes an entity to a tenant partition.
type Namespace struct {
	Name string `json:"name"`
}

// ComponentType implements Component.
func (Namespace) ComponentType() ComponentType { return NamespaceType }

// IdentityStrategy resolves agent roles into fully-formed entities
// with identity components.
type IdentityStrategy interface {
	Resolve(role, collective string) (EntityID, error)
}
