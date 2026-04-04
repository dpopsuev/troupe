package jericho

import (
	"context"

	"github.com/dpopsuev/jericho/world"
)

// Driver provisions and communicates with agents. The Broker delegates
// agent lifecycle to a Driver. Each Driver implementation handles a
// different transport protocol.
//
// Implementations:
//   - ACP Driver (internal/acp) — subprocess + stdio JSON-RPC
//   - HTTP Driver (planned) — REST/SSE to API endpoints
type Driver interface {
	// Start provisions an agent for the given entity.
	Start(ctx context.Context, id world.EntityID, config ActorConfig) error

	// Stop deprovisions the agent for the given entity.
	Stop(ctx context.Context, id world.EntityID) error
}
