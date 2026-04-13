package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
