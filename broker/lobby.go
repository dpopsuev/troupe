package broker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	troupe "github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/visual"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

const (
	logKeyAgentID    = "agent_id"
	logKeyLobbyCount = "lobby_count"
	logKeyEntityID   = "entity_id"
	logKeyReason     = "reason"
)

var _ troupe.Admission = (*Lobby)(nil)

// Lobby is the universal admission system for all agents.
// Both internal spawns and external registrations go through Admit.
type Lobby struct {
	world        *world.World
	transport    transport.Transport
	controlLog   signal.EventLog
	registry     *visual.Registry
	gate           troupe.Gate
	proxyFactory   ProxyFactory
	handlerFactory HandlerFactory
	hooks          []Hook

	mu      sync.RWMutex
	entries map[world.EntityID]*lobbyEntry
	banned  map[world.EntityID]string // id → reason
}

type lobbyEntry struct {
	config   troupe.ActorConfig
	admitted time.Time
	lastSeen time.Time
}

// ProxyFactory builds a transport message handler for an external agent.
// The callbackURL is the agent's A2A endpoint for forwarding messages.
type ProxyFactory func(callbackURL string) transport.MsgHandler

// HandlerFactory builds a transport message handler for an internal agent.
// Called during Admit when the agent is not external. If nil, a default
// ack handler is registered.
type HandlerFactory func(id world.EntityID, config troupe.ActorConfig) transport.MsgHandler

// LobbyConfig configures a Lobby.
type LobbyConfig struct {
	World          *world.World
	Transport      transport.Transport
	ControlLog     signal.EventLog
	Registry       *visual.Registry
	Gates          []troupe.Gate
	ProxyFactory   ProxyFactory
	HandlerFactory HandlerFactory
	Hooks          []Hook
}

// NewLobby creates an Admission implementation.
func NewLobby(cfg LobbyConfig) *Lobby {
	var gate troupe.Gate
	if len(cfg.Gates) > 0 {
		gate = troupe.ComposeGates(cfg.Gates...)
	}
	return &Lobby{
		world:        cfg.World,
		transport:    cfg.Transport,
		controlLog:   cfg.ControlLog,
		registry:     cfg.Registry,
		gate:           gate,
		proxyFactory:   cfg.ProxyFactory,
		handlerFactory: cfg.HandlerFactory,
		hooks:          cfg.Hooks,
		entries:        make(map[world.EntityID]*lobbyEntry),
		banned:         make(map[world.EntityID]string),
	}
}

// Admit registers an agent into the World.
func (l *Lobby) Admit(ctx context.Context, config troupe.ActorConfig) (world.EntityID, error) {
	if l.gate != nil {
		allowed, reason, err := l.gate(ctx, config)
		if err != nil {
			slog.WarnContext(ctx, "admission gate error",
				slog.String("role", config.Role),
				slog.String("error", err.Error()),
			)
			return 0, fmt.Errorf("admission gate: %w", err)
		}
		if !allowed {
			slog.WarnContext(ctx, "admission rejected",
				slog.String("role", config.Role),
				slog.String("reason", reason),
				slog.Bool("external", config.IsExternal()),
			)
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
	if config.Namespace != "" {
		world.Attach(l.world, id, world.Namespace{Name: config.Namespace})
	}

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
		var handler transport.MsgHandler
		if l.proxyFactory != nil {
			handler = l.proxyFactory(config.CallbackURL)
		} else {
			handler = func(_ context.Context, _ transport.Message) (transport.Message, error) {
				return transport.Message{From: agentID, Content: "proxy: " + config.CallbackURL}, nil
			}
		}
		if err := l.transport.Register(agentID, handler); err != nil {
			slog.WarnContext(ctx, "admission transport register failed",
				slog.String("role", role),
				slog.String("agent_id", string(agentID)),
				slog.String("error", err.Error()),
			)
			l.world.Despawn(id)
			return 0, fmt.Errorf("admission transport register: %w", err)
		}
	} else {
		var handler transport.MsgHandler
		if l.handlerFactory != nil {
			handler = l.handlerFactory(id, config)
		} else {
			handler = func(_ context.Context, _ transport.Message) (transport.Message, error) {
				return transport.Message{From: agentID, Content: "ack"}, nil
			}
		}
		if err := l.transport.Register(agentID, handler); err != nil {
			slog.WarnContext(ctx, "admission transport register failed",
				slog.String("role", role),
				slog.String("agent_id", string(agentID)),
				slog.String("error", err.Error()),
			)
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

	slog.InfoContext(ctx, "agent admitted",
		slog.String("agent_id", string(agentID)),
		slog.String("role", role),
		slog.Bool("external", config.IsExternal()),
		slog.Int("lobby_count", l.Count()),
	)

	return id, nil
}

// Kick removes an agent from the World. KickHooks can block.
func (l *Lobby) Kick(ctx context.Context, id world.EntityID) error {
	for _, h := range l.hooks {
		if kh, ok := h.(KickHook); ok {
			if err := kh.PreKick(ctx, id); err != nil {
				return fmt.Errorf("hook %s pre-kick: %w", kh.Name(), err)
			}
		}
	}
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
		"action":   "kick",
	})

	slog.InfoContext(ctx, "agent kicked",
		slog.String(logKeyAgentID, string(agentID)),
		slog.Int(logKeyLobbyCount, l.Count()),
	)

	for _, h := range l.hooks {
		if kh, ok := h.(KickHook); ok {
			kh.PostKick(ctx, id, nil)
		}
	}

	return nil
}

// Ban kicks an agent and adds it to the deny list. BanHooks can block.
func (l *Lobby) Ban(ctx context.Context, id world.EntityID, reason string) error {
	for _, h := range l.hooks {
		if bh, ok := h.(BanHook); ok {
			if err := bh.PreBan(ctx, id, reason); err != nil {
				return fmt.Errorf("hook %s pre-ban: %w", bh.Name(), err)
			}
		}
	}
	if err := l.Kick(ctx, id); err != nil {
		return err
	}
	l.mu.Lock()
	l.banned[id] = reason
	l.mu.Unlock()

	slog.InfoContext(ctx, "agent banned",
		slog.Uint64(logKeyEntityID, uint64(id)),
		slog.String(logKeyReason, reason),
	)

	for _, h := range l.hooks {
		if bh, ok := h.(BanHook); ok {
			bh.PostBan(ctx, id, reason, nil)
		}
	}

	return nil
}

// Unban removes an agent from the deny list.
func (l *Lobby) Unban(_ context.Context, id world.EntityID) error {
	l.mu.Lock()
	delete(l.banned, id)
	l.mu.Unlock()
	return nil
}

// IsBanned reports whether an agent is on the deny list.
func (l *Lobby) IsBanned(id world.EntityID) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, banned := l.banned[id]
	return banned
}

// Heartbeat updates the last-seen timestamp for an admitted agent.
func (l *Lobby) Heartbeat(id world.EntityID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[id]
	if !ok {
		return fmt.Errorf("heartbeat: unknown entity %d", id)
	}
	entry.lastSeen = time.Now()
	return nil
}

// EvictStale dismisses agents that haven't heartbeated within ttl.
// Returns the number of agents evicted.
func (l *Lobby) EvictStale(ctx context.Context, ttl time.Duration) int {
	l.mu.RLock()
	var stale []world.EntityID
	now := time.Now()
	for id, entry := range l.entries {
		if now.Sub(entry.lastSeen) > ttl {
			stale = append(stale, id)
		}
	}
	l.mu.RUnlock()

	for _, id := range stale {
		slog.WarnContext(ctx, "evicting stale agent",
			slog.String("agent_id", fmt.Sprintf("agent-%d", id)),
			slog.Duration("silent_for", now.Sub(l.entries[id].lastSeen)),
		)
		l.Kick(ctx, id) //nolint:errcheck // best-effort during eviction
	}
	return len(stale)
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
