package bugle

import "context"

// Identity represents an authenticated caller.
type Identity struct {
	Subject string            `json:"subject"`
	Claims  map[string]string `json:"claims,omitempty"`
}

// Authenticator resolves a token to an identity.
// Infrastructure-specific: bearer token, ServiceAccount, mTLS, SPIFFE.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Identity, error)
}

// Authorizer checks whether an identity may perform an action.
// Infrastructure-specific: RBAC, OPA, config-based.
type Authorizer interface {
	Authorize(identity Identity, action Action) error
}
