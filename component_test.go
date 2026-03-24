package bugle

import (
	"testing"

	"github.com/dpopsuev/bugle/world"
)

func TestComponent_InterfaceCompliance(t *testing.T) {
	// Verify all core components implement world.Component.
	var _ world.Component = Health{}
	var _ world.Component = Hierarchy{}
	var _ world.Component = Budget{}
	var _ world.Component = Progress{}
}

func TestComponent_UniqueTypes(t *testing.T) {
	types := map[world.ComponentType]string{
		Health{}.ComponentType():    "Health",
		Hierarchy{}.ComponentType(): "Hierarchy",
		Budget{}.ComponentType():    "Budget",
		Progress{}.ComponentType():  "Progress",
	}
	if len(types) != 4 {
		t.Errorf("expected 4 unique component types, got %d (collision)", len(types))
	}
}
