package troupe

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dpopsuev/troupe/identity"
	"github.com/dpopsuev/troupe/internal/acp"
	"github.com/dpopsuev/troupe/internal/agent"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

// multiDriverAdapter wraps public Drivers as a warden.AgentSupervisor.
// Resolves the correct driver at Start() time based on a per-entity provider map.
type multiDriverAdapter struct {
	defaultDriver Driver
	drivers       map[string]Driver
	providers     map[world.EntityID]string // entity → provider, set before Fork
	mu            sync.Mutex
}

func (a *multiDriverAdapter) setProvider(id world.EntityID, provider string) {
	a.mu.Lock()
	a.providers[id] = provider
	a.mu.Unlock()
}

func (a *multiDriverAdapter) resolve(id world.EntityID) Driver {
	a.mu.Lock()
	provider := a.providers[id]
	a.mu.Unlock()
	if provider != "" && a.drivers != nil {
		if d, ok := a.drivers[provider]; ok {
			return d
		}
	}
	return a.defaultDriver
}

func (a *multiDriverAdapter) Start(ctx context.Context, id world.EntityID, config warden.AgentConfig) error {
	// Resolve driver by provider from config, falling back to default.
	drv := a.defaultDriver
	if config.Provider != "" && a.drivers != nil {
		if d, ok := a.drivers[config.Provider]; ok {
			drv = d
		}
	}
	if drv == nil {
		return fmt.Errorf("no driver for entity %d: %w", id, ErrNoDriver)
	}
	// Track which driver was used for this entity (for Stop).
	a.setProvider(id, config.Provider)
	return drv.Start(ctx, id, ActorConfig{Model: config.Model, Role: config.Role, Provider: config.Provider})
}

func (a *multiDriverAdapter) Stop(ctx context.Context, id world.EntityID) error {
	drv := a.resolve(id)
	if drv == nil && a.defaultDriver != nil {
		drv = a.defaultDriver
	}
	if drv == nil {
		return nil
	}
	return drv.Stop(ctx, id)
}

func (a *multiDriverAdapter) Healthy(_ context.Context, _ world.EntityID) bool {
	return true
}

// DefaultBroker is the standard Broker implementation. Wires World, Warden,
// Transport, Driver, Registry, and Signal Bus internally.
type DefaultBroker struct {
	world     *world.World
	warden    *warden.AgentWarden
	transport *transport.LocalTransport
	bus       signal.Bus
	registry  *identity.Registry
	hooks     []Hook
	driver    Driver // default driver (for optional interface checks)
	adapter   *multiDriverAdapter
	meter     Meter
}

// BrokerOption configures a DefaultBroker.
type BrokerOption func(*brokerConfig)

type brokerConfig struct {
	driver       Driver
	drivers      map[string]Driver // provider → driver
	hooks        []Hook
	pickStrategy PickStrategy
	meter        Meter
}

// WithDriver sets the agent driver. Default: ACP (subprocess + JSON-RPC).
func WithDriver(d Driver) BrokerOption {
	return func(c *brokerConfig) { c.driver = d }
}

// WithHook registers a lifecycle hook. Nil hooks are ignored.
func WithHook(h Hook) BrokerOption {
	return func(c *brokerConfig) {
		if h != nil {
			c.hooks = append(c.hooks, h)
		}
	}
}

// WithDriverFor registers a driver for a specific provider.
// Broker.Spawn routes to the matching driver based on ActorConfig.Provider.
func WithDriverFor(provider string, d Driver) BrokerOption {
	return func(c *brokerConfig) {
		if c.drivers == nil {
			c.drivers = make(map[string]Driver)
		}
		c.drivers[provider] = d
	}
}

// WithMeter sets the resource usage meter. Default: none.
func WithMeter(m Meter) BrokerOption {
	return func(c *brokerConfig) { c.meter = m }
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

	// Resolve the warden supervisor: multi-driver adapter or default ACP.
	adapter := &multiDriverAdapter{
		defaultDriver: cfg.driver,
		drivers:       cfg.drivers,
		providers:     make(map[world.EntityID]string),
	}
	var supervisor warden.AgentSupervisor
	if cfg.driver != nil || len(cfg.drivers) > 0 {
		supervisor = adapter
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
		hooks:     cfg.hooks,
		driver:    cfg.driver,
		adapter:   adapter,
		meter:     cfg.meter,
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
	// Driver environment validation (optional interface).
	drv := b.adapter.resolve(0) // check default driver
	if config.Provider != "" && b.adapter.drivers != nil {
		if d, ok := b.adapter.drivers[config.Provider]; ok {
			drv = d
		}
	}
	if drv != nil {
		if v, ok := drv.(DriverValidator); ok {
			if err := v.ValidateEnvironment(ctx); err != nil {
				return nil, fmt.Errorf("driver validate: %w", err)
			}
		}
	}

	// Pre-spawn hooks: any SpawnHook can reject.
	for _, h := range b.hooks {
		if sh, ok := h.(SpawnHook); ok {
			if err := sh.PreSpawn(ctx, config); err != nil {
				return nil, fmt.Errorf("hook %s pre-spawn: %w", sh.Name(), err)
			}
		}
	}

	role := config.Role
	if role == "" {
		role = "actor"
	}

	id, err := b.warden.Fork(ctx, role, warden.AgentConfig{
		Model:    config.Model,
		Provider: config.Provider,
	}, 0)

	var actor Actor
	if err == nil {
		actor = agent.NewSolo(id, role, b.world, b.warden, b.transport)
	}

	// Post-spawn hooks: observe result.
	for _, h := range b.hooks {
		if sh, ok := h.(SpawnHook); ok {
			sh.PostSpawn(ctx, config, actor, err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("broker spawn: %w", err)
	}

	// Wrap with perform hooks if any are registered.
	var performHooks []PerformHook
	for _, h := range b.hooks {
		if ph, ok := h.(PerformHook); ok {
			performHooks = append(performHooks, ph)
		}
	}
	if len(performHooks) > 0 {
		actor = newHookedActor(actor, performHooks)
	}

	return actor, nil
}

// Meter returns the resource usage meter (nil if none configured).
func (b *DefaultBroker) Meter() Meter { return b.meter }

// Signal returns the event bus.
func (b *DefaultBroker) Signal() signal.Bus { return b.bus }

// World returns the underlying ECS world (for advanced consumers).
func (b *DefaultBroker) World() *world.World { return b.world }
