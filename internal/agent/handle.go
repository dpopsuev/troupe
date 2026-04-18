// Package agent provides a human-readable API over Bugle's internal
// subsystems (pool, transport, world, signal). Solo wraps a
// single agent handle — internal Actor implementation.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

// Solo wraps all subsystems into one human-readable object for a
// single agent. Created by Broker.Spawn via NewSolo.
type Solo struct {
	id        world.EntityID
	role      string
	world     *world.World
	pool      *warden.AgentWarden
	transport transport.Transport
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

// NewSolo creates a Solo handle for an existing entity.
func NewSolo(id world.EntityID, role string, w *world.World, p *warden.AgentWarden, t transport.Transport) *Solo {
	return &Solo{id: id, role: role, world: w, pool: p, transport: t}
}

// ID returns the agent's entity ID.
func (a *Solo) ID() world.EntityID { return a.id }

// Role returns the agent's staff role name.
func (a *Solo) Role() string { return a.role }

// String returns a human-readable label: "role(agent-N)".
func (a *Solo) String() string {
	return fmt.Sprintf("%s(%s)", a.role, a.agentID())
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

// Ready returns true if the agent can accept work (readiness probe).
// Implements troupe.Actor.
func (a *Solo) Ready() bool {
	ready, ok := world.TryGet[world.Ready](a.world, a.id)
	return ok && ready.Ready
}

// IsZombie returns true if the agent is finished but not yet reaped.
func (a *Solo) IsZombie() bool {
	return a.pool.IsZombie(a.id)
}

// Alive returns the agent's liveness component.
func (a *Solo) Alive() (world.Alive, bool) {
	return world.TryGet[world.Alive](a.world, a.id)
}

// Uptime returns how long the agent has been running (or total runtime if finished).
func (a *Solo) Uptime() time.Duration {
	return a.pool.Uptime(a.id)
}

// ---------------------------------------------------------------------------
// Messaging
// ---------------------------------------------------------------------------

// Perform sends a prompt to this agent and blocks until a response is received.
// Implements troupe.Actor.
func (a *Solo) Perform(ctx context.Context, content string) (string, error) {
	msg := transport.Message{
		From:         "agent",
		To:           a.agentID(),
		Performative: signal.Request,
		Content:      content,
	}
	resp, err := a.transport.Ask(ctx, a.agentID(), msg)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Broadcast sends a message to ALL agents with this agent's role.
func (a *Solo) Broadcast(ctx context.Context, content string) error {
	msg := transport.Message{
		From:         a.agentID(),
		Performative: signal.Inform,
		Content:      content,
	}
	_, err := a.transport.Broadcast(ctx, a.role, msg)
	return err
}

// Listen registers a simplified handler for incoming messages to this agent.
// The handler receives the message content and returns a response content string.
// It replaces any previously registered handler for this agent.
func (a *Solo) Listen(handler func(content string) string) {
	agentID := a.agentID()
	a.transport.Unregister(agentID)                                                                           // remove previous handler if any
	a.transport.Register(agentID, func(_ context.Context, msg transport.Message) (transport.Message, error) { //nolint:errcheck // Unregister guarantees slot is free
		resp := handler(msg.Content)
		return transport.Message{
			From:    agentID,
			Content: resp,
		}, nil
	})
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Spawn creates a child agent under this agent as parent.
func (a *Solo) Spawn(ctx context.Context, role string, config warden.AgentConfig) (*Solo, error) {
	id, err := a.pool.Fork(ctx, role, config, a.id)
	if err != nil {
		return nil, err
	}
	return &Solo{
		id:        id,
		role:      role,
		world:     a.world,
		pool:      a.pool,
		transport: a.transport,
	}, nil
}

// Kill stops this agent.
func (a *Solo) Kill(ctx context.Context) error {
	return a.pool.Kill(ctx, a.id)
}

// Wait blocks until this agent finishes and returns its exit status.
func (a *Solo) Wait(ctx context.Context) (*warden.ExitStatus, error) {
	return a.pool.Wait(ctx, a.id)
}

// Children returns handles for all direct children of this agent.
func (a *Solo) Children() []*Solo {
	childIDs := a.pool.Children(a.id)
	handles := make([]*Solo, 0, len(childIDs))
	for _, cid := range childIDs {
		role := a.transport.Roles().RoleOf(string(agentTransportID(cid)))
		handles = append(handles, &Solo{
			id:        cid,
			role:      role,
			world:     a.world,
			pool:      a.pool,
			transport: a.transport,
		})
	}
	return handles
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// agentID returns the transport-level identifier for this agent.
func (a *Solo) agentID() transport.AgentID {
	return agentTransportID(a.id)
}

// agentTransportID converts an EntityID to the transport agent ID string.
func agentTransportID(id world.EntityID) transport.AgentID {
	return transport.AgentID(fmt.Sprintf("agent-%d", id))
}
