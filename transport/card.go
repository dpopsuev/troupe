package transport

import (
	"fmt"

	"github.com/dpopsuev/bugle"
	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/world"
)

// AgentCard is the A2A-compatible agent descriptor published for discovery.
// It summarizes an entity's identity, capabilities, and transport endpoint.
type AgentCard struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Role      string            `json:"role"`
	Element   string            `json:"element,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Transport string            `json:"transport"`
	Skills    []string          `json:"skills,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CardFromEntity builds an AgentCard from an entity's ECS components.
//   - ColorIdentity → Name (Title()), Role
//   - AgentIdentity (via parent entity if attached) → Element
//   - Health → Metadata["health"]
//   - Transport defaults to "local"
//   - ID = "agent-{entityID}"
func CardFromEntity(w *world.World, id world.EntityID) AgentCard {
	card := AgentCard{
		ID:        fmt.Sprintf("agent-%d", id),
		Transport: "local",
		Metadata:  make(map[string]string),
	}

	if color, ok := world.TryGet[palette.ColorIdentity](w, id); ok {
		card.Name = color.Title()
		card.Role = color.Role
	}

	if health, ok := world.TryGet[bugle.Health](w, id); ok {
		card.Metadata["health"] = string(health.State)
	}

	// Clean up empty metadata to match JSON omitempty semantics.
	if len(card.Metadata) == 0 {
		card.Metadata = nil
	}

	return card
}
