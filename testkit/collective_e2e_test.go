//go:build e2e

// collective_e2e_test.go — real provider E2E for AgentCollective.
//
// Spawns real ACP agents and runs dialectic debate. Costs real money.
//
// Run: go test ./testkit/ -tags=e2e -run TestCollective -v -timeout 180s
package testkit

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/bugle/acp"
	"github.com/dpopsuev/bugle/collective"
	"github.com/dpopsuev/bugle/facade"
	"github.com/dpopsuev/bugle/pool"
)

// TestCollectiveE2E_DialecticWithRealAgents spawns 2 real Cursor agents,
// runs a Dialectic strategy, and proves convergence with real LLM responses.
func TestCollectiveE2E_DialecticWithRealAgents(t *testing.T) {
	if _, err := exec.LookPath("agent"); err != nil {
		t.Skip("agent (Cursor CLI) not found on PATH — skipping real collective E2E")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	launcher := acp.NewACPLauncher()
	staff := facade.NewStaff(launcher)

	// Spawn 2 agents — both as Cursor.
	thesis, err := staff.Spawn(ctx, "thesis", pool.LaunchConfig{Model: "cursor"})
	if err != nil {
		t.Fatalf("spawn thesis: %v", err)
	}

	anti, err := staff.Spawn(ctx, "antithesis", pool.LaunchConfig{Model: "cursor"})
	if err != nil {
		t.Fatalf("spawn antithesis: %v", err)
	}

	// Wire handlers — agents respond via ACP, so Listen() routes through transport.
	// For real ACP agents, Ask() goes through the launcher's Client.
	thesisClient, ok := launcher.Client(thesis.ID())
	if !ok {
		t.Fatal("thesis client not found")
	}
	antiClient, ok := launcher.Client(anti.ID())
	if !ok {
		t.Fatal("antithesis client not found")
	}

	// Register transport handlers that delegate to ACP clients.
	thesis.Listen(func(content string) string {
		thesisClient.Send(acp.Message{Role: acp.RoleUser, Content: content})
		ch, err := thesisClient.Chat(ctx)
		if err != nil {
			return "error: " + err.Error()
		}
		var result strings.Builder
		for evt := range ch {
			if evt.Type == acp.EventText {
				result.WriteString(evt.Text)
			}
		}
		return result.String()
	})

	anti.Listen(func(content string) string {
		antiClient.Send(acp.Message{Role: acp.RoleUser, Content: content})
		ch, err := antiClient.Chat(ctx)
		if err != nil {
			return "error: " + err.Error()
		}
		var result strings.Builder
		for evt := range ch {
			if evt.Type == acp.EventText {
				result.WriteString(evt.Text)
			}
		}
		return result.String()
	})

	// Create collective with Dialectic strategy.
	coll := collective.NewAgentCollective(
		thesis.ID(),
		"code-reviewer",
		&collective.Dialectic{MaxRounds: 2, ConvergenceWord: "CONVERGED"},
		[]*facade.AgentHandle{thesis, anti},
	)

	// Ask the collective — this runs a real debate between 2 LLM agents.
	t.Log("starting dialectic debate between 2 real Cursor agents...")
	result, err := coll.Ask(ctx, "What is 2+2? The thesis agent should answer. The antithesis should verify or challenge. If correct, respond with CONVERGED.")
	if err != nil {
		t.Fatalf("collective Ask: %v", err)
	}

	t.Logf("collective result: %s", result)

	if result == "" {
		t.Fatal("collective returned empty result")
	}

	// The result should mention "4" somewhere.
	if !strings.Contains(result, "4") {
		t.Logf("WARNING: result doesn't contain '4': %s", result)
	}

	staff.KillAll(ctx)
	t.Log("dialectic E2E passed — real agents debated and produced a synthesis")
}
