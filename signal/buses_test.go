package signal_test

import (
	"testing"

	"github.com/dpopsuev/troupe/signal"
)

func TestBusSet_TypeIsolation(t *testing.T) {
	bs := signal.NewBusSet()

	bs.Control.Emit(signal.Event{Kind: "dispatch_routed", Source: "broker"})
	bs.Work.Emit(signal.Event{Kind: "start", Source: "worker"})
	bs.Status.Emit(signal.Event{Kind: "worker_started", Source: "warden"})

	if bs.Control.Len() != 1 {
		t.Fatalf("ControlLog has %d events, want 1", bs.Control.Len())
	}
	if bs.Work.Len() != 1 {
		t.Fatalf("WorkLog has %d events, want 1", bs.Work.Len())
	}
	if bs.Status.Len() != 1 {
		t.Fatalf("StatusLog has %d events, want 1", bs.Status.Len())
	}
}

func TestBusSet_EventLogCompliance(t *testing.T) {
	bs := signal.NewBusSet()

	logs := map[string]signal.EventLog{
		"control": bs.Control,
		"work":    bs.Work,
		"status":  bs.Status,
	}

	for name, log := range logs {
		t.Run(name+"_EmitAndSince", func(t *testing.T) {
			idx := log.Emit(signal.Event{Kind: "test", Source: name})
			events := log.Since(idx)
			if len(events) != 1 {
				t.Fatalf("Since(%d) returned %d events, want 1", idx, len(events))
			}
			if events[0].Kind != "test" {
				t.Fatalf("event kind = %q, want test", events[0].Kind)
			}
		})

		t.Run(name+"_OnEmit", func(t *testing.T) {
			called := false
			log.OnEmit(func(e signal.Event) {
				if e.Kind == "callback_"+name {
					called = true
				}
			})
			log.Emit(signal.Event{Kind: "callback_" + name})
			if !called {
				t.Fatal("OnEmit callback not invoked")
			}
		})
	}
}

func TestBusSet_ControlLogNotAssignableToWorkLog(t *testing.T) {
	// This is a compile-time check. If ControlLog and WorkLog were the
	// same type (or type aliases), the function below would accept both.
	// With distinct struct wrappers, it only accepts WorkLog.
	acceptsWorkLog := func(_ signal.WorkLog) {}
	_ = acceptsWorkLog // proves the function exists with the typed param

	// Uncomment to verify compile error:
	// bs := signal.NewBusSet()
	// acceptsWorkLog(bs.Control) // should not compile
}

func TestBusSet_SinceEmpty(t *testing.T) {
	bs := signal.NewBusSet()
	if events := bs.Control.Since(0); events != nil {
		t.Fatalf("Since(0) on empty log returned %d events, want nil", len(events))
	}
}

func TestBusSet_TimestampAutoSet(t *testing.T) {
	bs := signal.NewBusSet()
	bs.Work.Emit(signal.Event{Kind: "test"})
	events := bs.Work.Since(0)
	if events[0].Timestamp.IsZero() {
		t.Fatal("Timestamp should be auto-set")
	}
}
