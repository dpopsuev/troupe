//go:build e2e

// acp_provider_e2e_test.go — real ACP provider E2E tests.
//
// Each test connects to a REAL ACP agent (Cursor, Claude, Gemini, Codex).
// Costs real money. Requires the agent binary on PATH.
//
// Run: go test ./testkit/ -tags=e2e -run TestRealACP -v -timeout 120s
package testkit

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/bugle/acp"
)

// runRealACPTest is the shared test body for all real ACP providers.
// It starts a real agent, sends a prompt, streams the response, and
// verifies the full lifecycle.
func runRealACPTest(t *testing.T, agentName, binary string) {
	t.Helper()

	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("%s not found on PATH — skipping real %s ACP E2E", binary, agentName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := acp.NewClient(agentName,
		acp.WithClientInfo(acp.ClientInfo{Name: "bugle-e2e", Version: "test"}),
	)
	if err != nil {
		t.Fatalf("NewClient(%q): %v", agentName, err)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer client.Stop(ctx) //nolint:errcheck

	if client.SessionID() == "" {
		t.Fatal("sessionID should not be empty after Start")
	}

	// Send a deterministic prompt.
	client.Send(acp.Message{
		Role:    acp.RoleUser,
		Content: "Respond with exactly the text: BUGLE_ACP_E2E_OK. Nothing else.",
	})

	ch, err := client.Chat(ctx)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var fullText strings.Builder
	var gotDone bool

	for evt := range ch {
		switch evt.Type {
		case acp.EventText:
			fullText.WriteString(evt.Text)
		case acp.EventDone:
			gotDone = true
		case acp.EventError:
			t.Fatalf("agent error: %s", evt.Error)
		}
	}

	if !gotDone {
		t.Fatal("never received EventDone")
	}

	response := fullText.String()
	if response == "" {
		t.Fatal("agent returned empty response")
	}

	t.Logf("agent %s response: %s", agentName, response)

	if !strings.Contains(response, "BUGLE_ACP_E2E_OK") {
		t.Errorf("response doesn't contain BUGLE_ACP_E2E_OK: %s", response)
	}

	// Verify conversation history.
	msgs := client.Messages()
	if len(msgs) < 2 {
		t.Fatalf("messages = %d, want >= 2 (user + assistant)", len(msgs))
	}
	if msgs[0].Role != acp.RoleUser {
		t.Fatalf("msg[0].Role = %q, want user", msgs[0].Role)
	}
	if msgs[len(msgs)-1].Role != acp.RoleAssistant {
		t.Fatalf("last msg role = %q, want assistant", msgs[len(msgs)-1].Role)
	}
}

// TestRealACP_Cursor connects to a live Cursor agent CLI.
// Requires: 'agent' binary on PATH. ~$0.02/run.
func TestRealACP_Cursor(t *testing.T) {
	runRealACPTest(t, "cursor", "agent")
}

// TestRealACP_Claude connects to a live Claude CLI.
// Requires: 'claude' binary on PATH with valid auth. ~$0.05/run.
func TestRealACP_Claude(t *testing.T) {
	runRealACPTest(t, "claude", "claude")
}

// TestRealACP_Gemini connects to a live Gemini CLI (ACP reference implementation).
// Requires: 'gemini' binary on PATH with valid auth.
func TestRealACP_Gemini(t *testing.T) {
	runRealACPTest(t, "gemini", "gemini")
}

// TestRealACP_Codex connects to a live Codex ACP agent.
// Requires: 'codex-acp' binary on PATH with valid auth.
func TestRealACP_Codex(t *testing.T) {
	runRealACPTest(t, "codex", "codex-acp")
}
