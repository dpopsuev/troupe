package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	troupe "github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/director"
)

// Sentinel errors for remote operations.
var (
	ErrRemoteSpawn   = errors.New("remote broker: spawn failed")
	ErrRemotePerform = errors.New("remote broker: perform failed")
)

// RemoteBroker proxies Broker calls to a remote HTTP server.
type RemoteBroker struct {
	endpoint string
	client   *http.Client
}

// newRemoteBroker creates an HTTP client Broker for the given endpoint.
func newRemoteBroker(endpoint string) *RemoteBroker {
	return &RemoteBroker{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 30 * time.Second}, //nolint:mnd // reasonable default
	}
}

// Pick proxies to the remote broker's /pick endpoint.
func (b *RemoteBroker) Pick(ctx context.Context, prefs troupe.Preferences) ([]troupe.ActorConfig, error) {
	body, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+"/pick", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var configs []troupe.ActorConfig
	if err := json.NewDecoder(resp.Body).Decode(&configs); err != nil {
		return nil, err
	}
	return configs, nil
}

// Spawn proxies to the remote broker's /spawn endpoint.
// Returns a ProxyActor that forwards Perform/Ready/Kill over HTTP.
func (b *RemoteBroker) Spawn(ctx context.Context, config troupe.ActorConfig) (troupe.Actor, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+"/spawn", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRemoteSpawn, err)
	}

	return &ProxyActor{
		id:       result.ID,
		endpoint: b.endpoint,
		client:   b.client,
	}, nil
}

// Discover proxies to the remote broker's /discover endpoint.
func (b *RemoteBroker) Discover(role string) []troupe.AgentCard {
	url := b.endpoint + "/discover"
	if role != "" {
		url += "?role=" + role
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var raw []struct {
		Name   string   `json:"name"`
		Role   string   `json:"role"`
		Skills []string `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil
	}
	cards := make([]troupe.AgentCard, len(raw))
	for i, r := range raw {
		cards[i] = &simpleCard{name: r.Name, role: r.Role, skills: r.Skills}
	}
	return cards
}

// ProxyActor proxies Actor calls to a remote broker over HTTP.
type ProxyActor struct {
	id       string
	endpoint string
	client   *http.Client
}

// Perform sends the prompt to the remote actor.
func (a *ProxyActor) Perform(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]string{"id": a.id, "prompt": prompt})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+"/perform", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: %w", ErrRemotePerform, err)
	}
	return result.Response, nil
}

// Ready checks the remote actor's readiness.
func (a *ProxyActor) Ready() bool {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, a.endpoint+"/ready/"+a.id, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Ready bool `json:"ready"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result.Ready
}

// Kill stops the remote actor.
func (a *ProxyActor) Kill(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+"/kill/"+a.id, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// BrokerHandler returns an http.Handler that serves a Broker over HTTP.
func BrokerHandler(b *DefaultBroker) http.Handler {
	mux := http.NewServeMux()
	var actorCounter atomic.Int64

	mux.HandleFunc("POST /pick", func(w http.ResponseWriter, r *http.Request) {
		var prefs troupe.Preferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		configs, err := b.Pick(r.Context(), prefs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(configs) //nolint:errcheck // best effort
	})

	actors := make(map[string]troupe.Actor)
	var mu = &sync.Mutex{}

	mux.HandleFunc("POST /spawn", func(w http.ResponseWriter, r *http.Request) {
		var config troupe.ActorConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		actor, err := b.Spawn(r.Context(), config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := fmt.Sprintf("actor-%d", actorCounter.Add(1))
		mu.Lock()
		actors[id] = actor
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck // best-effort JSON response
	})

	mux.HandleFunc("POST /perform", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		actor, ok := actors[req.ID]
		mu.Unlock()
		if !ok {
			http.Error(w, "actor not found", http.StatusNotFound)
			return
		}
		response, err := actor.Perform(r.Context(), req.Prompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": response}) //nolint:errcheck // best-effort JSON response
	})

	mux.HandleFunc("GET /ready/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		actor, ok := actors[id]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			json.NewEncoder(w).Encode(map[string]bool{"ready": false}) //nolint:errcheck // best-effort JSON response
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ready": actor.Ready()}) //nolint:errcheck // best-effort JSON response
	})

	mux.HandleFunc("GET /discover", func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		agents := b.Discover(role)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents) //nolint:errcheck // best-effort JSON response
	})

	mux.HandleFunc("POST /kill/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		actor, ok := actors[id]
		mu.Unlock()
		if !ok {
			http.Error(w, "actor not found", http.StatusNotFound)
			return
		}
		if err := actor.Kill(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true}) //nolint:errcheck // best-effort JSON response
	})

	return mux
}

// ConnectDirector returns a Director that streams events from a remote endpoint via SSE.
func ConnectDirector(endpoint string) director.Director {
	return &SSEDirector{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 0}, // no timeout for SSE stream
	}
}

// SSEDirector reads events from a remote Director via SSE.
type SSEDirector struct {
	endpoint string
	client   *http.Client
}

// Direct connects to the remote Director and streams events.
func (d *SSEDirector) Direct(ctx context.Context, _ troupe.Broker) (<-chan troupe.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint+"/direct", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := d.client.Do(req) //nolint:bodyclose // closed in goroutine
	if err != nil {
		return nil, err
	}

	ch := make(chan troupe.Event, 64) //nolint:mnd // reasonable buffer

	go func() {
		defer close(ch)
		defer resp.Body.Close()
		readSSEStream(resp.Body, ch)
	}()

	return ch, nil
}

// readSSEStream parses an SSE stream into Events.
func readSSEStream(r io.Reader, ch chan<- troupe.Event) {
	buf := make([]byte, 0, 4096) //nolint:mnd // SSE read buffer size
	tmp := make([]byte, 1024)    //nolint:mnd // SSE buffer size

	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			// Process complete SSE messages (separated by \n\n)
			for {
				idx := bytes.Index(buf, []byte("\n\n"))
				if idx < 0 {
					break
				}
				msg := string(buf[:idx])
				buf = buf[idx+2:]

				ev := parseSSEMessage(msg)
				if ev != nil {
					ch <- *ev
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// parseSSEMessage extracts an Event from an SSE message block.
func parseSSEMessage(msg string) *troupe.Event {
	var dataLine string
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}
	if dataLine == "" {
		return nil
	}

	var ev troupe.Event
	if err := json.Unmarshal([]byte(dataLine), &ev); err != nil {
		return nil
	}
	return &ev
}

// DirectorHandler wraps a Director as an SSE endpoint.
func DirectorHandler(d director.Director, b troupe.Broker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		events, err := d.Direct(r.Context(), b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		for ev := range events {
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, data)
			flusher.Flush()
		}
	}
}
