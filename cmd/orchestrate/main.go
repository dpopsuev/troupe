// Command orchestrate is a stdio MCP server that manages agent workers.
// Register in Claude Code: claude mcp add orchestrator -- bugle orchestrate --endpoint http://localhost:9000/mcp
//
// Usage:
//
//	orchestrate --endpoint http://localhost:9000/mcp
//	orchestrate --endpoint http://localhost:9000/mcp --session iron-stag --agent claude --count 12
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/dpopsuev/bugle/orchestrate"
)

// Log key constants for sloglint compliance.
const (
	logKeyError    = "error"
	logKeyEndpoint = "endpoint"
	logKeyMode     = "mode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "orchestrator: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := flag.String("endpoint", envOr("BUGLE_ENDPOINT", "http://localhost:9000/mcp"), "MCP endpoint to connect workers to")
	session := flag.String("session", "", "auto-start workers for this session (optional)")
	agent := flag.String("agent", "claude", "agent CLI name")
	count := flag.Int("count", 4, "number of workers (for auto-start)")
	tool := flag.String("tool", "circuit", "MCP tool name for step/submit")
	pullAction := flag.String("pull-action", "step", "action name for pulling work")
	pushAction := flag.String("push-action", "submit", "action name for submitting results")
	sessionKey := flag.String("session-key", "session_id", "session key name in arguments")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg := orchestrate.WorkerConfig{
		ToolName:   *tool,
		PullAction: *pullAction,
		PushAction: *pushAction,
		SessionKey: *sessionKey,
	}
	mgr := orchestrate.NewManager(*endpoint, cfg)

	// If --session is provided, auto-start workers immediately.
	if *session != "" {
		if err := mgr.Start(ctx, *session, *agent, *count); err != nil {
			slog.ErrorContext(ctx, "auto-start failed", slog.Any(logKeyError, err))
			return err
		}
	}

	// Run as stdio MCP server.
	slog.InfoContext(ctx, "orchestrator starting",
		slog.String(logKeyEndpoint, *endpoint),
		slog.String(logKeyMode, "stdio"))

	return orchestrate.ServeStdio(ctx, mgr)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
