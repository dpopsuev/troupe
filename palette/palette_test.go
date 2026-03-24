package palette

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

func TestColorIdentity_ComponentType(t *testing.T) {
	c := ColorIdentity{}
	if got := c.ComponentType(); got != ColorIdentityType {
		t.Errorf("ComponentType() = %q, want %q", got, ColorIdentityType)
	}
}

func TestLookupShade_Found(t *testing.T) {
	s := LookupShade("Azure")
	if s == nil {
		t.Fatal("LookupShade(Azure) returned nil")
	}
	if s.Name != "Azure" {
		t.Errorf("Name = %q, want Azure", s.Name)
	}
}

func TestLookupShade_NotFound(t *testing.T) {
	s := LookupShade("Nonexistent")
	if s != nil {
		t.Errorf("LookupShade(Nonexistent) should return nil, got %v", s)
	}
}

func TestLookupColour_Found(t *testing.T) {
	c, shade, ok := LookupColour("Cerulean")
	if !ok {
		t.Fatal("LookupColour(Cerulean) not found")
	}
	if shade != "Azure" {
		t.Errorf("shade = %q, want Azure", shade)
	}
	if c.Hex != "#007BA7" {
		t.Errorf("Hex = %q, want #007BA7", c.Hex)
	}
}

func TestLookupColour_NotFound(t *testing.T) {
	_, _, ok := LookupColour("Nonexistent")
	if ok {
		t.Error("LookupColour(Nonexistent) should return false")
	}
}

func TestRegistry_Assign(t *testing.T) {
	reg := NewRegistry()
	id, err := reg.Assign("Coder", "Team")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if id.Role != "Coder" {
		t.Errorf("Role = %q, want Coder", id.Role)
	}
	if reg.Active() != 1 {
		t.Errorf("Active = %d, want 1", reg.Active())
	}
}

func TestRegistry_AssignInGroup(t *testing.T) {
	reg := NewRegistry()
	id, err := reg.AssignInGroup("Azure", "Coder", "Team")
	if err != nil {
		t.Fatalf("AssignInGroup: %v", err)
	}
	if id.Shade != "Azure" {
		t.Errorf("Shade = %q, want Azure", id.Shade)
	}
}

func TestRegistry_Set(t *testing.T) {
	reg := NewRegistry()
	id, err := reg.Set("Azure", "Cerulean", "Coder", "Team")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if id.Colour != "Cerulean" {
		t.Errorf("Colour = %q, want Cerulean", id.Colour)
	}
}

func TestRegistry_Release(t *testing.T) {
	reg := NewRegistry()
	id, _ := reg.Set("Azure", "Cerulean", "Coder", "Team")
	reg.Release(id)
	if reg.Active() != 0 {
		t.Errorf("Active = %d, want 0 after Release", reg.Active())
	}
}
