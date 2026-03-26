package acp

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}

// mockACPServer is a bash script that simulates an ACP agent.
const mockACPServer = `#!/bin/bash
while IFS= read -r line; do
  method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
  id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d: -f2)

  case "$method" in
    initialize)
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"protocolVersion":1,"agentInfo":{"name":"mock","version":"0.1.0"}}}'
      ;;
    session/new)
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"sessionId":"test-session-1"}}'
      ;;
    session/prompt)
      echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello "}}}}'
      echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"world"}}}}'
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"stopReason":"end_turn"}}'
      ;;
    session/cancel)
      exit 0
      ;;
  esac
done
`

func TestNewClient_ValidAgent(t *testing.T) {
	for _, name := range []string{"cursor", "claude", "gemini", "codex", "kiro", "goose", "opencode", "cline", "auggie", "devstral", "qwen", "kimi"} {
		c, err := NewClient(name)
		if err != nil {
			t.Fatalf("NewClient(%q): %v", name, err)
		}
		if c.AgentName() != name {
			t.Fatalf("agent = %q, want %q", c.AgentName(), name)
		}
	}
}

func TestNewClient_InvalidAgent(t *testing.T) {
	_, err := NewClient("unknown")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestClient_FullLifecycle(t *testing.T) {
	c, err := NewClient("cursor", WithCommandFactory(
		func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "bash", "-c", mockACPServer)
		},
	))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Start — handshake.
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if c.SessionID() != "test-session-1" {
		t.Fatalf("sessionID = %q", c.SessionID())
	}

	// Send + Chat.
	c.Send(Message{Role: RoleUser, Content: "hello"})
	ch, err := c.Chat(ctx)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var texts []string
	var gotDone bool
	for evt := range ch {
		switch evt.Type {
		case EventText:
			texts = append(texts, evt.Text)
		case EventDone:
			gotDone = true
		case EventError:
			t.Fatalf("error: %s", evt.Error)
		}
	}

	if !gotDone {
		t.Fatal("missing done event")
	}
	if len(texts) != 2 || texts[0] != "hello " || texts[1] != "world" {
		t.Fatalf("texts = %v, want [hello , world]", texts)
	}

	// History should have user + assistant.
	msgs := c.Messages()
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	if msgs[1].Role != RoleAssistant {
		t.Fatalf("role = %q", msgs[1].Role)
	}
	if msgs[1].Content != "hello world" {
		t.Fatalf("content = %q", msgs[1].Content)
	}

	// Stop.
	c.Stop(ctx) //nolint:errcheck
}

func TestClient_ChatNoMessages(t *testing.T) {
	c, _ := NewClient("cursor")
	_, err := c.Chat(context.Background())
	if err == nil {
		t.Fatal("expected error with no messages")
	}
}

// mockACPServerBadSession simulates an ACP agent that rejects session/new.
const mockACPServerBadSession = `#!/bin/bash
while IFS= read -r line; do
  method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
  id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d: -f2)
  case "$method" in
    initialize)
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"protocolVersion":1,"agentInfo":{"name":"mock","version":"0.1.0"}}}'
      ;;
    session/new)
      echo '{"jsonrpc":"2.0","id":'$id',"error":{"code":-32603,"message":"mcpServers required"}}'
      ;;
  esac
done
`

func TestClient_SessionNewError(t *testing.T) {
	c, _ := NewClient("cursor", WithCommandFactory(
		func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "bash", "-c", mockACPServerBadSession)
		},
	))

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected session/new error")
	}
	if !strings.Contains(err.Error(), "mcpServers") {
		t.Fatalf("error = %q, should mention mcpServers", err)
	}
}

func TestClient_AgentCrashDuringChat(t *testing.T) {
	crashServer := `#!/bin/bash
while IFS= read -r line; do
  method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
  id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d: -f2)
  case "$method" in
    initialize)
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"protocolVersion":1,"agentInfo":{"name":"mock","version":"0.1.0"}}}'
      ;;
    session/new)
      echo '{"jsonrpc":"2.0","id":'$id',"result":{"sessionId":"crash-session"}}'
      ;;
    session/prompt)
      exit 1
      ;;
  esac
done
`
	c, _ := NewClient("cursor", WithCommandFactory(
		func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "bash", "-c", crashServer)
		},
	))

	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	c.Send(Message{Role: RoleUser, Content: "test"})
	ch, err := c.Chat(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var gotError bool
	for evt := range ch {
		if evt.Type == EventError {
			gotError = true
		}
	}
	if !gotError {
		t.Fatal("should get error event when agent crashes")
	}
}
