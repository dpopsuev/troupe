// http_agent_e2e_test.go — Real agent CLI E2E via HTTPTransport.
//
// Proves the full stack: HTTPTransport → ACP agent → real LLM → response.
// Skips if no agent CLI is available in PATH.
//
// TRP-TSK-99, TRP-GOL-15
package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/internal/acp"
	"github.com/dpopsuev/troupe/internal/transport"
)

// TestHTTPAgent_RealCLI_E2E spawns a real agent CLI via ACP, registers it
// on HTTPTransport, sends a prompt via HTTP, and verifies the response.
// Skipped if no agent CLI is in PATH.
func TestHTTPAgent_RealCLI_E2E(t *testing.T) {
	cli := detectAgentCLI()
	if cli == "" {
		t.Skip("no agent CLI found in PATH (cursor, claude, codex) — skipping real agent E2E")
	}
	t.Logf("using agent CLI: %s", cli)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) //nolint:mnd // generous timeout for LLM
	defer cancel()

	// 1. Start ACP client with real CLI.
	client, err := acp.NewClient(cli,
		acp.WithModel("sonnet"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	defer client.Stop(ctx) //nolint:errcheck // test cleanup

	t.Logf("agent started: %s (session: %s)", client.AgentName(), client.SessionID())

	// 2. Create HTTPTransport and register the ACP agent as handler.
	tr := transport.NewHTTPTransport()
	defer tr.Close()

	_ = tr.Register("real-agent", func(ctx context.Context, msg transport.Message) (transport.Message, error) {
		client.Send(acp.Message{Role: acp.RoleUser, Content: msg.Content})
		ch, err := client.Chat(ctx)
		if err != nil {
			return transport.Message{}, err
		}

		var fullText string
		for evt := range ch {
			switch evt.Type {
			case acp.EventText:
				fullText += evt.Text
			case acp.EventError:
				return transport.Message{}, errors.New(evt.Error)
			}
		}

		return transport.Message{From: "real-agent", Content: fullText}, nil
	})

	ts := httptest.NewServer(tr.Mux())
	defer ts.Close()

	// 3. Send prompt via HTTP.
	body, _ := json.Marshal(map[string]any{
		"to": "real-agent",
		"message": transport.Message{
			From:    "test",
			Content: "Reply with exactly: TROUPE_E2E_OK",
		},
	})

	resp, err := http.Post(ts.URL+"/a2a/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, rawBody)
	}

	// 4. Verify response.
	var result struct {
		TaskID string              `json:"task_id"`
		State  transport.TaskState `json:"state"`
		Data   *transport.Message  `json:"data"`
		Error  string              `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.State != transport.TaskCompleted {
		t.Fatalf("state = %q, error = %q", result.State, result.Error)
	}
	if result.Data == nil {
		t.Fatal("data is nil")
	}
	if !strings.Contains(result.Data.Content, "TROUPE_E2E_OK") {
		t.Fatalf("response = %q, want contains TROUPE_E2E_OK", result.Data.Content)
	}

	t.Logf("real agent response: %s", result.Data.Content)

	// 5. Verify agent discovery via A2A card endpoint.
	cardResp, err := http.Get(ts.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("GET cards: %v", err)
	}
	defer cardResp.Body.Close()

	var cards []map[string]any
	json.NewDecoder(cardResp.Body).Decode(&cards) //nolint:errcheck
	if len(cards) != 1 {
		t.Errorf("expected 1 card, got %d", len(cards))
	}
}

// detectAgentCLI checks for known agent CLIs in PATH.
func detectAgentCLI() string {
	for _, cli := range []string{"cursor", "claude", "codex"} {
		if _, err := exec.LookPath(cli); err == nil {
			return cli
		}
	}
	return ""
}
