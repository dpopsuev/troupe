package auth

import (
	"context"
	"errors"
	"os"

	"github.com/dpopsuev/jericho/bugle"
)

// Sentinel errors.
var (
	ErrMissingToken = errors.New("auth: missing or empty token")
	ErrInvalidToken = errors.New("auth: invalid token")
)

// Bearer validates tokens against an environment variable.
// Simple shared-secret auth for Docker Compose and single-node deployments.
type Bearer struct {
	envVar string
}

// NewBearer creates a bearer authenticator that reads the expected token
// from the given environment variable.
func NewBearer(envVar string) *Bearer {
	return &Bearer{envVar: envVar}
}

// Authenticate compares the provided token against the env var value.
func (b *Bearer) Authenticate(_ context.Context, token string) (bugle.Identity, error) {
	if token == "" {
		return bugle.Identity{}, ErrMissingToken
	}
	expected := os.Getenv(b.envVar)
	if expected == "" {
		return bugle.Identity{}, ErrMissingToken
	}
	if token != expected {
		return bugle.Identity{}, ErrInvalidToken
	}
	return bugle.Identity{Subject: "bearer:" + b.envVar}, nil
}
