package troupe

// AgentCard is the blueprint for an agent — what kind of agent it is
// and what it can do. Multiple Actors can share the same card definition.
// Card is the type (WHAT), ActorConfig is the runtime (HOW).
type AgentCard interface {
	Name() string
	Role() string
	Skills() []string
}
