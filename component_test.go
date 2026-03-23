package bugle

import "testing"

func TestColorIdentity_Title(t *testing.T) {
	c := ColorIdentity{Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor"}
	want := "Denim Writer of Indigo Refactor"
	if got := c.Title(); got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
}

func TestColorIdentity_Label(t *testing.T) {
	c := ColorIdentity{Shade: "Indigo", Colour: "Denim", Role: "Writer"}
	want := "[Indigo·Denim|Writer]"
	if got := c.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestColorIdentity_Short(t *testing.T) {
	c := ColorIdentity{Colour: "Denim"}
	if got := c.Short(); got != "Denim" {
		t.Errorf("Short() = %q, want Denim", got)
	}
}

func TestComponent_InterfaceCompliance(t *testing.T) {
	// Verify all core components implement Component.
	var _ Component = ColorIdentity{}
	var _ Component = Health{}
	var _ Component = Hierarchy{}
	var _ Component = Budget{}
	var _ Component = Progress{}
}

func TestComponent_UniqueTypes(t *testing.T) {
	types := map[ComponentType]string{
		ColorIdentity{}.componentType(): "ColorIdentity",
		Health{}.componentType():        "Health",
		Hierarchy{}.componentType():     "Hierarchy",
		Budget{}.componentType():        "Budget",
		Progress{}.componentType():      "Progress",
	}
	if len(types) != 5 {
		t.Errorf("expected 5 unique component types, got %d (collision)", len(types))
	}
}
