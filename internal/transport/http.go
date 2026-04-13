package transport

import (
	"encoding/json"
	"net/http"
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
	return t
}

// Mux returns the HTTP handler for mounting on a server.
func (t *HTTPTransport) Mux() *http.ServeMux {
	return t.mux
}

// handleSend is the HTTP handler for POST /a2a/send.
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
