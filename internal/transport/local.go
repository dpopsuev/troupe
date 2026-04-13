package transport

// Sentinel errors for transport operations.
// Defined here (not in base.go) to keep them importable from the
// canonical location where they've always lived.

import "errors"

var (
	ErrTransportClosed   = errors.New("transport: closed")
	ErrAgentNotFound     = errors.New("transport: agent not registered")
	ErrTaskNotFound      = errors.New("transport: task not found")
	ErrTaskChanClosed    = errors.New("transport: task channel closed without terminal state")
	ErrTaskFailed        = errors.New("transport: task failed")
	ErrNoAgentsForRole   = errors.New("transport: no agents for role")
	ErrAlreadyRegistered = errors.New("transport: agent already registered")
)

// LocalTransport is an in-process, channel-based A2A transport.
// Embeds baseTransport for all task management. No HTTP — pure Go channels.
type LocalTransport struct {
	baseTransport
}

// NewLocalTransport creates a new in-process transport.
func NewLocalTransport() *LocalTransport {
	return &LocalTransport{baseTransport: newBase()}
}
