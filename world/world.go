// Package world provides the ECS registry — entities, components, and systems.
// Thread-safe via RWMutex (read-heavy workload).
package world

import (
	"fmt"
	"sync"
)

// EntityID is a unique agent identifier within a World.
type EntityID uint64

// ComponentType identifies a component kind for storage and queries.
type ComponentType string

// Component is the marker interface for ECS data bags.
// Every component must return a unique ComponentType.
type Component interface {
	ComponentType() ComponentType
}

// DiffKind describes a component change type.
type DiffKind string

const (
	DiffAttached DiffKind = "attached"
	DiffDetached DiffKind = "detached"
	DiffUpdated  DiffKind = "updated"
)

// DiffHook is called when a component is attached, detached, or updated on an entity.
type DiffHook func(id EntityID, ct ComponentType, kind DiffKind, old, new Component)

// World is the ECS registry — entities, components, and systems.
// Thread-safe via RWMutex (read-heavy workload).
type World struct {
	mu         sync.RWMutex
	nextID     EntityID
	components map[EntityID]map[ComponentType]Component
	alive      map[EntityID]bool
	diffHooks  []DiffHook
}

// NewWorld creates an empty ECS world.
func NewWorld() *World {
	return &World{
		components: make(map[EntityID]map[ComponentType]Component),
		alive:      make(map[EntityID]bool),
	}
}

// OnDiff registers a hook that is called when a component is attached,
// detached, or updated on any entity. Hooks are called outside the write lock.
func (w *World) OnDiff(hook DiffHook) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.diffHooks = append(w.diffHooks, hook)
}

// Spawn creates a new entity (just an ID, no data).
func (w *World) Spawn() EntityID {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextID++
	id := w.nextID
	w.alive[id] = true
	w.components[id] = make(map[ComponentType]Component)
	return id
}

// Despawn removes an entity and all its components.
func (w *World) Despawn(id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.components, id)
	delete(w.alive, id)
}

// Alive returns true if the entity exists.
func (w *World) Alive(id EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.alive[id]
}

// Count returns the number of living entities.
func (w *World) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.alive)
}

// All returns all living entity IDs.
func (w *World) All() []EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ids := make([]EntityID, 0, len(w.alive))
	for id := range w.alive {
		ids = append(ids, id)
	}
	return ids
}

// Attach adds a component to an entity. Replaces if already present.
// Panics if the entity does not exist.
// Diff hooks are called outside the write lock.
func Attach[T Component](w *World, id EntityID, c T) {
	ct := c.ComponentType()

	var (
		old   Component
		kind  DiffKind
		hooks []DiffHook
	)

	w.mu.Lock()
	bag, ok := w.components[id]
	if !ok {
		w.mu.Unlock()
		panic(fmt.Sprintf("world: Attach on dead entity %d", id))
	}
	prev, existed := bag[ct]
	bag[ct] = c
	if len(w.diffHooks) > 0 {
		hooks = make([]DiffHook, len(w.diffHooks))
		copy(hooks, w.diffHooks)
		if existed {
			kind = DiffUpdated
			old = prev
		} else {
			kind = DiffAttached
		}
	}
	w.mu.Unlock()

	for _, h := range hooks {
		h(id, ct, kind, old, c)
	}
}

// Get retrieves a component. Panics if the entity or component is not present.
func Get[T Component](w *World, id EntityID) T {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var zero T
	bag, ok := w.components[id]
	if !ok {
		panic(fmt.Sprintf("world: Get on dead entity %d", id))
	}
	raw, ok := bag[zero.ComponentType()]
	if !ok {
		panic(fmt.Sprintf("world: Get %T on entity %d: not attached", zero, id))
	}
	return raw.(T)
}

// TryGet retrieves a component, returns (zero, false) if absent.
func TryGet[T Component](w *World, id EntityID) (T, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var zero T
	bag, ok := w.components[id]
	if !ok {
		return zero, false
	}
	raw, ok := bag[zero.ComponentType()]
	if !ok {
		return zero, false
	}
	return raw.(T), true
}

// Detach removes a component from an entity. No-op if not present.
// Diff hooks are called outside the write lock.
func Detach[T Component](w *World, id EntityID) {
	var zero T
	ct := zero.ComponentType()

	var (
		old   Component
		hooks []DiffHook
	)

	w.mu.Lock()
	if bag, ok := w.components[id]; ok {
		if prev, existed := bag[ct]; existed {
			delete(bag, ct)
			if len(w.diffHooks) > 0 {
				hooks = make([]DiffHook, len(w.diffHooks))
				copy(hooks, w.diffHooks)
				old = prev
			}
		}
	}
	w.mu.Unlock()

	for _, h := range hooks {
		h(id, ct, DiffDetached, old, nil)
	}
}

// QueryType returns all entity IDs that have the specified component type.
// Unlike Query[T], this takes a ComponentType value for dynamic dispatch.
func (w *World) QueryType(ct ComponentType) []EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ids := make([]EntityID, 0)
	for id, bag := range w.components {
		if _, ok := bag[ct]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetType retrieves a component by its ComponentType. Returns (nil, false) if absent.
func (w *World) GetType(id EntityID, ct ComponentType) (Component, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bag, ok := w.components[id]
	if !ok {
		return nil, false
	}
	c, ok := bag[ct]
	return c, ok
}

// Query returns all entity IDs that have the specified component type.
func Query[T Component](w *World) []EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var zero T
	ct := zero.ComponentType()
	ids := make([]EntityID, 0)
	for id, bag := range w.components {
		if _, ok := bag[ct]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
