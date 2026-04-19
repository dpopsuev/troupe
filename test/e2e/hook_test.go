package e2e_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/world"
)

type testKickHook struct {
	blockErr   error
	postCalled atomic.Int32
}

func (h *testKickHook) Name() string { return "test-kick" }

func (h *testKickHook) PreKick(_ context.Context, _ world.EntityID) error {
	return h.blockErr
}

func (h *testKickHook) PostKick(_ context.Context, _ world.EntityID, _ error) {
	h.postCalled.Add(1)
}

type testBanHook struct {
	blockErr   error
	postCalled atomic.Int32
}

func (h *testBanHook) Name() string { return "test-ban" }

func (h *testBanHook) PreBan(_ context.Context, _ world.EntityID, _ string) error {
	return h.blockErr
}

func (h *testBanHook) PostBan(_ context.Context, _ world.EntityID, _ string, _ error) {
	h.postCalled.Add(1)
}

func TestHook_PreKick_BlocksRemoval(t *testing.T) {
	hook := &testKickHook{blockErr: errors.New("agent is mid-transaction")}

	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		Hooks:     []broker.Hook{hook},
	})

	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "protected"})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	err = lobby.Kick(context.Background(), id)
	if err == nil {
		t.Fatal("Kick should be blocked by PreKick hook")
	}
	t.Logf("PreKick blocked: %v", err)

	if lobby.Count() != 1 {
		t.Error("agent should still be in lobby after blocked kick")
	}
}

func TestHook_PreKick_AllowsRemoval(t *testing.T) {
	hook := &testKickHook{}

	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		Hooks:     []broker.Hook{hook},
	})

	id, _ := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "removable"})
	lobby.Kick(context.Background(), id) //nolint:errcheck // test

	if hook.postCalled.Load() != 1 {
		t.Errorf("PostKick called %d times, want 1", hook.postCalled.Load())
	}
	if lobby.Count() != 0 {
		t.Error("agent should be removed after allowed kick")
	}
}

func TestHook_PreBan_BlocksBan(t *testing.T) {
	hook := &testBanHook{blockErr: errors.New("cannot ban VIP agent")}

	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		Hooks:     []broker.Hook{hook},
	})

	id, _ := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "vip"})

	err := lobby.Ban(context.Background(), id, "suspicious")
	if err == nil {
		t.Fatal("Ban should be blocked by PreBan hook")
	}
	t.Logf("PreBan blocked: %v", err)

	if lobby.IsBanned(id) {
		t.Error("should not be banned after blocked ban")
	}
}

func TestHook_PostBan_Observes(t *testing.T) {
	hook := &testBanHook{}

	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:     w,
		Transport: tr,
		Hooks:     []broker.Hook{hook},
	})

	id, _ := lobby.Admit(context.Background(), troupe.ActorConfig{Role: "suspect"})
	lobby.Ban(context.Background(), id, "testing") //nolint:errcheck // test

	if hook.postCalled.Load() != 1 {
		t.Errorf("PostBan called %d times, want 1", hook.postCalled.Load())
	}
}
