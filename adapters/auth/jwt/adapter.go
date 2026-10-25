// Package jwt provides a JWT authenticator adapter.
package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/types"
)

// Adapter implements the knowledge.Authenticator interface using JWT.
type Adapter struct {
	secret string
	issuer string
}

// New creates a new JWT authenticator.
func New(secret, issuer string) *Adapter {
	return &Adapter{secret: secret, issuer: issuer}
}

func (a *Adapter) Authenticate(tokenStr string) (*types.UserInfo, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer if configured
	if a.issuer != "" {
		iss, _ := claims.GetIssuer()
		if iss != a.issuer {
			return nil, fmt.Errorf("invalid issuer")
		}
	}

	userID, _ := claims["user_id"].(string)
	if userID == "" {
		userID, _ = claims.GetSubject()
	}
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)
	orgID, _ := claims["org_id"].(string)

	if userID == "" {
		return nil, fmt.Errorf("missing user_id in token")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("user_id must be a UUID: %w", err)
	}

	return &types.UserInfo{
		ID:    userID,
		Email: email,
		Role:  role,
		OrgID: orgID,
	}, nil
}
