package world_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dpopsuev/battery/event"
	"github.com/dpopsuev/battery/testkit"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

func TestEmitDiffsTo_Attach(t *testing.T) {
	w := world.NewWorld()
	log := testkit.NewStubEventLog()
	w.EmitDiffsTo(log)

	id := w.Spawn()
	world.Attach(w, id, world.Alive{State: world.AliveRunning})

	events := log.Since(0)
	if len(events) == 0 {
		t.Fatal("no events after Attach")
	}
	if events[0].Kind != "component.attached" {
		t.Fatalf("Kind = %q, want component.attached", events[0].Kind)
	}
}

func TestEmitDiffsTo_Detach(t *testing.T) {
	w := world.NewWorld()
	log := testkit.NewStubEventLog()
	w.EmitDiffsTo(log)

	id := w.Spawn()
	world.Attach(w, id, world.Alive{State: world.AliveRunning})
	world.Detach[world.Alive](w, id)

	var found bool
	for _, e := range log.Since(0) {
		if e.Kind == "component.detached" {
			found = true
		}
	}
	if !found {
		t.Fatal("no component.detached event after Detach")
	}
}

func TestEmitDiffsTo_Update(t *testing.T) {
	w := world.NewWorld()
	log := testkit.NewStubEventLog()
	w.EmitDiffsTo(log)

	id := w.Spawn()
	world.Attach(w, id, world.Alive{State: world.AliveRunning})
	world.Attach(w, id, world.Alive{State: world.AliveTerminated}) // update

	var found bool
	for _, e := range log.Since(0) {
		if e.Kind == "component.updated" {
			found = true
		}
	}
	if !found {
		t.Fatal("no component.updated event after re-Attach")
	}
}

func TestEmitDiffsTo_MetaFields(t *testing.T) {
	w := world.NewWorld()
	log := testkit.NewStubEventLog()
	w.EmitDiffsTo(log)

	id := w.Spawn()
	world.Attach(w, id, world.Alive{State: world.AliveRunning})

	events := log.Since(0)
	if len(events) == 0 {
		t.Fatal("no events")
	}

	e := events[0]
	if e.Source != "world" {
		t.Fatalf("Source = %q, want world", e.Source)
	}
	if e.Meta["entity_id"] == "" {
		t.Fatal("entity_id missing from Meta")
	}
	if e.Meta["component_type"] != "alive" {
		t.Fatalf("component_type = %q, want alive", e.Meta["component_type"])
	}
}

func TestEmitDiffsTo_DurableBus(t *testing.T) {
	w := world.NewWorld()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	log := signal.NewBusEventLog(bus)
	w.EmitDiffsTo(log)

	id := w.Spawn()
	world.Attach(w, id, world.Alive{State: world.AliveRunning})

	events := log.Since(0)
	if len(events) == 0 {
		t.Fatal("no events via DurableBus")
	}

	info, _ := os.Stat(path)
	if info.Size() == 0 {
		t.Fatal("durable log file empty")
	}
}

func TestEmitDiffsTo_ConcurrentSafe(t *testing.T) {
	w := world.NewWorld()
	log := testkit.NewStubEventLog()
	w.EmitDiffsTo(log)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			id := w.Spawn()
			world.Attach(w, id, world.Alive{State: world.AliveRunning})
			world.Attach(w, id, world.Ready{Ready: true})
			world.Detach[world.Ready](w, id)
		})
	}
	wg.Wait()

	// 20 goroutines × 3 mutations each = 60 events
	if log.Len() < 60 {
		t.Fatalf("events = %d, want >= 60", log.Len())
	}
}

// ensure event import is used
var _ = event.Event{}
