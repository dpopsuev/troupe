package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	troupe "github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/identity"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

var _ troupe.Admission = (*Lobby)(nil)

// Lobby is the universal admission system for all agents.
// Both internal spawns and external registrations go through Admit.
type Lobby struct {
	world      *world.World
	transport  transport.Transport
	controlLog signal.EventLog
	registry   *identity.Registry
	gate       troupe.Gate

	mu      sync.RWMutex
	entries map[world.EntityID]*lobbyEntry
}

type lobbyEntry struct {
	config   troupe.ActorConfig
	admitted time.Time
	lastSeen time.Time
}

// LobbyConfig configures a Lobby.
type LobbyConfig struct {
	World      *world.World
	Transport  transport.Transport
	ControlLog signal.EventLog
	Registry   *identity.Registry
	Gates      []troupe.Gate
}

// NewLobby creates an Admission implementation.
func NewLobby(cfg LobbyConfig) *Lobby {
	var gate troupe.Gate
	if len(cfg.Gates) > 0 {
		gate = troupe.ComposeGates(cfg.Gates...)
	}
	return &Lobby{
		world:      cfg.World,
		transport:  cfg.Transport,
		controlLog: cfg.ControlLog,
		registry:   cfg.Registry,
		gate:       gate,
		entries:    make(map[world.EntityID]*lobbyEntry),
	}
}

// Admit registers an agent into the World.
func (l *Lobby) Admit(ctx context.Context, config troupe.ActorConfig) (world.EntityID, error) {
	if l.gate != nil {
		allowed, reason, err := l.gate(ctx, config)
		if err != nil {
			return 0, fmt.Errorf("admission gate: %w", err)
		}
		if !allowed {
			l.emitControl(signal.EventVetoApplied, map[string]string{
				"role": config.Role, "reason": reason,
			})
			return 0, fmt.Errorf("admission rejected: %s", reason)
		}
	}

	id := l.world.Spawn()
	now := time.Now()

	world.Attach(l.world, id, world.Alive{State: world.AliveRunning, Since: now})
	world.Attach(l.world, id, world.Ready{Ready: true, LastSeen: now})

	role := config.Role
	if role == "" {
		role = "agent"
	}

	if l.registry != nil {
		if color, err := l.registry.Assign(role, ""); err == nil {
			world.Attach(l.world, id, color)
		}
	}

	agentID := transport.AgentID(fmt.Sprintf("agent-%d", id))
	if config.IsExternal() {
		if err := l.transport.Register(agentID, func(_ context.Context, msg transport.Message) (transport.Message, error) {
			return transport.Message{From: agentID, Content: "proxy: " + config.CallbackURL}, nil
		}); err != nil {
			l.world.Despawn(id)
			return 0, fmt.Errorf("admission transport register: %w", err)
		}
	} else {
		if err := l.transport.Register(agentID, func(_ context.Context, msg transport.Message) (transport.Message, error) {
			return transport.Message{From: agentID, Content: "ack"}, nil
		}); err != nil {
			l.world.Despawn(id)
			return 0, fmt.Errorf("admission transport register: %w", err)
		}
	}
	l.transport.Roles().Register(string(agentID), role)

	l.mu.Lock()
	l.entries[id] = &lobbyEntry{config: config, admitted: now, lastSeen: now}
	l.mu.Unlock()

	l.emitControl(signal.EventDispatchRouted, map[string]string{
		"role":     role,
		"agent_id": string(agentID),
		"external": fmt.Sprintf("%t", config.IsExternal()),
	})

	return id, nil
}

// Dismiss removes an agent from the World.
func (l *Lobby) Dismiss(_ context.Context, id world.EntityID) error {
	agentID := transport.AgentID(fmt.Sprintf("agent-%d", id))

	l.transport.Roles().Unregister(string(agentID))
	l.transport.Unregister(agentID)

	world.TryAttach(l.world, id, world.Alive{State: world.AliveTerminated, ExitedAt: time.Now()})
	world.TryAttach(l.world, id, world.Ready{Ready: false, LastSeen: time.Now(), Reason: world.ReasonTerminated})

	l.mu.Lock()
	delete(l.entries, id)
	l.mu.Unlock()

	l.world.Despawn(id)

	l.emitControl(signal.EventDispatchRouted, map[string]string{
		"agent_id": string(agentID),
		"action":   "dismiss",
	})

	return nil
}

// Count returns the number of admitted agents.
func (l *Lobby) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

func (l *Lobby) emitControl(kind string, meta map[string]string) {
	if l.controlLog == nil {
		return
	}
	l.controlLog.Emit(signal.Event{
		Source: "lobby",
		Kind:   kind,
		Data:   signal.Signal{Agent: "lobby", Event: kind, Meta: meta},
	})
}
