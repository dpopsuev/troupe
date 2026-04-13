package transport

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dpopsuev/troupe/signal"
)

// RunTransportContract runs the full transport contract test suite against
// any Transport implementation. Call from each implementation's test file.
//
//nolint:gocyclo,thelper // contract test suite — one function with 8 subtests, not a helper
func RunTransportContract(t *testing.T, factory func() Transport) {
	t.Run("SendMessage_RegisteredHandler", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		_ = tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
			return Message{From: "agent-a", Content: "reply: " + msg.Content}, nil
		})

		task, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "b", Content: "hello"})
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if task.ID == "" {
			t.Fatal("task ID must not be empty")
		}

		ch, err := tr.Subscribe(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		var completed bool
		for ev := range ch {
			if ev.State == TaskCompleted {
				completed = true
				if ev.Data == nil || ev.Data.Content != "reply: hello" {
					t.Errorf("Content = %v", ev.Data)
				}
			}
		}
		if !completed {
			t.Error("never received TaskCompleted")
		}
	})

	t.Run("SendMessage_UnregisteredAgent", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		_, err := tr.SendMessage(context.Background(), "ghost", Message{From: "a"})
		if err == nil {
			t.Fatal("expected error for unregistered agent")
		}
	})

	t.Run("Subscribe_TaskLifecycle", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		gate := make(chan struct{})
		_ = tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
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

		close(gate)

		hasCompleted := false
		for ev := range ch {
			if ev.State == TaskCompleted {
				hasCompleted = true
			}
		}
		// TaskWorking may be missed if subscriber registers after the
		// goroutine starts — this is acceptable. The hard contract is
		// that TaskCompleted is always delivered.
		if !hasCompleted {
			t.Error("expected TaskCompleted event")
		}
	})

	t.Run("HandlerError_TaskFailed", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		_ = tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
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
	})

	t.Run("Ask_BlocksUntilResponse", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		_ = tr.Register("agent-a", func(_ context.Context, msg Message) (Message, error) {
			return Message{From: "agent-a", Performative: signal.Confirm, Content: "ack: " + msg.Content}, nil
		})

		resp, err := tr.Ask(context.Background(), "agent-a", Message{From: "b", Performative: signal.Request, Content: "ping"})
		if err != nil {
			t.Fatalf("Ask: %v", err)
		}
		if resp.Content != "ack: ping" {
			t.Errorf("Content = %q", resp.Content)
		}
	})

	t.Run("Concurrent_10Agents", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		const agents = 10
		const msgsPerAgent = 5

		for i := range agents {
			aid := AgentID(fmt.Sprintf("agent-%d", i))
			_ = tr.Register(aid, func(_ context.Context, msg Message) (Message, error) {
				return Message{Content: "ack:" + msg.Content}, nil
			})
		}

		var wg sync.WaitGroup
		var completedCount atomic.Int64

		for i := range agents {
			for j := range msgsPerAgent {
				wg.Add(1)
				go func(ai, mi int) {
					defer wg.Done()
					aid := AgentID(fmt.Sprintf("agent-%d", ai))
					task, err := tr.SendMessage(context.Background(), aid, Message{
						From: "sender", Content: fmt.Sprintf("msg-%d-%d", ai, mi),
					})
					if err != nil {
						return
					}
					ch, err := tr.Subscribe(context.Background(), task.ID)
					if err != nil {
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
		want := int64(agents * msgsPerAgent)
		if got := completedCount.Load(); got != want {
			t.Errorf("completed = %d, want %d", got, want)
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		tr := factory()
		defer tr.Close()

		_ = tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
			return Message{}, nil
		})
		tr.Unregister("agent-a")

		_, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
		if err == nil {
			t.Fatal("expected error after Unregister")
		}
	})

	t.Run("Close", func(t *testing.T) {
		tr := factory()

		_ = tr.Register("agent-a", func(_ context.Context, _ Message) (Message, error) {
			return Message{}, nil
		})
		tr.Close()

		_, err := tr.SendMessage(context.Background(), "agent-a", Message{From: "x"})
		if err == nil {
			t.Fatal("expected error after Close")
		}
	})
}

// Run contract against LocalTransport to prove the contract itself works.
func TestContract_LocalTransport(t *testing.T) {
	RunTransportContract(t, func() Transport {
		return NewLocalTransport()
	})
}

// Run contract against HTTPTransport — same contract, HTTP wire.
func TestContract_HTTPTransport(t *testing.T) {
	RunTransportContract(t, func() Transport {
		return NewHTTPTransport()
	})
}
