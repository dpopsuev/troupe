// Package pool manages agent process lifecycles with Linux-inspired
// process supervision: parent-child tracking, zombie reaping, orphan
// adoption. Maps Bugle World entities to running processes via the
// AgentSupervisor interface. Process-agnostic: consumers inject their own AgentSupervisor.
package warden

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dpopsuev/troupe/identity"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

// Sentinel errors.
var (
	ErrNotFound      = errors.New("agent not found")
	ErrNotOwner      = errors.New("caller is not the parent of this agent")
	ErrQuotaExceeded = errors.New("agent quota exceeded")
)

// agentEntry tracks a running or zombie agent.
type agentEntry struct {
	ID       world.EntityID
	ParentID world.EntityID // 0 = root agent (no parent)
	Role     string
	Config   AgentConfig
	Started  time.Time
	ExitCode ExitCode  // set when agent finishes
	ExitTime time.Time // zero = still running
}

// AgentWarden manages agent process lifecycles with process supervision.
type AgentWarden struct {
	world     *world.World
	transport transport.Transport
	log       signal.EventLog
	statusLog signal.EventLog // optional: lifecycle events go here instead of log
	launcher  AgentSupervisor
	mu        sync.RWMutex
	agents    map[world.EntityID]*agentEntry   // running agents
	zombies   map[world.EntityID]*agentEntry   // finished but not reaped
	subreaper world.EntityID                   // orphan adopter (0 = pool-level)
	autoReap  map[world.EntityID]bool          // parents with auto-reap enabled
	waitCh    map[world.EntityID]chan struct{} // notify Wait() callers
	registry  *identity.Registry               // optional color registry (nil = no color assignment)
	maxAgents int                              // 0 = unlimited
}

// New creates an AgentWarden.
func NewWarden(w *world.World, t transport.Transport, log signal.EventLog, l AgentSupervisor) *AgentWarden {
	return &AgentWarden{
		world:     w,
		transport: t,
		log:       log,
		launcher:  l,
		agents:    make(map[world.EntityID]*agentEntry),
		zombies:   make(map[world.EntityID]*agentEntry),
		autoReap:  map[world.EntityID]bool{0: true}, // root parent auto-reaps by default
		waitCh:    make(map[world.EntityID]chan struct{}),
	}
}

// StartProcess starts a process for a pre-admitted entity. The entity
// must already exist in the World with Alive/Ready components and be
// registered in Transport (via Admission.Admit). This method only
// handles process lifecycle — no entity creation, no transport registration.
func (p *AgentWarden) StartProcess(ctx context.Context, id world.EntityID, role string, config AgentConfig, parentID world.EntityID) error {
	if p.maxAgents > 0 && p.Count() >= p.maxAgents {
		return fmt.Errorf("%w: max %d agents", ErrQuotaExceeded, p.maxAgents)
	}

	if parentID > 0 {
		_ = p.world.Link(parentID, world.Supervises, id)
	}
	if config.Budget > 0 {
		world.Attach(p.world, id, world.Budget{Ceiling: config.Budget})
	}

	if err := p.launcher.Start(ctx, id, config); err != nil {
		return fmt.Errorf("start process %s: %w", role, err)
	}

	p.mu.Lock()
	p.agents[id] = &agentEntry{
		ID:       id,
		ParentID: parentID,
		Role:     role,
		Config:   config,
		Started:  time.Now(),
	}
	p.waitCh[id] = make(chan struct{})
	p.mu.Unlock()

	return nil
}

// Fork spawns a new agent with parent tracking: creates entity, attaches
// components, starts process, registers in transport, emits signal.
// parentID=0 means root agent (no parent).
// Deprecated: use Admission.Admit + StartProcess for new code.
func (p *AgentWarden) Fork(ctx context.Context, role string, config AgentConfig, parentID world.EntityID) (world.EntityID, error) {
	// 0. Quota check.
	if p.maxAgents > 0 && p.Count() >= p.maxAgents {
		return 0, fmt.Errorf("%w: max %d agents", ErrQuotaExceeded, p.maxAgents)
	}

	// 1. Create entity.
	id := p.world.Spawn()

	// 2. Attach components.
	world.Attach(p.world, id, world.Alive{State: world.AliveRunning, Since: time.Now()})
	world.Attach(p.world, id, world.Ready{Ready: true, LastSeen: time.Now()})
	if parentID > 0 {
		_ = p.world.Link(parentID, world.Supervises, id) // Edge replaces Hierarchy
	}
	if config.Budget > 0 {
		world.Attach(p.world, id, world.Budget{Ceiling: config.Budget})
	}

	// 2b. Assign color identity if registry is set.
	if p.registry != nil {
		if color, err := p.registry.Assign(role, ""); err == nil {
			world.Attach(p.world, id, color)
		}
	}

	// 3. Start process via launcher.
	if err := p.launcher.Start(ctx, id, config); err != nil {
		p.world.Despawn(id)
		return 0, fmt.Errorf("fork %s: %w", role, err)
	}

	// 4. Register in transport.
	agentID := agentTransportID(id)
	if err := p.transport.Register(agentID, func(ctx context.Context, msg transport.Message) (transport.Message, error) {
		return transport.Message{From: agentID, Content: "ack"}, nil
	}); err != nil {
		p.launcher.Stop(ctx, id) //nolint:errcheck // best-effort on registration failure
		p.world.Despawn(id)
		return 0, fmt.Errorf("fork %s transport register: %w", role, err)
	}
	p.transport.Roles().Register(string(agentID), role)

	// 5. Track with parent.
	p.mu.Lock()
	p.agents[id] = &agentEntry{
		ID:       id,
		ParentID: parentID,
		Role:     role,
		Config:   config,
		Started:  time.Now(),
	}
	// Prepare wait channel so Wait() can block.
	p.waitCh[id] = make(chan struct{})
	p.mu.Unlock()

	// 6. Emit signal with parent info.
	meta := map[string]string{
		signal.MetaKeyWorkerID: string(agentID),
		"role":                 role,
	}
	if parentID > 0 {
		meta["parent"] = string(agentTransportID(parentID))
	}
	if color, ok := world.TryGet[identity.Color](p.world, id); ok {
		meta[signal.MetaKeyShade] = color.Shade
		meta[signal.MetaKeyColor] = color.Name
	}
	p.lifecycleLog().Emit(signal.Event{
		Source: signal.AgentWorker,
		Kind:   signal.EventWorkerStarted,
		Data:   signal.Signal{Agent: signal.AgentWorker, Meta: meta},
	})

	return id, nil
}

// Kill stops an agent: stops process, moves to zombie state.
// The entry is NOT removed — parent must call Wait() to reap.
// If parent has AutoReap, the entry is removed immediately.
func (p *AgentWarden) Kill(ctx context.Context, id world.EntityID) error {
	p.mu.Lock()
	entry, ok := p.agents[id]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("%w: %d", ErrNotFound, id)
	}

	// Reparent orphans before removing parent.
	p.reparentOrphansLocked(id)

	// Move from agents → zombies.
	delete(p.agents, id)
	entry.ExitTime = time.Now()
	// ExitCode may already be set by KillWithCode. Don't overwrite.

	shouldAutoReap := p.autoReap[entry.ParentID]
	ch := p.waitCh[id]

	if !shouldAutoReap {
		p.zombies[id] = entry
	} else {
		delete(p.waitCh, id)
	}
	p.mu.Unlock()

	// Stop process.
	if err := p.launcher.Stop(ctx, id); err != nil {
		_ = err // log but continue cleanup
	}

	// Unregister transport.
	agentID := agentTransportID(id)
	p.transport.Roles().Unregister(string(agentID))
	p.transport.Unregister(agentID)

	// Remove inbound supervises edge — zombie is no longer a child.
	if entry.ParentID > 0 {
		_ = p.world.Unlink(entry.ParentID, world.Supervises, id)
	}

	// Update liveness — BEFORE notifying Wait() callers, because reap()
	// calls Despawn() which could race. TryAttach is safe on dead entities.
	world.TryAttach(p.world, id, world.Alive{State: world.AliveTerminated, ExitedAt: time.Now()})
	world.TryAttach(p.world, id, world.Ready{Ready: false, LastSeen: time.Now(), Reason: world.ReasonTerminated})

	// Emit signal.
	p.lifecycleLog().Emit(signal.Event{
		Source: signal.AgentWorker,
		Kind:   signal.EventWorkerStopped,
		Data: signal.Signal{
			Agent: signal.AgentWorker,
			Meta: map[string]string{
				signal.MetaKeyWorkerID: string(agentID),
				"role":                 entry.Role,
			},
		},
	})

	// Only despawn if auto-reaped (not zombie).
	if shouldAutoReap {
		p.world.Despawn(id)
	}

	// Notify Wait() callers — LAST, after all cleanup is done.
	// Wait() → reap() → Despawn() is safe because Alive is already set.
	if ch != nil {
		close(ch)
	}

	// Check restart policy — auto-restart if configured.
	if shouldRestart(entry) {
		go p.restartAgent(ctx, entry)
	}

	return nil
}

// KillGraceful sends a stop signal and waits up to gracePeriod for the agent
// to finish current work before force-killing. If gracePeriod is 0, defaults
// to the agent's configured GracePeriod (or 30s).
func (p *AgentWarden) KillGraceful(ctx context.Context, id world.EntityID, gracePeriod time.Duration) error {
	p.mu.RLock()
	entry, ok := p.agents[id]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %d", ErrNotFound, id)
	}

	if gracePeriod == 0 {
		gracePeriod = entry.Config.GracePeriod
	}
	if gracePeriod == 0 {
		gracePeriod = 30 * time.Second
	}

	// Mark not-ready so scheduler stops routing work.
	world.Attach(p.world, id, world.Ready{Ready: false, LastSeen: time.Now(), Reason: world.ReasonTerminating})

	// Wait for grace period or context cancellation.
	graceCtx, cancel := context.WithTimeout(ctx, gracePeriod)
	defer cancel()

	// Check if agent finishes naturally during grace period.
	ch := p.waitCh[id]
	if ch != nil {
		select {
		case <-ch:
			return nil // agent finished on its own
		case <-graceCtx.Done():
			// grace period expired, force kill
		}
	}

	return p.Kill(ctx, id)
}

// shouldRestart returns true if the agent should be restarted based on its
// restart policy and exit code.
func shouldRestart(entry *agentEntry) bool {
	switch entry.Config.RestartPolicy {
	case RestartAlways:
		return entry.ExitCode != ExitBudget // never restart budget-exceeded
	case RestartOnFailure:
		return entry.ExitCode != ExitSuccess && entry.ExitCode != ExitBudget
	default: // RestartNever or empty
		return false
	}
}

// restartAgent re-forks a terminated agent with the same config under the same parent.
func (p *AgentWarden) restartAgent(ctx context.Context, entry *agentEntry) {
	_, err := p.Fork(ctx, entry.Role, entry.Config, entry.ParentID)
	if err != nil {
		p.lifecycleLog().Emit(signal.Event{
			Source: signal.AgentSupervisor,
			Kind:   signal.EventWorkerError,
			Data: signal.Signal{
				Agent: signal.AgentSupervisor,
				Meta: map[string]string{
					"role":   entry.Role,
					"error":  fmt.Sprintf("restart failed: %v", err),
					"reason": "restart_failure",
				},
			},
		})
	}
}

// KillAll stops all running agents. Called on shutdown.
func (p *AgentWarden) KillAll(ctx context.Context) {
	p.mu.RLock()
	ids := make([]world.EntityID, 0, len(p.agents))
	for id := range p.agents {
		ids = append(ids, id)
	}
	p.mu.RUnlock()

	for _, id := range ids {
		p.Kill(ctx, id) //nolint:errcheck // best-effort cleanup during shutdown
	}

	// Also clean up any remaining zombies.
	p.mu.Lock()
	for id := range p.zombies {
		p.world.Despawn(id)
	}
	p.zombies = make(map[world.EntityID]*agentEntry)
	p.mu.Unlock()
}

// Active returns all running (non-zombie) entity IDs.
func (p *AgentWarden) Active() []world.EntityID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ids := make([]world.EntityID, 0, len(p.agents))
	for id := range p.agents {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of running (non-zombie) agents.
func (p *AgentWarden) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.agents)
}

// ZombieCount returns the number of zombie agents awaiting reaping.
func (p *AgentWarden) ZombieCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.zombies)
}

// get returns the entry for a running agent.
func (p *AgentWarden) get(id world.EntityID) (*agentEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.agents[id]
	return e, ok
}

// SetRegistry sets the color registry for automatic color assignment on Fork.
func (p *AgentWarden) SetRegistry(reg *identity.Registry) {
	p.registry = reg
}

// SetMaxAgents sets the maximum number of agents this pool can manage.
// 0 means unlimited (default).
func (p *AgentWarden) SetMaxAgents(n int) {
	p.maxAgents = n
}

// SetStatusLog sets an optional StatusLog for lifecycle events.
// When set, worker_started/stopped emit here instead of the main log.
func (p *AgentWarden) SetStatusLog(log signal.EventLog) {
	p.statusLog = log
}

func (p *AgentWarden) lifecycleLog() signal.EventLog {
	if p.statusLog != nil {
		return p.statusLog
	}
	return p.log
}

func agentTransportID(id world.EntityID) transport.AgentID {
	return transport.AgentID(fmt.Sprintf("agent-%d", id))
}
