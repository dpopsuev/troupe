package jericho

import (
	"context"
	"fmt"
	"strings"

	"github.com/dpopsuev/jericho/identity"
	"github.com/dpopsuev/jericho/internal/acp"
	"github.com/dpopsuev/jericho/internal/agent"
	"github.com/dpopsuev/jericho/internal/transport"
	"github.com/dpopsuev/jericho/internal/warden"
	"github.com/dpopsuev/jericho/signal"
	"github.com/dpopsuev/jericho/world"
)

// driverAdapter wraps a public Driver as a warden.AgentSupervisor.
type driverAdapter struct {
	driver Driver
}

func (a *driverAdapter) Start(ctx context.Context, id world.EntityID, config warden.AgentConfig) error {
	return a.driver.Start(ctx, id, ActorConfig{Model: config.Model, Role: config.Role})
}

func (a *driverAdapter) Stop(ctx context.Context, id world.EntityID) error {
	return a.driver.Stop(ctx, id)
}

func (a *driverAdapter) Healthy(_ context.Context, _ world.EntityID) bool {
	return true // default: driver-managed agents are healthy
}

// DefaultBroker is the standard Broker implementation. Wires World, Warden,
// Transport, Driver, Registry, and Signal Bus internally.
type DefaultBroker struct {
	world     *world.World
	warden    *warden.AgentWarden
	transport *transport.LocalTransport
	bus       signal.Bus
	registry  *identity.Registry
}

// BrokerOption configures a DefaultBroker.
type BrokerOption func(*brokerConfig)

type brokerConfig struct {
	driver Driver
}

// WithDriver sets the agent driver. Default: ACP (subprocess + JSON-RPC).
func WithDriver(d Driver) BrokerOption {
	return func(c *brokerConfig) { c.driver = d }
}

// NewBroker creates a Broker. If the endpoint is a remote URL (https://),
// returns a RemoteBroker that proxies over HTTP. Otherwise, returns a
// local DefaultBroker. Default driver: ACP.
func NewBroker(endpoint string, opts ...BrokerOption) Broker {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return newRemoteBroker(endpoint)
	}
	return newLocalBroker(opts...)
}

// newLocalBroker creates an in-process DefaultBroker.
func newLocalBroker(opts ...BrokerOption) *DefaultBroker {
	cfg := &brokerConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Resolve the warden supervisor: use custom Driver adapter or default ACP.
	var supervisor warden.AgentSupervisor
	if cfg.driver != nil {
		supervisor = &driverAdapter{driver: cfg.driver}
	} else {
		supervisor = acp.NewACPLauncher()
	}

	w := world.NewWorld()
	t := transport.NewLocalTransport()
	b := signal.NewMemBus()
	p := warden.NewWarden(w, t, b, supervisor)

	return &DefaultBroker{
		world:     w,
		warden:    p,
		transport: t,
		bus:       b,
		registry:  identity.NewRegistry(),
	}
}

// Pick returns actor configs matching preferences.
func (b *DefaultBroker) Pick(_ context.Context, prefs Preferences) ([]ActorConfig, error) {
	count := prefs.Count
	if count <= 0 {
		count = 1
	}

	configs := make([]ActorConfig, count)
	for i := range count {
		configs[i] = ActorConfig{
			Model: prefs.Model,
			Role:  prefs.Role,
		}
	}
	return configs, nil
}

// Spawn creates a running actor.
func (b *DefaultBroker) Spawn(ctx context.Context, config ActorConfig) (Actor, error) {
	role := config.Role
	if role == "" {
		role = "actor"
	}

	id, err := b.warden.Fork(ctx, role, warden.AgentConfig{
		Model: config.Model,
	}, 0)
	if err != nil {
		return nil, fmt.Errorf("broker spawn: %w", err)
	}

	return agent.NewSolo(id, role, b.world, b.warden, b.transport), nil
}

// Signal returns the event bus.
func (b *DefaultBroker) Signal() signal.Bus { return b.bus }

// World returns the underlying ECS world (for advanced consumers).
func (b *DefaultBroker) World() *world.World { return b.world }
