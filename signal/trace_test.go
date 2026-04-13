package signal

import "testing"

// --- E2E Skeleton (TSK-116) ---

func TestTrace_E2E_BrokerAgentTool(t *testing.T) {
	// Full trace: broker spawns agent → agent thinks → agent uses tool → done.
	// One ByTraceID returns all events in order.
	log := NewMemLog()
	tid := "tr-e2e-001"

	log.Emit(Event{TraceID: tid, Source: "broker", Kind: "agent.spawned"})
	log.Emit(Think(tid, "agent-1", "analyzing error pattern"))
	log.Emit(Decide(tid, "agent-1", "use arch tool to check cycles"))
	log.Emit(Event{TraceID: tid, Source: "agent-1", Kind: "tool.invoked", Data: map[string]any{"tool": "arch"}})
	log.Emit(Event{TraceID: tid, Source: "agent-1", Kind: "tool.completed", Data: map[string]any{"tool": "arch", "tokens": 800}})
	log.Emit(Event{TraceID: tid, Source: "broker", Kind: "agent.completed"})

	events := log.ByTraceID(tid)
	if len(events) != 6 {
		t.Fatalf("ByTraceID returned %d events, want 6", len(events))
	}

	// All events carry the same trace ID.
	for i, e := range events {
		if e.TraceID != tid {
			t.Errorf("events[%d].TraceID = %q, want %q", i, e.TraceID, tid)
		}
	}

	// Order preserved.
	expectedKinds := []string{
		"agent.spawned",
		KindThink,
		KindDecide,
		"tool.invoked",
		"tool.completed",
		"agent.completed",
	}
	for i, want := range expectedKinds {
		if events[i].Kind != want {
			t.Errorf("events[%d].Kind = %q, want %q", i, events[i].Kind, want)
		}
	}
}

// --- Contract Tests (TSK-117) ---

func TestTrace_ByTraceID_FiltersCorrectly(t *testing.T) {
	log := NewMemLog()

	log.Emit(Event{TraceID: "tr-a", Source: "x", Kind: "one"})
	log.Emit(Event{TraceID: "tr-b", Source: "y", Kind: "two"})
	log.Emit(Event{TraceID: "tr-a", Source: "z", Kind: "three"})

	a := log.ByTraceID("tr-a")
	if len(a) != 2 {
		t.Errorf("tr-a events = %d, want 2", len(a))
	}

	b := log.ByTraceID("tr-b")
	if len(b) != 1 {
		t.Errorf("tr-b events = %d, want 1", len(b))
	}

	none := log.ByTraceID("tr-nonexistent")
	if len(none) != 0 {
		t.Errorf("nonexistent trace = %d, want 0", len(none))
	}
}

func TestTrace_ByTraceID_EmptyLog(t *testing.T) {
	log := NewMemLog()
	if events := log.ByTraceID("anything"); len(events) != 0 {
		t.Errorf("empty log should return 0, got %d", len(events))
	}
}

func TestTrace_CognitiveHelpers_ProduceEvents(t *testing.T) {
	tests := []struct {
		fn   func(string, string, string) Event
		kind string
	}{
		{Think, KindThink},
		{Decide, KindDecide},
		{Retry, KindRetry},
		{GiveUp, KindGiveUp},
	}

	for _, tt := range tests {
		e := tt.fn("tr-001", "agent-1", "test message")
		if e.TraceID != "tr-001" {
			t.Errorf("%s: TraceID = %q", tt.kind, e.TraceID)
		}
		if e.Source != "agent-1" {
			t.Errorf("%s: Source = %q", tt.kind, e.Source)
		}
		if e.Kind != tt.kind {
			t.Errorf("%s: Kind = %q", tt.kind, e.Kind)
		}
		if e.Data != "test message" {
			t.Errorf("%s: Data = %v", tt.kind, e.Data)
		}
	}
}

func TestTrace_EventsWithoutTraceID_StillStored(t *testing.T) {
	log := NewMemLog()
	log.Emit(Event{Source: "x", Kind: "untraced"})
	log.Emit(Event{TraceID: "tr-001", Source: "y", Kind: "traced"})

	// Untraced events don't appear in ByTraceID.
	traced := log.ByTraceID("tr-001")
	if len(traced) != 1 {
		t.Errorf("traced = %d, want 1", len(traced))
	}

	// But they're still in the log.
	all := log.Since(0)
	if len(all) != 2 {
		t.Errorf("all = %d, want 2", len(all))
	}
}
