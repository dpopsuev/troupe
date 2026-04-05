package troupe_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/collective"
	"github.com/dpopsuev/troupe/testkit"
)

// --- Compile-time interface checks ---

var _ troupe.Broker = (*troupe.DefaultBroker)(nil)
var _ troupe.Actor = (*collective.Collective)(nil)
var _ troupe.PickStrategy = troupe.FirstMatch{}
var _ troupe.Meter = (*troupe.InMemoryMeter)(nil)

// --- Behavioral contract tests ---

func TestContract_MockBroker_SatisfiesBroker(t *testing.T) {
	ctx := context.Background()
	broker := testkit.NewMockBroker(3)

	// Pick must respect count.
	configs, err := broker.Pick(ctx, troupe.Preferences{Count: 2})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("Pick count = %d, want 2", len(configs))
	}

	// Spawn must return a ready actor.
	actor, err := broker.Spawn(ctx, configs[0])
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !actor.Ready() {
		t.Error("actor not ready after Spawn")
	}

	// Perform must return non-empty.
	resp, err := actor.Perform(ctx, "contract test")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Error("Perform returned empty response")
	}

	// Kill must make actor not ready.
	if err := actor.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if actor.Ready() {
		t.Error("actor ready after Kill")
	}
}

func TestContract_MockActor_SatisfiesActor(t *testing.T) {
	ctx := context.Background()
	actor := &testkit.MockActor{Name: "contract"}

	if !actor.Ready() {
		t.Error("fresh actor not ready")
	}

	resp, err := actor.Perform(ctx, "hello")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Error("empty response")
	}

	actor.Kill(ctx) //nolint:errcheck // best-effort cleanup
	if actor.Ready() {
		t.Error("actor ready after kill")
	}
}

func TestContract_LinearDirector_SatisfiesDirector(t *testing.T) {
	ctx := context.Background()
	broker := testkit.NewMockBroker(1)
	director := &testkit.LinearDirector{
		Steps: []testkit.Step{{Name: "s1", Prompt: "test"}},
	}

	events, err := director.Direct(ctx, broker)
	if err != nil {
		t.Fatalf("Direct: %v", err)
	}

	kinds := make([]troupe.EventKind, 0, 5) //nolint:mnd // started+completed+done
	for ev := range events {
		kinds = append(kinds, ev.Kind)
	}

	// Must emit at least Started, Completed, Done.
	if len(kinds) < 3 { //nolint:mnd // started+completed per step + done
		t.Errorf("got %d events, want >= 3", len(kinds))
	}
	if kinds[len(kinds)-1] != troupe.Done {
		t.Errorf("last event = %s, want Done", kinds[len(kinds)-1])
	}
}
