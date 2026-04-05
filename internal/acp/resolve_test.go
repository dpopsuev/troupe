package acp

import (
	"context"
	"os/exec"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════
// Unit Tests — each layer in isolation
// ═══════════════════════════════════════════════════════════════════════

func TestResolveAgent_ExplicitWins(t *testing.T) {
	// Layer 1: explicit overrides everything.
	t.Setenv(EnvAgentCLI, "copilot")
	r := ResolveAgent("claude", "openai", "opus")
	if r.CLI != "claude" {
		t.Errorf("CLI = %q, want claude (explicit)", r.CLI)
	}
	if r.Provider != "openai" {
		t.Errorf("Provider = %q, want openai (explicit)", r.Provider)
	}
	if r.Model != "opus" {
		t.Errorf("Model = %q, want opus (explicit)", r.Model)
	}
}

func TestResolveAgent_EnvVar_CLI(t *testing.T) {
	// Layer 2: env var when no explicit.
	t.Setenv(EnvAgentCLI, "copilot")
	r := ResolveAgent("", "", "")
	if r.CLI != "copilot" {
		t.Errorf("CLI = %q, want copilot (env)", r.CLI)
	}
}

func TestResolveAgent_EnvVar_Provider(t *testing.T) {
	t.Setenv(EnvAgentProvider, "google")
	r := ResolveAgent("", "", "")
	if r.Provider != "google" {
		t.Errorf("Provider = %q, want google (env)", r.Provider)
	}
}

func TestResolveAgent_EnvVar_Model(t *testing.T) {
	t.Setenv(EnvAgentModel, "o3")
	r := ResolveAgent("", "", "")
	if r.Model != "o3" {
		t.Errorf("Model = %q, want o3 (env)", r.Model)
	}
}

func TestResolveAgent_DefaultFallback(t *testing.T) {
	// Layer 4: no explicit, no env, no detection → defaults.
	r := ResolveAgent("", "", "")
	// CLI falls to detectAgent or "cursor" fallback.
	if r.CLI == "" {
		t.Error("CLI should not be empty")
	}
	if r.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic (fallback)", r.Provider)
	}
	if r.Model != "sonnet" {
		t.Errorf("Model = %q, want sonnet (fallback)", r.Model)
	}
}

func TestResolveAgent_LayerPriority(t *testing.T) {
	// Layer 1 beats Layer 2.
	t.Setenv(EnvAgentProvider, "google")
	r := ResolveAgent("", "openai", "")
	if r.Provider != "openai" {
		t.Errorf("Provider = %q, want openai (explicit beats env)", r.Provider)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// HTTP resolution
// ═══════════════════════════════════════════════════════════════════════

func TestResolveHTTP_ValidURL(t *testing.T) {
	t.Setenv(EnvAgentHTTP, "https://api.openai.com")
	r := ResolveAgent("", "", "")
	if r.HTTP != "https://api.openai.com" {
		t.Errorf("HTTP = %q, want https://api.openai.com", r.HTTP)
	}
}

func TestResolveHTTP_EmptyWhenUnset(t *testing.T) {
	r := ResolveAgent("", "", "")
	if r.HTTP != "" {
		t.Errorf("HTTP = %q, want empty (unset)", r.HTTP)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Security Tests — trust boundary: env vars are external input
// ═══════════════════════════════════════════════════════════════════════

func TestSanitizeCLI_ValidNames(t *testing.T) {
	valid := []string{"cursor", "claude", "copilot-cli", "agent_v2", "my.agent"}
	for _, name := range valid {
		if s := sanitizeCLI(name); s != name {
			t.Errorf("sanitizeCLI(%q) = %q, want %q", name, s, name)
		}
	}
}

func TestSanitizeCLI_MalformedNames(t *testing.T) {
	// Shell injection attempts must be rejected.
	malicious := []string{
		"; rm -rf /",
		"$(whoami)",
		"`id`",
		"agent && cat /etc/passwd",
		"agent | grep secret",
		"agent\nnewline",
		"agent name with spaces",
	}
	for _, name := range malicious {
		if s := sanitizeCLI(name); s != "" {
			t.Errorf("sanitizeCLI(%q) = %q, want empty (rejected)", name, s)
		}
	}
}

func TestSanitizeCLI_Empty(t *testing.T) {
	if s := sanitizeCLI(""); s != "" {
		t.Errorf("sanitizeCLI(\"\") = %q, want empty", s)
	}
}

func TestResolveAgent_MalformedEnvVar(t *testing.T) {
	// Malicious CLI name in env var should be sanitized.
	t.Setenv(EnvAgentCLI, "; rm -rf /")
	r := ResolveAgent("", "", "")
	// Should fall through to detection/fallback, NOT use the malicious value.
	if r.CLI == "; rm -rf /" {
		t.Fatal("malicious CLI name was not sanitized")
	}
}

func TestResolveAgent_EmptyEnvVar(t *testing.T) {
	// Empty env var treated as unset.
	t.Setenv(EnvAgentCLI, "")
	t.Setenv(EnvAgentProvider, "   ") // whitespace-only
	r := ResolveAgent("", "", "")
	if r.Provider == "   " {
		t.Error("whitespace-only env var was not treated as unset")
	}
}

func TestResolveHTTP_InvalidURL(t *testing.T) {
	invalid := []string{
		"not-a-url",
		"ftp://files.example.com",
		"//missing-scheme",
		"",
	}
	for _, v := range invalid {
		t.Setenv(EnvAgentHTTP, v)
		r := ResolveAgent("", "", "")
		if r.HTTP != "" {
			t.Errorf("HTTP = %q for invalid input %q, want empty", r.HTTP, v)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Convention fallback — unknown agent tries <name> --acp
// ═══════════════════════════════════════════════════════════════════════

func TestNewClient_ConventionFallback(t *testing.T) {
	// An agent not in AgentCommands should get convention args.
	c, err := NewClient("futureagent", WithCommandFactory(noopFactory))
	if err != nil {
		t.Fatalf("NewClient(futureagent): %v", err)
	}
	if c.agentCmd != "futureagent" {
		t.Errorf("cmd = %q, want futureagent", c.agentCmd)
	}
	if len(c.agentArgs) != 1 || c.agentArgs[0] != "--acp" {
		t.Errorf("args = %v, want [--acp]", c.agentArgs)
	}
}

func TestNewClient_RejectsUnsafeName(t *testing.T) {
	_, err := NewClient("; rm -rf /")
	if err == nil {
		t.Fatal("expected error for unsafe agent name")
	}
}

func TestRegisterAgent_Extensible(t *testing.T) {
	RegisterAgent("myagent", []string{"myagent", "serve", "--acp"})
	c, err := NewClient("myagent", WithCommandFactory(noopFactory))
	if err != nil {
		t.Fatalf("NewClient(myagent): %v", err)
	}
	if c.agentCmd != "myagent" {
		t.Errorf("cmd = %q, want myagent", c.agentCmd)
	}
	// Cleanup.
	delete(AgentCommands, "myagent")
}

// noopFactory prevents actual process execution in tests.
func noopFactory(_ context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.Command("true")
}
