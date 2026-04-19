package troupe

import "context"

// AgentInfo describes a live agent in the troupe.
type AgentInfo struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Model   string `json:"model,omitempty"`
	Ready   bool   `json:"ready"`
	Healthy bool   `json:"healthy"`
}

// Caster selects and spawns actors. The narrow interface for Directors
// and Collectives — they need a factory, not the full Troupe facade.
// Troupe facade satisfies this. So does the internal Broker.
type Caster interface {
	Pick(ctx context.Context, prefs Preferences) ([]ActorConfig, error)
	Spawn(ctx context.Context, config ActorConfig) (Actor, error)
}

// Broker extends Caster with agent discovery. Used internally by the
// broker package. Consumers should use the Troupe facade or Caster.
type Broker interface {
	Caster
	Discover(role string) []AgentCard
}

// Preferences describes what kind of actor is needed.
type Preferences struct {
	Role  string `json:"role,omitempty"`
	Model string `json:"model,omitempty"`
	Count int    `json:"count,omitempty"`
}

// ActorConfig is the resolved configuration for spawning an actor.
type ActorConfig struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	Role        string   `json:"role,omitempty"`
	Thinking    float64  `json:"thinking,omitempty"`
	Domain      string   `json:"domain,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	CallbackURL string   `json:"callback_url,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
}

// IsExternal returns true if this config represents an external agent.
func (c ActorConfig) IsExternal() bool { return c.CallbackURL != "" }
