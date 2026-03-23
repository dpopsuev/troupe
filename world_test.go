package bugle

import (
	"sync"
	"testing"
)

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
	Attach(w, id, ColorIdentity{Shade: "Azure", Colour: "Cerulean", Role: "Coder"})

	got := Get[ColorIdentity](w, id)
	if got.Colour != "Cerulean" {
		t.Errorf("Get Colour = %q, want Cerulean", got.Colour)
	}
}

func TestWorld_AttachReplaces(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, Health{State: Active})
	Attach(w, id, Health{State: Errored, Error: "timeout"})

	got := Get[Health](w, id)
	if got.State != Errored {
		t.Errorf("expected replaced Health state Errored, got %s", got.State)
	}
}

func TestWorld_TryGetMissing(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	_, ok := TryGet[ColorIdentity](w, id)
	if ok {
		t.Error("TryGet should return false for unattached component")
	}
}

func TestWorld_TryGetDeadEntity(t *testing.T) {
	w := NewWorld()
	_, ok := TryGet[Health](w, EntityID(999))
	if ok {
		t.Error("TryGet should return false for nonexistent entity")
	}
}

func TestWorld_DetachRemoves(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, Health{State: Active})
	Detach[Health](w, id)

	_, ok := TryGet[Health](w, id)
	if ok {
		t.Error("Detach should remove component")
	}
}

func TestWorld_DespawnRemovesAll(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	Attach(w, id, Health{State: Active})
	Attach(w, id, ColorIdentity{Colour: "Denim"})

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

	Attach(w, a, Health{State: Active})
	Attach(w, b, Health{State: Idle})
	// c has no Health

	ids := Query[Health](w)
	if len(ids) != 2 {
		t.Fatalf("Query[Health] returned %d entities, want 2", len(ids))
	}

	found := make(map[EntityID]bool)
	for _, id := range ids {
		found[id] = true
	}
	if !found[a] || !found[b] {
		t.Errorf("Query should return entities a and b, got %v", ids)
	}
	if found[c] {
		t.Error("Query should NOT return entity c (no Health)")
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
	Get[Health](w, EntityID(999))
}

func TestWorld_GetPanicsOnMissingComponent(t *testing.T) {
	w := NewWorld()
	id := w.Spawn()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Get on missing component should panic")
		}
	}()
	Get[Health](w, id)
}

func TestWorld_AttachPanicsOnDeadEntity(t *testing.T) {
	w := NewWorld()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Attach on dead entity should panic")
		}
	}()
	Attach(w, EntityID(999), Health{State: Active})
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
				Attach(w, id, Health{State: Active})
				Attach(w, id, ColorIdentity{Colour: "Test"})
				_ = Get[Health](w, id)
				_, _ = TryGet[ColorIdentity](w, id)
				_ = Query[Health](w)
			}
		}()
	}
	wg.Wait()

	if w.Count() != 1000 {
		t.Errorf("Count = %d, want 1000 after concurrent spawns", w.Count())
	}
}
