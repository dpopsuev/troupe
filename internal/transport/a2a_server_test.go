package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"github.com/dpopsuev/troupe/auth"
)

func testCard(url string) a2a.AgentCard {
	return a2a.AgentCard{
		Name:               "test-agent",
		Version:            "1.0.0",
		ProtocolVersion:    "1.0",
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		URL:                url,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []a2a.AgentSkill{{
			ID:   "echo",
			Name: "echo",
		}},
	}
}

func TestA2AServer_MessageSendRoundTrip(t *testing.T) {
	tr := NewA2ATransport(testCard("http://localhost"))
	defer tr.Close()

	_ = tr.Register("agent-1", func(_ context.Context, msg Message) (Message, error) {
		return Message{Content: "echo: " + msg.Content, Role: "agent"}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	card := testCard(ts.URL)
	client, err := a2aclient.NewFromCard(context.Background(), &card, a2aclient.WithJSONRPCTransport(http.DefaultClient))
	if err != nil {
		t.Fatalf("NewFromCard: %v", err)
	}

	result, err := client.SendMessage(context.Background(), &a2a.MessageSendParams{
		Message: a2a.NewMessage(a2a.MessageRoleUser, &a2a.TextPart{Text: "hello"}),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	task, ok := result.(*a2a.Task)
	if !ok {
		t.Fatalf("result type = %T, want *a2a.Task", result)
	}

	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("task state = %s, want completed", task.Status.State)
	}

	t.Logf("Task status: state=%s", task.Status.State)
	if task.Status.Message != nil {
		t.Logf("Status message role=%s parts=%d", task.Status.Message.Role, len(task.Status.Message.Parts))
		for i, part := range task.Status.Message.Parts {
			t.Logf("  part[%d] type=%T", i, part)
			if tp, ok := part.(*a2a.TextPart); ok {
				t.Logf("  part[%d] text=%q", i, tp.Text)
			}
		}
	} else {
		t.Log("Status message is nil")
	}

	if len(task.History) > 0 {
		t.Logf("History: %d messages", len(task.History))
		for i, m := range task.History {
			t.Logf("  history[%d] role=%s parts=%d", i, m.Role, len(m.Parts))
		}
	}

	var content string
	if task.Status.Message != nil {
		content = extractText(task.Status.Message.Parts)
	}
	if content == "" && len(task.History) > 0 {
		content = extractText(task.History[len(task.History)-1].Parts)
	}

	if content == "" {
		t.Fatal("no content in status message or history")
	}
	t.Logf("A2A round-trip: %s", content)
}

func TestA2AServer_AgentCardDiscovery(t *testing.T) {
	tr := NewA2ATransport(testCard("http://localhost"))
	defer tr.Close()

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	resolver := agentcard.NewResolver(http.DefaultClient)
	resolved, err := resolver.Resolve(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Name != "test-agent" {
		t.Fatalf("name = %q, want test-agent", resolved.Name)
	}
	if resolved.ProtocolVersion != "1.0" {
		t.Fatalf("protocol = %q, want 1.0", resolved.ProtocolVersion)
	}
	if len(resolved.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(resolved.Skills))
	}
}

func TestA2AServer_StreamingRoundTrip(t *testing.T) {
	tr := NewA2ATransport(testCard("http://localhost"))
	defer tr.Close()

	_ = tr.Register("agent-1", func(_ context.Context, msg Message) (Message, error) {
		return Message{Content: "streamed: " + msg.Content, Role: "agent"}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	card := testCard(ts.URL)
	client, err := a2aclient.NewFromCard(context.Background(), &card, a2aclient.WithJSONRPCTransport(http.DefaultClient))
	if err != nil {
		t.Fatalf("NewFromCard: %v", err)
	}

	var events []a2a.Event
	for ev, err := range client.SendStreamingMessage(context.Background(), &a2a.MessageSendParams{
		Message: a2a.NewMessage(a2a.MessageRoleUser, &a2a.TextPart{Text: "stream test"}),
	}) {
		if err != nil {
			t.Fatalf("streaming error: %v", err)
		}
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatal("no streaming events received")
	}

	if len(events) == 0 {
		t.Fatal("no streaming events received")
	}
	t.Logf("Streaming: %d events", len(events))
	for i, ev := range events {
		t.Logf("  event[%d] type=%T", i, ev)
	}

	// Final event should be a completed Task.
	last := events[len(events)-1]
	task, ok := last.(*a2a.Task)
	if !ok {
		t.Fatalf("last event type = %T, want *a2a.Task", last)
	}
	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %s, want completed", task.Status.State)
	}
	content := extractText(task.Status.Message.Parts)
	if content == "" {
		t.Fatal("streamed response content is empty")
	}
	t.Logf("Streaming response: %s", content)
}

func TestA2AServer_BearerAuth_Rejected(t *testing.T) {
	tr := NewA2ATransport(testCard("http://localhost"), "secret-token-123")
	defer tr.Close()

	_ = tr.Register("agent-1", func(_ context.Context, msg Message) (Message, error) {
		return Message{Content: "secret"}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	// No auth header — should get 401.
	resp, err := http.Post(ts.URL, "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestA2AServer_BearerAuth_CardIsPublic(t *testing.T) {
	tr := NewA2ATransport(testCard("http://localhost"), "secret-token-123")
	defer tr.Close()

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	// Agent card should be accessible without auth.
	resolver := agentcard.NewResolver(http.DefaultClient)
	card, err := resolver.Resolve(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Resolve without auth: %v", err)
	}
	if card.Name != "test-agent" {
		t.Fatalf("name = %q, want test-agent", card.Name)
	}
}

func TestA2AServer_AuthenticatorMiddleware_RejectsInvalid(t *testing.T) {
	authn := &stubAuthenticator{valid: "good-token"}
	card := testCard("http://localhost")
	tr := NewA2ATransportWithAuth(&card, authn)
	defer tr.Close()

	_ = tr.Register("agent-1", func(_ context.Context, msg Message) (Message, error) {
		return Message{Content: "protected"}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest("POST", ts.URL, http.NoBody)
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token: status = %d, want 403", resp.StatusCode)
	}
}

type stubAuthenticator struct {
	valid string
}

func (s *stubAuthenticator) Authenticate(_ context.Context, token string) (auth.Identity, error) {
	if token == s.valid {
		return auth.Identity{Subject: "test-user"}, nil
	}
	return auth.Identity{}, auth.ErrInvalidToken
}

func extractText(parts a2a.ContentParts) string {
	for _, part := range parts {
		switch tp := part.(type) {
		case *a2a.TextPart:
			return tp.Text
		case a2a.TextPart:
			return tp.Text
		}
	}
	return ""
}
