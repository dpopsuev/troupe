package troupe

import "context"

// AgentInfo describes a live agent in the troupe.
// Human-facing: no protocol details, just who they are and how they're doing.
type AgentInfo struct {
	// ID is the unique agent identifier.
	ID string `json:"id"`

	// Role is the functional role (e.g., "investigator", "reviewer").
	Role string `json:"role"`

	// Model is the resolved model identifier.
	Model string `json:"model,omitempty"`

	// Ready reports whether the agent can accept work.
	Ready bool `json:"ready"`

	// Healthy reports whether the agent process is alive.
	Healthy bool `json:"healthy"`
}

// Broker is the Actor Broker — it casts and hires actors for Directors.
// Facade over arsenal (Pick), pool+acp (Spawn), roster (Discover).
type Broker interface {
	// Pick returns actor configurations matching the given preferences.
	// Backed by the Arsenal catalog.
	Pick(ctx context.Context, prefs Preferences) ([]ActorConfig, error)

	// Spawn creates a running actor from the given configuration.
	Spawn(ctx context.Context, config ActorConfig) (Actor, error)

	// Discover returns agent cards for live agents, optionally filtered by role.
	// Empty role returns all agents.
	Discover(role string) []AgentCard
}

// Preferences describes what kind of actor the Director needs.
type Preferences struct {
	// Role is the functional role (e.g., "investigator", "reviewer").
	Role string `json:"role,omitempty"`

	// Model requests a specific model (e.g., "sonnet", "opus").
	// Empty = let the Broker decide.
	Model string `json:"model,omitempty"`

	// Count is how many actors to pick. Default 1.
	Count int `json:"count,omitempty"`
}

// ActorConfig is the resolved configuration for spawning an actor.
// Returned by Broker.Pick, consumed by Broker.Spawn.
type ActorConfig struct {
	// Model is the resolved model identifier.
	Model string `json:"model"`

	// Provider is the resolved provider (e.g., "anthropic", "openai").
	Provider string `json:"provider,omitempty"`

	// Role is the assigned role.
	Role string `json:"role,omitempty"`

	// CallbackURL is the A2A endpoint for external agents.
	// When set, the agent is external — messages are proxied to this URL.
	// When empty, the agent is internal — started via Driver.
	CallbackURL string `json:"callback_url,omitempty"`
}

// IsExternal returns true if this config represents an external agent.
func (c ActorConfig) IsExternal() bool { return c.CallbackURL != "" }
