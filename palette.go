package bugle

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

// Palette defines 7 shade families × 8 colours = 56 unique agent identities.
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
