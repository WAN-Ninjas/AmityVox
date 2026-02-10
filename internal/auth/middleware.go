// Package auth — middleware.go provides HTTP middleware for extracting and
// validating Bearer tokens from the Authorization header, injecting the
// authenticated user ID into the request context for downstream handlers.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const (
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID contextKey = "user_id"
	// ContextKeySessionID is the context key for the current session token.
	ContextKeySessionID contextKey = "session_id"
)

// UserIDFromContext retrieves the authenticated user ID from the request context.
// Returns empty string if no user is authenticated.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyUserID).(string)
	return v
}

// SessionIDFromContext retrieves the session ID from the request context.
// Returns empty string if not present.
func SessionIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeySessionID).(string)
	return v
}

// RequireAuth returns middleware that validates the Bearer token and injects
// the authenticated user ID into the request context. Requests without a valid
// token receive a 401 Unauthorized response.
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing_token", "Authorization header with Bearer token is required")
				return
			}

			userID, err := svc.ValidateSession(r.Context(), token)
			if err != nil {
				if authErr, ok := err.(*AuthError); ok {
					writeAuthError(w, authErr.Status, authErr.Code, authErr.Message)
					return
				}
				writeAuthError(w, http.StatusInternalServerError, "internal_error", "Failed to validate session")
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, ContextKeySessionID, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that validates a Bearer token if present but
// does not require it. If a valid token is present, the user ID is injected
// into the context. If not, the request proceeds without authentication.
func OptionalAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID, err := svc.ValidateSession(r.Context(), token)
			if err == nil && userID != "" {
				ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
				ctx = context.WithValue(ctx, ContextKeySessionID, token)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts the token from "Authorization: Bearer <token>".
func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// writeAuthError writes a JSON error response matching the API error envelope
// format. This avoids importing the api package, which would create a circular
// dependency since api imports auth.
func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
