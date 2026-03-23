package bugle

import "time"

// IdentityStrategy resolves agent roles into fully-formed entities
// with identity components. Consumers implement this to map their
// domain concepts (elements, ranks) onto Bugle's color system.
type IdentityStrategy interface {
	Resolve(role, collective string) (EntityID, error)
}

// DefaultStrategy assigns random colors from the palette.
// Used when no consumer strategy is provided.
type DefaultStrategy struct {
	world    *World
	registry *Registry
}

// NewDefaultStrategy creates a strategy that assigns random color identities.
func NewDefaultStrategy(w *World, r *Registry) *DefaultStrategy {
	return &DefaultStrategy{world: w, registry: r}
}

// Resolve creates a new entity with ColorIdentity + Health(Active).
func (s *DefaultStrategy) Resolve(role, collective string) (EntityID, error) {
	color, err := s.registry.Assign(role, collective)
	if err != nil {
		return 0, err
	}

	id := s.world.Spawn()
	Attach(s.world, id, color)
	Attach(s.world, id, Health{State: Active, LastSeen: time.Now()})
	return id, nil
}
