package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/presence"
)

// Rate limit tiers for different endpoint categories.
const (
	// Authenticated user global rate limit: 6000 requests per minute.
	// Users clicking through settings/menus trigger many parallel API calls,
	// so this needs to be generous.
	authedRateLimit  = 6000
	authedRateWindow = 1 * time.Minute

	// Unauthenticated global rate limit: 1200 requests per minute per IP.
	// Lower than authed to discourage scraping while still allowing browsing.
	unauthRateLimit  = 1200
	unauthRateWindow = 1 * time.Minute

	// Auth rate limit: 100 requests per minute per IP (login/register).
	// Kept strict to protect against credential brute-force attacks.
	authRateLimit  = 100
	authRateWindow = 1 * time.Minute

	// Message creation: 100 messages per 10 seconds per user.
	messageRateLimit  = 100
	messageRateWindow = 10 * time.Second

	// Search: 300 queries per minute per user.
	searchRateLimit  = 300
	searchRateWindow = 1 * time.Minute

	// Webhook execution: 300 calls per minute per webhook.
	webhookRateLimit  = 300
	webhookRateWindow = 1 * time.Minute
)

// RateLimitGlobal returns middleware that enforces rate limits using
// DragonflyDB/Redis. It applies a global rate limit per user (or IP for
// unauthenticated requests) and tighter limits for specific endpoint categories.
// Must be applied AFTER auth middleware on authenticated routes so that
// auth.UserIDFromContext returns the authenticated user ID.
func (s *Server) RateLimitGlobal() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.Cache == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Use different rate limits for authenticated vs unauthenticated requests.
			userID := auth.UserIDFromContext(r.Context())
			var key string
			var limit int
			var window time.Duration

			if userID != "" {
				key = "global:" + userID
				limit = authedRateLimit
				window = authedRateWindow
			} else {
				key = "global:" + clientIP(r)
				limit = unauthRateLimit
				window = unauthRateWindow
			}

			result, err := s.Cache.CheckRateLimitInfo(r.Context(), key, limit, window)
			if err != nil {
				s.Logger.Debug("rate limit check failed", slog.String("error", err.Error()))
				next.ServeHTTP(w, r)
				return
			}
			setRateLimitHeaders(w, result, window)
			if !result.Allowed {
				// Log rate-limited request to database for admin dashboard stats.
				if s.DB != nil {
					ip := clientIP(r)
					endpoint := r.URL.Path
					used := result.Limit - result.Remaining
					windowStart := time.Now().Add(-window)
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						defer cancel()
						id := models.NewULID().String()
						if _, err := s.DB.Pool.Exec(ctx,
							`INSERT INTO rate_limit_log (id, ip_address, endpoint, requests_count, window_start, blocked, created_at)
							 VALUES ($1, $2, $3, $4, $5, true, now())`,
							id, ip, endpoint, used, windowStart); err != nil {
							s.Logger.Debug("rate limit log insert failed", slog.String("error", err.Error()))
						}
					}()
				}
				writeRateLimitResponse(w, window)
				return
			}

			// Check endpoint-specific rate limits.
			if isAuthEndpoint(r) {
				ip := clientIP(r)
				authResult, err := s.Cache.CheckRateLimitInfo(r.Context(), "auth:"+ip, authRateLimit, authRateWindow)
				if err == nil && !authResult.Allowed {
					setRateLimitHeaders(w, authResult, authRateWindow)
					writeRateLimitResponse(w, authRateWindow)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMessages is middleware for the message creation endpoint with
// tighter rate limits. Apply this to POST /channels/{id}/messages.
func (s *Server) RateLimitMessages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cache == nil {
			next.ServeHTTP(w, r)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if userID == "" {
			next.ServeHTTP(w, r)
			return
		}

		result, err := s.Cache.CheckRateLimitInfo(r.Context(), "msg:"+userID, messageRateLimit, messageRateWindow)
		if err != nil {
			s.Logger.Debug("message rate limit check failed", slog.String("error", err.Error()))
			next.ServeHTTP(w, r)
			return
		}
		setRateLimitHeaders(w, result, messageRateWindow)
		if !result.Allowed {
			writeRateLimitResponse(w, messageRateWindow)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitSearch is middleware for search endpoints with moderate rate limits.
func (s *Server) RateLimitSearch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cache == nil {
			next.ServeHTTP(w, r)
			return
		}

		userID := auth.UserIDFromContext(r.Context())
		if userID == "" {
			next.ServeHTTP(w, r)
			return
		}

		result, err := s.Cache.CheckRateLimitInfo(r.Context(), "search:"+userID, searchRateLimit, searchRateWindow)
		if err != nil {
			s.Logger.Debug("search rate limit check failed", slog.String("error", err.Error()))
			next.ServeHTTP(w, r)
			return
		}
		setRateLimitHeaders(w, result, searchRateWindow)
		if !result.Allowed {
			writeRateLimitResponse(w, searchRateWindow)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitWebhooks is middleware for webhook execution with per-webhook rate limits.
func (s *Server) RateLimitWebhooks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cache == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Use the webhook ID from URL path as the rate limit key.
		key := "webhook:" + r.URL.Path
		result, err := s.Cache.CheckRateLimitInfo(r.Context(), key, webhookRateLimit, webhookRateWindow)
		if err != nil {
			s.Logger.Debug("webhook rate limit check failed", slog.String("error", err.Error()))
			next.ServeHTTP(w, r)
			return
		}
		setRateLimitHeaders(w, result, webhookRateWindow)
		if !result.Allowed {
			writeRateLimitResponse(w, webhookRateWindow)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setRateLimitHeaders sets X-RateLimit-* headers on every response so clients
// can track their remaining quota proactively.
func setRateLimitHeaders(w http.ResponseWriter, result presence.RateLimitResult, window time.Duration) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(window).Unix()))
}

// writeRateLimitResponse sends a 429 Too Many Requests response with
// standard rate limit headers.
func writeRateLimitResponse(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	WriteError(w, http.StatusTooManyRequests, "rate_limited", "You are being rate limited. Please try again later.")
}

// isAuthEndpoint returns true if the request targets an auth endpoint.
func isAuthEndpoint(r *http.Request) bool {
	path := r.URL.Path
	return path == "/api/v1/auth/login" ||
		path == "/api/v1/auth/register"
}

// clientIP extracts the client IP from the request. Chi's RealIP middleware
// already sets r.RemoteAddr from trusted proxy headers, so we just strip the
// port from RemoteAddr. We do NOT re-parse X-Forwarded-For here to avoid
// trusting arbitrary client-supplied headers.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
