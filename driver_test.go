package jericho_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/jericho"
	"github.com/dpopsuev/jericho/world"
)

// RED: Driver must be a proper public interface, not a type alias.

type testDriver struct {
	started map[world.EntityID]bool
	stopped map[world.EntityID]bool
}

func newTestDriver() *testDriver {
	return &testDriver{
		started: make(map[world.EntityID]bool),
		stopped: make(map[world.EntityID]bool),
	}
}

func (d *testDriver) Start(_ context.Context, id world.EntityID, _ jericho.ActorConfig) error {
	d.started[id] = true
	return nil
}

func (d *testDriver) Stop(_ context.Context, id world.EntityID) error {
	d.stopped[id] = true
	return nil
}

func TestDriver_Interface(t *testing.T) {
	// A custom Driver can be passed to NewBroker via WithDriver
	driver := newTestDriver()
	broker := jericho.NewBroker("", jericho.WithDriver(driver))

	actor, err := broker.Spawn(context.Background(), jericho.ActorConfig{Role: "test"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Driver.Start should have been called
	if len(driver.started) == 0 {
		t.Error("Driver.Start was not called")
	}

	// Actor should be usable
	if !actor.Ready() {
		t.Error("actor not ready after spawn")
	}
}

func TestDriver_PublicType(t *testing.T) {
	// Driver must be a public interface, not a type alias to internal
	// This test proves consumers can implement Driver without importing internal/
	var _ jericho.Driver = newTestDriver()
}
