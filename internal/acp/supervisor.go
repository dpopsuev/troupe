// launcher.go — ACPLauncher implements warden.AgentSupervisor.
//
// Spawns ACP agent processes — one process per entity.
// Consumers get a warden.AgentSupervisor that manages ACP-backed agents.
package acp

import (
	"context"
	"fmt"
	"sync"

	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/world"
)

// ACPLauncher implements warden.AgentSupervisor by spawning ACP agent processes.
type ACPLauncher struct {
	mu      sync.RWMutex
	clients map[world.EntityID]*Client
}

// NewACPLauncher creates a launcher for ACP-based agents.
func NewACPLauncher() *ACPLauncher {
	return &ACPLauncher{
		clients: make(map[world.EntityID]*Client),
	}
}

// Start spawns an ACP agent process for the given entity.
// Uses defense-in-depth resolution: explicit → env var → PATH detect → fallback.
func (l *ACPLauncher) Start(ctx context.Context, id world.EntityID, config warden.AgentConfig) error {
	resolved := ResolveAgent(config.Model, config.Provider, config.Model)

	client, err := NewClient(resolved.CLI,
		WithModel(resolved.Model),
	)
	if err != nil {
		return fmt.Errorf("create ACP client for entity %d: %w", id, err)
	}

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start ACP agent for entity %d: %w", id, err)
	}

	l.mu.Lock()
	l.clients[id] = client
	l.mu.Unlock()

	return nil
}

// Stop kills the ACP agent process for the given entity.
func (l *ACPLauncher) Stop(ctx context.Context, id world.EntityID) error {
	l.mu.Lock()
	client, ok := l.clients[id]
	if ok {
		delete(l.clients, id)
	}
	l.mu.Unlock()

	if !ok {
		return nil
	}
	return client.Stop(ctx)
}

// Healthy returns true if the ACP agent process is still running.
// Checks both map presence and actual process state.
func (l *ACPLauncher) Healthy(_ context.Context, id world.EntityID) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	client, ok := l.clients[id]
	if !ok {
		return false
	}
	return client.ProcessAlive()
}

// Client returns the ACP Client for an entity (for sending messages).
func (l *ACPLauncher) Client(id world.EntityID) (*Client, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	c, ok := l.clients[id]
	return c, ok
}

// Compile-time check.
var _ warden.AgentSupervisor = (*ACPLauncher)(nil)
