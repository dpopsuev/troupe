package e2e_test

import (
	"context"
	"testing"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/collective"
	"github.com/dpopsuev/troupe/referee"
	"github.com/dpopsuev/troupe/testkit"
)

func TestCollective_WiredStack_Race(t *testing.T) {
	stub := testkit.NewStubProvider(
		testkit.TextResponse("agent-0", 10, 5),
		testkit.TextResponse("agent-1", 10, 5),
		testkit.TextResponse("agent-2", 10, 5),
	)
	tracker := billing.NewTracker()
	sc := referee.Scorecard{
		Name:      "collective_race",
		Threshold: 1,
		Rules:     []referee.ScorecardRule{{On: "dispatch_routed", Weight: 1}},
	}
	ref := referee.New(sc)

	b, err := broker.Default(
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
		broker.WithTracker(tracker),
		broker.WithReferee(ref),
	)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	actor, err := collective.SpawnCollective(context.Background(), b, 3, collective.Race{})
	if err != nil {
		t.Fatalf("SpawnCollective: %v", err)
	}

	resp, err := actor.Perform(context.Background(), "who wins the race?")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}

	summary := tracker.Summary()
	if summary.TotalTokens == 0 {
		t.Error("billing should have recorded tokens")
	}

	result := ref.Result()
	if result.Score < 3 {
		t.Errorf("referee score = %d, want >= 3 (3 spawns)", result.Score)
	}

	t.Logf("Collective Race: resp=%q tokens=%d referee=%d", resp, summary.TotalTokens, result.Score)
}

func TestCollective_WiredStack_RoundRobin(t *testing.T) {
	responses := make([]*anyllm.ChatCompletion, 10)
	for i := range responses {
		responses[i] = testkit.TextResponse("rr-response", 5, 3)
	}
	stub := testkit.NewStubProvider(responses...)

	b, err := broker.Default(
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	rr := &collective.RoundRobin{}
	actor, err := collective.SpawnCollective(context.Background(), b, 3, rr)
	if err != nil {
		t.Fatalf("SpawnCollective: %v", err)
	}

	for i := range 5 {
		resp, err := actor.Perform(context.Background(), "round-robin prompt")
		if err != nil {
			t.Fatalf("Perform %d: %v", i, err)
		}
		if resp != "rr-response" {
			t.Errorf("Perform %d: got %q", i, resp)
		}
	}

	t.Logf("RoundRobin: 5 calls distributed across 3 agents")
}

func TestCollective_WiredStack_AddRemove(t *testing.T) {
	stub := testkit.NewStubProvider(
		testkit.TextResponse("original", 10, 5),
		testkit.TextResponse("added", 10, 5),
	)

	b, err := broker.Default(
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	actor, err := collective.SpawnCollective(context.Background(), b, 1, collective.Race{})
	if err != nil {
		t.Fatalf("SpawnCollective: %v", err)
	}

	coll := actor.(*collective.Collective)
	if coll.Size() != 1 {
		t.Fatalf("size = %d, want 1", coll.Size())
	}

	extra, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "extra",
	})
	if err != nil {
		t.Fatalf("Spawn extra: %v", err)
	}

	coll.Add(extra) //nolint:errcheck // test
	if coll.Size() != 2 {
		t.Fatalf("size after add = %d, want 2", coll.Size())
	}

	coll.Remove(context.Background(), 0) //nolint:errcheck // test
	if coll.Size() != 1 {
		t.Fatalf("size after remove = %d, want 1", coll.Size())
	}

	t.Logf("Add/Remove through wired stack: spawn → add → remove")
}
