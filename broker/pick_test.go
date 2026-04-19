package broker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dpopsuev/troupe"
	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe/arsenal"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/collective"
	"github.com/dpopsuev/troupe/referee"
	"github.com/dpopsuev/troupe/resilience"
	"github.com/dpopsuev/troupe/testkit"
)

func TestFirstMatch_ReturnsRequestedCount(t *testing.T) {
	candidates := []troupe.ActorConfig{{Role: "a"}, {Role: "b"}, {Role: "c"}}
	result := broker.FirstMatch{}.Choose(context.Background(), candidates, troupe.Preferences{Count: 2})
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
}

func TestFirstMatch_ClampsToAvailable(t *testing.T) {
	candidates := []troupe.ActorConfig{{Role: "a"}}
	result := broker.FirstMatch{}.Choose(context.Background(), candidates, troupe.Preferences{Count: 5})
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
}

func TestFirstMatch_DefaultCountOne(t *testing.T) {
	candidates := []troupe.ActorConfig{{Role: "a"}, {Role: "b"}}
	result := broker.FirstMatch{}.Choose(context.Background(), candidates, troupe.Preferences{})
	if len(result) != 1 {
		t.Fatalf("got %d, want 1 (default)", len(result))
	}
}

var _ broker.PickStrategy = broker.FirstMatch{}

func TestPick_WithArsenal_SelectsModel(t *testing.T) {
	a, err := arsenal.NewArsenal("")
	if err != nil {
		t.Fatalf("NewArsenal: %v", err)
	}
	b := broker.New("", broker.WithArsenal(a), broker.WithDriver(noopDriver{}))
	configs, err := b.Pick(context.Background(), troupe.Preferences{Role: "coder"})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("len = %d, want 1", len(configs))
	}
	if configs[0].Model == "" {
		t.Error("expected Arsenal to resolve a model, got empty")
	}
	if configs[0].Provider == "" {
		t.Error("expected Arsenal to resolve a provider, got empty")
	}
	if configs[0].Role != "coder" {
		t.Errorf("Role = %q, want coder", configs[0].Role)
	}
	t.Logf("Arsenal picked: model=%s provider=%s", configs[0].Model, configs[0].Provider)
}

func TestPick_WithoutArsenal_Passthrough(t *testing.T) {
	b := broker.New("", broker.WithDriver(noopDriver{}))
	configs, err := b.Pick(context.Background(), troupe.Preferences{Model: "my-model", Role: "tester"})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if configs[0].Model != "my-model" {
		t.Errorf("Model = %q, want my-model (passthrough)", configs[0].Model)
	}
}

func TestSpawn_WithProviderResolver_CallsLLM(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("hello from LLM", 10, 5))

	resolver := func(name string) (anyllm.Provider, error) {
		return stub, nil
	}

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(resolver),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model:    "test-model",
		Provider: "test-provider",
		Role:     "worker",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	resp, err := actor.Perform(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp != "hello from LLM" {
		t.Errorf("got %q, want %q", resp, "hello from LLM")
	}
	if len(stub.Calls()) != 1 {
		t.Errorf("provider called %d times, want 1", len(stub.Calls()))
	}
}

func TestSpawn_WithTracker_RecordsTokens(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("tracked", 50, 25))
	tracker := billing.NewTracker()

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
		broker.WithTracker(tracker),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test-model", Provider: "test", Role: "worker",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	_, err = actor.Perform(context.Background(), "count my tokens")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}

	summary := tracker.Summary()
	if summary.TotalPromptTokens != 50 {
		t.Errorf("prompt tokens = %d, want 50", summary.TotalPromptTokens)
	}
	if summary.TotalArtifactTokens != 25 {
		t.Errorf("artifact tokens = %d, want 25", summary.TotalArtifactTokens)
	}
	t.Logf("Billing: %d prompt + %d artifact = %d total",
		summary.TotalPromptTokens, summary.TotalArtifactTokens, summary.TotalTokens)
}

func TestSpawnCollective_RaceStrategy(t *testing.T) {
	stub := testkit.NewStubProvider(
		testkit.TextResponse("agent-1 response", 10, 5),
		testkit.TextResponse("agent-2 response", 10, 5),
		testkit.TextResponse("agent-3 response", 10, 5),
	)

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	)

	actor, err := collective.SpawnCollective(context.Background(), b, 3, collective.Race{})
	if err != nil {
		t.Fatalf("SpawnCollective: %v", err)
	}

	resp, err := actor.Perform(context.Background(), "who wins?")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response from collective")
	}
	t.Logf("Race winner: %q", resp)
}

func TestDefault_PickSpawnPerform(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("default works", 10, 5))

	b, err := broker.Default(
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	configs, err := b.Pick(context.Background(), troupe.Preferences{Role: "test"})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if configs[0].Model == "" {
		t.Fatal("Pick should return a model from Arsenal")
	}
	t.Logf("Default Pick: model=%s provider=%s", configs[0].Model, configs[0].Provider)

	actor, err := b.Spawn(context.Background(), configs[0])
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	resp, err := actor.Perform(context.Background(), "hello from default")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp != "default works" {
		t.Errorf("got %q, want %q", resp, "default works")
	}
}

func TestSpawn_WithReferee_ScoresEvents(t *testing.T) {
	sc := referee.Scorecard{
		Name:      "spawn_test",
		Threshold: 1,
		Rules: []referee.ScorecardRule{
			{On: "dispatch_routed", Weight: 10},
		},
	}
	ref := referee.New(sc)

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithReferee(ref),
	)

	_, err := b.Spawn(context.Background(), troupe.ActorConfig{Role: "scorer"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	result := ref.Result()
	if result.Score == 0 {
		t.Error("referee should have scored events from spawn, got 0")
	}
	t.Logf("Referee: score=%d pass=%t events=%d", result.Score, result.Pass, len(result.Events))
}

func TestSpawn_WithRetry_RecoversTransient(t *testing.T) {
	callCount := 0
	stub := testkit.NewStubProvider()
	stub.Error = nil

	failTwiceThenSucceed := testkit.NewStubProvider(
		testkit.TextResponse("recovered", 10, 5),
	)
	failTwiceThenSucceed.Error = errors.New("transient")

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) {
			return &countingProvider{
				count:     &callCount,
				failUntil: 2,
				success:   testkit.TextResponse("retry worked", 10, 5),
			}, nil
		}),
		broker.WithRetry(resilience.RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   1 * time.Millisecond,
		}),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "retrier",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	resp, err := actor.Perform(context.Background(), "please retry")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp != "retry worked" {
		t.Errorf("resp = %q, want retry worked", resp)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls (2 failures + 1 success), got %d", callCount)
	}
	t.Logf("Retry: succeeded after %d attempts", callCount)
}

type countingProvider struct {
	count     *int
	failUntil int
	success   *anyllm.ChatCompletion
}

func (p *countingProvider) Name() string { return "counting" }

func (p *countingProvider) Completion(_ context.Context, _ anyllm.CompletionParams) (*anyllm.ChatCompletion, error) {
	*p.count++
	if *p.count <= p.failUntil {
		return nil, errors.New("transient failure")
	}
	return p.success, nil
}

func (p *countingProvider) CompletionStream(_ context.Context, _ anyllm.CompletionParams) (chunks <-chan anyllm.ChatCompletionChunk, errs <-chan error) {
	return nil, nil
}
