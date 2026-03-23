package bugle

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
	componentType() ComponentType
}

// World is the ECS registry — entities, components, and systems.
// Thread-safe via RWMutex (read-heavy workload).
type World struct {
	mu         sync.RWMutex
	nextID     EntityID
	components map[EntityID]map[ComponentType]Component
	alive      map[EntityID]bool
}

// NewWorld creates an empty ECS world.
func NewWorld() *World {
	return &World{
		components: make(map[EntityID]map[ComponentType]Component),
		alive:      make(map[EntityID]bool),
	}
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
func Attach[T Component](w *World, id EntityID, c T) {
	w.mu.Lock()
	defer w.mu.Unlock()
	bag, ok := w.components[id]
	if !ok {
		panic(fmt.Sprintf("bugle: Attach on dead entity %d", id))
	}
	bag[c.componentType()] = c
}

// Get retrieves a component. Panics if the entity or component is not present.
func Get[T Component](w *World, id EntityID) T {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var zero T
	bag, ok := w.components[id]
	if !ok {
		panic(fmt.Sprintf("bugle: Get on dead entity %d", id))
	}
	raw, ok := bag[zero.componentType()]
	if !ok {
		panic(fmt.Sprintf("bugle: Get %T on entity %d: not attached", zero, id))
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
	raw, ok := bag[zero.componentType()]
	if !ok {
		return zero, false
	}
	return raw.(T), true
}

// Detach removes a component from an entity. No-op if not present.
func Detach[T Component](w *World, id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var zero T
	if bag, ok := w.components[id]; ok {
		delete(bag, zero.componentType())
	}
}

// Query returns all entity IDs that have the specified component type.
func Query[T Component](w *World) []EntityID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var zero T
	ct := zero.componentType()
	ids := make([]EntityID, 0)
	for id, bag := range w.components {
		if _, ok := bag[ct]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
