// Package orchestrate manages agent workers that connect to MCP endpoints
// and loop pull-work/pipe-to-agent/submit. Generic — works with any MCP
// server that has a step/submit pattern (Origami circuits, or anything else).
package orchestrate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/bugle/acp"
	"github.com/dpopsuev/bugle/facade"
	"github.com/dpopsuev/bugle/pool"
)

// Log key constants for sloglint compliance.
const (
	logKeyWorker = "worker"
	logKeyAgent  = "agent"
	logKeySteps  = "steps"
	logKeyStep   = "step"
	logKeyError  = "error"
)

// WorkerConfig configures the step/submit loop.
type WorkerConfig struct {
	// MCP tool name for pulling work (default: "circuit").
	ToolName string
	// Action value for pulling work (default: "step").
	PullAction string
	// Action value for submitting results (default: "submit").
	PushAction string
	// Session key name in arguments (default: "session_id").
	SessionKey string
}

func (c *WorkerConfig) defaults() {
	if c.ToolName == "" {
		c.ToolName = "circuit"
	}
	if c.PullAction == "" {
		c.PullAction = "step"
	}
	if c.PushAction == "" {
		c.PushAction = "submit"
	}
	if c.SessionKey == "" {
		c.SessionKey = "session_id"
	}
}

// RunWorker is a single worker loop: spawn agent, connect to endpoint,
// pull steps, pipe to agent, submit artifacts. Blocks until done or ctx canceled.
func RunWorker(ctx context.Context, endpoint, agentName, sessionID, workerName string, cfg WorkerConfig) error {
	cfg.defaults()

	launcher := acp.NewACPLauncher()
	staff := facade.NewStaff(launcher)
	handle, err := staff.Spawn(ctx, "worker", pool.LaunchConfig{
		Model: agentName,
		Role:  "worker",
	})
	if err != nil {
		return fmt.Errorf("spawn agent %q: %w", agentName, err)
	}
	defer staff.KillAll(ctx)

	slog.InfoContext(ctx, "agent spawned",
		slog.String(logKeyWorker, workerName),
		slog.String(logKeyAgent, agentName))

	transport := &sdkmcp.StreamableClientTransport{Endpoint: endpoint}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "bugle-" + workerName, Version: "v0.1.0"},
		nil,
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect to endpoint: %w", err)
	}
	defer session.Close()

	steps := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		pullArgs := map[string]any{
			"action":       cfg.PullAction,
			cfg.SessionKey: sessionID,
			"timeout_ms":   30000,
		}
		result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      cfg.ToolName,
			Arguments: marshalArgs(pullArgs),
		})
		if err != nil {
			return fmt.Errorf("%s/%s: %w", cfg.ToolName, cfg.PullAction, err)
		}

		if result.IsError {
			return fmt.Errorf("%w: %s", ErrStepFailed, textContent(result))
		}

		text := textContent(result)
		var step struct {
			Done       bool   `json:"done"`
			Available  bool   `json:"available"`
			Step       string `json:"step"`
			Prompt     string `json:"prompt_content"`
			DispatchID int64  `json:"dispatch_id"`
		}
		if err := json.Unmarshal([]byte(text), &step); err != nil {
			return fmt.Errorf("parse step: %w", err)
		}

		if step.Done {
			slog.InfoContext(ctx, "work complete",
				slog.String(logKeyWorker, workerName),
				slog.Int(logKeySteps, steps))
			return nil
		}
		if !step.Available {
			continue
		}

		response, err := handle.Ask(ctx, step.Prompt)
		if err != nil {
			slog.ErrorContext(ctx, "agent ask failed",
				slog.String(logKeyWorker, workerName),
				slog.String(logKeyStep, step.Step),
				slog.Any(logKeyError, err))
			continue
		}

		pushArgs := map[string]any{
			"action":       cfg.PushAction,
			cfg.SessionKey: sessionID,
			"dispatch_id":  step.DispatchID,
			"step":         step.Step,
			"fields":       json.RawMessage(response),
		}
		_, err = session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      cfg.ToolName,
			Arguments: marshalArgs(pushArgs),
		})
		if err != nil {
			slog.WarnContext(ctx, "submit failed",
				slog.String(logKeyWorker, workerName),
				slog.String(logKeyStep, step.Step),
				slog.Any(logKeyError, err))
		}
		steps++
	}
}

func marshalArgs(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func textContent(result *sdkmcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
