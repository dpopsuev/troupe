// Package palette defines the heraldic color system — Shade families,
// Colour values, ColorIdentity component, and the collision-free Registry.
package palette

import (
	"fmt"

	"github.com/dpopsuev/bugle/world"
)

// ColorIdentityType is the ComponentType for ColorIdentity.
const ColorIdentityType world.ComponentType = "color_identity"

// Shade is a color family grouping for agent collectives.
type Shade struct {
	Name    string
	Colours []Colour
}

// Colour is a specific color within a shade family.
type Colour struct {
	Name string
	Hex  string
}

// Palette defines 7 shade families x 8 colours = 56 unique agent identities.
var Palette = []Shade{
	{Name: "Azure", Colours: []Colour{
		{"Cerulean", "#007BA7"},
		{"Cobalt", "#0047AB"},
		{"Sapphire", "#0F52BA"},
		{"Indigo", "#4B0082"},
		{"Navy", "#000080"},
		{"Periwinkle", "#CCCCFF"},
		{"Steel", "#4682B4"},
		{"Teal", "#008080"},
	}},
	{Name: "Crimson", Colours: []Colour{
		{"Scarlet", "#FF2400"},
		{"Vermillion", "#E34234"},
		{"Ruby", "#E0115F"},
		{"Garnet", "#733635"},
		{"Cardinal", "#C41E3A"},
		{"Carmine", "#960018"},
		{"Rust", "#B7410E"},
		{"Coral", "#FF7F50"},
	}},
	{Name: "Forest", Colours: []Colour{
		{"Emerald", "#50C878"},
		{"Jade", "#00A86B"},
		{"Sage", "#BCB88A"},
		{"Olive", "#808000"},
		{"Mint", "#3EB489"},
		{"Hunter", "#355E3B"},
		{"Moss", "#8A9A5B"},
		{"Viridian", "#40826D"},
	}},
	{Name: "Amber", Colours: []Colour{
		{"Saffron", "#F4C430"},
		{"Gold", "#FFD700"},
		{"Marigold", "#EAA221"},
		{"Tangerine", "#FF9966"},
		{"Apricot", "#FBCEB1"},
		{"Ochre", "#CC7722"},
		{"Bronze", "#CD7F32"},
		{"Copper", "#B87333"},
	}},
	{Name: "Violet", Colours: []Colour{
		{"Amethyst", "#9966CC"},
		{"Lavender", "#E6E6FA"},
		{"Plum", "#8E4585"},
		{"Mauve", "#E0B0FF"},
		{"Orchid", "#DA70D6"},
		{"Thistle", "#D8BFD8"},
		{"Iris", "#5A4FCF"},
		{"Heather", "#B7C3D0"},
	}},
	{Name: "Slate", Colours: []Colour{
		{"Charcoal", "#36454F"},
		{"Ash", "#B2BEB5"},
		{"Pewter", "#8BA8B7"},
		{"Silver", "#C0C0C0"},
		{"Smoke", "#738276"},
		{"Graphite", "#383838"},
		{"Iron", "#48494B"},
		{"Flint", "#6F6A63"},
	}},
	{Name: "Ivory", Colours: []Colour{
		{"Pearl", "#EAE0C8"},
		{"Cream", "#FFFDD0"},
		{"Linen", "#FAF0E6"},
		{"Snow", "#FFFAFA"},
		{"Alabaster", "#F2F0E6"},
		{"Bone", "#E3DAC9"},
		{"Shell", "#FFF5EE"},
		{"Chalk", "#FDFDFD"},
	}},
}

// LookupShade finds a shade by name. Returns nil if not found.
func LookupShade(name string) *Shade {
	for i := range Palette {
		if Palette[i].Name == name {
			return &Palette[i]
		}
	}
	return nil
}

// LookupColour finds a colour by name across all shades.
// Returns the colour and its parent shade name.
func LookupColour(name string) (Colour, string, bool) {
	for _, shade := range Palette {
		for _, c := range shade.Colours {
			if c.Name == name {
				return c, shade.Name, true
			}
		}
	}
	return Colour{}, "", false
}

// ColorIdentity is the visual identity for humans.
// Format: "Denim Writer of Indigo Refactor" (Colour Role of Shade Collective).
type ColorIdentity struct {
	Shade      string `json:"shade"`      // group family: "Indigo", "Crimson"
	Colour     string `json:"colour"`     // individual: "Denim", "Scarlet"
	Role       string `json:"role"`       // function: "Writer", "Reviewer"
	Collective string `json:"collective"` // formation: "Refactor", "Triage"
	Hex        string `json:"hex"`        // CSS hex: "#6F8FAF"
}

// ComponentType implements world.Component.
func (ColorIdentity) ComponentType() world.ComponentType { return ColorIdentityType }

// Title returns the heraldic name: "Denim Writer of Indigo Refactor".
func (c ColorIdentity) Title() string { //nolint:gocritic // value receiver needed for ECS Get[T]
	return fmt.Sprintf("%s %s of %s %s", c.Colour, c.Role, c.Shade, c.Collective)
}

// Label returns the compact log format: "[Indigo·Denim|Writer]".
func (c ColorIdentity) Label() string { //nolint:gocritic // value receiver needed for ECS Get[T]
	return fmt.Sprintf("[%s·%s|%s]", c.Shade, c.Colour, c.Role)
}

// Short returns just the colour name: "Denim".
func (c ColorIdentity) Short() string { return c.Colour } //nolint:gocritic // value receiver
