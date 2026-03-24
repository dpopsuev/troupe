package palette

import (
	"fmt"
	"math/rand/v2"
	"sync"
)

// Registry manages color identity assignment with collision prevention.
type Registry struct {
	mu       sync.Mutex
	assigned map[string]bool // "shade·colour" → true
}

// NewRegistry creates an empty identity registry.
func NewRegistry() *Registry {
	return &Registry{
		assigned: make(map[string]bool),
	}
}

func registryKey(shade, colour string) string {
	return shade + "·" + colour
}

// Assign returns a ColorIdentity with a random available colour.
// Picks a random shade, then a random available colour within it.
func (r *Registry) Assign(role, collective string) (ColorIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Shuffle shade order for randomness
	shades := make([]int, len(Palette))
	for i := range shades {
		shades[i] = i
	}
	rand.Shuffle(len(shades), func(i, j int) { shades[i], shades[j] = shades[j], shades[i] })

	for _, si := range shades {
		shade := Palette[si]
		for _, colour := range shade.Colours {
			key := registryKey(shade.Name, colour.Name)
			if !r.assigned[key] {
				r.assigned[key] = true
				return ColorIdentity{
					Shade:      shade.Name,
					Colour:     colour.Name,
					Role:       role,
					Collective: collective,
					Hex:        colour.Hex,
				}, nil
			}
		}
	}
	return ColorIdentity{}, fmt.Errorf("palette: all 56 colour slots are assigned")
}

// AssignInGroup returns a ColorIdentity from a specific shade family.
func (r *Registry) AssignInGroup(shade, role, collective string) (ColorIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s := LookupShade(shade)
	if s == nil {
		return ColorIdentity{}, fmt.Errorf("palette: unknown shade %q", shade)
	}

	for _, colour := range s.Colours {
		key := registryKey(s.Name, colour.Name)
		if !r.assigned[key] {
			r.assigned[key] = true
			return ColorIdentity{
				Shade:      s.Name,
				Colour:     colour.Name,
				Role:       role,
				Collective: collective,
				Hex:        colour.Hex,
			}, nil
		}
	}
	return ColorIdentity{}, fmt.Errorf("palette: all colours in shade %q are assigned", shade)
}

// Set explicitly assigns a specific shade+colour combination.
func (r *Registry) Set(shade, colour, role, collective string) (ColorIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, foundShade, ok := LookupColour(colour)
	if !ok {
		return ColorIdentity{}, fmt.Errorf("palette: unknown colour %q", colour)
	}
	if foundShade != shade {
		return ColorIdentity{}, fmt.Errorf("palette: colour %q belongs to shade %q, not %q", colour, foundShade, shade)
	}

	key := registryKey(shade, colour)
	if r.assigned[key] {
		return ColorIdentity{}, fmt.Errorf("palette: %s·%s is already assigned", shade, colour)
	}

	r.assigned[key] = true
	return ColorIdentity{
		Shade:      shade,
		Colour:     c.Name,
		Role:       role,
		Collective: collective,
		Hex:        c.Hex,
	}, nil
}

// Release returns a colour to the available pool.
func (r *Registry) Release(id ColorIdentity) { //nolint:gocritic // value param for API simplicity
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.assigned, registryKey(id.Shade, id.Colour))
}

// Active returns all currently assigned identities' keys.
func (r *Registry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.assigned)
}
