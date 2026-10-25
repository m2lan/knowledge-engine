package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/omnidev/knowledge-engine/adapters/auth/apikey"
	jwtadapter "github.com/omnidev/knowledge-engine/adapters/auth/jwt"
	"github.com/omnidev/knowledge-engine/adapters/auth/noop"
	"github.com/omnidev/knowledge-engine/types"
)

func TestJWTAuthentication(t *testing.T) {
	secret := "test-secret"
	userID := uuid.NewString()
	authenticator := jwtadapter.New(secret, "knowledge-engine")

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": userID,
		"iss":     "knowledge-engine",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	info, status := authenticate(t, authenticator, req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if info == nil || info.ID != userID {
		t.Fatalf("user info = %+v, want id %s", info, userID)
	}
}

func TestJWTRejectsNonUUIDSubject(t *testing.T) {
	secret := "test-secret"
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": "anonymous",
		"iss":     "knowledge-engine",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	_, status := authenticate(t, jwtadapter.New(secret, "knowledge-engine"), req)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestAPIKeyAuthentication(t *testing.T) {
	userID := uuid.NewString()
	authenticator, err := apikey.New("secret-key", userID)
	if err != nil {
		t.Fatalf("apikey.New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	req.Header.Set("X-API-Key", "secret-key")
	info, status := authenticate(t, authenticator, req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if info == nil || info.ID != userID {
		t.Fatalf("user info = %+v, want id %s", info, userID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	req.Header.Set("X-API-Key", "wrong")
	if _, status := authenticate(t, authenticator, req); status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestNoopAuthenticationUsesUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	info, status := authenticate(t, noop.New(), req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if _, err := uuid.Parse(info.ID); err != nil {
		t.Fatalf("noop user id %q is not a UUID: %v", info.ID, err)
	}
}

func authenticate(t *testing.T, authenticator types.Authenticator, req *http.Request) (*types.UserInfo, int) {
	t.Helper()
	var captured *types.UserInfo
	handler := Auth(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetUserInfo(r)
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return captured, recorder.Code
}
