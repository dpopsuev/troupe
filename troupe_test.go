package troupe_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/testkit"
)

func TestTroupe_FullWorkflow(t *testing.T) {
	broker := testkit.NewMockBroker(3)

	tr := troupe.New(
		troupe.WithBroker(broker),
	)

	ctx := context.Background()

	// Spawn.
	actor, err := tr.Spawn(ctx, troupe.ActorConfig{Role: "worker"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Perform.
	resp, err := tr.Perform(ctx, actor, "hello")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if resp == "" {
		t.Fatal("Perform returned empty response")
	}
	t.Logf("Perform: %s", resp)

	// Discover.
	cards := tr.Discover("worker")
	if len(cards) == 0 {
		t.Fatal("Discover returned no cards")
	}
	t.Logf("Discover: %d agents", len(cards))
}

func TestTroupe_NoBroker_ReturnsError(t *testing.T) {
	tr := troupe.New()

	_, err := tr.Spawn(context.Background(), troupe.ActorConfig{Role: "test"})
	if err == nil {
		t.Fatal("Spawn without broker should error")
	}
}

func TestTroupe_NoAdmission_ReturnsError(t *testing.T) {
	tr := troupe.New()

	_, err := tr.Admit(context.Background(), troupe.ActorConfig{Role: "test"})
	if err == nil {
		t.Fatal("Admit without admission should error")
	}
}
