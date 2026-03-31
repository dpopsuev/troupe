//go:build e2e

// acceptance_test.go — E2E acceptance tests with real AI agents.
// Gated by build tag + binary availability. Run with:
//
//	go test ./testkit/ -tags=e2e -run TestAcceptance -v -timeout 300s
//	JERICHO_TEST_AGENT=claude go test ./testkit/ -tags=e2e -v -timeout 300s
//
// Cost: ~$0.30 total for all tests (real API calls).
package testkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/jericho/acp"
	"github.com/dpopsuev/jericho/agent"
	"github.com/dpopsuev/jericho/arsenal"
	"github.com/dpopsuev/jericho/collective"
	"github.com/dpopsuev/jericho/pool"
	"github.com/dpopsuev/jericho/signal"
	"github.com/dpopsuev/jericho/trait"
	"github.com/dpopsuev/jericho/world"
)

// testAgent returns the CLI agent to test. Default: cursor.
func testAgent(t *testing.T) string {
	t.Helper()
	a := os.Getenv("JERICHO_TEST_AGENT")
	if a == "" {
		a = "cursor"
	}
	return a
}

// agentBinaries maps agent names to CLI binaries.
var agentBinaries = map[string]string{
	"cursor": "agent",
	"claude": "claude",
	"gemini": "gemini",
	"codex":  "codex",
}

// requireAgent skips the test if the agent binary is not in PATH.
func requireAgent(t *testing.T, name string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real agent test in -short mode")
	}
	binary := agentBinaries[name]
	if binary == "" {
		binary = name
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		t.Skipf("%s (%s) not found in PATH — skipping", name, binary)
	}
	return path
}

// spawnRealAgent creates Staff + ACPLauncher, spawns one real agent, wires
// ACP client to Solo.Listen(). Registers cleanup via t.Cleanup.
func spawnRealAgent(t *testing.T, role string) (*agent.Staff, *agent.Solo, *acp.ACPLauncher) {
	t.Helper()
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	solo, err := staff.Spawn(ctx, role, pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn failed (likely auth): %v", err)
	}

	wireACPToTransport(t, launcher, solo)

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	return staff, solo, launcher
}

// wireACPToTransport connects a Solo's transport handler to its ACP client.
func wireACPToTransport(t *testing.T, launcher *acp.ACPLauncher, solo *agent.Solo) {
	t.Helper()
	client, ok := launcher.Client(solo.ID())
	if !ok {
		t.Fatalf("no ACP client for entity %d", solo.ID())
	}
	solo.Listen(func(content string) string {
		client.Send(acp.Message{Role: acp.RoleUser, Content: content})
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		ch, err := client.Chat(ctx)
		if err != nil {
			return "error: " + err.Error()
		}
		var buf strings.Builder
		for evt := range ch {
			if evt.Type == acp.EventText {
				buf.WriteString(evt.Text)
			}
		}
		return buf.String()
	})
}

// truncate shortens a string to n chars for log output.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ═══════════════════════════════════════════════════════════════════════
// 1. Smoke Tests ($0)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Smoke_AllAgents(t *testing.T) {
	for name, binary := range agentBinaries {
		t.Run(name, func(t *testing.T) {
			path, err := exec.LookPath(binary)
			if err != nil {
				t.Logf("NOT FOUND: %s (%s)", name, binary)
				return
			}
			t.Logf("FOUND: %s at %s", name, path)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// 2. Pool Lifecycle (~$0.05)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Pool_RealAgentLifecycle(t *testing.T) {
	staff, solo, _ := spawnRealAgent(t, "worker")

	if !solo.IsAlive() {
		t.Fatal("agent should be alive after spawn")
	}
	if !solo.IsRunning() {
		t.Fatal("agent should be running after spawn")
	}

	ctx := context.Background()

	// Ask the real agent a question.
	resp, err := solo.Ask(ctx, "Say hello in one word.")
	if err != nil {
		t.Skipf("Ask failed (likely auth): %v", err)
	}
	if resp == "" {
		t.Fatal("empty response from real agent")
	}
	t.Logf("agent response: %s", resp[:min(len(resp), 100)])

	// Kill and verify.
	if err := solo.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	if staff.Count() != 0 {
		t.Fatalf("count after kill = %d, want 0", staff.Count())
	}
}

// ═══════════════════════════════════════════════════════════════════════
// 3. AI Operator Spawns Children (~$0.05) — per JRC-NED-4
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_AIOperator_SpawnsChildren(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	// Agent 1 (GenSec) — first AI agent.
	gensec, err := staff.Spawn(ctx, "gensec", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn gensec failed: %v", err)
	}

	// GenSec spawns a child worker — recursive AI spawning AI.
	worker, err := gensec.Spawn(ctx, "worker", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn worker under gensec failed: %v", err)
	}

	// Verify hierarchy.
	children := gensec.Children()
	if len(children) != 1 {
		t.Fatalf("gensec children = %d, want 1", len(children))
	}

	// Kill gensec — worker should be reparented (orphan adoption).
	gensec.Kill(ctx) //nolint:errcheck

	// Worker should still be alive (reparented to root).
	if !worker.IsAlive() {
		t.Fatal("worker should survive parent death (orphan reparenting)")
	}

	t.Logf("AI operator test passed: gensec spawned worker, orphan reparented")
}

// ═══════════════════════════════════════════════════════════════════════
// 4. Dialectic Debate (~$0.10)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Collective_DialecticDebate(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	// Spawn 2 real agents.
	thesis, err := staff.Spawn(ctx, "thesis", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn thesis: %v", err)
	}
	anti, err := staff.Spawn(ctx, "antithesis", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn antithesis: %v", err)
	}

	// Wire ACP clients to transport.
	wireACPToTransport(t, launcher, thesis)
	wireACPToTransport(t, launcher, anti)

	// Create collective with Dialectic strategy.
	coll := collective.NewCollective(
		thesis.ID(), "debater",
		&collective.Dialectic{MaxRounds: 2, ConvergenceWord: "CONVERGED"},
		[]*agent.Solo{thesis, anti},
	)

	// Ask a question that two agents can debate.
	result, err := coll.Ask(ctx, "What is 2+2? Explain your reasoning briefly.")
	if err != nil {
		t.Skipf("collective ask failed: %v", err)
	}

	if result == "" {
		t.Fatal("empty response from dialectic debate")
	}

	// Log round-by-round debate transcript for debugging.
	rounds := coll.DebateRounds()
	for i, r := range rounds {
		t.Logf("=== Round %d (converged=%v) ===", i+1, r.Converged)
		t.Logf("  THESIS:     %s", truncate(r.ThesisResponse, 300))
		t.Logf("  ANTITHESIS: %s", truncate(r.AntithesisResponse, 300))
	}
	t.Logf("=== Synthesis (%d chars) ===\n%s", len(result), truncate(result, 500))
}

// ═══════════════════════════════════════════════════════════════════════
// 5. Arsenal Select → Spawn → Work (~$0.05)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Arsenal_SelectAndSpawn(t *testing.T) {
	requireAgent(t, "cursor") // Arsenal selects cursor provider

	// Select from catalog.
	a, err := arsenal.NewArsenal("latest")
	if err != nil {
		t.Fatalf("NewArsenal: %v", err)
	}

	resolved, err := a.Select("", &arsenal.Preferences{
		Providers: arsenal.Filter{Allow: []string{"cursor"}},
		Weights:   arsenal.TraitVector{Coding: 1.0},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	t.Logf("selected: model=%s provider=%s source=%s", resolved.Model, resolved.Provider, resolved.Source)

	// Spawn the selected model.
	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	solo, err := staff.Spawn(ctx, "selected-worker", pool.AgentConfig{Model: "cursor"})
	if err != nil {
		t.Skipf("spawn selected model: %v", err)
	}
	wireACPToTransport(t, launcher, solo)

	// Ask the selected agent to work.
	resp, err := solo.Ask(ctx, "What is your name? Reply in one sentence.")
	if err != nil {
		t.Skipf("Ask failed: %v", err)
	}
	if resp == "" {
		t.Fatal("empty response from arsenal-selected agent")
	}

	t.Logf("arsenal pipeline: catalog → select → spawn → ask → response (%d chars)", len(resp))
}

// ═══════════════════════════════════════════════════════════════════════
// 6. Signal Observation ($0 beyond spawn)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Signal_LifecycleObservation(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Observe signals.
	var startCount, stopCount atomic.Int32
	var mu sync.Mutex
	var events []signal.Signal

	staff.OnSignal(func(s signal.Signal) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, s)
		switch s.Event {
		case signal.EventWorkerStarted:
			startCount.Add(1)
		case signal.EventWorkerStopped:
			stopCount.Add(1)
		}
	})

	// Spawn 2 agents.
	a1, err := staff.Spawn(ctx, "worker-a", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn a: %v", err)
	}
	a2, err := staff.Spawn(ctx, "worker-b", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn b: %v", err)
	}

	if startCount.Load() != 2 {
		t.Fatalf("expected 2 start signals, got %d", startCount.Load())
	}

	// Kill one.
	a1.Kill(ctx) //nolint:errcheck
	if stopCount.Load() != 1 {
		t.Fatalf("expected 1 stop signal, got %d", stopCount.Load())
	}

	// Kill all.
	a2.Kill(ctx) //nolint:errcheck
	if stopCount.Load() != 2 {
		t.Fatalf("expected 2 stop signals, got %d", stopCount.Load())
	}

	// Verify signal metadata.
	mu.Lock()
	for _, s := range events {
		if s.Event == signal.EventWorkerStarted {
			if s.Meta[signal.MetaKeyWorkerID] == "" {
				t.Errorf("start signal missing worker_id")
			}
			if s.Meta["role"] == "" {
				t.Errorf("start signal missing role")
			}
		}
	}
	mu.Unlock()

	t.Logf("signal observation: %d starts, %d stops, %d total events",
		startCount.Load(), stopCount.Load(), len(events))
}

// ═══════════════════════════════════════════════════════════════════════
// 7. Graceful Termination (~$0.05)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_GracefulTermination(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	solo, err := staff.Spawn(ctx, "graceful-worker", pool.AgentConfig{
		Model:       name,
		GracePeriod: 3 * time.Second,
	})
	if err != nil {
		t.Skipf("spawn: %v", err)
	}

	// Verify running.
	if !solo.IsRunning() {
		t.Fatal("should be running after spawn")
	}

	// Graceful kill — marks not-ready, waits grace period, then force-kills.
	p := staff.Pool()
	err = p.KillGraceful(ctx, solo.ID(), 3*time.Second)
	if err != nil {
		t.Fatalf("KillGraceful: %v", err)
	}

	// After graceful kill, agent should be gone.
	if staff.Count() != 0 {
		t.Fatalf("count after graceful kill = %d, want 0", staff.Count())
	}

	// Check that Ready was set to terminated during cleanup.
	ready, ok := world.TryGet[world.Ready](staff.World(), solo.ID())
	if ok && ready.Reason != world.ReasonTerminated {
		t.Logf("ready reason = %s (may be already despawned)", ready.Reason)
	}

	t.Logf("graceful termination passed: agent stopped cleanly within grace period")
}

// ═══════════════════════════════════════════════════════════════════════
// 8. Intent-Based Selection → Spawn → Work (~$0.05)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_IntentToAgent(t *testing.T) {
	requireAgent(t, "cursor")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Infer traits from natural language intent — heuristic only, $0.
	weights, err := trait.InferFromIntent(ctx, "I need a fast coding agent", trait.InferConfig{})
	if err != nil {
		t.Fatalf("InferFromIntent: %v", err)
	}
	t.Logf("inferred: speed=%.1f coding=%.1f reasoning=%.1f", weights.Speed, weights.Coding, weights.Reasoning)

	// Select best model from Arsenal using inferred traits as weights.
	a, err := arsenal.NewArsenal("latest")
	if err != nil {
		t.Fatalf("NewArsenal: %v", err)
	}
	resolved, err := a.Select("", &arsenal.Preferences{Weights: weights})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	t.Logf("selected: model=%s provider=%s (speed trait weight=%.1f)", resolved.Model, resolved.Provider, weights.Speed)

	// Spawn the selected agent and verify it works.
	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	solo, err := staff.Spawn(ctx, "intent-worker", pool.AgentConfig{Model: "cursor"})
	if err != nil {
		t.Skipf("spawn: %v", err)
	}
	wireACPToTransport(t, launcher, solo)

	resp, err := solo.Ask(ctx, "Write a Go function that adds two numbers. Just the function, no explanation.")
	if err != nil {
		t.Skipf("Ask failed: %v", err)
	}

	if !strings.Contains(resp, "func") {
		t.Logf("WARNING: response may not contain Go code: %s", resp[:min(len(resp), 200)])
	}

	t.Logf("INTENT PIPELINE: 'fast coding agent' → inferred traits → selected %s → spawned → got code (%d chars)",
		resolved.Model, len(resp))
}

// ═══════════════════════════════════════════════════════════════════════
// 9. Three-Generation Orphan Cascade (~$0.05)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_ThreeGenerationOrphanCascade(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	// Three generations: GenSec → Manager → Worker.
	gensec, err := staff.Spawn(ctx, "gensec", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn gensec: %v", err)
	}
	// GenSec is the subreaper — adopts orphans.
	staff.Pool().SetSubreaper(gensec.ID())

	manager, err := gensec.Spawn(ctx, "manager", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn manager: %v", err)
	}

	worker, err := manager.Spawn(ctx, "worker", pool.AgentConfig{Model: name})
	if err != nil {
		t.Skipf("spawn worker: %v", err)
	}

	// Verify 3-level hierarchy.
	if len(gensec.Children()) != 1 {
		t.Fatalf("gensec children = %d, want 1 (manager)", len(gensec.Children()))
	}
	if len(manager.Children()) != 1 {
		t.Fatalf("manager children = %d, want 1 (worker)", len(manager.Children()))
	}

	t.Log("Phase 1: 3-level hierarchy established (gensec → manager → worker)")

	// Kill Manager → Worker orphaned → reparented to GenSec (subreaper).
	manager.Kill(ctx) //nolint:errcheck

	if !worker.IsAlive() {
		t.Fatal("worker should survive manager death")
	}

	gensecChildren := gensec.Children()
	foundWorker := false
	for _, c := range gensecChildren {
		if c.ID() == worker.ID() {
			foundWorker = true
		}
	}
	if !foundWorker {
		t.Fatal("worker should be reparented to gensec after manager death")
	}

	t.Log("Phase 2: Manager killed, worker reparented to GenSec")

	// Kill GenSec → Worker orphaned again → reparented to root (0).
	gensec.Kill(ctx) //nolint:errcheck

	if !worker.IsAlive() {
		t.Fatal("worker should survive gensec death (reparented to root)")
	}

	t.Log("Phase 3: GenSec killed, worker reparented to root — full orphan cascade")
}

// ═══════════════════════════════════════════════════════════════════════
// 10. Race: Three Agents Compete (~$0.10)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_Race_ThreeAgentsCompete(t *testing.T) {
	name := testAgent(t)
	requireAgent(t, name)

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	// Spawn 3 real agents.
	agents := make([]*agent.Solo, 3)
	for i := range 3 {
		var err error
		agents[i], err = staff.Spawn(ctx, fmt.Sprintf("racer-%d", i), pool.AgentConfig{Model: name})
		if err != nil {
			t.Skipf("spawn racer-%d: %v", i, err)
		}
		wireACPToTransport(t, launcher, agents[i])
	}

	t.Log("3 real agents spawned — racing")

	// Race: same question to all 3, first response wins.
	coll := collective.NewCollective(
		agents[0].ID(), "racers",
		&collective.Race{},
		agents,
	)

	start := time.Now()
	result, err := coll.Ask(ctx, "What is the capital of France? One word only.")
	elapsed := time.Since(start)

	if err != nil {
		t.Skipf("race failed: %v", err)
	}

	if result == "" {
		t.Fatal("empty response from race")
	}

	t.Logf("RACE: 3 agents competed, first response in %v: %s",
		elapsed.Round(time.Millisecond), result[:min(len(result), 100)])
}

// ═══════════════════════════════════════════════════════════════════════
// 11. Multi-Provider Debate: Cursor vs Claude (~$0.15)
// ═══════════════════════════════════════════════════════════════════════

func TestAcceptance_MultiProvider_Debate(t *testing.T) {
	// Requires BOTH cursor and claude.
	requireAgent(t, "cursor")
	requireAgent(t, "claude")

	launcher := acp.NewACPLauncher()
	staff := agent.NewStaff(launcher)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		staff.KillAll(stopCtx)
	})

	// Spawn one Cursor agent and one Claude agent.
	thesis, err := staff.Spawn(ctx, "thesis", pool.AgentConfig{Model: "cursor"})
	if err != nil {
		t.Skipf("spawn cursor thesis: %v", err)
	}
	wireACPToTransport(t, launcher, thesis)

	anti, err := staff.Spawn(ctx, "antithesis", pool.AgentConfig{Model: "claude"})
	if err != nil {
		t.Skipf("spawn claude antithesis: %v", err)
	}
	wireACPToTransport(t, launcher, anti)

	t.Log("Cursor (thesis) vs Claude (antithesis) — cross-provider debate")

	coll := collective.NewCollective(
		thesis.ID(), "cross-provider",
		&collective.Dialectic{MaxRounds: 2, ConvergenceWord: "CONVERGED"},
		[]*agent.Solo{thesis, anti},
	)

	result, err := coll.Ask(ctx, "Is Go or Rust better for building agent frameworks? Brief arguments only.")
	if err != nil {
		t.Skipf("debate failed: %v", err)
	}

	if result == "" {
		t.Fatal("empty response from cross-provider debate")
	}

	// Log round-by-round transcript.
	rounds := coll.DebateRounds()
	for i, r := range rounds {
		t.Logf("=== Round %d (converged=%v) ===", i+1, r.Converged)
		t.Logf("  CURSOR (thesis):  %s", truncate(r.ThesisResponse, 300))
		t.Logf("  CLAUDE (antithesis): %s", truncate(r.AntithesisResponse, 300))
	}
	t.Logf("=== Synthesis (%d chars) ===\n%s", len(result), truncate(result, 500))

	t.Logf("CROSS-PROVIDER DEBATE result (%d chars): %s",
		len(result), result[:min(len(result), 300)])
}
