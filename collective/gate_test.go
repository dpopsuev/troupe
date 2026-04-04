package collective

import (
	"context"
	"strings"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════
// RED: Ambiguous / error cases
// ═══════════════════════════════════════════════════════════════════════

func TestAgentGatekeeper_GibberishDefaultsToPass(t *testing.T) {
	parts := newTestParts()
	ctx := context.Background()

	agent, _ := parts.spawn(ctx, "gate")
	agent.Listen(func(_ string) string { return "I don't understand the question" })

	gate := &AgentGatekeeper{Agent: agent}
	ok, _, err := gate.Pass(ctx, "test content")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatal("ambiguous response should default to PASS (fail-open)")
	}
}

// ═══════════════════════════════════════════════════════════════════════
// GREEN: Happy path
// ═══════════════════════════════════════════════════════════════════════

func TestAgentGatekeeper_PassResponse(t *testing.T) {
	parts := newTestParts()
	ctx := context.Background()

	agent, _ := parts.spawn(ctx, "gate")
	agent.Listen(func(_ string) string { return "PASS: looks good" })

	gate := &AgentGatekeeper{Agent: agent}
	ok, reason, err := gate.Pass(ctx, "review this code")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatal("PASS response should allow")
	}
	if !strings.Contains(reason, "looks good") {
		t.Fatalf("reason = %q", reason)
	}
}

func TestAgentGatekeeper_RejectResponse(t *testing.T) {
	parts := newTestParts()
	ctx := context.Background()

	agent, _ := parts.spawn(ctx, "gate")
	agent.Listen(func(_ string) string { return "REJECT: destructive request" })

	gate := &AgentGatekeeper{Agent: agent}
	ok, reason, err := gate.Pass(ctx, "delete everything")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Fatal("REJECT response should block")
	}
	if !strings.Contains(reason, "destructive") {
		t.Fatalf("reason = %q", reason)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// BLUE: Edge cases
// ═══════════════════════════════════════════════════════════════════════

func TestAgentGatekeeper_CaseInsensitive(t *testing.T) {
	parts := newTestParts()
	ctx := context.Background()

	cases := []struct {
		response string
		wantPass bool
	}{
		{"reject: bad", false},
		{"Reject: bad", false},
		{"REJECT: bad", false},
		{"pass: ok", true},
		{"Pass: ok", true},
		{"PASS: ok", true},
	}

	for _, tc := range cases {
		agent, _ := parts.spawn(ctx, "gate")
		resp := tc.response
		agent.Listen(func(_ string) string { return resp })

		gate := &AgentGatekeeper{Agent: agent}
		ok, _, err := gate.Pass(ctx, "test")
		if err != nil {
			t.Fatalf("err = %v for %q", err, tc.response)
		}
		if ok != tc.wantPass {
			t.Fatalf("response %q: got pass=%v, want %v", tc.response, ok, tc.wantPass)
		}
	}
}

func TestAgentGatekeeper_EmptyResponse(t *testing.T) {
	parts := newTestParts()
	ctx := context.Background()

	agent, _ := parts.spawn(ctx, "gate")
	agent.Listen(func(_ string) string { return "" })

	gate := &AgentGatekeeper{Agent: agent}
	ok, _, _ := gate.Pass(ctx, "test")
	if !ok {
		t.Fatal("empty response should default to PASS")
	}
}

// Compile-time interface check.
var _ Gatekeeper = (*AgentGatekeeper)(nil)
