package transport

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/bugle"
	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/world"
)

// ---------------------------------------------------------------------------
// LocalTransport tests
// ---------------------------------------------------------------------------

func TestLocal_SendMessage_RegisteredHandler(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
		return Message{
			From:    "agent-a",
			To:      msg.From,
			Content: "reply: " + msg.Content,
		}, nil
	})

	task, err := tr.SendMessage(context.Background(), "agent-a", Message{
		From:    "agent-b",
		To:      "agent-a",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if task.ID == "" {
		t.Fatal("task ID must not be empty")
	}

	// Wait for completion via subscribe.
	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var completed bool
	for ev := range ch {
		if ev.State == TaskCompleted {
			completed = true
			if ev.Data == nil {
				t.Fatal("completed event should have data")
			}
			if ev.Data.Content != "reply: hello" {
				t.Errorf("Content = %q, want %q", ev.Data.Content, "reply: hello")
			}
		}
	}
	if !completed {
		t.Error("never received TaskCompleted event")
	}
}

func TestLocal_SendMessage_UnregisteredAgent(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	_, err := tr.SendMessage(context.Background(), "ghost", Message{
		From:    "agent-a",
		Content: "hello",
	})
	if err == nil {
		t.Fatal("expected error for unregistered agent")
	}
}

func TestLocal_Subscribe_ReceivesEvents(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	// Use a channel to control when the handler proceeds.
	gate := make(chan struct{})
	tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
		<-gate
		return Message{Content: "done"}, nil
	})

	task, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Let the handler finish.
	close(gate)

	states := make([]TaskState, 0, 4) //nolint:mnd // small pre-alloc for expected events
	for ev := range ch {
		states = append(states, ev.State)
	}

	// We should see at least "working" and "completed".
	hasWorking := false
	hasCompleted := false
	for _, s := range states {
		switch s {
		case TaskWorking:
			hasWorking = true
		case TaskCompleted:
			hasCompleted = true
		}
	}
	if !hasWorking {
		t.Error("expected TaskWorking event")
	}
	if !hasCompleted {
		t.Error("expected TaskCompleted event")
	}
}

func TestLocal_HandlerError_TaskFailed(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, fmt.Errorf("boom")
	})

	task, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var failed bool
	for ev := range ch {
		if ev.State == TaskFailed {
			failed = true
		}
	}
	if !failed {
		t.Error("expected TaskFailed event")
	}
}

func TestLocal_Concurrent_10Agents(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	const agents = 10
	const messagesPerAgent = 10

	for i := range agents {
		agentID := fmt.Sprintf("agent-%d", i)
		tr.Register(agentID, func(_ context.Context, msg Message) (Message, error) {
			return Message{Content: "ack:" + msg.Content}, nil
		})
	}

	var wg sync.WaitGroup
	var completedCount atomic.Int64

	for i := range agents {
		for j := range messagesPerAgent {
			wg.Add(1)
			go func(agentIdx, msgIdx int) {
				defer wg.Done()
				agentID := fmt.Sprintf("agent-%d", agentIdx)
				task, err := tr.SendMessage(context.Background(), agentID, Message{
					From:    "sender",
					Content: fmt.Sprintf("msg-%d-%d", agentIdx, msgIdx),
				})
				if err != nil {
					t.Errorf("SendMessage to %s: %v", agentID, err)
					return
				}

				ch, err := tr.Subscribe(context.Background(), task.ID)
				if err != nil {
					t.Errorf("Subscribe %s: %v", task.ID, err)
					return
				}

				for ev := range ch {
					if ev.State == TaskCompleted {
						completedCount.Add(1)
					}
				}
			}(i, j)
		}
	}

	wg.Wait()

	want := int64(agents * messagesPerAgent)
	got := completedCount.Load()
	if got != want {
		t.Errorf("completed = %d, want %d", got, want)
	}
}

func TestLocal_Unregister(t *testing.T) {
	tr := NewLocalTransport()
	defer tr.Close()

	tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, nil
	})
	tr.Unregister("agent-a")

	_, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
	if err == nil {
		t.Fatal("expected error after Unregister")
	}
}

func TestLocal_Close(t *testing.T) {
	tr := NewLocalTransport()

	tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
		return Message{}, nil
	})

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
	if err == nil {
		t.Fatal("expected error after Close")
	}
}

// ---------------------------------------------------------------------------
// AgentCard tests
// ---------------------------------------------------------------------------

func TestCardFromEntity_ColorIdentity(t *testing.T) {
	w := world.NewWorld()
	agent := w.Spawn()
	world.Attach(w, agent, palette.ColorIdentity{
		Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
	})

	card := CardFromEntity(w, agent)

	if card.ID != "agent-1" {
		t.Errorf("ID = %q, want %q", card.ID, "agent-1")
	}
	if card.Name != "Denim Writer of Indigo Refactor" {
		t.Errorf("Name = %q, want %q", card.Name, "Denim Writer of Indigo Refactor")
	}
	if card.Role != "Writer" {
		t.Errorf("Role = %q, want %q", card.Role, "Writer")
	}
	if card.Transport != "local" {
		t.Errorf("Transport = %q, want %q", card.Transport, "local")
	}
}

func TestCardFromEntity_WithHealth(t *testing.T) {
	w := world.NewWorld()
	agent := w.Spawn()
	world.Attach(w, agent, palette.ColorIdentity{
		Shade: "Azure", Colour: "Cerulean", Role: "Reviewer", Collective: "QA",
	})
	world.Attach(w, agent, bugle.Health{
		State:    bugle.Active,
		LastSeen: time.Now(),
	})

	card := CardFromEntity(w, agent)

	if card.Metadata == nil {
		t.Fatal("Metadata should not be nil when Health is attached")
	}
	if card.Metadata["health"] != "active" {
		t.Errorf("Metadata[health] = %q, want %q", card.Metadata["health"], "active")
	}
}

func TestCardFromEntity_NoComponents(t *testing.T) {
	w := world.NewWorld()
	agent := w.Spawn()

	card := CardFromEntity(w, agent)

	if card.ID != "agent-1" {
		t.Errorf("ID = %q, want %q", card.ID, "agent-1")
	}
	if card.Name != "" {
		t.Errorf("Name should be empty, got %q", card.Name)
	}
	if card.Role != "" {
		t.Errorf("Role should be empty, got %q", card.Role)
	}
	if card.Metadata != nil {
		t.Errorf("Metadata should be nil for bare entity, got %v", card.Metadata)
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests (BDD)
// ---------------------------------------------------------------------------

func TestAcceptance_LocalRoundTrip(t *testing.T) {
	// Feature: A2A Local Transport
	// Scenario: Full send -> handle -> complete round trip
	//   Given a LocalTransport with agent "responder" registered
	//   When agent "requester" sends a Request message
	//   Then a Task is created and transitions submitted -> working -> completed
	//   And the response message is received

	tr := NewLocalTransport()
	defer tr.Close()

	tr.Register("responder", func(_ context.Context, msg Message) (Message, error) {
		return Message{
			From:         "responder",
			To:           msg.From,
			Performative: signal.Confirm,
			Content:      "confirmed: " + msg.Content,
		}, nil
	})

	task, err := tr.SendMessage(context.Background(), "responder", Message{
		From:         "requester",
		To:           "responder",
		Performative: signal.Request,
		Content:      "analyze this",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var finalEvent Event
	for ev := range ch {
		finalEvent = ev
	}

	if finalEvent.State != TaskCompleted {
		t.Errorf("final state = %s, want completed", finalEvent.State)
	}
	if finalEvent.Data == nil {
		t.Fatal("completed event should carry response data")
	}
	if finalEvent.Data.Content != "confirmed: analyze this" {
		t.Errorf("response = %q", finalEvent.Data.Content)
	}
}

func TestAcceptance_PerformativeInMessage(t *testing.T) {
	// Feature: A2A Message Performative
	// Scenario: Performative propagates through transport
	//   Given a handler that echoes the performative
	//   When a Directive message is sent
	//   Then the handler receives the Directive performative
	//   And the response carries the echoed performative

	tr := NewLocalTransport()
	defer tr.Close()

	tr.Register("worker", func(_ context.Context, msg Message) (Message, error) {
		// Echo the received performative back.
		return Message{
			From:         "worker",
			Performative: msg.Performative,
			Content:      "echoed",
		}, nil
	})

	task, err := tr.SendMessage(context.Background(), "worker", Message{
		From:         "supervisor",
		Performative: signal.Directive,
		Content:      "do the thing",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	for ev := range ch {
		if ev.State == TaskCompleted {
			if ev.Data == nil {
				t.Fatal("expected data in completed event")
			}
			if ev.Data.Performative != signal.Directive {
				t.Errorf("Performative = %q, want %q", ev.Data.Performative, signal.Directive)
			}
		}
	}
}
