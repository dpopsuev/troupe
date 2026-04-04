package collective

import (
	"context"

	"github.com/dpopsuev/jericho/internal/agent"
	"github.com/dpopsuev/jericho/internal/transport"
	"github.com/dpopsuev/jericho/internal/warden"
	"github.com/dpopsuev/jericho/signal"
	"github.com/dpopsuev/jericho/world"
)

// testBrokerParts creates the subsystems for testing (replaces Staff).
type testBrokerParts struct {
	world     *world.World
	warden    *warden.AgentWarden
	transport *transport.LocalTransport
	bus       signal.Bus
}

func newTestParts() *testBrokerParts {
	w := world.NewWorld()
	t := transport.NewLocalTransport()
	b := signal.NewMemBus()
	p := warden.NewWarden(w, t, b, newMockDriver())
	return &testBrokerParts{world: w, warden: p, transport: t, bus: b}
}

func (tp *testBrokerParts) spawn(ctx context.Context, role string) (*agent.Solo, error) {
	id, err := tp.warden.Fork(ctx, role, warden.AgentConfig{}, 0)
	if err != nil {
		return nil, err
	}
	return agent.NewSolo(id, role, tp.world, tp.warden, tp.transport), nil
}
