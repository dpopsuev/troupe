package bugle

// Action identifies a Bugle Protocol operation.
type Action string

// Core actions (Layer 0 — required).
const (
	ActionStart  Action = "start"
	ActionPull   Action = "pull"
	ActionPush   Action = "push"
	ActionCancel Action = "cancel"
)

// Observability actions (Layer 4 — recommended).
const (
	ActionStatus Action = "status"
)

// HITL actions (Layer 3 — optional).
const (
	ActionCordon   Action = "cordon"
	ActionUncordon Action = "uncordon"
)

// DefaultToolName is the MCP tool name for the Bugle Protocol.
const DefaultToolName = "bugle"

// DefaultSessionKey is the standard key for session identification.
const DefaultSessionKey = "session_id"
