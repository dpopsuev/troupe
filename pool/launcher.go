// Package pool manages agent process lifecycles.
// Maps Bugle World entities to running OS processes via the Launcher interface.
// Process-agnostic: consumers (Djinn, Origami) inject their own Launcher.
package pool

import (
	"context"

	"github.com/dpopsuev/bugle/world"
)

// LaunchConfig describes how to start an agent process.
type LaunchConfig struct {
	Role    string            // staff role name (e.g., "executor", "inspector")
	Prompt  string            // system prompt
	Model   string            // LLM model name
	Tools   []string          // allowed tool names
	WorkDir string            // working directory
	Env     map[string]string // environment variables
	Budget  float64           // cost ceiling (0 = unlimited)
}

// Launcher is the process-agnostic interface for starting/stopping agents.
// Djinn implements this with ACP. Origami implements with CLI subprocesses.
type Launcher interface {
	// Start launches an agent process for the given entity.
	Start(ctx context.Context, id world.EntityID, config LaunchConfig) error

	// Stop kills the agent process for the given entity.
	Stop(ctx context.Context, id world.EntityID) error

	// Healthy returns true if the agent process is still running.
	Healthy(ctx context.Context, id world.EntityID) bool
}
