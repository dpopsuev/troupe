package resilience

import (
	"errors"
	"testing"
)

var (
	errAuth     = errors.New("auth failed")
	errNotFound = errors.New("not found")
	errTimeout  = errors.New("timeout")
)

func TestDefaultClassifier_FatalErrors(t *testing.T) {
	classify := DefaultClassifier(errAuth, errNotFound)

	if classify(errAuth) != Fatal {
		t.Error("errAuth should be fatal")
	}
	if classify(errNotFound) != Fatal {
		t.Error("errNotFound should be fatal")
	}
	if classify(errTimeout) != Transient {
		t.Error("errTimeout should be transient")
	}
}

func TestDefaultClassifier_WrappedErrors(t *testing.T) {
	classify := DefaultClassifier(errAuth)
	wrapped := errors.Join(errors.New("context"), errAuth)

	if classify(wrapped) != Fatal {
		t.Error("wrapped errAuth should still be fatal")
	}
}

func TestRetryPolicy_SkipsFatalErrors(t *testing.T) {
	classify := DefaultClassifier(errAuth)
	cfg := RetryPolicy(RetryConfig{MaxAttempts: 3}, classify)

	if cfg.Retryable(errTimeout) != true {
		t.Error("transient errors should be retryable")
	}
	if cfg.Retryable(errAuth) != false {
		t.Error("fatal errors should not be retryable")
	}
}
