package execution

import (
	"context"
	"fmt"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"
)

// LLMActorFunc creates an ActorFunc that calls an any-llm-go Provider.
// The provider connection persists across calls — warm, not cold.
// Each call sends the input as a user message and returns the assistant's response.
func LLMActorFunc(provider anyllm.Provider, model string) ActorFunc {
	return func(ctx context.Context, input string) (string, error) {
		resp, err := provider.Completion(ctx, anyllm.CompletionParams{
			Model: model,
			Messages: []anyllm.Message{
				{Role: "user", Content: input},
			},
		})
		if err != nil {
			return "", fmt.Errorf("llm completion: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("llm completion: no choices returned")
		}
		content, ok := resp.Choices[0].Message.Content.(string)
		if !ok {
			return "", fmt.Errorf("llm completion: content is not a string")
		}
		return content, nil
	}
}
