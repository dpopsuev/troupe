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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
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
}

// Option configures a Client.
type Option func(*Client)

func WithModel(m string) Option              { return func(c *Client) { c.model = m } }
func WithLogger(l *slog.Logger) Option       { return func(c *Client) { c.log = l } }
func WithCommandFactory(f CommandFactory) Option { return func(c *Client) { c.cmdFactory = f } }
func WithClientInfo(info ClientInfo) Option   { return func(c *Client) { c.clientInfo = info } }

// NewClient creates an ACP client for the named agent.
func NewClient(agentName string, opts ...Option) (*Client, error) {
	args, ok := AgentCommands[agentName]
	if !ok {
		return nil, fmt.Errorf("unknown ACP agent %q (supported: cursor, claude, gemini, codex)", agentName)
	}

	c := &Client{
		agentName:  agentName,
		agentCmd:   args[0],
		agentArgs:  args[1:],
		log:        slog.Default(),
		cmdFactory: exec.CommandContext,
		clientInfo: ClientInfo{Name: "bugle", Version: "0.10.0"},
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

// Start launches the agent process and performs the ACP handshake.
func (c *Client) Start(ctx context.Context) error {
	args := make([]string, len(c.agentArgs))
	copy(args, c.agentArgs)

	c.cmd = c.cmdFactory(ctx, c.agentCmd, args...)
	c.cmd.Stderr = os.Stderr

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

	c.log.Info("ACP agent started", "agent", c.agentName, "pid", c.cmd.Process.Pid)

	// Initialize handshake.
	initResp, err := c.call("initialize", initializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      clientInfo{Name: c.clientInfo.Name, Version: c.clientInfo.Version},
	})
	if err != nil {
		c.cmd.Process.Kill() //nolint:errcheck
		return fmt.Errorf("ACP initialize: %w", err)
	}

	var initResult initializeResult
	if initResp != nil {
		json.Unmarshal(*initResp, &initResult) //nolint:errcheck
	}
	c.log.Info("ACP initialized", "agent_name", initResult.AgentInfo.Name, "protocol", initResult.ProtocolVersion)

	// Create session.
	cwd, _ := os.Getwd()
	c.log.Debug("ACP session/new", "cwd", cwd)
	sessResp, err := c.call("session/new", newSessionParams{CWD: cwd, MCPServers: []any{}})
	if err != nil {
		c.log.Error("ACP session/new failed", "error", err)
		c.cmd.Process.Kill() //nolint:errcheck
		return fmt.Errorf("ACP session/new: %w", err)
	}

	var sessResult newSessionResult
	if sessResp != nil {
		json.Unmarshal(*sessResp, &sessResult) //nolint:errcheck
	}
	c.sessionID = sessResult.SessionID
	c.log.Info("ACP session created", "session_id", c.sessionID)

	return nil
}

// Stop cancels the session and kills the agent process.
func (c *Client) Stop(_ context.Context) error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	c.notify("session/cancel", map[string]string{"sessionId": c.sessionID})
	c.cmd.Process.Kill() //nolint:errcheck
	return c.cmd.Wait()
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
func (c *Client) Chat(ctx context.Context) (<-chan StreamEvent, error) {
	c.mu.Lock()
	if len(c.messages) == 0 {
		c.mu.Unlock()
		return nil, fmt.Errorf("no messages to send")
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
func (c *Client) call(method string, params any) (*json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	c.log.Debug("ACP call", "method", method, "id", id)
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
		c.log.Debug("ACP recv", "method", method, "line_len", len(line))

		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			c.log.Warn("ACP parse error", "method", method, "error", err)
			continue
		}

		if resp.ID == id {
			if resp.Error != nil {
				c.log.Error("ACP error response", "method", method, "code", resp.Error.Code, "message", resp.Error.Message)
				return nil, fmt.Errorf("%s error: %s", method, resp.Error.Message)
			}
			c.log.Debug("ACP result", "method", method, "id", id)
			return resp.Result, nil
		}
	}

	return nil, fmt.Errorf("%s: no response (agent exited)", method)
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) {
	c.stdin.Encode(map[string]any{ //nolint:errcheck
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}
