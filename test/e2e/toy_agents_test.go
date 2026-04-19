package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/collective"
	"github.com/dpopsuev/troupe/resilience"
	"github.com/dpopsuev/troupe/testkit"
)

func TestToyAgent_EchoThroughCollectiveRace(t *testing.T) {
	agents := []troupe.Actor{
		&testkit.EchoAgent{},
		&testkit.EchoAgent{},
		&testkit.EchoAgent{},
	}

	c := collective.NewCollective(1, "echo-race", collective.Race{}, agents)
	ctx := context.Background()

	got, err := c.Perform(ctx, "ping")
	if err != nil {
		t.Fatalf("Race: %v", err)
	}
	if got != "ping" {
		t.Errorf("got %q, want %q", got, "ping")
	}
}

func TestToyAgent_SlowAgentTimesOutInRace(t *testing.T) {
	agents := []troupe.Actor{
		&testkit.SlowAgent{Delay: 5 * time.Second},
		&testkit.EchoAgent{},
	}

	c := collective.NewCollective(1, "slow-race", collective.Race{}, agents)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	got, err := c.Perform(ctx, "fast wins")
	if err != nil {
		t.Fatalf("Race: %v", err)
	}
	if got != "fast wins" {
		t.Errorf("got %q, want %q", got, "fast wins")
	}
}

func TestToyAgent_FailAgentTripsCircuitBreaker(t *testing.T) {
	agent := &testkit.FailAgent{FailEvery: 1}
	cb := resilience.NewCircuitBreaker(resilience.CircuitConfig{
		Threshold: 3,
		Cooldown:  100 * time.Millisecond,
	})
	ctx := context.Background()

	for range 3 {
		cb.Call(func() error {
			_, err := agent.Perform(ctx, "test")
			return err
		})
	}

	if cb.State() != resilience.CircuitOpen {
		t.Fatalf("state = %v, want open", cb.State())
	}

	err := cb.Call(func() error {
		_, err := agent.Perform(ctx, "should not reach")
		return err
	})
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestToyAgent_FailAgentWithRetryActor(t *testing.T) {
	agent := &testkit.FailAgent{FailEvery: 3}
	retryActor := resilience.NewRetryActor(agent, resilience.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
	})
	ctx := context.Background()

	got, err := retryActor.Perform(ctx, "hello")
	if err != nil {
		t.Fatalf("RetryActor: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestToyAgent_RoundRobinSkipsKilledAgent(t *testing.T) {
	a1 := &testkit.EchoAgent{}
	a2 := &testkit.EchoAgent{}
	a3 := &testkit.EchoAgent{}

	_ = a2.Kill(context.Background())

	rr := &collective.RoundRobin{}
	agents := []troupe.Actor{a1, a2, a3}
	c := collective.NewCollective(1, "rr-skip", rr, agents)
	ctx := context.Background()

	for range 4 {
		_, err := c.Perform(ctx, "test")
		if err != nil {
			t.Fatalf("RoundRobin: %v", err)
		}
	}

	if a2.Calls() != 0 {
		t.Errorf("killed agent received %d calls, want 0", a2.Calls())
	}
	if a1.Calls() == 0 || a3.Calls() == 0 {
		t.Errorf("healthy agents should have received calls: a1=%d a3=%d", a1.Calls(), a3.Calls())
	}
}

func TestToyAgent_BudgetAgentExceedsLimit(t *testing.T) {
	tracker := billing.NewTracker()
	enforcer := billing.NewBudgetEnforcer(tracker, nil)
	enforcer.SetLimit("spender", 0.0001)

	agent := &testkit.BudgetAgent{
		TokensPerCall: 500,
		Tracker:       tracker,
		AgentID:       "spender",
	}
	ctx := context.Background()

	_, _ = agent.Perform(ctx, "burn tokens")

	gate := enforcer.AsGate("spender")
	ok, _, err := gate(ctx, nil)
	if err != nil {
		t.Fatalf("gate error: %v", err)
	}
	if ok {
		t.Fatal("gate should reject over-budget agent")
	}
}

func TestToyAgent_ScaleCollectiveWithEcho(t *testing.T) {
	broker := testkit.NewMockBroker(5)
	agents := []troupe.Actor{&testkit.EchoAgent{}}

	c := collective.NewCollective(1, "scalable", collective.Race{}, agents)
	ctx := context.Background()

	err := c.Scale(ctx, 3, troupe.ActorConfig{Role: "echo"}, broker)
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	if !c.Ready() {
		t.Fatal("should be ready after scale")
	}
}

func TestToyAgent_KillStopsAllAgents(t *testing.T) {
	agents := []troupe.Actor{
		&testkit.EchoAgent{},
		&testkit.EchoAgent{},
		&testkit.EchoAgent{},
	}

	c := collective.NewCollective(1, "killable", collective.Race{}, agents)
	ctx := context.Background()

	if err := c.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if c.Ready() {
		t.Fatal("should not be ready after kill")
	}
	if c.Phase() != collective.PhaseSucceeded {
		t.Errorf("phase = %v, want succeeded", c.Phase())
	}
}
