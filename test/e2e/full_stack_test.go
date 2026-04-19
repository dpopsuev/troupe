package e2e_test

import (
	"context"
	"testing"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/referee"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

type noopDriver struct{}

func (noopDriver) Start(_ context.Context, _ world.EntityID, _ troupe.ActorConfig) error { return nil }
func (noopDriver) Stop(_ context.Context, _ world.EntityID) error                        { return nil }

func fullStackBroker(t *testing.T, stub *testkit.StubProvider, tracker *billing.InMemoryTracker, ref *referee.Referee) troupe.Broker {
	t.Helper()
	opts := []broker.Option{
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	}
	if tracker != nil {
		opts = append(opts, broker.WithTracker(tracker))
	}
	if ref != nil {
		opts = append(opts, broker.WithReferee(ref))
	}
	b, err := broker.Default(opts...)
	if err != nil {
		t.Fatalf("broker.Default: %v", err)
	}
	return b
}

func TestFullStack_SpawnPerformWithAllPackages(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("full stack ok", 100, 50))
	tracker := billing.NewTracker()
	sc := referee.Scorecard{
		Name:      "full_stack",
		Threshold: 1,
		Rules:     []referee.ScorecardRule{{On: "dispatch_routed", Weight: 10}},
	}
	ref := referee.New(sc)

	b := fullStackBroker(t, stub, tracker, ref)
	ctx := context.Background()

	configs, err := b.Pick(ctx, troupe.Preferences{Role: "worker"})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if configs[0].Model == "" {
		t.Fatal("Arsenal should resolve a model")
	}

	actor, err := b.Spawn(ctx, configs[0])
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	resp, err := actor.Perform(ctx, "test prompt")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp != "full stack ok" {
		t.Errorf("resp = %q, want full stack ok", resp)
	}

	summary := tracker.Summary()
	if summary.TotalPromptTokens != 100 {
		t.Errorf("prompt tokens = %d, want 100", summary.TotalPromptTokens)
	}
	if summary.TotalArtifactTokens != 50 {
		t.Errorf("artifact tokens = %d, want 50", summary.TotalArtifactTokens)
	}

	result := ref.Result()
	if !result.Pass {
		t.Errorf("referee should pass: score=%d threshold=%d", result.Score, result.Threshold)
	}

	db := b.(*broker.DefaultBroker)
	ids := world.Query[world.Alive](db.World())
	if len(ids) == 0 {
		t.Fatal("World should have at least one entity")
	}
	alive, ok := world.TryGet[world.Alive](db.World(), ids[0])
	if !ok || alive.State != world.AliveRunning {
		t.Errorf("entity should be AliveRunning, got %v", alive.State)
	}

	t.Logf("Full stack: model=%s tokens=%d referee=%d/%d entities=%d",
		configs[0].Model, summary.TotalTokens, result.Score, result.Threshold, len(ids))
}

func TestFullStack_BudgetEnforcement(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("expensive", 50000, 50000))
	tracker := billing.NewTracker()

	b := fullStackBroker(t, stub, tracker, nil)
	ctx := context.Background()

	actor, err := b.Spawn(ctx, troupe.ActorConfig{Model: "test", Provider: "test", Role: "spender"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	_, err = actor.Perform(ctx, "burn tokens")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}

	summary := tracker.Summary()
	if summary.TotalTokens == 0 {
		t.Error("tracker should have recorded tokens")
	}
	t.Logf("Budget: %d tokens recorded after 1 call", summary.TotalTokens)
}

func TestFullStack_SlowAgent_Timeout(t *testing.T) {
	stub := testkit.NewStubProvider()
	stub.Error = context.DeadlineExceeded

	b := fullStackBroker(t, stub, nil, nil)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{Model: "test", Provider: "test", Role: "slow"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = actor.Perform(ctx, "this should timeout")
	if err == nil {
		t.Fatal("expected error from deadline-exceeded provider")
	}
}

func TestFullStack_RefereeScoresSpawnAndPerform(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("scored", 10, 5))
	sc := referee.Scorecard{
		Name:      "scoring_test",
		Threshold: 10,
		Rules: []referee.ScorecardRule{
			{On: "dispatch_routed", Weight: 5},
		},
	}
	ref := referee.New(sc)

	b := fullStackBroker(t, stub, nil, ref)
	ctx := context.Background()

	a1, _ := b.Spawn(ctx, troupe.ActorConfig{Model: "test", Provider: "test", Role: "a"})
	a2, _ := b.Spawn(ctx, troupe.ActorConfig{Model: "test", Provider: "test", Role: "b"})

	a1.Perform(ctx, "prompt1") //nolint:errcheck // test
	a2.Perform(ctx, "prompt2") //nolint:errcheck // test

	result := ref.Result()
	if result.Score < 10 {
		t.Errorf("score = %d, want >= 10 (2 spawns × 5 weight)", result.Score)
	}
	t.Logf("Referee: score=%d events=%d", result.Score, len(result.Events))
}
