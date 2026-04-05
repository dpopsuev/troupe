// resolve.go — Defense-in-depth agent resolution.
//
// Four env vars under TROUPE_AGENT_ prefix:
//   - TROUPE_AGENT_CLI:      ACP binary name (cursor, copilot, claude)
//   - TROUPE_AGENT_HTTP:     HTTP API endpoint (not used by ACP, selects HTTP driver)
//   - TROUPE_AGENT_PROVIDER: LLM backend (anthropic, openai, google)
//   - TROUPE_AGENT_MODEL:    Specific model (sonnet, opus, o3)
//
// Resolution per dimension (4 layers):
//   1. Consumer explicit (ActorConfig fields / WithDriver)
//   2. Env var
//   3. Auto-detection (PATH probe for CLI, inference for provider/model)
//   4. Hardcoded fallback
package acp

import (
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Env var names.
const (
	EnvAgentCLI      = "TROUPE_AGENT_CLI"
	EnvAgentHTTP     = "TROUPE_AGENT_HTTP"
	EnvAgentProvider = "TROUPE_AGENT_PROVIDER"
	EnvAgentModel    = "TROUPE_AGENT_MODEL"
)

// safeName matches only alphanumeric, dash, underscore, dot.
var safeName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ResolvedAgent holds the fully resolved agent configuration.
type ResolvedAgent struct {
	CLI      string // binary name (e.g., "cursor")
	HTTP     string // HTTP endpoint (e.g., "https://api.openai.com") — empty = use CLI
	Provider string // LLM backend (e.g., "anthropic")
	Model    string // specific model (e.g., "sonnet")
}

// ResolveAgent applies 4-layer resolution across all dimensions.
// explicitCLI/explicitProvider/explicitModel come from ActorConfig (Layer 1).
func ResolveAgent(explicitCLI, explicitProvider, explicitModel string) ResolvedAgent {
	r := ResolvedAgent{
		CLI:      resolveField(sanitizeCLI(explicitCLI), EnvAgentCLI, detectAgent, "cursor", sanitizeCLI),
		HTTP:     resolveHTTP(),
		Provider: resolveField(explicitProvider, EnvAgentProvider, nil, "anthropic", nil),
		Model:    resolveField(explicitModel, EnvAgentModel, nil, "sonnet", nil),
	}
	return r
}

// resolveField applies the 4-layer chain for a single dimension.
// sanitize is optional — applied to env var values (trust boundary).
func resolveField(explicit, envKey string, detect func() string, fallback string, sanitize func(string) string) string {
	// Layer 1: Consumer explicit (already sanitized by caller if needed).
	if explicit != "" {
		return explicit
	}

	// Layer 2: Env var (external input — sanitize at trust boundary).
	if v := readEnv(envKey); v != "" {
		if sanitize != nil {
			v = sanitize(v)
		}
		if v != "" {
			return v
		}
		// Sanitization rejected it — fall through.
	}

	// Layer 3: Auto-detection.
	if detect != nil {
		if v := detect(); v != "" {
			return v
		}
	}

	// Layer 4: Hardcoded fallback.
	return fallback
}

// detectAgent probes PATH for known ACP binaries in priority order.
func detectAgent() string {
	// Priority: cursor first (most common), then others.
	priority := []string{
		"cursor", "claude", "copilot", "codex", "gemini",
		"kiro", "goose", "opencode", "cline", "auggie",
	}
	for _, name := range priority {
		args, ok := AgentCommands[name]
		if !ok {
			continue
		}
		binary := resolveBinary(name, args)
		if _, err := exec.LookPath(binary); err == nil {
			return name
		}
	}
	return ""
}

// resolveBinary determines the actual executable for an agent.
// For most agents the binary IS the agent name. For special cases
// (e.g., claude → npx), use the first arg from AgentCommands.
func resolveBinary(name string, args []string) string {
	if len(args) > 0 && args[0] != name {
		return args[0] // e.g., "npx" for claude
	}
	return name
}

// resolveHTTP reads TROUPE_AGENT_HTTP and validates it.
// Returns empty string if unset or invalid.
func resolveHTTP() string {
	v := readEnv(EnvAgentHTTP)
	if v == "" {
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "" // invalid URL, treat as unset
	}
	return v
}

// readEnv reads an env var and sanitizes it.
// Returns empty string for empty or whitespace-only values.
func readEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return ""
	}
	return v
}

// sanitizeCLI validates a CLI agent name to prevent shell injection.
// Returns empty string if the name contains dangerous characters.
func sanitizeCLI(name string) string {
	if name == "" {
		return ""
	}
	if !safeName.MatchString(name) {
		return ""
	}
	return name
}

// RegisterAgent adds or updates an entry in the AgentCommands registry.
// Consumers can extend the known agent list at runtime.
func RegisterAgent(name string, args []string) {
	AgentCommands[name] = args
}
