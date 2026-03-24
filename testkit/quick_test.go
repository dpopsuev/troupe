package testkit

import (
	"context"
	"fmt"
	"testing"

	"github.com/dpopsuev/bugle"
	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
)

func TestQuickWorld_CreatesNAgents(t *testing.T) {
	w, agents := QuickWorld(5, "TestTeam")

	if len(agents) != 5 {
		t.Fatalf("len(agents) = %d, want 5", len(agents))
	}
	if w.Count() != 5 {
		t.Errorf("world.Count() = %d, want 5", w.Count())
	}
	for i, id := range agents {
		if !w.Alive(id) {
			t.Errorf("agent %d (id=%d) should be alive", i, id)
		}
		AssertEntityHas[palette.ColorIdentity](t, w, id)
		AssertEntityHas[bugle.Health](t, w, id)
	}
}

func TestQuickWorld_UniqueIdentities(t *testing.T) {
	_, agents := QuickWorld(10, "UniqueTeam")

	// QuickWorld uses DefaultStrategy which assigns unique colours via Registry.
	// We just need to verify the count is correct; Registry guarantees uniqueness
	// and would panic on collision.
	if len(agents) != 10 {
		t.Fatalf("len(agents) = %d, want 10", len(agents))
	}
}

func TestQuickTransport_RegistersHandlers(t *testing.T) {
	w, agents := QuickWorld(3, "TransportTeam")
	tr := QuickTransport(w, agents)
	defer tr.Close()

	// Each agent's Short() (colour name) should be registered as a handler.
	for _, id := range agents {
		color := world.Get[palette.ColorIdentity](w, id)
		task, err := tr.SendMessage(context.Background(), color.Short(), transport.Message{
			From:         "test-sender",
			To:           color.Short(),
			Performative: signal.Request,
			Content:      "ping",
		})
		if err != nil {
			t.Fatalf("SendMessage to %s: %v", color.Short(), err)
		}

		ch, err := tr.Subscribe(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("Subscribe %s: %v", task.ID, err)
		}

		var completed bool
		for ev := range ch {
			if ev.State == transport.TaskCompleted {
				completed = true
			}
		}
		if !completed {
			t.Errorf("handler for %s did not complete", color.Short())
		}
	}
}

func TestStubHandler_RepliesWithPerformative(t *testing.T) {
	handler := StubHandler(signal.Refuse)
	resp, err := handler(context.Background(), transport.Message{
		From:         "sender",
		To:           "receiver",
		Performative: signal.Request,
		Content:      "test",
	})
	if err != nil {
		t.Fatalf("StubHandler returned error: %v", err)
	}
	if resp.Performative != signal.Refuse {
		t.Errorf("Performative = %q, want %q", resp.Performative, signal.Refuse)
	}
}

func TestErrorHandler_ReturnsFail(t *testing.T) {
	handler := ErrorHandler(fmt.Errorf("boom"))
	_, err := handler(context.Background(), transport.Message{
		From:    "sender",
		Content: "test",
	})
	if err == nil {
		t.Fatal("ErrorHandler should return error")
	}
	if err.Error() != "boom" {
		t.Errorf("error = %q, want %q", err.Error(), "boom")
	}
}
