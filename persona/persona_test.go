package persona

import (
	"testing"

	"github.com/dpopsuev/bugle/element"
	"github.com/dpopsuev/bugle/identity"
)

func TestAll_Count(t *testing.T) {
	all := All()
	if len(all) != 8 {
		t.Errorf("len(All) = %d, want 8", len(all))
	}
}

func TestThesis_Count(t *testing.T) {
	thesis := Thesis()
	if len(thesis) != 4 {
		t.Errorf("len(Thesis) = %d, want 4", len(thesis))
	}
	for _, p := range thesis {
		if p.Identity.Alignment != identity.AlignmentThesis {
			t.Errorf("persona %q has alignment %q, want thesis", p.Identity.PersonaName, p.Identity.Alignment)
		}
	}
}

func TestAntithesis_Count(t *testing.T) {
	antithesis := Antithesis()
	if len(antithesis) != 4 {
		t.Errorf("len(Antithesis) = %d, want 4", len(antithesis))
	}
	for _, p := range antithesis {
		if p.Identity.Alignment != identity.AlignmentAntithesis {
			t.Errorf("persona %q has alignment %q, want antithesis", p.Identity.PersonaName, p.Identity.Alignment)
		}
	}
}

func TestByName_Herald(t *testing.T) {
	p, ok := ByName("Herald")
	if !ok {
		t.Fatal("ByName(Herald) not found")
	}
	if p.Identity.Color.Name != "Crimson" {
		t.Errorf("Herald color = %q, want Crimson", p.Identity.Color.Name)
	}
	if p.Identity.Element != element.ElementFire {
		t.Errorf("Herald element = %q, want fire", p.Identity.Element)
	}
	if p.Identity.Position != identity.PositionPG {
		t.Errorf("Herald position = %q, want PG", p.Identity.Position)
	}
	if p.Identity.Alignment != identity.AlignmentThesis {
		t.Errorf("Herald alignment = %q, want thesis", p.Identity.Alignment)
	}
}

func TestByName_CaseInsensitive(t *testing.T) {
	_, ok := ByName("herald")
	if !ok {
		t.Error("ByName should be case-insensitive")
	}
	_, ok = ByName("CHALLENGER")
	if !ok {
		t.Error("ByName should be case-insensitive")
	}
}

func TestByName_NotFound(t *testing.T) {
	_, ok := ByName("nonexistent")
	if ok {
		t.Error("ByName should return false for nonexistent name")
	}
}

func TestPersonas_UniqueNames(t *testing.T) {
	all := All()
	seen := make(map[string]bool, len(all))
	for _, p := range all {
		name := p.Identity.PersonaName
		if seen[name] {
			t.Errorf("duplicate persona name: %s", name)
		}
		seen[name] = true
	}
}

func TestPersonas_UniqueColors(t *testing.T) {
	all := All()
	seen := make(map[string]bool, len(all))
	for _, p := range all {
		hex := p.Identity.Color.Hex
		if seen[hex] {
			t.Errorf("duplicate color hex: %s (persona %s)", hex, p.Identity.PersonaName)
		}
		seen[hex] = true
	}
}

func TestPersonas_AllPositionsCovered(t *testing.T) {
	positions := map[identity.Position]int{identity.PositionPG: 0, identity.PositionSG: 0, identity.PositionPF: 0, identity.PositionC: 0}
	for _, p := range All() {
		positions[p.Identity.Position]++
	}
	for pos, count := range positions {
		if count != 2 {
			t.Errorf("position %s has %d personas, want 2 (1 thesis + 1 antithesis)", pos, count)
		}
	}
}

func TestPersonas_AllHaveStepAffinity(t *testing.T) {
	for _, p := range All() {
		if len(p.Identity.StepAffinity) == 0 {
			t.Errorf("persona %s has no step affinity", p.Identity.PersonaName)
		}
	}
}

func TestPersonas_AllHavePromptPreamble(t *testing.T) {
	for _, p := range All() {
		if p.Identity.PromptPreamble == "" {
			t.Errorf("persona %s has empty prompt preamble", p.Identity.PersonaName)
		}
	}
}

func TestPersonas_HomeZoneMatchesPosition(t *testing.T) {
	for _, p := range All() {
		expected := identity.HomeZoneFor(p.Identity.Position)
		if p.Identity.HomeZone != expected {
			t.Errorf("persona %s: HomeZone=%q but HomeZoneFor(%s)=%q",
				p.Identity.PersonaName, p.Identity.HomeZone, p.Identity.Position, expected)
		}
	}
}

func TestColorPalette_HexFormat(t *testing.T) {
	colors := []identity.Color{
		ColorCrimson, ColorCerulean, ColorCobalt, ColorAmber,
		ColorScarlet, ColorSapphire, ColorObsidian, ColorSteel,
	}
	for _, c := range colors {
		if len(c.Hex) != 7 || c.Hex[0] != '#' {
			t.Errorf("color %s has invalid hex: %q", c.Name, c.Hex)
		}
		if c.Family == "" {
			t.Errorf("color %s has empty family", c.Name)
		}
	}
}
