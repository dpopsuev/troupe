package broker_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

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

func TestLobby_ProxyFactory_CalledForExternal(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	proxyCalled := false
	proxyURL := ""
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		ProxyFactory: func(callbackURL string) transport.MsgHandler {
			proxyCalled = true
			proxyURL = callbackURL
			return func(_ context.Context, msg transport.Message) (transport.Message, error) {
				return transport.Message{Content: "proxied to " + callbackURL}, nil
			}
		},
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{
		Role:        "remote",
		CallbackURL: "https://remote.example.com/a2a",
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if !proxyCalled {
		t.Fatal("ProxyFactory should have been called for external agent")
	}
	if proxyURL != "https://remote.example.com/a2a" {
		t.Fatalf("ProxyFactory got URL %q, want https://remote.example.com/a2a", proxyURL)
	}
}

func TestLobby_ProxyFactory_NotCalledForInternal(t *testing.T) {
	proxyCalled := false
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     world.NewWorld(),
		Transport: transport.NewLocalTransport(),
		ProxyFactory: func(_ string) transport.MsgHandler {
			proxyCalled = true
			return nil
		},
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "internal"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if proxyCalled {
		t.Fatal("ProxyFactory should NOT be called for internal agent")
	}
}

// --- Heartbeat + stale detection ---

func TestLobby_Heartbeat(t *testing.T) {
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

	if err := lobby.Heartbeat(id); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestLobby_Heartbeat_UnknownEntity(t *testing.T) {
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     world.NewWorld(),
		Transport: transport.NewLocalTransport(),
	})

	if err := lobby.Heartbeat(999); err == nil {
		t.Fatal("heartbeat for unknown entity should error")
	}
}

func TestLobby_EvictStale(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "stale-worker"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	evicted := lobby.EvictStale(context.Background(), 0)
	if evicted != 1 {
		t.Fatalf("evicted %d, want 1", evicted)
	}
	if lobby.Count() != 0 {
		t.Fatalf("lobby has %d entries, want 0", lobby.Count())
	}
}

func TestLobby_ConcurrentAdmit_Race(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			_, _ = lobby.Admit(context.Background(), troupe.ActorConfig{
				Role: fmt.Sprintf("worker-%d", i),
			})
		}(i)
	}
	wg.Wait()

	if lobby.Count() != n {
		t.Fatalf("lobby has %d entries, want %d", lobby.Count(), n)
	}
	if w.Count() != n {
		t.Fatalf("World has %d entities, want %d", w.Count(), n)
	}
}

func TestLobby_EvictStale_HeartbeatKeepsAlive(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "active-worker"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	_ = lobby.Heartbeat(id)

	evicted := lobby.EvictStale(context.Background(), 10*time.Second)
	if evicted != 0 {
		t.Fatalf("evicted %d, want 0 (agent just heartbeated)", evicted)
	}
	if lobby.Count() != 1 {
		t.Fatalf("lobby has %d entries, want 1", lobby.Count())
	}
}
