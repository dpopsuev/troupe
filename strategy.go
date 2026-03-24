package bugle

import (
	"time"

	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/world"
)

// IdentityStrategy resolves agent roles into fully-formed entities
// with identity components. Consumers implement this to map their
// domain concepts (elements, ranks) onto Bugle's color system.
type IdentityStrategy interface {
	Resolve(role, collective string) (world.EntityID, error)
}

// DefaultStrategy assigns random colors from the palette.
// Used when no consumer strategy is provided.
type DefaultStrategy struct {
	world    *world.World
	registry *palette.Registry
}

// NewDefaultStrategy creates a strategy that assigns random color identities.
func NewDefaultStrategy(w *world.World, r *palette.Registry) *DefaultStrategy {
	return &DefaultStrategy{world: w, registry: r}
}

// Resolve creates a new entity with ColorIdentity + Health(Active).
func (s *DefaultStrategy) Resolve(role, collective string) (world.EntityID, error) {
	color, err := s.registry.Assign(role, collective)
	if err != nil {
		return 0, err
	}

	id := s.world.Spawn()
	world.Attach(s.world, id, color)
	world.Attach(s.world, id, Health{State: Active, LastSeen: time.Now()})
	return id, nil
}
