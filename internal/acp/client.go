// client.go — ACP JSON-RPC client over stdio.
//
// Client manages a single ACP agent process: handshake, session lifecycle,
// prompt streaming. Protocol-aware, driver-agnostic — consumers (Djinn,
// Origami) wrap Client with their own driver interfaces.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dpopsuev/troupe/resilience"
)

// Slog attribute key constants.
const (
	logKeyAgent     = "agent"
	logKeyPID       = "pid"
	logKeyAgentName = "agent_name"
	logKeyProtocol  = "protocol"
	logKeyCWD       = "cwd"
	logKeyError     = "error"
	logKeySessionID = "session_id"
	logKeyMethod    = "method"
	logKeyID        = "id"
	logKeyLineLen   = "line_len"
	logKeyCode      = "code"
	logKeyMessage   = "message"
)

// Sentinel errors for ACP operations.
var (
	ErrUnknownAgent = errors.New("unknown ACP agent")
	ErrNoMessages   = errors.New("no messages to send")
	ErrAgentExited  = errors.New("no response (agent exited)")
	ErrAgentError   = errors.New("ACP error response")
)

// CommandFactory creates exec.Cmd — injectable for testing.
type CommandFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// Known ACP agent launch commands.
var AgentCommands = map[string][]string{
	"cursor":   {"agent", "acp"},
	"claude":   {"npx", "@agentclientprotocol/claude-agent-acp"},
	"gemini":   {"gemini", "--acp"},
	"codex":    {"codex-acp"},
	"kiro":     {"kiro-cli", "acp"},
	"goose":    {"goose", "acp"},
	"opencode": {"opencode", "acp"},
	"cline":    {"cline", "acp"},
	"auggie":   {"auggie", "--acp"},
	"devstral": {"devstral", "acp"},
	"qwen":     {"qwen-code", "acp"},
	"kimi":     {"kimi", "acp"},
}

// ClientInfo identifies the ACP client during handshake.
type ClientInfo struct {
	Name    string
	Version string
}

// Client is an ACP JSON-RPC client that manages a single agent process.
type Client struct {
	agentName string   // "cursor", "claude", "gemini", "codex"
	agentCmd  string   // binary name
	agentArgs []string // launch args
	model     string

	cmd       *exec.Cmd
	stdin     *json.Encoder
	scanner   *bufio.Scanner
	sessionID string
	messages  []Message
	mu        sync.Mutex
	nextID    atomic.Int64
	log       *slog.Logger

	clientInfo ClientInfo
	cmdFactory CommandFactory

	// Resilience options (all optional).
	retry   *resilience.RetryConfig
	circuit *resilience.CircuitBreaker
	limiter *resilience.RateLimiter

	// Timeout options.
	handshakeTimeout time.Duration // default 10s
	sessionTimeout   time.Duration // default 10s
	promptTimeout    time.Duration // default 60s (0 = no limit)

	// Environment.
	extraEnv map[string]string // additional env vars for child process
}

// Option configures a Client.
type Option func(*Client)

func WithModel(m string) Option                   { return func(c *Client) { c.model = m } }
func WithLogger(l *slog.Logger) Option            { return func(c *Client) { c.log = l } }
func WithCommandFactory(f CommandFactory) Option  { return func(c *Client) { c.cmdFactory = f } }
func WithClientInfo(info ClientInfo) Option       { return func(c *Client) { c.clientInfo = info } }
func WithRetry(cfg resilience.RetryConfig) Option { return func(c *Client) { c.retry = &cfg } }
func WithCircuitBreaker(cfg resilience.CircuitConfig) Option {
	return func(c *Client) { c.circuit = resilience.NewCircuitBreaker(cfg) }
}
func WithRateLimiter(cfg resilience.RateLimitConfig) Option {
	return func(c *Client) { c.limiter = resilience.NewRateLimiter(cfg) }
}
func WithHandshakeTimeout(d time.Duration) Option { return func(c *Client) { c.handshakeTimeout = d } }
func WithSessionTimeout(d time.Duration) Option   { return func(c *Client) { c.sessionTimeout = d } }
func WithPromptTimeout(d time.Duration) Option    { return func(c *Client) { c.promptTimeout = d } }

// NewClient creates an ACP client for the named agent.
// If the agent is not in the AgentCommands registry, it falls back to
// convention: <name> --acp (Layer 4 of defense-in-depth resolution).
func NewClient(agentName string, opts ...Option) (*Client, error) {
	if s := sanitizeCLI(agentName); s == "" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, agentName)
	}

	args, ok := AgentCommands[agentName]
	if !ok {
		// Convention fallback: <name> --acp
		args = []string{agentName, "--acp"}
	}

	c := &Client{
		agentName:        agentName,
		agentCmd:         args[0],
		agentArgs:        args[1:],
		log:              slog.Default(),
		cmdFactory:       exec.CommandContext,
		clientInfo:       ClientInfo{Name: "bugle", Version: "0.11.0"},
		handshakeTimeout: 10 * time.Second,
		sessionTimeout:   10 * time.Second,
		promptTimeout:    60 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// AgentName returns the configured agent name.
func (c *Client) AgentName() string { return c.agentName }

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

// SessionID returns the current ACP session ID.
func (c *Client) SessionID() string { return c.sessionID }

// ProcessAlive returns true if the underlying agent process is still running.
func (c *Client) ProcessAlive() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	return c.cmd.ProcessState == nil // nil = not yet exited
}

// Start launches the agent process and performs the ACP handshake.
// Applies per-operation timeouts and optional retry with circuit breaker.
func (c *Client) Start(ctx context.Context) error {
	startFn := func() error { return c.doStart(ctx) }

	// Wrap with circuit breaker if configured.
	if c.circuit != nil {
		wrapped := startFn
		startFn = func() error { return c.circuit.Call(wrapped) }
	}

	// Wrap with retry if configured.
	if c.retry != nil {
		return resilience.Retry(ctx, *c.retry, startFn)
	}
	return startFn()
}

// doStart is the actual start implementation with per-operation timeouts.
func (c *Client) doStart(ctx context.Context) error {
	args := make([]string, len(c.agentArgs))
	copy(args, c.agentArgs)

	c.cmd = c.cmdFactory(ctx, c.agentCmd, args...)
	c.cmd.Stderr = os.Stderr
	c.cmd.WaitDelay = 5 * time.Second // drain grandchild I/O after process exits
	c.cmd.Env = safeEnv(c.extraEnv)

	stdinPipe, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.agentCmd, err)
	}

	c.stdin = json.NewEncoder(stdinPipe)
	c.scanner = bufio.NewScanner(stdoutPipe)
	c.scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB buffer

	c.log.InfoContext(ctx, "ACP agent started", slog.String(logKeyAgent, c.agentName), slog.Int(logKeyPID, c.cmd.Process.Pid))

	// Initialize handshake — with timeout.
	initCtx, initCancel := context.WithTimeout(ctx, c.handshakeTimeout)
	defer initCancel()

	type callResult struct {
		resp *json.RawMessage
		err  error
	}
	initCh := make(chan callResult, 1)
	go func() {
		resp, err := c.call(initCtx, "initialize", initializeParams{
			ProtocolVersion: ProtocolVersion,
			ClientInfo:      clientInfo{Name: c.clientInfo.Name, Version: c.clientInfo.Version},
		})
		initCh <- callResult{resp, err}
	}()

	select {
	case <-initCtx.Done():
		c.cmd.Process.Kill() //nolint:errcheck // best-effort kill on timeout
		return fmt.Errorf("ACP initialize: %w", initCtx.Err())
	case r := <-initCh:
		if r.err != nil {
			c.cmd.Process.Kill() //nolint:errcheck // best-effort kill on handshake failure
			return fmt.Errorf("ACP initialize: %w", r.err)
		}
		var initResult initializeResult
		if r.resp != nil {
			json.Unmarshal(*r.resp, &initResult) //nolint:errcheck // non-critical metadata parse
		}
		c.log.InfoContext(ctx, "ACP initialized", slog.String(logKeyAgentName, initResult.AgentInfo.Name), slog.Int(logKeyProtocol, initResult.ProtocolVersion))
	}

	// Create session — with timeout.
	sessCtx, sessCancel := context.WithTimeout(ctx, c.sessionTimeout)
	defer sessCancel()

	cwd, _ := os.Getwd()
	c.log.DebugContext(ctx, "ACP session/new", slog.String(logKeyCWD, cwd))

	sessCh := make(chan callResult, 1)
	go func() {
		resp, err := c.call(sessCtx, "session/new", newSessionParams{CWD: cwd, MCPServers: []any{}})
		sessCh <- callResult{resp, err}
	}()

	select {
	case <-sessCtx.Done():
		c.cmd.Process.Kill() //nolint:errcheck // best-effort kill on session timeout
		return fmt.Errorf("ACP session/new: %w", sessCtx.Err())
	case r := <-sessCh:
		if r.err != nil {
			c.log.ErrorContext(ctx, "ACP session/new failed", slog.Any(logKeyError, r.err))
			c.cmd.Process.Kill() //nolint:errcheck // best-effort kill on session failure
			return fmt.Errorf("ACP session/new: %w", r.err)
		}
		var sessResult newSessionResult
		if r.resp != nil {
			json.Unmarshal(*r.resp, &sessResult) //nolint:errcheck // non-critical metadata parse
		}
		c.sessionID = sessResult.SessionID
		c.log.InfoContext(ctx, "ACP session created", slog.String(logKeySessionID, c.sessionID))
	}

	return nil
}

// Stop cancels the session and kills the agent process.
// Sends a graceful cancel notification, then kills after 5s if still alive.
func (c *Client) Stop(_ context.Context) error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	c.notify("session/cancel", map[string]string{"sessionId": c.sessionID})

	// Give the process 5s to exit gracefully.
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		c.cmd.Process.Kill() //nolint:errcheck // best-effort kill after graceful timeout
		return <-done
	}
}

// Send appends a message to the conversation history.
func (c *Client) Send(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
}

// Messages returns a copy of the conversation history.
func (c *Client) Messages() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Message, len(c.messages))
	copy(out, c.messages)
	return out
}

// Chat sends the last message as a prompt and streams ACP session/update events.
// Applies rate limiter (if configured) before sending the prompt.
func (c *Client) Chat(ctx context.Context) (<-chan StreamEvent, error) { //nolint:gocyclo // streaming protocol dispatch is inherently branchy
	// Rate limit — block until token available.
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit: %w", err)
		}
	}

	c.mu.Lock()
	if len(c.messages) == 0 {
		c.mu.Unlock()
		return nil, ErrNoMessages
	}
	lastMsg := c.messages[len(c.messages)-1]
	c.mu.Unlock()

	id := c.nextID.Add(1)
	err := c.stdin.Encode(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      int(id),
		Method:  "session/prompt",
		Params: promptParams{
			SessionID: c.sessionID,
			Prompt:    []promptBlock{{Type: "text", Text: lastMsg.Content}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		var fullText string

		for c.scanner.Scan() {
			line := c.scanner.Text()
			if line == "" {
				continue
			}

			var msg jsonRPCResponse
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			// Response to our prompt request.
			if msg.ID == int(id) && msg.Result != nil {
				ch <- StreamEvent{Type: EventDone}

				if fullText != "" {
					c.mu.Lock()
					c.messages = append(c.messages, Message{
						Role:    RoleAssistant,
						Content: fullText,
					})
					c.mu.Unlock()
				}
				return
			}

			// Notification (session/update).
			if msg.Method == "session/update" && msg.Params != nil {
				var notif sessionUpdateNotification
				if err := json.Unmarshal(*msg.Params, &notif); err != nil {
					continue
				}

				switch notif.Update.SessionUpdate {
				case "agent_message_chunk":
					if notif.Update.Content != nil && notif.Update.Content.Type == "text" {
						ch <- StreamEvent{Type: EventText, Text: notif.Update.Content.Text}
						fullText += notif.Update.Content.Text
					}
				case "tool_call":
					ch <- StreamEvent{
						Type: EventToolUse,
						ToolCall: &ToolCall{
							ID:   notif.Update.ToolCallID,
							Name: notif.Update.Title,
						},
					}
				case "tool_call_update":
					if notif.Update.Content != nil && notif.Update.Content.Text != "" {
						ch <- StreamEvent{Type: EventText, Text: notif.Update.Content.Text}
					}
				}
			}

			// Error response.
			if msg.Error != nil {
				ch <- StreamEvent{Type: EventError, Error: msg.Error.Message}
				return
			}
		}

		ch <- StreamEvent{Type: EventError, Error: "ACP agent process exited"}
	}()

	return ch, nil
}

// call sends a JSON-RPC request and reads the response.
func (c *Client) call(ctx context.Context, method string, params any) (*json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	c.log.DebugContext(ctx, "ACP call", slog.String(logKeyMethod, method), slog.Int(logKeyID, id))
	if err := c.stdin.Encode(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	for c.scanner.Scan() {
		line := c.scanner.Text()
		if line == "" {
			continue
		}
		c.log.DebugContext(ctx, "ACP recv", slog.String(logKeyMethod, method), slog.Int(logKeyLineLen, len(line)))

		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			c.log.WarnContext(ctx, "ACP parse error", slog.String(logKeyMethod, method), slog.Any(logKeyError, err))
			continue
		}

		if resp.ID == id {
			if resp.Error != nil {
				c.log.ErrorContext(ctx, "ACP error response", slog.String(logKeyMethod, method), slog.Int(logKeyCode, resp.Error.Code), slog.String(logKeyMessage, resp.Error.Message))
				return nil, fmt.Errorf("%w: %s: %s", ErrAgentError, method, resp.Error.Message)
			}
			c.log.DebugContext(ctx, "ACP result", slog.String(logKeyMethod, method), slog.Int(logKeyID, id))
			return resp.Result, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrAgentExited, method)
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) {
	c.stdin.Encode(map[string]any{ //nolint:errcheck // fire-and-forget notification, no response expected
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}
