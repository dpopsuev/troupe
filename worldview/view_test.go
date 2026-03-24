package worldview_test

import (
	"testing"
	"time"

	"github.com/dpopsuev/bugle"
	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/world"
	"github.com/dpopsuev/bugle/worldview"
)

// ---------------------------------------------------------------------------
// Snapshot tests
// ---------------------------------------------------------------------------

func TestSnapshot_MatchesComponentTypes(t *testing.T) {
	w := world.NewWorld()
	a := w.Spawn()
	b := w.Spawn()
	c := w.Spawn()

	world.Attach(w, a, bugle.Health{State: bugle.Active})
	world.Attach(w, a, palette.ColorIdentity{Colour: "Denim", Collective: "Refactor"})

	world.Attach(w, b, bugle.Health{State: bugle.Idle})
	world.Attach(w, b, palette.ColorIdentity{Colour: "Scarlet", Collective: "Triage"})

	// c has only Health — should NOT match a 2-type query.
	world.Attach(w, c, bugle.Health{State: bugle.Done})

	v := worldview.NewView(w)
	snaps := v.Snapshot(bugle.HealthType, palette.ColorIdentityType)

	if len(snaps) != 2 {
		t.Fatalf("Snapshot returned %d entities, want 2", len(snaps))
	}

	ids := make(map[world.EntityID]bool)
	for _, s := range snaps {
		ids[s.ID] = true
		if len(s.Components) != 2 {
			t.Errorf("entity %d has %d components, want 2", s.ID, len(s.Components))
		}
	}
	if !ids[a] || !ids[b] {
		t.Errorf("expected entities %d and %d, got %v", a, b, ids)
	}
	if ids[c] {
		t.Error("entity c should not be in the snapshot")
	}
}

func TestSnapshot_NoMatches(t *testing.T) {
	w := world.NewWorld()
	w.Spawn() // entity with no components

	v := worldview.NewView(w)
	snaps := v.Snapshot(bugle.BudgetType)

	if len(snaps) != 0 {
		t.Errorf("Snapshot returned %d entities, want 0", len(snaps))
	}
}

func TestSnapshot_ReflectsLatestState(t *testing.T) {
	w := world.NewWorld()
	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	v := worldview.NewView(w)

	// First snapshot.
	snaps := v.Snapshot(bugle.HealthType)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	h := snaps[0].Components[bugle.HealthType].(bugle.Health)
	if h.State != bugle.Active {
		t.Errorf("state = %s, want active", h.State)
	}

	// Update and re-snapshot.
	world.Attach(w, id, bugle.Health{State: bugle.Errored, Error: "timeout"})

	snaps = v.Snapshot(bugle.HealthType)
	h = snaps[0].Components[bugle.HealthType].(bugle.Health)
	if h.State != bugle.Errored {
		t.Errorf("state = %s, want errored", h.State)
	}
}

// ---------------------------------------------------------------------------
// Subscribe tests
// ---------------------------------------------------------------------------

func TestSubscribe_AttachEmitsDiff(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)
	ch := v.Subscribe()

	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	select {
	case d := <-ch:
		if d.Kind != world.DiffAttached {
			t.Errorf("kind = %s, want attached", d.Kind)
		}
		if d.Entity != id {
			t.Errorf("entity = %d, want %d", d.Entity, id)
		}
		if d.Component != bugle.HealthType {
			t.Errorf("component = %s, want %s", d.Component, bugle.HealthType)
		}
		if d.Old != nil {
			t.Error("Old should be nil for attached")
		}
		if d.New == nil {
			t.Error("New should not be nil for attached")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diff")
	}
}

func TestSubscribe_UpdateEmitsDiff(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)

	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	ch := v.Subscribe()

	// Second attach triggers DiffUpdated.
	world.Attach(w, id, bugle.Health{State: bugle.Errored, Error: "timeout"})

	select {
	case d := <-ch:
		if d.Kind != world.DiffUpdated {
			t.Errorf("kind = %s, want updated", d.Kind)
		}
		if d.Old == nil {
			t.Fatal("Old should not be nil for updated")
		}
		oldH := d.Old.(bugle.Health)
		if oldH.State != bugle.Active {
			t.Errorf("old state = %s, want active", oldH.State)
		}
		newH := d.New.(bugle.Health)
		if newH.State != bugle.Errored {
			t.Errorf("new state = %s, want errored", newH.State)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diff")
	}
}

func TestSubscribe_DetachEmitsDiff(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)

	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	ch := v.Subscribe()
	world.Detach[bugle.Health](w, id)

	select {
	case d := <-ch:
		if d.Kind != world.DiffDetached {
			t.Errorf("kind = %s, want detached", d.Kind)
		}
		if d.Old == nil {
			t.Fatal("Old should not be nil for detached")
		}
		if d.New != nil {
			t.Error("New should be nil for detached")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diff")
	}
}

func TestSubscribe_FiltersByType(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)

	// Subscribe to Health only.
	ch := v.Subscribe(bugle.HealthType)

	id := w.Spawn()
	// Attach a ColorIdentity (should NOT trigger diff on this channel).
	world.Attach(w, id, palette.ColorIdentity{Colour: "Denim"})

	select {
	case d := <-ch:
		t.Errorf("should not have received diff, got %+v", d)
	case <-time.After(50 * time.Millisecond):
		// Expected: no diff received.
	}

	// Now attach Health — should trigger.
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	select {
	case d := <-ch:
		if d.Component != bugle.HealthType {
			t.Errorf("component = %s, want %s", d.Component, bugle.HealthType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Health diff")
	}
}

func TestSubscribe_Unsubscribe(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)

	ch := v.Subscribe()
	v.Unsubscribe(ch)

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after Unsubscribe")
	}

	// Attach should not panic (no subscriber).
	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})
}

func TestSubscribe_MultipleSubs(t *testing.T) {
	w := world.NewWorld()
	v := worldview.NewView(w)

	ch1 := v.Subscribe()
	ch2 := v.Subscribe()

	id := w.Spawn()
	world.Attach(w, id, bugle.Health{State: bugle.Active})

	for i, ch := range []<-chan worldview.Diff{ch1, ch2} {
		select {
		case d := <-ch:
			if d.Kind != world.DiffAttached {
				t.Errorf("sub %d: kind = %s, want attached", i, d.Kind)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timed out waiting for diff", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Hierarchy tests
// ---------------------------------------------------------------------------

func TestHierarchy_BuildsTree(t *testing.T) {
	w := world.NewWorld()
	parent := w.Spawn()
	child := w.Spawn()
	grandchild := w.Spawn()

	world.Attach(w, parent, bugle.Hierarchy{Parent: 0})
	world.Attach(w, child, bugle.Hierarchy{Parent: parent})
	world.Attach(w, grandchild, bugle.Hierarchy{Parent: child})

	v := worldview.NewView(w)
	tree := v.Hierarchy()

	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	root := tree[0]
	if root.ID != parent {
		t.Errorf("root ID = %d, want %d", root.ID, parent)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(root.Children))
	}
	childNode := root.Children[0]
	if childNode.ID != child {
		t.Errorf("child ID = %d, want %d", childNode.ID, child)
	}
	if len(childNode.Children) != 1 {
		t.Fatalf("child children = %d, want 1", len(childNode.Children))
	}
	if childNode.Children[0].ID != grandchild {
		t.Errorf("grandchild ID = %d, want %d", childNode.Children[0].ID, grandchild)
	}
}

func TestHierarchy_RootsHaveNoParent(t *testing.T) {
	w := world.NewWorld()
	a := w.Spawn()
	b := w.Spawn()
	c := w.Spawn()

	// a and b are roots (Parent=0), c is child of a.
	world.Attach(w, a, bugle.Hierarchy{Parent: 0})
	world.Attach(w, b, bugle.Hierarchy{Parent: 0})
	world.Attach(w, c, bugle.Hierarchy{Parent: a})

	v := worldview.NewView(w)
	tree := v.Hierarchy()

	if len(tree) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree))
	}

	rootIDs := make(map[world.EntityID]bool)
	for _, n := range tree {
		rootIDs[n.ID] = true
	}
	if !rootIDs[a] || !rootIDs[b] {
		t.Errorf("expected roots %d and %d, got %v", a, b, rootIDs)
	}
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestStats_CountsByState(t *testing.T) {
	w := world.NewWorld()

	// 3 active, 2 idle, 1 errored.
	for range 3 {
		id := w.Spawn()
		world.Attach(w, id, bugle.Health{State: bugle.Active})
	}
	for range 2 {
		id := w.Spawn()
		world.Attach(w, id, bugle.Health{State: bugle.Idle})
	}
	{
		id := w.Spawn()
		world.Attach(w, id, bugle.Health{State: bugle.Errored})
	}

	v := worldview.NewView(w)
	s := v.Stats()

	if s.TotalEntities != 6 {
		t.Errorf("TotalEntities = %d, want 6", s.TotalEntities)
	}
	if s.ByState[bugle.Active] != 3 {
		t.Errorf("Active = %d, want 3", s.ByState[bugle.Active])
	}
	if s.ByState[bugle.Idle] != 2 {
		t.Errorf("Idle = %d, want 2", s.ByState[bugle.Idle])
	}
	if s.ByState[bugle.Errored] != 1 {
		t.Errorf("Errored = %d, want 1", s.ByState[bugle.Errored])
	}
}

func TestStats_CountsCollectives(t *testing.T) {
	w := world.NewWorld()

	// 3 in "Refactor", 2 in "Triage".
	for range 3 {
		id := w.Spawn()
		world.Attach(w, id, palette.ColorIdentity{Colour: "A", Collective: "Refactor"})
	}
	for range 2 {
		id := w.Spawn()
		world.Attach(w, id, palette.ColorIdentity{Colour: "B", Collective: "Triage"})
	}

	v := worldview.NewView(w)
	s := v.Stats()

	if s.Collectives != 2 {
		t.Errorf("Collectives = %d, want 2", s.Collectives)
	}
}

// ---------------------------------------------------------------------------
// Acceptance test
// ---------------------------------------------------------------------------

func TestAcceptance_MinimapPattern(t *testing.T) {
	// Full pattern: spawn agents with identity+health, create View,
	// snapshot, verify readable output.
	w := world.NewWorld()

	agents := []struct {
		colour     string
		shade      string
		role       string
		collective string
		state      bugle.AgentState
	}{
		{"Denim", "Indigo", "Writer", "Refactor", bugle.Active},
		{"Scarlet", "Crimson", "Reviewer", "Refactor", bugle.Active},
		{"Cerulean", "Azure", "Coder", "Triage", bugle.Idle},
	}

	for _, a := range agents {
		id := w.Spawn()
		world.Attach(w, id, palette.ColorIdentity{
			Shade:      a.shade,
			Colour:     a.colour,
			Role:       a.role,
			Collective: a.collective,
		})
		world.Attach(w, id, bugle.Health{State: a.state, LastSeen: time.Now()})
	}

	v := worldview.NewView(w)

	// Snapshot all agents with both components.
	snaps := v.Snapshot(palette.ColorIdentityType, bugle.HealthType)
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}

	// Verify each snapshot has readable data.
	for _, s := range snaps {
		ci := s.Components[palette.ColorIdentityType].(palette.ColorIdentity)
		h := s.Components[bugle.HealthType].(bugle.Health)
		if ci.Colour == "" {
			t.Errorf("entity %d: empty Colour", s.ID)
		}
		if h.State == "" {
			t.Errorf("entity %d: empty State", s.ID)
		}
		t.Logf("entity %d: %s — %s", s.ID, ci.Title(), h.State)
	}

	// Stats should reflect the world.
	stats := v.Stats()
	if stats.TotalEntities != 3 {
		t.Errorf("TotalEntities = %d, want 3", stats.TotalEntities)
	}
	if stats.ByState[bugle.Active] != 2 {
		t.Errorf("Active = %d, want 2", stats.ByState[bugle.Active])
	}
	if stats.Collectives != 2 {
		t.Errorf("Collectives = %d, want 2", stats.Collectives)
	}
}
