package transport

import (
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/dpopsuev/troupe/signal"
)

func TestFromA2AMessage_TextPart(t *testing.T) {
	a2aMsg := a2a.Message{
		Role: a2a.MessageRoleUser,
		Parts: a2a.ContentParts{
			&a2a.TextPart{Text: "hello world"},
		},
	}

	msg := FromA2AMessage(a2aMsg, "agent-1")
	if msg.Content != "hello world" {
		t.Fatalf("Content = %q, want hello world", msg.Content)
	}
	if msg.Performative != signal.Request {
		t.Fatalf("Performative = %q, want request", msg.Performative)
	}
	if msg.To != "agent-1" {
		t.Fatalf("To = %q, want agent-1", msg.To)
	}
}

func TestFromA2AMessage_AgentRole(t *testing.T) {
	a2aMsg := a2a.Message{
		Role: a2a.MessageRoleAgent,
		Parts: a2a.ContentParts{
			&a2a.TextPart{Text: "response"},
		},
	}

	msg := FromA2AMessage(a2aMsg, "agent-2")
	if msg.Performative != signal.Inform {
		t.Fatalf("Performative = %q, want inform for agent role", msg.Performative)
	}
}

func TestToA2AMessage_Request(t *testing.T) {
	msg := Message{
		Content:      "analyze this",
		Performative: signal.Request,
	}

	a2aMsg := ToA2AMessage(msg)
	if a2aMsg.Role != a2a.MessageRoleUser {
		t.Fatalf("Role = %q, want user", a2aMsg.Role)
	}
	if len(a2aMsg.Parts) != 1 {
		t.Fatalf("Parts count = %d, want 1", len(a2aMsg.Parts))
	}
	tp, ok := a2aMsg.Parts[0].(*a2a.TextPart)
	if !ok {
		t.Fatal("Part should be TextPart")
	}
	if tp.Text != "analyze this" {
		t.Fatalf("Text = %q, want analyze this", tp.Text)
	}
}

func TestToA2AMessage_Inform(t *testing.T) {
	msg := Message{
		Content:      "result",
		Performative: signal.Inform,
	}

	a2aMsg := ToA2AMessage(msg)
	if a2aMsg.Role != a2a.MessageRoleAgent {
		t.Fatalf("Role = %q, want agent", a2aMsg.Role)
	}
}

func TestRoundTrip_FromToFrom(t *testing.T) {
	original := Message{
		Content:      "round trip test",
		Performative: signal.Request,
		To:           "target",
	}

	a2aMsg := ToA2AMessage(original)
	recovered := FromA2AMessage(a2aMsg, "target")

	if recovered.Content != original.Content {
		t.Fatalf("Content = %q, want %q", recovered.Content, original.Content)
	}
	if recovered.Performative != original.Performative {
		t.Fatalf("Performative = %q, want %q", recovered.Performative, original.Performative)
	}
}

func TestFromA2AMessage_EmptyParts(t *testing.T) {
	a2aMsg := a2a.Message{
		Role:  a2a.MessageRoleUser,
		Parts: a2a.ContentParts{},
	}

	msg := FromA2AMessage(a2aMsg, "agent-1")
	if msg.Content != "" {
		t.Fatalf("Content = %q, want empty for no parts", msg.Content)
	}
}

func TestFromA2AMessage_NilParts(t *testing.T) {
	a2aMsg := a2a.Message{
		Role:  a2a.MessageRoleUser,
		Parts: nil,
	}

	msg := FromA2AMessage(a2aMsg, "agent-1")
	if msg.Content != "" {
		t.Fatalf("Content = %q, want empty for nil parts", msg.Content)
	}
}

func TestFromA2AMessage_MultiPart_TakesFirstText(t *testing.T) {
	a2aMsg := a2a.Message{
		Role: a2a.MessageRoleUser,
		Parts: a2a.ContentParts{
			&a2a.DataPart{Data: map[string]any{"key": "value"}},
			&a2a.TextPart{Text: "first text"},
			&a2a.TextPart{Text: "second text"},
		},
	}

	msg := FromA2AMessage(a2aMsg, "agent-1")
	if msg.Content != "first text" {
		t.Fatalf("Content = %q, want first text (skips DataPart)", msg.Content)
	}
}
