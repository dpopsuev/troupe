package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
)

// agentEntry tracks a running agent.
type agentEntry struct {
	ID      world.EntityID
	Role    string
	Config  LaunchConfig
	Started time.Time
}

// AgentPool manages agent process lifecycles.
// Maps World entities to running processes via the Launcher interface.
type AgentPool struct {
	world     *world.World
	transport *transport.LocalTransport
	bus       signal.Bus
	launcher  Launcher
	mu        sync.RWMutex
	agents    map[world.EntityID]*agentEntry
}

// New creates an AgentPool.
func New(w *world.World, t *transport.LocalTransport, b signal.Bus, l Launcher) *AgentPool {
	return &AgentPool{
		world:     w,
		transport: t,
		bus:       b,
		launcher:  l,
		agents:    make(map[world.EntityID]*agentEntry),
	}
}

// Fork spawns a new agent: creates entity, attaches components,
// starts process, registers in transport, emits signal.
func (p *AgentPool) Fork(ctx context.Context, role string, config LaunchConfig) (world.EntityID, error) {
	// 1. Create entity.
	id := p.world.Spawn()

	// 2. Attach components.
	world.Attach(p.world, id, world.Health{State: world.Active, LastSeen: time.Now()})
	if config.Budget > 0 {
		world.Attach(p.world, id, world.Budget{Ceiling: config.Budget})
	}

	// 3. Start process via launcher.
	if err := p.launcher.Start(ctx, id, config); err != nil {
		p.world.Despawn(id)
		return 0, fmt.Errorf("fork %s: %w", role, err)
	}

	// 4. Register in transport.
	agentID := agentTransportID(id)
	p.transport.Register(agentID, func(ctx context.Context, msg transport.Message) (transport.Message, error) {
		// Default handler: echo. Consumers override via transport.Register.
		return transport.Message{From: agentID, Content: "ack"}, nil
	})

	// 5. Track.
	p.mu.Lock()
	p.agents[id] = &agentEntry{
		ID:      id,
		Role:    role,
		Config:  config,
		Started: time.Now(),
	}
	p.mu.Unlock()

	// 6. Emit signal.
	p.bus.Emit(&signal.Signal{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     signal.EventWorkerStarted,
		Agent:     signal.AgentWorker,
		Meta: map[string]string{
			signal.MetaKeyWorkerID: agentID,
			"role":                role,
		},
	})

	return id, nil
}

// Kill stops an agent: kills process, emits signal, despawns entity.
func (p *AgentPool) Kill(ctx context.Context, id world.EntityID) error {
	p.mu.Lock()
	entry, ok := p.agents[id]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("agent %d not found", id)
	}
	delete(p.agents, id)
	p.mu.Unlock()

	// Stop process.
	if err := p.launcher.Stop(ctx, id); err != nil {
		// Log but continue cleanup.
		_ = err
	}

	// Unregister transport.
	agentID := agentTransportID(id)
	p.transport.Unregister(agentID)

	// Update health.
	world.Attach(p.world, id, world.Health{State: world.Done, LastSeen: time.Now()})

	// Emit signal.
	p.bus.Emit(&signal.Signal{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     signal.EventWorkerStopped,
		Agent:     signal.AgentWorker,
		Meta: map[string]string{
			signal.MetaKeyWorkerID: agentID,
			"role":                entry.Role,
		},
	})

	// Despawn entity.
	p.world.Despawn(id)

	return nil
}

// KillAll stops all running agents. Called on shutdown.
func (p *AgentPool) KillAll(ctx context.Context) {
	p.mu.RLock()
	ids := make([]world.EntityID, 0, len(p.agents))
	for id := range p.agents {
		ids = append(ids, id)
	}
	p.mu.RUnlock()

	for _, id := range ids {
		p.Kill(ctx, id) //nolint:errcheck
	}
}

// Active returns all running entity IDs.
func (p *AgentPool) Active() []world.EntityID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ids := make([]world.EntityID, 0, len(p.agents))
	for id := range p.agents {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of running agents.
func (p *AgentPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}

// Get returns the entry for a running agent.
func (p *AgentPool) Get(id world.EntityID) (*agentEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.agents[id]
	return e, ok
}

func agentTransportID(id world.EntityID) string {
	return fmt.Sprintf("agent-%d", id)
}
