//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/visual"
	"github.com/dpopsuev/troupe/world"
)

// Feature: 2 same-provider agents.
func TestE2E_TwoAgents_RequestConfirm(t *testing.T) {
	w, agents := testkit.QuickWorld(2, "Refactor")
	tr := testkit.QuickTransport(w, agents)
	defer tr.Close()

	ctx := context.Background()
	color0 := world.Get[visual.Color](w, agents[0])
	color1 := world.Get[visual.Color](w, agents[1])

	task, err := tr.SendMessage(ctx, color1.Short(), transport.Message{
		From:         color0.Short(),
		To:           color1.Short(),
		Role: "user",
		Content:      "please review this code",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, subErr := tr.Subscribe(ctx, task.ID)
	if subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	var completed bool
	for ev := range ch {
		if ev.State == transport.TaskCompleted {
			completed = true
			if ev.Data == nil {
				t.Fatal("completed event should carry data")
			}
			if ev.Data.Role != "agent" {
				t.Errorf("Role = %q, want agent", ev.Data.Role)
			}
			if ev.Data.Content != "please review this code" {
				t.Errorf("Content = %q, want echo", ev.Data.Content)
			}
		}
	}
	if !completed {
		t.Error("task never completed")
	}
}

// Feature: Mixed elements collaborate.
func TestE2E_MixedElements_Collaborate(t *testing.T) {
	w := world.NewWorld()
	reg := visual.NewRegistry()

	fire, err := reg.AssignInGroup("Crimson", "Coder", "Team")
	if err != nil {
		t.Fatalf("AssignInGroup Crimson: %v", err)
	}
	water, err := reg.AssignInGroup("Azure", "Reviewer", "Team")
	if err != nil {
		t.Fatalf("AssignInGroup Azure: %v", err)
	}

	a := w.Spawn()
	world.Attach(w, a, fire)
	world.Attach(w, a, world.Alive{State: world.AliveRunning, Since: time.Now()})

	b := w.Spawn()
	world.Attach(w, b, water)
	world.Attach(w, b, world.Alive{State: world.AliveRunning, Since: time.Now()})

	tr := transport.NewLocalTransport()
	defer tr.Close()
	_ = tr.Register(fire.Short(), testkit.EchoHandler())
	_ = tr.Register(water.Short(), testkit.EchoHandler())

	// fire sends to water, verify round-trip.
	ctx := context.Background()
	task, err := tr.SendMessage(ctx, water.Short(), transport.Message{
		From:         fire.Short(),
		To:           water.Short(),
		Role: "user",
		Content:      "review patch",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, subErr := tr.Subscribe(ctx, task.ID)
	if subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	var completed bool
	for ev := range ch {
		if ev.State == transport.TaskCompleted {
			completed = true
			if ev.Data == nil {
				t.Fatal("completed event should carry data")
			}
			if ev.Data.Role != "agent" {
				t.Errorf("Role = %q, want agent", ev.Data.Role)
			}
		}
	}
	if !completed {
		t.Error("task never completed")
	}
}

// Feature: Full stack — World + Identity + Transport + Signal + WorldView.
func TestE2E_FullStack_WorldIdentityTransportSignalView(t *testing.T) {
	w, agents := testkit.QuickWorld(3, "Calibration")
	tr := testkit.QuickTransport(w, agents)
	defer tr.Close()

	bus := signal.NewMemBus()
	view := visual.NewView(w)

	// 1. Subscribe for health changes.
	diffs := view.Subscribe(world.AliveType)

	ctx := context.Background()
	color0 := world.Get[visual.Color](w, agents[0])
	color1 := world.Get[visual.Color](w, agents[1])
	color2 := world.Get[visual.Color](w, agents[2])

	// 2. Agent 0 sends to Agent 1.
	task01, err := tr.SendMessage(ctx, color1.Short(), transport.Message{
		From:         color0.Short(),
		To:           color1.Short(),
		Role: "user",
		Content:      "step-1",
	})
	if err != nil {
		t.Fatalf("SendMessage 0->1: %v", err)
	}
	waitComplete(t, tr, task01)

	// 3. Agent 1 sends to Agent 2.
	task12, err := tr.SendMessage(ctx, color2.Short(), transport.Message{
		From:         color1.Short(),
		To:           color2.Short(),
		Role: "user",
		Content:      "step-2",
	})
	if err != nil {
		t.Fatalf("SendMessage 1->2: %v", err)
	}
	waitComplete(t, tr, task12)

	// 4. Agent 2 sends back to Agent 0.
	task20, err := tr.SendMessage(ctx, color0.Short(), transport.Message{
		From:         color2.Short(),
		To:           color0.Short(),
		Role: "agent",
		Content:      "step-3",
	})
	if err != nil {
		t.Fatalf("SendMessage 2->0: %v", err)
	}
	waitComplete(t, tr, task20)

	// 5. Emit signals to bus for each message.
	bus.Emit(&signal.Signal{Event: "message_sent", Agent: color0.Short()})
	bus.Emit(&signal.Signal{Event: "message_sent", Agent: color1.Short()})
	bus.Emit(&signal.Signal{Event: "message_sent", Agent: color2.Short()})

	// 6. Verify WorldView snapshot shows all active.
	snaps := view.Snapshot(world.AliveType)
	if len(snaps) != 3 {
		t.Errorf("snapshot count = %d, want 3", len(snaps))
	}
	for _, snap := range snaps {
		h, ok := snap.Components[world.AliveType]
		if !ok {
			t.Errorf("entity %d missing health in snapshot", snap.ID)
			continue
		}
		alive := h.(world.Alive)
		if alive.State != world.AliveRunning {
			t.Errorf("entity %d state = %s, want running", snap.ID, alive.State)
		}
	}

	// 7. Verify Stats.
	stats := view.Stats()
	if stats.TotalEntities != 3 {
		t.Errorf("Stats.TotalEntities = %d, want 3", stats.TotalEntities)
	}
	if stats.ByAlive[world.AliveRunning] != 3 {
		t.Errorf("Stats.ByState[Active] = %d, want 3", stats.ByAlive[world.AliveRunning])
	}
	if stats.Collectives != 1 {
		t.Errorf("Stats.Collectives = %d, want 1", stats.Collectives)
	}

	// 8. Verify Hierarchy (flat -- no parent).
	tree := view.Hierarchy()
	if len(tree) != 0 {
		t.Errorf("Hierarchy should be empty (no Hierarchy components), got %d roots", len(tree))
	}

	// 9. Verify signal bus recorded messages.
	testkit.AssertSignalCount(t, bus, "message_sent", 3)

	view.Unsubscribe(diffs)

	// Drain any buffered diffs to avoid goroutine leaks.
	for range diffs { //nolint:revive // drain loop
	}
}

// waitComplete subscribes to a task and blocks until it reaches TaskCompleted.
func waitComplete(t *testing.T, tr *transport.LocalTransport, task *transport.Task) {
	t.Helper()
	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe %s: %v", task.ID, err)
	}
	for ev := range ch {
		if ev.State == transport.TaskCompleted {
			return
		}
		if ev.State == transport.TaskFailed {
			t.Fatalf("task %s failed", task.ID)
		}
	}
}
