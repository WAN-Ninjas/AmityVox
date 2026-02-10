package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid simple", "alice", false},
		{"valid with numbers", "alice123", false},
		{"valid with dots", "alice.bob", false},
		{"valid with underscores", "alice_bob", false},
		{"valid with hyphens", "alice-bob", false},
		{"valid min length", "ab", false},
		{"valid max length", "abcdefghijklmnopqrstuvwxyz123456", false},
		{"too short", "a", true},
		{"empty", "", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567", true}, // 33 chars
		{"has spaces", "alice bob", true},
		{"has special chars", "alice@bob", true},
		{"has emoji", "alice😀", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUsername(tc.username)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateUsername(%q) error = %v, wantErr = %v", tc.username, err, tc.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid 8 chars", "12345678", false},
		{"valid long", "a very long and secure password indeed!", false},
		{"too short", "1234567", true},
		{"empty", "", true},
		{"exactly 128 chars", string(make([]byte, 128)), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePassword(tc.password)
			if (err != nil) != tc.wantErr {
				t.Errorf("validatePassword(len=%d) error = %v, wantErr = %v", len(tc.password), err, tc.wantErr)
			}
		})
	}
}

func TestValidatePassword_TooLong(t *testing.T) {
	// 129 runes should be too long.
	runes := make([]rune, 129)
	for i := range runes {
		runes[i] = 'a'
	}
	err := validatePassword(string(runes))
	if err == nil {
		t.Error("expected error for password > 128 chars")
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"case insensitive", "bearer abc123", "abc123"},
		{"BEARER", "BEARER abc123", "abc123"},
		{"with spaces in token", "Bearer  abc123 ", "abc123"},
		{"empty", "", ""},
		{"no bearer prefix", "Token abc123", ""},
		{"bearer only", "Bearer", ""},
		{"basic auth", "Basic dXNlcjpwYXNz", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			got := extractBearerToken(req)
			if got != tc.want {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestUserIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyUserID, "user123")
	if got := UserIDFromContext(ctx); got != "user123" {
		t.Errorf("UserIDFromContext = %q, want %q", got, "user123")
	}

	// Empty context.
	if got := UserIDFromContext(context.Background()); got != "" {
		t.Errorf("UserIDFromContext(empty) = %q, want empty", got)
	}
}

func TestSessionIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeySessionID, "sess456")
	if got := SessionIDFromContext(ctx); got != "sess456" {
		t.Errorf("SessionIDFromContext = %q, want %q", got, "sess456")
	}

	if got := SessionIDFromContext(context.Background()); got != "" {
		t.Errorf("SessionIDFromContext(empty) = %q, want empty", got)
	}
}

func TestWriteAuthError(t *testing.T) {
	w := httptest.NewRecorder()
	writeAuthError(w, http.StatusUnauthorized, "test_code", "test message")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAuthError_Error(t *testing.T) {
	err := &AuthError{Code: "test", Message: "test message", Status: 401}
	if got := err.Error(); got != "test message" {
		t.Errorf("Error() = %q, want %q", got, "test message")
	}
}
