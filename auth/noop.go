// Package auth provides Authenticator and Authorizer adapters for the
// Bugle Protocol. Consumers select the adapter matching their infrastructure.
package auth

import (
	"context"

	"github.com/dpopsuev/jericho/bugle"
)

// Noop allows all requests. Use for development and testing.
type Noop struct{}

// Authenticate always succeeds with a generic identity.
func (Noop) Authenticate(_ context.Context, token string) (bugle.Identity, error) {
	return bugle.Identity{Subject: "anonymous"}, nil
}

// Authorize always allows.
func (Noop) Authorize(_ bugle.Identity, _ bugle.Action) error {
	return nil
}
