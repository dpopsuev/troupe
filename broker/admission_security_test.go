package broker_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

func TestSecurity_GateRejection_LoggedToControlLog(t *testing.T) {
	log := testkit.NewStubEventLog()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      world.NewWorld(),
		Transport:  transport.NewLocalTransport(),
		ControlLog: log,
		Gates:      []troupe.Gate{troupe.AlwaysDeny},
	})

	_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "intruder"})
	if err == nil {
		t.Fatal("gate should reject")
	}

	events := log.Since(0)
	found := false
	for _, e := range events {
		if e.Kind == signal.EventVetoApplied {
			found = true
		}
	}
	if !found {
		t.Fatal("veto_applied should be logged to ControlLog on rejection")
	}
}

func TestSecurity_QuotaGate_PreventsFlooding(t *testing.T) {
	const maxAgents = 5
	admitted := int32(0)

	quotaGate := troupe.Gate(func(_ context.Context, _ any) (bool, string, error) {
		if atomic.LoadInt32(&admitted) >= maxAgents {
			return false, "quota exceeded", nil
		}
		atomic.AddInt32(&admitted, 1)
		return true, "", nil
	})

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     world.NewWorld(),
		Transport: transport.NewLocalTransport(),
		Gates:     []troupe.Gate{quotaGate},
	})

	const attempts = 20
	var wg sync.WaitGroup
	successes := int32(0)
	failures := int32(0)

	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			_, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "flood"})
			if err != nil {
				atomic.AddInt32(&failures, 1)
			} else {
				atomic.AddInt32(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes > maxAgents {
		t.Fatalf("admitted %d agents, max should be %d", successes, maxAgents)
	}
	if failures == 0 {
		t.Fatal("some admissions should have been rejected by quota gate")
	}
}

func TestSecurity_Dismiss_UnknownEntity_NoPanic(t *testing.T) {
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     world.NewWorld(),
		Transport: transport.NewLocalTransport(),
	})

	err := lobby.Dismiss(context.Background(), 99999)
	if err != nil {
		t.Logf("Dismiss unknown entity returned error (acceptable): %v", err)
	}
}

func TestSecurity_GateRejection_NoEntityCreated(t *testing.T) {
	w := world.NewWorld()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: transport.NewLocalTransport(),
		Gates:     []troupe.Gate{troupe.AlwaysDeny},
	})

	before := w.Count()
	_, _ = lobby.Admit(context.Background(), troupe.ActorConfig{Role: "rejected"})
	after := w.Count()

	if after != before {
		t.Fatalf("World count changed from %d to %d — rejected agent should not create entity", before, after)
	}
}
