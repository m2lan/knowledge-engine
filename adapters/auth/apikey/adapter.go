// Package apikey provides static API-key authentication for service-to-service use.
package apikey

import (
	"crypto/subtle"
	"fmt"

	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/types"
)

// Adapter implements the types.Authenticator interface with a shared API key.
type Adapter struct {
	key    string
	userID string
}

// New creates a new API-key authenticator.
func New(key, userID string) (*Adapter, error) {
	if key == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if userID == "" {
		userID = "00000000-0000-0000-0000-000000000001"
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("api key user id must be a UUID: %w", err)
	}
	return &Adapter{key: key, userID: userID}, nil
}

func (a *Adapter) Authenticate(token string) (*types.UserInfo, error) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(a.key)) != 1 {
		return nil, fmt.Errorf("invalid api key")
	}
	return &types.UserInfo{
		ID:   a.userID,
		Role: "admin",
	}, nil
}
