package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	troupe "github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

var _ troupe.Admin = (*DefaultAdmin)(nil)

// DefaultAdmin implements the Admin control plane by wiring
// World (queries), Warden (kill/tree), and Lobby (kick/ban/cordon).
type DefaultAdmin struct {
	world   *world.World
	warden  *warden.AgentWarden
	lobby   *Lobby
	control signal.EventLog

	mu       sync.RWMutex
	cordoned bool
	reason   string
}

// NewAdmin creates an Admin implementation.
func NewAdmin(w *world.World, p *warden.AgentWarden, l *Lobby, control signal.EventLog) *DefaultAdmin {
	return &DefaultAdmin{
		world:   w,
		warden:  p,
		lobby:   l,
		control: control,
	}
}

func (a *DefaultAdmin) Agents(_ context.Context, filter troupe.AgentFilter) []troupe.AgentDetail {
	ids := world.Query[world.Alive](a.world)
	details := make([]troupe.AgentDetail, 0, len(ids))

	for _, id := range ids {
		d := a.buildDetail(id)

		if filter.Role != "" && d.Role != filter.Role {
			continue
		}
		if filter.Namespace != "" {
			ns, ok := world.TryGet[world.Namespace](a.world, id)
			if !ok || ns.Name != filter.Namespace {
				continue
			}
		}
		if filter.Alive != nil {
			isAlive := d.Alive == world.AliveRunning
			if *filter.Alive != isAlive {
				continue
			}
		}
		if filter.Ready != nil && *filter.Ready != d.Ready {
			continue
		}

		details = append(details, d)
		if filter.Limit > 0 && len(details) >= filter.Limit {
			break
		}
	}

	return details
}

func (a *DefaultAdmin) Inspect(_ context.Context, id world.EntityID) (troupe.AgentDetail, error) {
	if !a.world.Alive(id) {
		return troupe.AgentDetail{}, fmt.Errorf("admin inspect: %w: entity %d", troupe.ErrNotFound, id)
	}
	return a.buildDetail(id), nil
}

func (a *DefaultAdmin) Tree(_ context.Context) []troupe.TreeNode {
	ids := a.warden.Active()
	roots := make(map[world.EntityID]bool)
	for _, id := range ids {
		parent := a.warden.ParentOf(id)
		if parent == 0 || parent == id {
			roots[id] = true
		}
	}

	nodes := make([]troupe.TreeNode, 0, len(roots))
	for id := range roots {
		wt := a.warden.Tree(id)
		if wt != nil {
			nodes = append(nodes, convertTree(wt, a.world))
		}
	}
	return nodes
}

func (a *DefaultAdmin) Kill(ctx context.Context, id world.EntityID, reason string) error {
	a.emitAudit(ctx, "kill", id, reason)
	return a.warden.Kill(ctx, id)
}

func (a *DefaultAdmin) Drain(_ context.Context, id world.EntityID) error {
	world.TryAttach(a.world, id, world.Ready{
		Ready:    false,
		LastSeen: time.Now(),
		Reason:   world.ReasonDrained,
	})
	return nil
}

func (a *DefaultAdmin) Undrain(_ context.Context, id world.EntityID) error {
	world.TryAttach(a.world, id, world.Ready{
		Ready:    true,
		LastSeen: time.Now(),
		Reason:   world.ReasonIdle,
	})
	return nil
}

func (a *DefaultAdmin) SetBudget(_ context.Context, id world.EntityID, ceiling float64) error {
	budget, ok := world.TryGet[world.Budget](a.world, id)
	if !ok {
		budget = world.Budget{}
	}
	budget.Ceiling = ceiling
	world.TryAttach(a.world, id, budget)
	return nil
}

func (a *DefaultAdmin) SetQuota(_ context.Context, limit int) error {
	a.warden.SetMaxAgents(limit)
	return nil
}

func (a *DefaultAdmin) Cordon(ctx context.Context, reason string) error {
	a.mu.Lock()
	a.cordoned = true
	a.reason = reason
	a.mu.Unlock()
	a.emitAudit(ctx, "cordon", 0, reason)
	return nil
}

func (a *DefaultAdmin) Uncordon(ctx context.Context) error {
	a.mu.Lock()
	a.cordoned = false
	a.reason = ""
	a.mu.Unlock()
	a.emitAudit(ctx, "uncordon", 0, "")
	return nil
}

// IsCordoned reports whether the World is cordoned.
func (a *DefaultAdmin) IsCordoned() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cordoned
}

// CordonGate returns a Gate that rejects when cordoned.
func (a *DefaultAdmin) CordonGate() troupe.Gate {
	return func(_ context.Context, _ any) (bool, string, error) {
		if a.IsCordoned() {
			a.mu.RLock()
			reason := a.reason
			a.mu.RUnlock()
			return false, "cordoned: " + reason, nil
		}
		return true, "", nil
	}
}

func (a *DefaultAdmin) Annotate(_ context.Context, id world.EntityID, key, value string) error {
	ann, ok := world.TryGet[world.Annotation](a.world, id)
	if !ok {
		ann = world.Annotation{Data: make(map[string]string)}
	}
	if ann.Data == nil {
		ann.Data = make(map[string]string)
	}
	ann.Data[key] = value
	world.TryAttach(a.world, id, ann)
	return nil
}

func (a *DefaultAdmin) Annotations(_ context.Context, id world.EntityID) map[string]string {
	ann, ok := world.TryGet[world.Annotation](a.world, id)
	if !ok || ann.Data == nil {
		return nil
	}
	out := make(map[string]string, len(ann.Data))
	for k, v := range ann.Data {
		out[k] = v
	}
	return out
}

func (a *DefaultAdmin) Watch(ctx context.Context) <-chan troupe.AgentEvent {
	ch := make(chan troupe.AgentEvent, 64)
	if a.control != nil {
		a.control.OnEmit(func(e signal.Event) {
			select {
			case <-ctx.Done():
				return
			case ch <- troupe.AgentEvent{
				Kind:   e.Kind,
				Source: e.Source,
			}:
			default:
			}
		})
	}
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

func (a *DefaultAdmin) KillAll(ctx context.Context, reason string) error {
	a.emitAudit(ctx, "kill_all", 0, reason)
	a.warden.KillAll(ctx)
	return nil
}

func (a *DefaultAdmin) buildDetail(id world.EntityID) troupe.AgentDetail {
	d := troupe.AgentDetail{ID: id}

	if a.lobby != nil {
		a.lobby.mu.RLock()
		if entry, ok := a.lobby.entries[id]; ok {
			d.Role = entry.config.Role
		}
		a.lobby.mu.RUnlock()
	}

	if alive, ok := world.TryGet[world.Alive](a.world, id); ok {
		d.Alive = alive.State
		d.Since = alive.Since
	}
	if ready, ok := world.TryGet[world.Ready](a.world, id); ok {
		d.Ready = ready.Ready
		d.Reason = string(ready.Reason)
	}
	if budget, ok := world.TryGet[world.Budget](a.world, id); ok {
		d.Budget = troupe.BudgetView{
			TokensUsed: budget.TokensUsed,
			Cost:       budget.Cost,
			Ceiling:    budget.Ceiling,
		}
	}

	d.Children = a.warden.Children(id)
	parent := a.warden.ParentOf(id)
	if parent != 0 && parent != id {
		d.Parent = &parent
	}

	return d
}

func (a *DefaultAdmin) emitAudit(_ context.Context, action string, id world.EntityID, reason string) {
	if a.control == nil {
		return
	}
	a.control.Emit(signal.Event{
		Source: "admin",
		Kind:   "admin_" + action,
		Data: signal.Signal{
			Agent: "admin",
			Event: action,
			Meta: map[string]string{
				"entity_id":  fmt.Sprintf("%d", id),
				logKeyReason: reason,
			},
		},
	})
}

func convertTree(wt *warden.TreeNode, w *world.World) troupe.TreeNode {
	node := troupe.TreeNode{
		ID:   wt.ID,
		Role: wt.Role,
	}
	if alive, ok := world.TryGet[world.Alive](w, wt.ID); ok {
		node.Alive = alive.State
	}
	for _, child := range wt.Children {
		node.Children = append(node.Children, convertTree(child, w))
	}
	return node
}
