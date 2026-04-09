// observe.go — World emits component mutations as events.
//
// Industry pattern: Bevy world.trigger(), Flecs world.observer().event(OnSet).
// The World IS the event source. Observers register on the World.
package world

import (
	"fmt"

	"github.com/dpopsuev/battery/event"
)

// EmitDiffsTo registers a DiffHook that emits component mutations
// as events to the given EventLog. Call once at composition time.
//
// Events emitted:
//   - component.attached — a component was added to an entity
//   - component.detached — a component was removed from an entity
//   - component.updated — a component was replaced on an entity
func (w *World) EmitDiffsTo(log event.EventLog) {
	w.OnDiff(func(id EntityID, ct ComponentType, kind DiffKind, _, _ Component) {
		log.Emit(event.Event{
			Source: "world",
			Kind:   "component." + string(kind),
			Meta: map[string]string{
				"entity_id":      fmt.Sprintf("%d", id),
				"component_type": string(ct),
			},
		})
	})
}
