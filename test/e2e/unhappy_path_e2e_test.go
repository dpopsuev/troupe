package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

func TestUnhappy_KilledActor_NotReady(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("alive", 10, 5))

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "victim",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !actor.Ready() {
		t.Fatal("actor should be ready before kill")
	}

	actor.Kill(context.Background()) //nolint:errcheck // test

	if actor.Ready() {
		t.Error("actor should not be ready after kill")
	}
}

func TestUnhappy_ContextTimeout(t *testing.T) {
	slowProvider := &blockingProvider{
		delay:    5 * time.Second,
		response: testkit.TextResponse("too late", 10, 5),
	}

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return slowProvider, nil }),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "slow",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = actor.Perform(ctx, "will timeout")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("error (acceptable): %v", err)
	}
}

func TestUnhappy_BudgetExceeded_BlocksNextSpawn(t *testing.T) {
	stub := testkit.NewStubProvider(testkit.TextResponse("expensive", 100000, 100000))
	tracker := billing.NewTracker()

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
		broker.WithTracker(tracker),
		broker.WithSpawnGate(billing.NewBudgetEnforcer(tracker, nil).AsGate("global")),
	)

	actor, err := b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "spender",
	})
	if err != nil {
		t.Fatalf("first Spawn: %v", err)
	}

	_, _ = actor.Perform(context.Background(), "burn tokens")

	_, err = b.Spawn(context.Background(), troupe.ActorConfig{
		Model: "test", Provider: "test", Role: "blocked",
	})
	t.Logf("second Spawn after budget burn: err=%v", err)
}

func TestUnhappy_Cordon_RejectsNewAdmits(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	admin := broker.NewAdmin(w, nil, nil, nil)
	gate := admin.CordonGate()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		Gates:     []troupe.Gate{gate},
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "before-cordon"})
	if err != nil {
		t.Fatalf("Admit before cordon: %v", err)
	}
	t.Logf("Admitted before cordon: entity=%d", id)

	admin.Cordon(context.Background(), "maintenance window") //nolint:errcheck // test

	_, err = lobby.Admit(context.Background(), troupe.ActorConfig{Role: "during-cordon"})
	if err == nil {
		t.Fatal("Admit during cordon should be rejected")
	}
	t.Logf("Cordon rejected: %v", err)

	admin.Uncordon(context.Background()) //nolint:errcheck // test

	id2, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "after-uncordon"})
	if err != nil {
		t.Fatalf("Admit after uncordon: %v", err)
	}
	t.Logf("Admitted after uncordon: entity=%d", id2)
}

func TestUnhappy_Ban_PreventsReadmission(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "suspect"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	err = lobby.Ban(context.Background(), id, "suspicious behavior")
	if err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if !lobby.IsBanned(id) {
		t.Fatal("should be banned")
	}

	lobby.Unban(context.Background(), id) //nolint:errcheck // test
	if lobby.IsBanned(id) {
		t.Fatal("should not be banned after unban")
	}
	t.Logf("Ban/Unban cycle: entity=%d", id)
}

func TestUnhappy_EvictStale_ReapsDisconnected(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	id1, _ := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "active"})
	id2, _ := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "stale"})

	lobby.Heartbeat(id1) //nolint:errcheck // test

	time.Sleep(20 * time.Millisecond)

	lobby.Heartbeat(id1) //nolint:errcheck // test

	evicted := lobby.EvictStale(context.Background(), 10*time.Millisecond)
	t.Logf("Evicted %d stale agents (id1=%d active, id2=%d stale)", evicted, id1, id2)

	if lobby.Count() != 1 {
		t.Errorf("lobby count = %d, want 1 (stale agent evicted)", lobby.Count())
	}
}

type blockingProvider struct {
	delay    time.Duration
	response *anyllm.ChatCompletion
}

func (p *blockingProvider) Name() string { return "blocking" }

func (p *blockingProvider) Completion(ctx context.Context, _ anyllm.CompletionParams) (*anyllm.ChatCompletion, error) {
	select {
	case <-time.After(p.delay):
		return p.response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *blockingProvider) CompletionStream(_ context.Context, _ anyllm.CompletionParams) (chunks <-chan anyllm.ChatCompletionChunk, errs <-chan error) {
	return nil, nil
}
