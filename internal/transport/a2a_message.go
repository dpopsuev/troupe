package transport

import (
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/dpopsuev/troupe/signal"
)

// FromA2AMessage converts an A2A v1.0 Message to an internal transport Message.
func FromA2AMessage(msg a2a.Message, to AgentID) Message {
	var content string
	for _, part := range msg.Parts {
		if tp, ok := part.(*a2a.TextPart); ok {
			content = tp.Text
			break
		}
	}

	perf := signal.Request
	if msg.Role == a2a.MessageRoleAgent {
		perf = signal.Inform
	}

	return Message{
		To:           to,
		Performative: perf,
		Content:      content,
	}
}

// ToA2AMessage converts an internal transport Message to an A2A v1.0 Message.
func ToA2AMessage(msg Message) a2a.Message {
	role := a2a.MessageRoleAgent
	if msg.Performative == signal.Request || msg.Performative == signal.Directive {
		role = a2a.MessageRoleUser
	}

	return a2a.Message{
		Role: role,
		Parts: a2a.ContentParts{
			&a2a.TextPart{Text: msg.Content},
		},
	}
}
