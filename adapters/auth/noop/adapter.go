// Package noop provides a no-op authenticator for development/testing.
package noop

import "github.com/omnidev/knowledge-engine/types"

// Adapter implements the types.Authenticator interface with no-op behavior.
type Adapter struct{}

// New creates a new no-op authenticator.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Authenticate(_ string) (*types.UserInfo, error) {
	return &types.UserInfo{
		ID:    "anonymous",
		Email: "anonymous@localhost",
		Role:  "admin",
	}, nil
}
