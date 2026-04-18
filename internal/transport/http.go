package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HTTPTransport is an HTTP-based A2A transport. Embeds baseTransport
// for all task management. Adds an HTTP handler at POST /a2a/send
// for receiving messages over the network.
type HTTPTransport struct {
	baseTransport
	mux *http.ServeMux
}

// NewHTTPTransport creates an HTTP-based transport.
func NewHTTPTransport() *HTTPTransport {
	t := &HTTPTransport{
		baseTransport: newBase(),
		mux:           http.NewServeMux(),
	}
	t.mux.HandleFunc("POST /a2a/send", t.handleSend)
	t.mux.HandleFunc("GET /.well-known/agent.json", t.handleAgentCards)
	t.mux.HandleFunc("GET /.well-known/agent-card.json", t.handleAgentCards) // legacy compat
	return t
}

// Mux returns the HTTP handler for mounting on a server.
func (t *HTTPTransport) Mux() *http.ServeMux {
	return t.mux
}

// handleSend is the HTTP handler for POST /a2a/send.
// Supports two modes based on Accept header:
//   - text/event-stream → SSE: streams every TaskState transition as an event
//   - otherwise → JSON: blocks until terminal state, returns one response
func (t *HTTPTransport) handleSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To  AgentID `json:"to"`
		Msg Message `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	task, err := t.SendMessage(r.Context(), req.To, req.Msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ch, err := t.Subscribe(r.Context(), task.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if wantsSSE(r) {
		t.handleSendSSE(w, ch)
	} else {
		t.handleSendJSON(w, task, ch)
	}
}

// handleSendSSE streams every task state transition as an SSE event.
func (t *HTTPTransport) handleSendSSE(w http.ResponseWriter, ch <-chan Event) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for ev := range ch {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.State, data)
		flusher.Flush()

		if ev.State == TaskCompleted || ev.State == TaskFailed {
			return
		}
	}
}

// handleSendJSON blocks until a terminal state and returns one JSON response.
func (t *HTTPTransport) handleSendJSON(w http.ResponseWriter, task *Task, ch <-chan Event) {
	for ev := range ch {
		if ev.State == TaskCompleted || ev.State == TaskFailed {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // HTTP response encoding
				"task_id": task.ID,
				"state":   ev.State,
				"data":    ev.Data,
				"error":   task.Error,
			})
			return
		}
	}

	http.Error(w, "task did not complete", http.StatusInternalServerError)
}

// wantsSSE checks if the client requested SSE via Accept header.
func wantsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// handleAgentCards serves the A2A v1.0 agent card discovery endpoint.
// Returns JSON array of A2A AgentCards for all registered agents.
func (t *HTTPTransport) handleAgentCards(w http.ResponseWriter, r *http.Request) {
	t.mu.RLock()
	handlers := make(map[AgentID]MsgHandler, len(t.handlers))
	for id, h := range t.handlers {
		handlers[id] = h
	}
	roles := t.roles
	t.mu.RUnlock()

	endpointURL := "http://" + r.Host
	cards := BuildA2ACardsFromHandlers(handlers, roles, endpointURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cards) //nolint:errcheck // HTTP response encoding
}
