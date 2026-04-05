package troupe

import (
	"context"
	"errors"

	"github.com/dpopsuev/troupe/world"
)

// ErrNoDriver is returned when no driver is configured for a provider.
var ErrNoDriver = errors.New("no driver configured")

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

// DriverDescriptor is an optional interface that Drivers implement to
// declare capabilities. Drivers that don't implement it are treated as
// opaque. Go assertion pattern: if d, ok := driver.(DriverDescriptor); ok.
type DriverDescriptor interface {
	Describe() DriverInfo
}

// DriverInfo describes a Driver's capabilities and constraints.
type DriverInfo struct {
	// Name is a human-readable driver name (e.g., "acp", "http-anthropic").
	Name string
	// Models lists the model identifiers this driver can provision.
	// Empty = unknown/any.
	Models []string
	// MaxConcurrent is how many simultaneous agents this driver supports.
	// 0 = unlimited.
	MaxConcurrent int
}

// DriverValidator is an optional interface for pre-flight environment checks.
// Broker.Spawn calls ValidateEnvironment before Driver.Start if implemented.
type DriverValidator interface {
	ValidateEnvironment(ctx context.Context) error
}
