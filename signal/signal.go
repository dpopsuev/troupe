package signal

import "github.com/dpopsuev/troupe/protocol"

// Re-export Performative from protocol/ for backwards compatibility.
type Performative = protocol.Performative

const (
	Inform    = protocol.Inform
	Request   = protocol.Request
	Confirm   = protocol.Confirm
	Refuse    = protocol.Refuse
	Handoff   = protocol.Handoff
	Directive = protocol.Directive
)

// Signal represents a single event on the agent message bus.
type Signal struct {
	Timestamp    string            `json:"ts"`
	Event        string            `json:"event"`
	Agent        string            `json:"agent"`
	CaseID       string            `json:"case_id,omitempty"`
	Step         string            `json:"step,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
	Performative Performative      `json:"performative,omitempty"`
}
