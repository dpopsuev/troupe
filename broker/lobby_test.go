package broker_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

func TestLobby_AdmitInternal(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	log := testkit.NewStubEventLog()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      w,
		Transport:  tr,
		ControlLog: log,
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "worker"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if id == 0 {
		t.Fatal("Admit returned zero ID")
	}

	if w.Count() != 1 {
		t.Fatalf("World has %d entities, want 1", w.Count())
	}

	events := log.Since(0)
	if len(events) == 0 {
		t.Fatal("ControlLog should have dispatch_routed event")
	}
	if events[0].Kind != signal.EventDispatchRouted {
		t.Fatalf("event kind = %q, want dispatch_routed", events[0].Kind)
	}
}

func TestLobby_AdmitExternal(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{
		Role:        "remote-worker",
		CallbackURL: "https://remote-agent.example.com",
	})
	if err != nil {
		t.Fatalf("Admit external: %v", err)
	}
	if id == 0 {
		t.Fatal("Admit returned zero ID")
	}

	if w.Count() != 1 {
		t.Fatalf("World has %d entities, want 1", w.Count())
	}
}

func TestLobby_GateRejects(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	log := testkit.NewStubEventLog()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      w,
		Transport:  tr,
		ControlLog: log,
		Gates:      []troupe.Gate{troupe.AlwaysDeny},
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "worker"})
	if err == nil {
		t.Fatal("gate should have rejected")
	}

	if w.Count() != 0 {
		t.Fatalf("World has %d entities, want 0 after rejection", w.Count())
	}

	events := log.Since(0)
	if len(events) == 0 {
		t.Fatal("ControlLog should have veto_applied event")
	}
	if events[0].Kind != signal.EventVetoApplied {
		t.Fatalf("event kind = %q, want veto_applied", events[0].Kind)
	}
}

func TestLobby_Dismiss(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "worker"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	if err := lobby.Dismiss(context.Background(), id); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	if lobby.Count() != 0 {
		t.Fatalf("Lobby has %d entries, want 0 after dismiss", lobby.Count())
	}
}

func TestLobby_SamePathInternalAndExternal(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	log := testkit.NewStubEventLog()

	gateCallCount := 0
	countingGate := troupe.Gate(func(_ context.Context, _ any) (bool, string, error) {
		gateCallCount++
		return true, "", nil
	})

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      w,
		Transport:  tr,
		ControlLog: log,
		Gates:      []troupe.Gate{countingGate},
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "internal"})
	if err != nil {
		t.Fatalf("Admit internal: %v", err)
	}

	_, err = lobby.Admit(context.Background(), troupe.ActorConfig{
		Role:        "external",
		CallbackURL: "https://remote.example.com",
	})
	if err != nil {
		t.Fatalf("Admit external: %v", err)
	}

	if gateCallCount != 2 {
		t.Fatalf("gate called %d times, want 2 (once per admission)", gateCallCount)
	}

	if w.Count() != 2 {
		t.Fatalf("World has %d entities, want 2", w.Count())
	}

	events := log.Since(0)
	if len(events) != 2 {
		t.Fatalf("ControlLog has %d events, want 2", len(events))
	}
}

func TestBroker_WithAdmission(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	b := broker.New("",
		broker.WithDriver(newProviderDriver()),
		broker.WithAdmission(lobby),
	)

	_, err := b.Spawn(context.Background(), troupe.ActorConfig{Role: "worker"})
	if err != nil {
		t.Fatalf("Spawn with Admission: %v", err)
	}

	if w.Count() != 1 {
		t.Fatalf("World has %d entities, want 1", w.Count())
	}
}
