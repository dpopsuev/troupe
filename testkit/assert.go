package testkit

import (
	"testing"

	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
)

// AssertTaskState verifies a task reached the expected state.
func AssertTaskState(t *testing.T, task *transport.Task, expected transport.TaskState) {
	t.Helper()
	if task.State != expected {
		t.Errorf("task %s state = %s, want %s", task.ID, task.State, expected)
	}
}

// AssertSignalCount verifies the bus has exactly n signals with the given event.
func AssertSignalCount(t *testing.T, bus signal.Bus, event string, expected int) {
	t.Helper()
	all := bus.Since(0)
	count := 0
	for _, s := range all {
		if s.Event == event {
			count++
		}
	}
	if count != expected {
		t.Errorf("signal count for event %q = %d, want %d", event, count, expected)
	}
}

// AssertEntityHas verifies an entity has a component of the given type.
func AssertEntityHas[T world.Component](t *testing.T, w *world.World, id world.EntityID) {
	t.Helper()
	if _, ok := world.TryGet[T](w, id); !ok {
		var zero T
		t.Errorf("entity %d missing component %T", id, zero)
	}
}
