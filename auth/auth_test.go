package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/jericho/bugle"
)

func TestNoop_Authenticate(t *testing.T) {
	n := Noop{}
	id, err := n.Authenticate(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Noop.Authenticate() error: %v", err)
	}
	if id.Subject != "anonymous" {
		t.Errorf("subject = %q, want %q", id.Subject, "anonymous")
	}
}

func TestNoop_Authorize(t *testing.T) {
	n := Noop{}
	if err := n.Authorize(bugle.Identity{}, bugle.ActionStart); err != nil {
		t.Fatalf("Noop.Authorize() error: %v", err)
	}
}

func TestBearer_Authenticate(t *testing.T) {
	const envVar = "TEST_BUGLE_AUTH_TOKEN"
	t.Setenv(envVar, "secret-123")

	b := NewBearer(envVar)

	t.Run("valid token", func(t *testing.T) {
		id, err := b.Authenticate(context.Background(), "secret-123")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if id.Subject != "bearer:"+envVar {
			t.Errorf("subject = %q", id.Subject)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		_, err := b.Authenticate(context.Background(), "wrong")
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("error = %v, want ErrInvalidToken", err)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := b.Authenticate(context.Background(), "")
		if !errors.Is(err, ErrMissingToken) {
			t.Errorf("error = %v, want ErrMissingToken", err)
		}
	})
}

func TestBearer_MissingEnvVar(t *testing.T) {
	b := NewBearer("NONEXISTENT_VAR_12345")
	_, err := b.Authenticate(context.Background(), "any")
	if !errors.Is(err, ErrMissingToken) {
		t.Errorf("error = %v, want ErrMissingToken", err)
	}
}
