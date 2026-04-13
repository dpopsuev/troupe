package world

// ToolCard describes a tool registered in the World.
// Tools are entities alongside agents — discoverable, trackable.
type ToolCard struct {
	Name         string   `json:"name"`
	Provider     string   `json:"provider,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	CostModel    string   `json:"cost_model,omitempty"` // e.g., "per-call", "per-token"
}

// ComponentType implements Component.
func (ToolCard) ComponentType() ComponentType { return ToolCardType }

// RegisterTool creates a new entity with a ToolCard component.
func RegisterTool(w *World, card ToolCard) EntityID {
	id := w.Spawn()
	Attach(w, id, card)
	return id
}

// QueryToolsByCapability returns ToolCards for entities that have
// at least one matching capability.
func QueryToolsByCapability(w *World, capability string) []ToolCard {
	ids := Query[ToolCard](w)
	var result []ToolCard
	for _, id := range ids {
		tc, ok := TryGet[ToolCard](w, id)
		if !ok {
			continue
		}
		for _, cap := range tc.Capabilities {
			if cap == capability {
				result = append(result, tc)
				break
			}
		}
	}
	return result
}

// AllTools returns all registered ToolCards.
func AllTools(w *World) []ToolCard {
	ids := Query[ToolCard](w)
	result := make([]ToolCard, 0, len(ids))
	for _, id := range ids {
		if tc, ok := TryGet[ToolCard](w, id); ok {
			result = append(result, tc)
		}
	}
	return result
}
