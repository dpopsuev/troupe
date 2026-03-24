//go:build e2e

package testkit

import (
	"context"
	"testing"

	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
	"github.com/dpopsuev/bugle/worldview"
)

// Feature: 2 same-provider agents.
func TestE2E_TwoAgents_RequestConfirm(t *testing.T) {
	w, agents := QuickWorld(2, "Refactor")
	tr := QuickTransport(w, agents)
	defer tr.Close()

	ctx := context.Background()
	color0 := world.Get[palette.ColorIdentity](w, agents[0])
	color1 := world.Get[palette.ColorIdentity](w, agents[1])

	task, err := tr.SendMessage(ctx, color1.Short(), transport.Message{
		From:         color0.Short(),
		To:           color1.Short(),
		Performative: signal.Request,
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
			if ev.Data.Performative != signal.Confirm {
				t.Errorf("Performative = %q, want %q", ev.Data.Performative, signal.Confirm)
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
	reg := palette.NewRegistry()

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
	world.Attach(w, a, world.Health{State: world.Active})

	b := w.Spawn()
	world.Attach(w, b, water)
	world.Attach(w, b, world.Health{State: world.Active})

	tr := transport.NewLocalTransport()
	defer tr.Close()
	tr.Register(fire.Short(), EchoHandler())
	tr.Register(water.Short(), EchoHandler())

	// fire sends to water, verify round-trip.
	ctx := context.Background()
	task, err := tr.SendMessage(ctx, water.Short(), transport.Message{
		From:         fire.Short(),
		To:           water.Short(),
		Performative: signal.Request,
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
			if ev.Data.Performative != signal.Confirm {
				t.Errorf("Performative = %q, want %q", ev.Data.Performative, signal.Confirm)
			}
		}
	}
	if !completed {
		t.Error("task never completed")
	}
}

// Feature: Full stack — World + Identity + Transport + Signal + WorldView.
func TestE2E_FullStack_WorldIdentityTransportSignalView(t *testing.T) {
	w, agents := QuickWorld(3, "Calibration")
	tr := QuickTransport(w, agents)
	defer tr.Close()

	bus := signal.NewMemBus()
	view := worldview.NewView(w)

	// 1. Subscribe for health changes.
	diffs := view.Subscribe(world.HealthType)

	ctx := context.Background()
	color0 := world.Get[palette.ColorIdentity](w, agents[0])
	color1 := world.Get[palette.ColorIdentity](w, agents[1])
	color2 := world.Get[palette.ColorIdentity](w, agents[2])

	// 2. Agent 0 sends to Agent 1.
	task01, err := tr.SendMessage(ctx, color1.Short(), transport.Message{
		From:         color0.Short(),
		To:           color1.Short(),
		Performative: signal.Request,
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
		Performative: signal.Request,
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
		Performative: signal.Confirm,
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
	snaps := view.Snapshot(world.HealthType)
	if len(snaps) != 3 {
		t.Errorf("snapshot count = %d, want 3", len(snaps))
	}
	for _, snap := range snaps {
		h, ok := snap.Components[world.HealthType]
		if !ok {
			t.Errorf("entity %d missing health in snapshot", snap.ID)
			continue
		}
		health := h.(world.Health)
		if health.State != world.Active {
			t.Errorf("entity %d state = %s, want active", snap.ID, health.State)
		}
	}

	// 7. Verify Stats.
	stats := view.Stats()
	if stats.TotalEntities != 3 {
		t.Errorf("Stats.TotalEntities = %d, want 3", stats.TotalEntities)
	}
	if stats.ByState[world.Active] != 3 {
		t.Errorf("Stats.ByState[Active] = %d, want 3", stats.ByState[world.Active])
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
	AssertSignalCount(t, bus, "message_sent", 3)

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
