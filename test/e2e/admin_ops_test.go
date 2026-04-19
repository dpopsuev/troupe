package e2e_test

import (
	"context"
	"testing"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

type noopSupervisor struct{}

func (noopSupervisor) Start(_ context.Context, _ world.EntityID, _ warden.AgentConfig) error {
	return nil
}
func (noopSupervisor) Stop(_ context.Context, _ world.EntityID) error   { return nil }
func (noopSupervisor) Healthy(_ context.Context, _ world.EntityID) bool { return true }

func TestAdmin_E2E_FullLifecycle(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	buses := signal.NewBusSet()
	p := warden.NewWarden(w, tr, buses.Status, noopSupervisor{})

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      w,
		Transport:  tr,
		ControlLog: buses.Control,
	})

	admin := broker.NewAdmin(w, p, lobby, buses.Control)
	ctx := context.Background()

	// 1. Admit agents via Lobby.
	id1, err := lobby.Admit(ctx, troupe.ActorConfig{Role: "worker"})
	if err != nil {
		t.Fatalf("Admit worker: %v", err)
	}
	id2, err := lobby.Admit(ctx, troupe.ActorConfig{Role: "reviewer"})
	if err != nil {
		t.Fatalf("Admit reviewer: %v", err)
	}

	// 2. Admin.Agents lists both.
	agents := admin.Agents(ctx, troupe.AgentFilter{})
	if len(agents) != 2 {
		t.Fatalf("Agents: got %d, want 2", len(agents))
	}

	// 3. Admin.Agents filters by role.
	workers := admin.Agents(ctx, troupe.AgentFilter{Role: "worker"})
	if len(workers) != 1 {
		t.Fatalf("Agents(worker): got %d, want 1", len(workers))
	}

	// 4. Admin.Inspect returns full detail.
	detail, err := admin.Inspect(ctx, id1)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if detail.ID != id1 {
		t.Errorf("Inspect ID = %d, want %d", detail.ID, id1)
	}
	if detail.Alive != world.AliveRunning {
		t.Errorf("Inspect Alive = %q, want running", detail.Alive)
	}
	if !detail.Ready {
		t.Error("Inspect Ready should be true")
	}

	// 5. Admin.Drain marks not ready.
	admin.Drain(ctx, id1) //nolint:errcheck // test
	detail, _ = admin.Inspect(ctx, id1)
	if detail.Ready {
		t.Error("should not be ready after drain")
	}
	if detail.Reason != "drained" {
		t.Errorf("reason = %q, want drained", detail.Reason)
	}

	// 6. Admin.Undrain restores.
	admin.Undrain(ctx, id1) //nolint:errcheck // test
	detail, _ = admin.Inspect(ctx, id1)
	if !detail.Ready {
		t.Error("should be ready after undrain")
	}

	// 7. Admin.SetBudget.
	admin.SetBudget(ctx, id1, 10.0) //nolint:errcheck // test
	detail, _ = admin.Inspect(ctx, id1)
	if detail.Budget.Ceiling != 10.0 {
		t.Errorf("budget ceiling = %f, want 10.0", detail.Budget.Ceiling)
	}

	// 8. Admin.Annotate.
	admin.Annotate(ctx, id1, "team", "platform") //nolint:errcheck // test
	anns := admin.Annotations(ctx, id1)
	if anns["team"] != "platform" {
		t.Errorf("annotation team = %q, want platform", anns["team"])
	}

	// 9. Admin.Kick removes agent.
	lobby.Kick(ctx, id2) //nolint:errcheck // test
	agents = admin.Agents(ctx, troupe.AgentFilter{})
	if len(agents) != 1 {
		t.Errorf("Agents after kick: got %d, want 1", len(agents))
	}

	t.Logf("Admin E2E: 2 admitted, filtered, inspected, drained, undrained, budgeted, annotated, kicked")
}

func TestAdmin_E2E_Cordon_BlocksSpawn(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	buses := signal.NewBusSet()
	p := warden.NewWarden(w, tr, buses.Status, noopSupervisor{})

	admin := broker.NewAdmin(w, p, nil, buses.Control)
	gate := admin.CordonGate()

	stub := testkit.NewStubProvider(testkit.TextResponse("ok", 10, 5))

	b := broker.New("",
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) { return stub, nil }),
		broker.WithSpawnGate(gate),
	)
	ctx := context.Background()

	// Spawn works before cordon.
	_, err := b.Spawn(ctx, troupe.ActorConfig{Role: "pre-cordon"})
	if err != nil {
		t.Fatalf("Spawn before cordon: %v", err)
	}

	// Cordon.
	admin.Cordon(ctx, "deploy in progress") //nolint:errcheck // test

	// Spawn rejected during cordon.
	_, err = b.Spawn(ctx, troupe.ActorConfig{Role: "during-cordon"})
	if err == nil {
		t.Fatal("Spawn during cordon should be rejected")
	}
	t.Logf("Cordon rejected spawn: %v", err)

	// Uncordon.
	admin.Uncordon(ctx) //nolint:errcheck // test

	// Spawn works after uncordon.
	_, err = b.Spawn(ctx, troupe.ActorConfig{Role: "post-cordon"})
	if err != nil {
		t.Fatalf("Spawn after uncordon: %v", err)
	}
}
