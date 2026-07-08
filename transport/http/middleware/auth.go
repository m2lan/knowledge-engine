// Package middleware provides HTTP middleware for the Knowledge Engine.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/omnidev/knowledge-engine/types"
)

type contextKey string

const UserInfoKey contextKey = "user_info"

// Auth returns a middleware that authenticates requests using the Authenticator interface.
func Auth(authenticator types.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				ctx := context.WithValue(r.Context(), UserInfoKey, &types.UserInfo{
					ID:   "anonymous",
					Role: "admin",
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"error":{"code":401,"message":"unauthorized"}}`, http.StatusUnauthorized)
				return
			}

			userInfo, err := authenticator.Authenticate(token)
			if err != nil {
				http.Error(w, `{"error":{"code":401,"message":"unauthorized"}}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserInfoKey, userInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserInfo extracts the authenticated user from the request context.
func GetUserInfo(r *http.Request) *types.UserInfo {
	if info, ok := r.Context().Value(UserInfoKey).(*types.UserInfo); ok {
		return info
	}
	return nil
}

func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	return ""
}
