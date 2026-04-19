package resilience

import "errors"

// ErrorClass categorizes an error for retry/fallback decisions.
type ErrorClass int

const (
	// Transient errors are retryable (rate limit, timeout, temporary network).
	Transient ErrorClass = iota
	// Fatal errors should not be retried (auth failure, model not found, bad request).
	Fatal
)

// ErrorClassifier maps an error to a class. Used by RetryPolicy to
// decide whether to retry, fallback, or fail fast.
type ErrorClassifier func(err error) ErrorClass

// DefaultClassifier classifies errors based on common sentinel patterns.
// Context errors and unknown errors are transient (retry). Auth and
// config errors are fatal (fail fast).
func DefaultClassifier(fatalErrors ...error) ErrorClassifier {
	return func(err error) ErrorClass {
		for _, fatal := range fatalErrors {
			if errors.Is(err, fatal) {
				return Fatal
			}
		}
		return Transient
	}
}

// RetryPolicy combines a RetryConfig with an ErrorClassifier.
// Retries transient errors, fails fast on fatal errors.
func RetryPolicy(cfg RetryConfig, classify ErrorClassifier) RetryConfig {
	cfg.Retryable = func(err error) bool {
		if classify == nil {
			return true
		}
		return classify(err) == Transient
	}
	return cfg
}
