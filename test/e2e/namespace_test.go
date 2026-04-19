package e2e_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

func TestNamespace_IsolatesAgentQueries(t *testing.T) {
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

	lobby.Admit(ctx, troupe.ActorConfig{Role: "worker", Namespace: "team-a"})   //nolint:errcheck // test
	lobby.Admit(ctx, troupe.ActorConfig{Role: "worker", Namespace: "team-a"})   //nolint:errcheck // test
	lobby.Admit(ctx, troupe.ActorConfig{Role: "worker", Namespace: "team-b"})   //nolint:errcheck // test
	lobby.Admit(ctx, troupe.ActorConfig{Role: "reviewer", Namespace: "team-b"}) //nolint:errcheck // test

	all := admin.Agents(ctx, troupe.AgentFilter{})
	if len(all) != 4 {
		t.Fatalf("all agents = %d, want 4", len(all))
	}

	teamA := admin.Agents(ctx, troupe.AgentFilter{Namespace: "team-a"})
	if len(teamA) != 2 {
		t.Fatalf("team-a agents = %d, want 2", len(teamA))
	}

	teamB := admin.Agents(ctx, troupe.AgentFilter{Namespace: "team-b"})
	if len(teamB) != 2 {
		t.Fatalf("team-b agents = %d, want 2", len(teamB))
	}

	teamBWorkers := admin.Agents(ctx, troupe.AgentFilter{Namespace: "team-b", Role: "worker"})
	if len(teamBWorkers) != 1 {
		t.Fatalf("team-b workers = %d, want 1", len(teamBWorkers))
	}

	noNs := admin.Agents(ctx, troupe.AgentFilter{Namespace: "nonexistent"})
	if len(noNs) != 0 {
		t.Fatalf("nonexistent namespace = %d, want 0", len(noNs))
	}

	t.Logf("Namespace isolation: 4 total, team-a=2, team-b=2, team-b workers=1")
}
