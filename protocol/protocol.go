// Package protocol defines wire types for Troupe's agent-to-agent communication.
// These are the public contract between server and client SDKs.
// Implementation details (LocalTransport, HTTPTransport) stay in internal/transport/.
package protocol

import "context"

// Performative classifies the intent of a message in agent-to-agent
// communication, following FIPA-ACL speech-act semantics.
type Performative string

const (
	Inform    Performative = "inform"
	Request   Performative = "request"
	Confirm   Performative = "confirm"
	Refuse    Performative = "refuse"
	Handoff   Performative = "handoff"
	Directive Performative = "directive"
)

// AgentID is a typed identifier for agents in the transport layer.
type AgentID string

// Message is the envelope for agent-to-agent communication.
type Message struct {
	From         AgentID           `json:"from"`
	To           AgentID           `json:"to"`
	Performative Performative      `json:"performative"`
	Content      string            `json:"content"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	TraceID      string            `json:"trace_id,omitempty"`
}

// Task represents an in-flight message processing job.
type Task struct {
	ID      string    `json:"id"`
	State   TaskState `json:"state"`
	Result  *Message  `json:"result,omitempty"`
	Error   string    `json:"error,omitempty"`
	History []Message `json:"history,omitempty"`
}

// TaskState is the lifecycle state of a Task.
type TaskState string

const (
	TaskSubmitted TaskState = "submitted"
	TaskWorking   TaskState = "working"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskCanceled  TaskState = "canceled"
)

// Event is a state-change notification for a Task.
type Event struct {
	TaskID string    `json:"task_id"`
	State  TaskState `json:"state"`
	Data   *Message  `json:"data,omitempty"`
}

// MsgHandler processes a received message and returns a response.
type MsgHandler func(ctx context.Context, msg Message) (Message, error)
