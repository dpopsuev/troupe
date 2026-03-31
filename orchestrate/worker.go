// Package orchestrate manages agent workers that connect to MCP endpoints
// and loop pull-work/pipe-to-agent/submit via the Bugle Protocol.
package orchestrate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/jericho/acp"
	"github.com/dpopsuev/jericho/bugle"
	"github.com/dpopsuev/jericho/facade"
	"github.com/dpopsuev/jericho/pool"
)

// Log key constants for sloglint compliance.
const (
	logKeyWorker = "worker"
	logKeyAgent  = "agent"
	logKeyItems  = "steps"
	logKeyItem   = "item"
	logKeyError  = "error"
)

// WorkerConfig configures the Bugle Protocol step/submit loop.
type WorkerConfig struct {
	// MCP tool name (default: bugle.DefaultToolName).
	ToolName string
	// Action for pulling work (default: bugle.ActionPull).
	PullAction string
	// Action for pushing results (default: bugle.ActionPush).
	PushAction string
	// Session key name in arguments (default: bugle.DefaultSessionKey).
	SessionKey string
	// WorkerID sent on step/submit. If empty, uses workerName argument.
	WorkerID string
	// HornFunc is called before submit to report worker health. Nil = omit.
	HornFunc func() *bugle.Horn
	// BudgetFunc is called before submit to report resource consumption. Nil = omit.
	BudgetFunc func() *bugle.BudgetActual
	// OnPull is called after each pull response with protocol metadata. Nil = no-op.
	OnPull func(bugle.PullMeta)
}

func (c *WorkerConfig) defaults() {
	if c.ToolName == "" {
		c.ToolName = bugle.DefaultToolName
	}
	if c.PullAction == "" {
		c.PullAction = string(bugle.ActionPull)
	}
	if c.PushAction == "" {
		c.PushAction = string(bugle.ActionPush)
	}
	if c.SessionKey == "" {
		c.SessionKey = bugle.DefaultSessionKey
	}
}

// RunWorker is a single worker loop: spawn agent, connect to endpoint,
// pull steps, pipe to agent, submit artifacts. Blocks until done or ctx canceled.
//
//nolint:funlen // protocol loop with step/submit/abort/blocked paths
func RunWorker(ctx context.Context, endpoint, agentName, sessionID, workerName string, cfg WorkerConfig) error {
	cfg.defaults()

	workerID := cfg.WorkerID
	if workerID == "" {
		workerID = workerName
	}

	handle, staff, err := spawnAgent(ctx, agentName, workerName)
	if err != nil {
		return err
	}
	defer staff.KillAll(ctx)

	session, err := connectEndpoint(ctx, endpoint, workerName)
	if err != nil {
		return err
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
			"worker_id":    workerID,
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
		var pullResp bugle.PullResponse
		if err := json.Unmarshal([]byte(text), &pullResp); err != nil {
			return fmt.Errorf("parse step: %w", err)
		}

		// Notify callback with protocol metadata.
		if cfg.OnPull != nil {
			cfg.OnPull(bugle.PullMeta{
				Horn:            pullResp.Horn,
				BudgetRemaining: pullResp.BudgetRemaining,
			})
		}

		// Horn black = abort signal.
		if pullResp.Horn == bugle.HornBlack {
			slog.WarnContext(ctx, "abort signal received",
				slog.String(logKeyWorker, workerName))
			return nil
		}

		if pullResp.Done {
			slog.InfoContext(ctx, "work complete",
				slog.String(logKeyWorker, workerName),
				slog.Int(logKeyItems, steps))
			return nil
		}
		if !pullResp.Available {
			continue
		}

		response, err := handle.Ask(ctx, pullResp.PromptContent)
		if err != nil {
			slog.ErrorContext(ctx, "agent ask failed",
				slog.String(logKeyWorker, workerName),
				slog.String(logKeyItem, pullResp.Item),
				slog.Any(logKeyError, err))

			// Submit as blocked instead of silently continuing.
			submitBlocked(ctx, session, cfg, sessionID, workerID, pullResp, err)
			continue
		}

		submitArgs := map[string]any{
			"action":       cfg.PushAction,
			cfg.SessionKey: sessionID,
			"worker_id":    workerID,
			"dispatch_id":  pullResp.DispatchID,
			"item":         pullResp.Item,
			"fields":       json.RawMessage(response),
		}
		if cfg.HornFunc != nil {
			if h := cfg.HornFunc(); h != nil {
				submitArgs["horn"] = h
			}
		}
		if cfg.BudgetFunc != nil {
			if b := cfg.BudgetFunc(); b != nil {
				submitArgs["budget_actual"] = b
			}
		}

		_, err = session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      cfg.ToolName,
			Arguments: marshalArgs(submitArgs),
		})
		if err != nil {
			slog.WarnContext(ctx, "submit failed",
				slog.String(logKeyWorker, workerName),
				slog.String(logKeyItem, pullResp.Item),
				slog.Any(logKeyError, err))
		}
		steps++
	}
}

func spawnAgent(ctx context.Context, agentName, workerName string) (*facade.AgentHandle, *facade.Staff, error) {
	launcher := acp.NewACPLauncher()
	staff := facade.NewStaff(launcher)
	handle, err := staff.Spawn(ctx, "worker", pool.LaunchConfig{
		Model: agentName,
		Role:  "worker",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("spawn agent %q: %w", agentName, err)
	}
	slog.InfoContext(ctx, "agent spawned",
		slog.String(logKeyWorker, workerName),
		slog.String(logKeyAgent, agentName))
	return handle, staff, nil
}

func connectEndpoint(ctx context.Context, endpoint, workerName string) (*sdkmcp.ClientSession, error) {
	transport := &sdkmcp.StreamableClientTransport{Endpoint: endpoint}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "jericho-" + workerName, Version: "v0.1.0"},
		nil,
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to endpoint: %w", err)
	}
	return session, nil
}

// submitBlocked sends a blocked status when the agent fails.
func submitBlocked(ctx context.Context, session *sdkmcp.ClientSession, cfg WorkerConfig, sessionID, workerID string, pullResp bugle.PullResponse, askErr error) {
	blockedArgs := map[string]any{
		"action":       cfg.PushAction,
		cfg.SessionKey: sessionID,
		"worker_id":    workerID,
		"dispatch_id":  pullResp.DispatchID,
		"item":         pullResp.Item,
		"status":       bugle.StatusBlocked,
		"fields":       map[string]any{"reason": askErr.Error()},
	}
	_, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      cfg.ToolName,
		Arguments: marshalArgs(blockedArgs),
	})
	if err != nil {
		slog.WarnContext(ctx, "submit blocked failed",
			slog.String(logKeyItem, pullResp.Item),
			slog.Any(logKeyError, err))
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
