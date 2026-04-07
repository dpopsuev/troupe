package execution

import (
	"fmt"
	"os"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"
	anyllmAnthropic "github.com/mozilla-ai/any-llm-go/providers/anthropic"
	anyllmGemini "github.com/mozilla-ai/any-llm-go/providers/gemini"
	anyllmOpenAI "github.com/mozilla-ai/any-llm-go/providers/openai"
)

// Env var names for provider detection.
const (
	envUseVertex     = "CLAUDE_CODE_USE_VERTEX"
	envVertexRegion  = "CLOUD_ML_REGION"
	envVertexProject = "ANTHROPIC_VERTEX_PROJECT_ID"
	envAnthropicKey  = "ANTHROPIC_API_KEY"
	envOpenAIKey     = "OPENAI_API_KEY"
	envGeminiKey     = "GEMINI_API_KEY"
)

// NewProviderFromEnv detects available LLM providers from environment
// variables and returns the best available one.
//
// Priority: Anthropic direct > OpenAI > Gemini.
// Vertex AI support requires upstream any-llm-go changes or a direct
// anthropic-sdk-go integration (see TRP-TSK-35).
func NewProviderFromEnv() (anyllm.Provider, error) {
	if os.Getenv(envUseVertex) == "1" {
		// Vertex is available but any-llm-go's Anthropic provider
		// doesn't support it yet. Fall through to other providers.
		// TODO: Wire vertex.WithGoogleAuth once any-llm-go supports
		// custom client options, or implement a direct vertex provider.
	}

	if os.Getenv(envAnthropicKey) != "" {
		return anyllmAnthropic.New()
	}

	if os.Getenv(envOpenAIKey) != "" {
		return anyllmOpenAI.New()
	}

	if os.Getenv(envGeminiKey) != "" {
		return anyllmGemini.New()
	}

	return nil, fmt.Errorf("no LLM provider found: set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
}

// NewProviderByName creates a provider by explicit name.
func NewProviderByName(name string) (anyllm.Provider, error) {
	switch name {
	case "anthropic", "claude":
		return anyllmAnthropic.New()
	case "openai", "gpt":
		return anyllmOpenAI.New()
	case "gemini":
		return anyllmGemini.New()
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
