package security

import (
	"context"
	"net/http"
	"strings"

	"github.com/avaropoint/rmm/internal/store"
)

// AuthMiddleware validates API key authentication on HTTP requests.
type AuthMiddleware struct {
	store store.Store
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(s store.Store) *AuthMiddleware {
	return &AuthMiddleware{store: s}
}

// Wrap returns an http.HandlerFunc that requires valid API key authentication.
// The key can be provided via Authorization header or "token" query parameter.
func (a *AuthMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := extractKey(r)
		if key == "" {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		keyHash := HashAPIKey(key)
		apiKey, err := a.store.VerifyAPIKey(context.Background(), keyHash)
		if err != nil || apiKey == nil {
			http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// extractKey gets the API key from the request.
// Checks Authorization: Bearer <key> header first, then "token" query param.
func extractKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return r.URL.Query().Get("token")
}
