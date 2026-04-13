package transport

import "context"

// Transport is the interface for agent-to-agent message passing.
// LocalTransport implements it in-process via channels.
// HTTPTransport implements it over HTTP/SSE.
type Transport interface {
	// Register associates a MsgHandler with the given agent ID.
	Register(agentID AgentID, handler MsgHandler) error

	// Unregister removes the handler for the given agent ID.
	Unregister(agentID AgentID)

	// SendMessage dispatches a message to the named agent.
	// Returns a Task that tracks the lifecycle.
	SendMessage(ctx context.Context, to AgentID, msg Message) (*Task, error)

	// Subscribe returns a channel that receives Events for a task.
	Subscribe(ctx context.Context, taskID string) (<-chan Event, error)

	// Ask sends a message and blocks until the response or context cancellation.
	Ask(ctx context.Context, to AgentID, msg Message) (Message, error)

	// SendToRole sends a message to one agent with the given role (round-robin).
	SendToRole(ctx context.Context, role string, msg Message) (*Task, error)

	// AskRole sends to one agent by role and blocks until response.
	AskRole(ctx context.Context, role string, msg Message) (Message, error)

	// Broadcast sends a message to ALL agents with the given role.
	Broadcast(ctx context.Context, role string, msg Message) ([]*Task, error)

	// Close shuts down the transport.
	Close() error

	// Roles returns the role registry for role-based routing.
	Roles() *RoleRegistry
}

// Verify both transports implement the interface.
var (
	_ Transport = (*LocalTransport)(nil)
	_ Transport = (*HTTPTransport)(nil)
)
