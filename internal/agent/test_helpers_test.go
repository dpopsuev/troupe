package agent

import (
	"context"
	"fmt"

	"github.com/dpopsuev/jericho/internal/transport"
	"github.com/dpopsuev/jericho/internal/warden"
	"github.com/dpopsuev/jericho/signal"
	"github.com/dpopsuev/jericho/world"
)

type testHarness struct {
	world     *world.World
	warden    *warden.AgentWarden
	transport *transport.LocalTransport
	bus       signal.Bus
}

func setup() *testHarness {
	ml := &mockLauncher{started: make(map[world.EntityID]bool), stopped: make(map[world.EntityID]bool)}
	w := world.NewWorld()
	t := transport.NewLocalTransport()
	b := signal.NewMemBus()
	p := warden.NewWarden(w, t, b, ml)
	return &testHarness{world: w, warden: p, transport: t, bus: b}
}

func (h *testHarness) Spawn(ctx context.Context, role string, config warden.AgentConfig) (*Solo, error) {
	id, err := h.warden.Fork(ctx, role, config, 0)
	if err != nil {
		return nil, err
	}
	solo := NewSolo(id, role, h.world, h.warden, h.transport)
	if config.Display != nil {
		solo.SetDisplay(*config.Display)
	}
	return solo, nil
}

func (h *testHarness) SpawnUnder(ctx context.Context, parent *Solo, role string, config warden.AgentConfig) (*Solo, error) {
	id, err := h.warden.Fork(ctx, role, config, parent.ID())
	if err != nil {
		return nil, err
	}
	return NewSolo(id, role, h.world, h.warden, h.transport), nil
}

func (h *testHarness) Pool() *warden.AgentWarden { return h.warden }
func (h *testHarness) Count() int                { return h.warden.Count() }
func (h *testHarness) Bus() signal.Bus           { return h.bus }

func (h *testHarness) FindByRole(role string) []*Solo {
	agentIDs := h.transport.Roles().AgentsForRole(role)
	handles := make([]*Solo, 0, len(agentIDs))
	for _, aid := range agentIDs {
		var eid world.EntityID
		if _, err := fmt.Sscanf(aid, "agent-%d", &eid); err != nil {
			continue
		}
		handles = append(handles, NewSolo(eid, role, h.world, h.warden, h.transport))
	}
	return handles
}

func (h *testHarness) KillAll(ctx context.Context) {
	h.warden.KillAll(ctx)
}
