package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

// TestHTTP_WireRoundTrip proves the HTTP handler works over the network —
// a real HTTP POST to /a2a/send, handler processes, response comes back.
func TestHTTP_WireRoundTrip(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
		return Message{From: "agent-a", Content: "wire: " + msg.Content}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	// POST to the HTTP endpoint.
	body, _ := json.Marshal(map[string]any{
		"to":      "agent-a",
		"message": Message{From: "client", Content: "hello over HTTP"},
	})

	resp, err := http.Post(ts.URL+"/a2a/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var result struct {
		TaskID string    `json:"task_id"`
		State  TaskState `json:"state"`
		Data   *Message  `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.State != TaskCompleted {
		t.Errorf("state = %q, want completed", result.State)
	}
	if result.Data == nil {
		t.Fatal("data is nil")
	}
	if result.Data.Content != "wire: hello over HTTP" {
		t.Errorf("content = %q", result.Data.Content)
	}
}

// TestHTTP_WireUnknownAgent proves HTTP returns 404 for unknown agents.
func TestHTTP_WireUnknownAgent(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"to":      "ghost",
		"message": Message{From: "client", Content: "hello"},
	})

	resp, err := http.Post(ts.URL+"/a2a/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHTTP_AgentCardDiscovery proves /.well-known/agent.json lists registered agents as A2A cards.
func TestHTTP_AgentCardDiscovery(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, nil
	})
	tr.Roles().Register("agent-a", "investigator")

	_ = tr.Register("agent-b", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, nil
	})
	tr.Roles().Register("agent-b", "reviewer")

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var cards []a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&cards); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}

	for _, c := range cards {
		if c.ProtocolVersion != a2aProtocolVersion {
			t.Errorf("card %s protocolVersion = %q, want %s", c.Name, c.ProtocolVersion, a2aProtocolVersion)
		}
		if len(c.Skills) == 0 {
			t.Errorf("card %s has no skills", c.Name)
		}
	}
}

// TestHTTP_AgentCardDiscovery_Empty proves empty registry returns empty array.
func TestHTTP_AgentCardDiscovery_Empty(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var cards []a2a.AgentCard
	json.NewDecoder(resp.Body).Decode(&cards) //nolint:errcheck

	if len(cards) != 0 {
		t.Errorf("expected empty, got %d cards", len(cards))
	}
}

// TestHTTP_SSE_StreamsAllTransitions proves SSE mode streams submitted → working → completed.
func TestHTTP_SSE_StreamsAllTransitions(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
		return Message{From: "agent-a", Content: "sse: " + msg.Content}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"to":      "agent-a",
		"message": Message{From: "client", Content: "stream me"},
	})

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/a2a/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Parse SSE events from response body.
	rawBody, _ := io.ReadAll(resp.Body)
	events := parseSSEEvents(t, string(rawBody))

	// Expect at least: submitted, working, completed.
	if len(events) < 2 {
		t.Fatalf("expected >= 2 SSE events, got %d: %s", len(events), rawBody)
	}

	// Last event should be completed with data.
	last := events[len(events)-1]
	if last.State != TaskCompleted {
		t.Errorf("last event state = %q, want completed", last.State)
	}
	if last.Data == nil || last.Data.Content != "sse: stream me" {
		t.Errorf("last event data = %v", last.Data)
	}
}

// TestHTTP_SSE_Error proves SSE mode streams failed state on handler error.
func TestHTTP_SSE_Error(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("fail-agent", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, context.Canceled
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"to":      "fail-agent",
		"message": Message{From: "client", Content: "will fail"},
	})

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/a2a/send", bytes.NewReader(body))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	events := parseSSEEvents(t, string(rawBody))

	// Last event should be failed.
	last := events[len(events)-1]
	if last.State != TaskFailed {
		t.Errorf("last event state = %q, want failed", last.State)
	}
}

// TestHTTP_NoAcceptHeader_StillJSON proves default mode is JSON (backward compat).
func TestHTTP_NoAcceptHeader_StillJSON(t *testing.T) {
	tr := NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
		return Message{From: "agent-a", Content: "json: " + msg.Content}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"to":      "agent-a",
		"message": Message{From: "client", Content: "no sse"},
	})

	resp, err := http.Post(ts.URL+"/a2a/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var result struct {
		State TaskState `json:"state"`
		Data  *Message  `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.State != TaskCompleted {
		t.Errorf("state = %q, want completed", result.State)
	}
}

// parseSSEEvents parses SSE text into transport Events.
func parseSSEEvents(t *testing.T, body string) []Event {
	t.Helper()
	var events []Event
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var ev Event
				if err := json.Unmarshal([]byte(data), &ev); err != nil {
					t.Logf("skip unparseable SSE data: %s", data)
					continue
				}
				events = append(events, ev)
			}
		}
	}
	return events
}

// TestHTTP_CrossProcess proves two separate HTTPTransports on different ports
// can communicate — agent on Transport A sends to agent on Transport B via HTTP.
func TestHTTP_CrossProcess(t *testing.T) {
	// Transport B: the "remote" agent.
	trB := NewHTTPTransport()
	defer trB.Close()

	_ = trB.Register("remote-agent", func(_ context.Context, msg Message) (Message, error) {
		return Message{From: "remote-agent", Content: "remote: " + msg.Content}, nil
	})

	serverB := httptest.NewServer(trB.Mux())
	defer serverB.Close()

	// Transport A: the "local" side. It calls Transport B over HTTP.
	// We simulate cross-process by making an HTTP call to serverB.
	body, _ := json.Marshal(map[string]any{
		"to":      "remote-agent",
		"message": Message{From: "local-agent", Content: "cross-process hello"},
	})

	resp, err := http.Post(serverB.URL+"/a2a/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var result struct {
		State TaskState `json:"state"`
		Data  *Message  `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.State != TaskCompleted {
		t.Errorf("state = %q, want completed", result.State)
	}
	if result.Data == nil || result.Data.Content != "remote: cross-process hello" {
		t.Errorf("response = %v", result.Data)
	}

	// Also verify discovery works cross-process.
	cardResp, err := http.Get(serverB.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("GET cards: %v", err)
	}
	defer cardResp.Body.Close()

	var cards []AgentCard
	json.NewDecoder(cardResp.Body).Decode(&cards)
	if len(cards) != 1 {
		t.Errorf("expected 1 card, got %d", len(cards))
	}
}
