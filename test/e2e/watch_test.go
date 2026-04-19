package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

func TestWatch_DeliversEventsOnAdmit(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := admin.Watch(ctx)

	lobby.Admit(context.Background(), troupe.ActorConfig{Role: "watched"}) //nolint:errcheck // test

	select {
	case evt := <-ch:
		if evt.Kind == "" {
			t.Error("expected non-empty event kind")
		}
		t.Logf("Watch received: kind=%s source=%s", evt.Kind, evt.Source)
	case <-time.After(1 * time.Second):
		t.Fatal("Watch channel did not receive event within 1s")
	}
}

func TestWatch_ClosesOnContextCancel(t *testing.T) {
	w := world.NewWorld()
	buses := signal.NewBusSet()

	admin := broker.NewAdmin(w, nil, nil, buses.Control)

	ctx, cancel := context.WithCancel(context.Background())
	ch := admin.Watch(ctx)

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Log("received buffered event before close (acceptable)")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Watch channel did not close within 1s after cancel")
	}
}
