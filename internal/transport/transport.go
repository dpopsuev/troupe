// Package transport defines the A2A transport abstraction for agent-to-agent
// communication. LocalTransport provides in-process channel-based messaging;
// HTTPTransport connects to remote A2A agents via a2a-go SDK.
package transport

import "github.com/dpopsuev/troupe/protocol"

// Re-export wire types from protocol/ for backwards compatibility.
// Internal code continues using transport.Message etc.
type (
	AgentID    = protocol.AgentID
	Message    = protocol.Message
	Task       = protocol.Task
	TaskState  = protocol.TaskState
	Event      = protocol.Event
	MsgHandler = protocol.MsgHandler
)

const (
	TaskSubmitted = protocol.TaskSubmitted
	TaskWorking   = protocol.TaskWorking
	TaskCompleted = protocol.TaskCompleted
	TaskFailed    = protocol.TaskFailed
	TaskCanceled  = protocol.TaskCanceled
)
