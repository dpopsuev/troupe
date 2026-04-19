//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/visual"
	"github.com/dpopsuev/troupe/world"
)

// providerSpec defines how to invoke a CLI provider in non-interactive mode.
type providerSpec struct {
	name    string   // human label
	command string   // binary name
	args    []string // flags before prompt
}

// Native CLI providers — each is a separate binary.
var (
	claude = providerSpec{name: "Claude", command: "claude", args: []string{"-p", "--bare"}}
	codex  = providerSpec{name: "Codex", command: "codex", args: []string{"exec"}}
	gemini = providerSpec{name: "Gemini", command: "gemini", args: []string{"-p"}}
)

// cursorModel returns a providerSpec that uses the Cursor agent CLI
// with a specific --model. Cursor is a facade for all providers.
func cursorModel(model, label string) providerSpec {
	return providerSpec{
		name:    "Cursor/" + label,
		command: "agent",
		args:    []string{"-p", "--model", model},
	}
}

// requireCLI skips the test if the given binary is not in PATH.
func requireCLI(t *testing.T, command string) {
	t.Helper()
	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("%s not found in PATH, skipping", command)
	}
}

// cliHandler returns a transport.MsgHandler that invokes the CLI with the
// message content as a prompt argument, returning stdout as the response.
func cliHandler(command string, args []string) transport.MsgHandler {
	return func(ctx context.Context, msg transport.Message) (transport.Message, error) {
		cmdArgs := make([]string, len(args), len(args)+1)
		copy(cmdArgs, args)
		cmdArgs = append(cmdArgs, msg.Content)

		tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(tctx, command, cmdArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return transport.Message{}, fmt.Errorf("%s %v: %w (stderr: %s)", command, cmdArgs, err, stderr.String())
		}

		return transport.Message{
			From:         msg.To,
			To:           msg.From,
			Role: "agent",
			Content:      stdout.String(),
		}, nil
	}
}

// spawnAgent creates an entity with ColorIdentity + Health, registers it on
// the transport with a CLI-backed handler, and returns the agent ID string.
func spawnAgent(t *testing.T, w *world.World, reg *visual.Registry, tr *transport.LocalTransport, role, collective string, p providerSpec) string {
	t.Helper()
	color, err := reg.Assign(role, collective)
	if err != nil {
		t.Fatalf("assign color for %s: %v", p.name, err)
	}
	id := w.Spawn()
	world.Attach(w, id, color)
	world.Attach(w, id, world.Alive{State: world.AliveRunning, Since: time.Now()})
	_ = tr.Register(color.Short(), cliHandler(p.command, p.args))
	t.Logf("spawned %s agent: %s (entity %d)", p.name, color.Title(), id)
	return color.Short()
}

// runPairTest spawns two agents and sends a message from A to B,
// verifying the full stack works.
func runPairTest(t *testing.T, p providerSpec) {
	t.Helper()
	w := world.NewWorld()
	reg := visual.NewRegistry()
	tr := transport.NewLocalTransport()
	defer tr.Close()
	view := visual.NewView(w)

	agentA := spawnAgent(t, w, reg, tr, "Writer", "Pair", p)
	agentB := spawnAgent(t, w, reg, tr, "Reviewer", "Pair", p)

	ctx := context.Background()
	task, err := tr.SendMessage(ctx, agentB, transport.Message{
		From:         agentA,
		To:           agentB,
		Role: "user",
		Content:      "Reply with exactly one word: hello",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(ctx, task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var response string
	for ev := range ch {
		if ev.State == transport.TaskCompleted {
			if ev.Data == nil {
				t.Fatal("completed event has no data")
			}
			response = ev.Data.Content
			t.Logf("response from %s: %q", agentB, response)
		}
		if ev.State == transport.TaskFailed {
			t.Fatalf("task failed (check CLI stderr for details)")
		}
	}

	if response == "" {
		t.Fatal("got empty response")
	}

	stats := view.Stats()
	if stats.TotalEntities != 2 {
		t.Errorf("entities = %d, want 2", stats.TotalEntities)
	}
	if stats.ByAlive[world.AliveRunning] != 2 {
		t.Errorf("active = %d, want 2", stats.ByAlive[world.AliveRunning])
	}
}

// --- Native CLI pair tests ---

func TestE2E_ClaudePair(t *testing.T) {
	requireCLI(t, "claude")
	runPairTest(t, claude)
}

func TestE2E_CodexPair(t *testing.T) {
	requireCLI(t, "codex")
	runPairTest(t, codex)
}

func TestE2E_GeminiPair(t *testing.T) {
	requireCLI(t, "gemini")
	runPairTest(t, gemini)
}

// --- Cursor facade tests (same binary, different models) ---

func TestE2E_CursorSonnetPair(t *testing.T) {
	requireCLI(t, "agent")
	runPairTest(t, cursorModel("claude-4.6-sonnet-medium", "Sonnet"))
}

func TestE2E_CursorGPTPair(t *testing.T) {
	requireCLI(t, "agent")
	runPairTest(t, cursorModel("gpt-5.4-medium", "GPT"))
}

func TestE2E_CursorGeminiPair(t *testing.T) {
	requireCLI(t, "agent")
	runPairTest(t, cursorModel("gemini-3.1-pro", "Gemini"))
}

// --- Mixed quartet: one agent per native CLI ---

func TestE2E_MixedQuartet(t *testing.T) {
	allProviders := []providerSpec{claude, codex, gemini, {name: "Cursor", command: "agent", args: []string{"-p"}}}
	for _, p := range allProviders {
		requireCLI(t, p.command)
	}

	w := world.NewWorld()
	reg := visual.NewRegistry()
	tr := transport.NewLocalTransport()
	defer tr.Close()
	bus := signal.NewMemBus()
	view := visual.NewView(w)

	roles := []string{"Writer", "Reviewer", "Analyst", "Architect"}
	agents := make([]string, len(allProviders))
	for i, p := range allProviders {
		agents[i] = spawnAgent(t, w, reg, tr, roles[i], "Quartet", p)
	}

	ctx := context.Background()

	// Chain: Claude → Codex → Gemini → Cursor
	for i := 0; i < len(agents)-1; i++ {
		from := agents[i]
		to := agents[i+1]
		t.Logf("sending %s → %s", allProviders[i].name, allProviders[i+1].name)

		task, err := tr.SendMessage(ctx, to, transport.Message{
			From:         from,
			To:           to,
			Role: "user",
			Content:      "Reply with exactly one word: hello",
		})
		if err != nil {
			t.Fatalf("SendMessage %s→%s: %v", from, to, err)
		}

		ch, err := tr.Subscribe(ctx, task.ID)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		for ev := range ch {
			if ev.State == transport.TaskCompleted {
				if ev.Data == nil {
					t.Fatalf("completed event %s→%s has no data", from, to)
				}
				t.Logf("response from %s (%s): %q", to, allProviders[i+1].name, ev.Data.Content)
			}
			if ev.State == transport.TaskFailed {
				t.Fatalf("task %s→%s failed", from, to)
			}
		}

		bus.Emit(&signal.Signal{Event: "message_sent", Agent: from})
	}

	testkit.AssertSignalCount(t, bus, "message_sent", len(agents)-1)

	stats := view.Stats()
	if stats.TotalEntities != len(agents) {
		t.Errorf("entities = %d, want %d", stats.TotalEntities, len(agents))
	}
	if stats.ByAlive[world.AliveRunning] != len(agents) {
		t.Errorf("active = %d, want %d", stats.ByAlive[world.AliveRunning], len(agents))
	}
	if stats.Collectives != 1 {
		t.Errorf("collectives = %d, want 1", stats.Collectives)
	}

	t.Log("mixed quartet complete — all 4 providers communicated through Troupe A2A")
}
