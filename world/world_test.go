package world

import (
	"sync"
	"testing"
)

// testComponent is a minimal component for testing.
type testComponent struct {
	Value string
}

func (testComponent) ComponentType() ComponentType { return "test_component" }

// testComponent2 is a second component type for testing multi-component scenarios.
type testComponent2 struct {
	State string
}

func (testComponent2) ComponentType() ComponentType { return "test_component_2" }

func TestWorld_SpawnReturnsUniqueIDs(t *testing.T) {
	w := NewWorld()
	a := w.Spawn()
	b := w.Spawn()
	c := w.Spawn()
	if a == b || b == c || a == c {
		t.Errorf("Spawn returned duplicate IDs: %d, %d, %d", a, b, c)
	}
}

func TestWorld_AttachGetRoundTrip(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, testComponent{Value: "Cerulean"})

	got := Get[testComponent](w, id)
	if got.Value != "Cerulean" {
		t.Errorf("Get Value = %q, want Cerulean", got.Value)
	}
}

func TestWorld_AttachReplaces(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, testComponent2{State: "active"})
	Attach(w, id, testComponent2{State: "errored"})

	got := Get[testComponent2](w, id)
	if got.State != "errored" {
		t.Errorf("expected replaced state errored, got %s", got.State)
	}
}

func TestWorld_TryGetMissing(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	_, ok := TryGet[testComponent](w, id)
	if ok {
		t.Error("TryGet should return false for unattached component")
	}
}

func TestWorld_TryGetDeadEntity(t *testing.T) {
	w := NewWorld()
	_, ok := TryGet[testComponent2](w, EntityID(999))
	if ok {
		t.Error("TryGet should return false for nonexistent entity")
	}
}

func TestWorld_DetachRemoves(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, testComponent2{State: "active"})
	Detach[testComponent2](w, id)

	_, ok := TryGet[testComponent2](w, id)
	if ok {
		t.Error("Detach should remove component")
	}
}

func TestWorld_DespawnRemovesAll(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, testComponent2{State: "active"})
	Attach(w, id, testComponent{Value: "Denim"})

	w.Despawn(id)
	if w.Alive(id) {
		t.Error("entity should not be alive after Despawn")
	}
	if w.Count() != 0 {
		t.Errorf("Count = %d, want 0", w.Count())
	}
}

func TestWorld_QueryMatchesComponents(t *testing.T) {
	w := NewWorld()
	a := w.Spawn()
	b := w.Spawn()
	c := w.Spawn()

	Attach(w, a, testComponent2{State: "active"})
	Attach(w, b, testComponent2{State: "idle"})
	// c has no testComponent2

	ids := Query[testComponent2](w)
	if len(ids) != 2 {
		t.Fatalf("Query[testComponent2] returned %d entities, want 2", len(ids))
	}

	found := make(map[EntityID]bool)
	for _, id := range ids {
		found[id] = true
	}
	if !found[a] || !found[b] {
		t.Errorf("Query should return entities a and b, got %v", ids)
	}
	if found[c] {
		t.Error("Query should NOT return entity c (no testComponent2)")
	}
}

func TestWorld_CountAndAll(t *testing.T) {
	w := NewWorld()
	w.Spawn()
	w.Spawn()
	w.Spawn()

	if w.Count() != 3 {
		t.Errorf("Count = %d, want 3", w.Count())
	}
	if len(w.All()) != 3 {
		t.Errorf("All len = %d, want 3", len(w.All()))
	}
}

func TestWorld_GetPanicsOnDeadEntity(t *testing.T) {
	w := NewWorld()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Get on dead entity should panic")
		}
	}()
	Get[testComponent2](w, EntityID(999))
}

func TestWorld_GetPanicsOnMissingComponent(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Get on missing component should panic")
		}
	}()
	Get[testComponent2](w, id)
}

func TestWorld_AttachPanicsOnDeadEntity(t *testing.T) {
	w := NewWorld()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Attach on dead entity should panic")
		}
	}()
	Attach(w, EntityID(999), testComponent2{State: "active"})
}

func TestWorld_ConcurrentSafety(t *testing.T) {
	w := NewWorld()
	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				id := w.Spawn()
				Attach(w, id, testComponent2{State: "active"})
				Attach(w, id, testComponent{Value: "Test"})
				_ = Get[testComponent2](w, id)
				_, _ = TryGet[testComponent](w, id)
				_ = Query[testComponent2](w)
			}
		}()
	}
	wg.Wait()

	if w.Count() != 1000 {
		t.Errorf("Count = %d, want 1000 after concurrent spawns", w.Count())
	}
}
