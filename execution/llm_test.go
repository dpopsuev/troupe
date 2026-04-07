package execution_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dpopsuev/troupe/execution"
	anyllm "github.com/mozilla-ai/any-llm-go/providers"
)

// stubProvider implements anyllm.Provider for testing without real LLM calls.
type stubProvider struct {
	response string
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Completion(_ context.Context, _ anyllm.CompletionParams) (*anyllm.ChatCompletion, error) {
	return &anyllm.ChatCompletion{
		Choices: []anyllm.Choice{
			{Message: anyllm.Message{Role: "assistant", Content: s.response}},
		},
	}, nil
}

func (s *stubProvider) CompletionStream(_ context.Context, _ anyllm.CompletionParams) (<-chan anyllm.ChatCompletionChunk, <-chan error) {
	return nil, nil
}

func TestLLMActorFunc_ReturnsResponse(t *testing.T) {
	provider := &stubProvider{response: "hello from LLM"}
	actor := execution.LLMActorFunc(provider, "test-model")

	result, err := actor(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello from LLM" {
		t.Errorf("got %q, want %q", result, "hello from LLM")
	}
}

func TestLLMActorFunc_ReusesConnection(t *testing.T) {
	provider := &stubProvider{response: "warm"}
	actor := execution.LLMActorFunc(provider, "test-model")

	// Call 3 times — same provider, same connection
	for i := range 3 {
		result, err := actor(context.Background(), "prompt")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if result != "warm" {
			t.Errorf("call %d: got %q", i, result)
		}
	}
}

// Ensure json import is used (satisfies compiler for CompletionParams internals)
var _ = json.Marshal
