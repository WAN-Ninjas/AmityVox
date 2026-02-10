package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/presence"
)

func TestIsAuthEndpoint(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"/api/v1/auth/login", true},
		{"/api/v1/auth/register", true},
		{"/api/v1/auth/logout", false},
		{"/api/v1/users/@me", false},
		{"/api/v1/guilds/123", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if got := isAuthEndpoint(req); got != tc.expect {
				t.Errorf("isAuthEndpoint(%q) = %v, want %v", tc.path, got, tc.expect)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	// With X-Forwarded-For.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.RemoteAddr = "10.0.0.1:12345"
	if got := clientIP(req); got != "1.2.3.4" {
		t.Errorf("clientIP with XFF = %q, want %q", got, "1.2.3.4")
	}

	// Without X-Forwarded-For.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	if got := clientIP(req2); got != "10.0.0.1:12345" {
		t.Errorf("clientIP without XFF = %q, want %q", got, "10.0.0.1:12345")
	}
}

func TestWriteRateLimitResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeRateLimitResponse(w, globalRateWindow)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if ra := w.Header().Get("Retry-After"); ra == "" {
		t.Error("Retry-After header should be set")
	}
}

func TestSetRateLimitHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	result := presence.RateLimitResult{
		Allowed:   true,
		Limit:     120,
		Remaining: 100,
		Count:     20,
	}
	setRateLimitHeaders(w, result, globalRateWindow)

	if v := w.Header().Get("X-RateLimit-Limit"); v != "120" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", v, "120")
	}
	if v := w.Header().Get("X-RateLimit-Remaining"); v != "100" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", v, "100")
	}
	if v := w.Header().Get("X-RateLimit-Reset"); v == "" {
		t.Error("X-RateLimit-Reset should be set")
	}
}

func TestRateLimitMiddleware_NoCache(t *testing.T) {
	// When Cache is nil, middleware should pass through.
	s := &Server{Cache: nil}
	mw := s.rateLimitMiddleware()

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when cache is nil")
	}
}

func TestRateLimitMessages_NoCache(t *testing.T) {
	s := &Server{Cache: nil}

	called := false
	handler := s.RateLimitMessages(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when cache is nil")
	}
}

func TestRateLimitSearch_NoCache(t *testing.T) {
	s := &Server{Cache: nil}

	called := false
	handler := s.RateLimitSearch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when cache is nil")
	}
}
