// Package pool manages agent process lifecycles.
// Maps Bugle World entities to running OS processes via the AgentSupervisor interface.
// Process-agnostic: consumers (Djinn, Origami) inject their own AgentSupervisor.
package warden

import (
	"context"
	"time"

	"github.com/dpopsuev/troupe/world"
)

// RestartPolicy controls automatic restart behavior after an agent exits.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"      // reap only, no restart (default)
	RestartOnFailure RestartPolicy = "on_failure" // restart on non-zero exit
	RestartAlways    RestartPolicy = "always"     // restart on any exit
)

// AgentConfig describes how to start an agent process.
type AgentConfig struct {
	Role          string            // staff role name (e.g., "executor", "inspector")
	Prompt        string            // system prompt
	Model         string            // LLM model name
	Provider      string            // provider name for multi-driver routing
	Tools         []string          // allowed tool names
	WorkDir       string            // working directory
	Env           map[string]string // environment variables
	Budget        float64           // cost ceiling (0 = unlimited)
	Display       *world.Display    // optional display identity (name, color, icon)
	RestartPolicy RestartPolicy     // restart behavior (default: never)
	GracePeriod   time.Duration     // graceful shutdown window (default: 30s)
}

// AgentSupervisor is the process-agnostic interface for starting/stopping agents.
// Djinn implements this with ACP. Origami implements with CLI subprocesses.
type AgentSupervisor interface {
	// Start launches an agent process for the given entity.
	Start(ctx context.Context, id world.EntityID, config AgentConfig) error

	// Stop kills the agent process for the given entity.
	Stop(ctx context.Context, id world.EntityID) error

	// Healthy returns true if the agent process is still running.
	Healthy(ctx context.Context, id world.EntityID) bool
}
