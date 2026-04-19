package providers

import "github.com/dpopsuev/troupe/resilience"

// LLMClassifier returns an ErrorClassifier that knows provider error semantics.
// Rate limits and transient failures are retryable. Auth, model not found,
// and config errors are fatal.
func LLMClassifier() resilience.ErrorClassifier {
	return resilience.DefaultClassifier(
		ErrAuthFailed,
		ErrCredentialsMissing,
		ErrModelNotFound,
		ErrModelRequired,
		ErrProviderNotSet,
		ErrProviderUnknown,
		ErrStreamingNotSupported,
	)
}
