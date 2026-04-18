package transport

import (
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/dpopsuev/troupe/world"
)

const a2aProtocolVersion = "1.0"

// BuildA2ACard creates an A2A v1.0 AgentCard from an entity's ECS components.
func BuildA2ACard(_ *world.World, id world.EntityID, role, endpointURL string) a2a.AgentCard {
	card := a2a.AgentCard{
		Name:            fmt.Sprintf("%s-agent-%d", role, id),
		Description:     "Troupe agent: " + role,
		URL:             endpointURL,
		Version:         "1.0.0",
		ProtocolVersion: a2aProtocolVersion,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities: a2a.AgentCapabilities{
			Streaming:              true,
			StateTransitionHistory: true,
		},
		Skills: []a2a.AgentSkill{{
			ID:          role,
			Name:        role,
			Description: "Performs tasks as " + role,
		}},
	}
	return card
}

// BuildA2ACardsFromHandlers creates A2A cards for all registered handlers.
func BuildA2ACardsFromHandlers(handlers map[AgentID]MsgHandler, roles *RoleRegistry, endpointURL string) []a2a.AgentCard {
	cards := make([]a2a.AgentCard, 0, len(handlers))
	for id := range handlers {
		role := roles.RoleOf(string(id))
		cards = append(cards, a2a.AgentCard{
			Name:            string(id),
			Description:     "Troupe agent: " + role,
			URL:             endpointURL,
			Version:         "1.0.0",
			ProtocolVersion: a2aProtocolVersion,
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
			Capabilities: a2a.AgentCapabilities{
				Streaming:              true,
				StateTransitionHistory: true,
			},
			Skills: []a2a.AgentSkill{{
				ID:          role,
				Name:        role,
				Description: "Performs tasks as " + role,
			}},
		})
	}
	return cards
}
