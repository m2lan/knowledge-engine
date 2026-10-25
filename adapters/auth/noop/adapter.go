// Package noop provides a no-op authenticator for development/testing.
package noop

import "github.com/omnidev/knowledge-engine/types"

const defaultUserID = "00000000-0000-0000-0000-000000000001"

// Adapter implements the types.Authenticator interface with no-op behavior.
type Adapter struct {
	userID string
}

// New creates a new no-op authenticator.
func New() *Adapter {
	return NewWithUserID(defaultUserID)
}

// NewWithUserID creates a no-op authenticator that attributes requests to userID.
func NewWithUserID(userID string) *Adapter {
	if userID == "" {
		userID = defaultUserID
	}
	return &Adapter{userID: userID}
}

func (a *Adapter) Authenticate(_ string) (*types.UserInfo, error) {
	return &types.UserInfo{
		ID:    a.userID,
		Email: "anonymous@localhost",
		Role:  "admin",
	}, nil
}
