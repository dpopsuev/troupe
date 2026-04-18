package troupe

import (
	"context"

	"github.com/dpopsuev/troupe/world"
)

// Admission is the single entry point for all agents into the World.
// Both internal spawns (Broker.Spawn) and external registrations
// (A2A, Lobby) go through this interface. This is the trust boundary —
// Gates enforce policy on every Admit call.
type Admission interface {
	// Admit registers an agent into the World. Runs admission gates,
	// creates an ECS entity, attaches components, registers in Transport,
	// and emits to ControlLog. Returns the entity ID.
	Admit(ctx context.Context, config ActorConfig) (world.EntityID, error)

	// Dismiss removes an agent from the World. Unregisters from Transport,
	// marks entity terminated, emits to ControlLog.
	Dismiss(ctx context.Context, id world.EntityID) error
}
