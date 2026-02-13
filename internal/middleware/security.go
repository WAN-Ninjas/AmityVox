package middleware

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- Session Security: Concurrent Session Detection ---

// SessionSecurityConfig controls session anomaly detection behavior.
type SessionSecurityConfig struct {
	// Enabled controls whether session security checks are active.
	Enabled bool `toml:"enabled"`

	// MaxConcurrentSessions is the maximum number of active sessions per user
	// from different IP subnets before triggering an alert.
	MaxConcurrentSessions int `toml:"max_concurrent_sessions"`

	// AlertOnNewLocation triggers a notification when a login occurs from a
	// previously unseen IP subnet for the user.
	AlertOnNewLocation bool `toml:"alert_on_new_location"`

	// SubnetMaskIPv4 is the CIDR prefix length for grouping IPv4 addresses.
	// /24 groups addresses in the same 255.255.255.0 block.
	SubnetMaskIPv4 int `toml:"subnet_mask_ipv4"`

	// SubnetMaskIPv6 is the CIDR prefix length for grouping IPv6 addresses.
	// /48 groups addresses in the same site allocation.
	SubnetMaskIPv6 int `toml:"subnet_mask_ipv6"`
}

// DefaultSessionSecurityConfig returns sensible defaults for session security.
func DefaultSessionSecurityConfig() SessionSecurityConfig {
	return SessionSecurityConfig{
		Enabled:               true,
		MaxConcurrentSessions: 5,
		AlertOnNewLocation:    true,
		SubnetMaskIPv4:        24,
		SubnetMaskIPv6:        48,
	}
}

// SessionInfo holds metadata about a user session for security analysis.
type SessionInfo struct {
	SessionID string
	UserID    string
	IPAddress string
	Subnet    string
	UserAgent string
	CreatedAt time.Time
}

// NormalizeIPSubnet extracts the network subnet from an IP address for
// geolocation-approximate grouping. Uses /24 for IPv4 and /48 for IPv6.
func NormalizeIPSubnet(ipStr string, ipv4Mask, ipv6Mask int) string {
	// Strip port if present.
	host, _, err := net.SplitHostPort(ipStr)
	if err != nil {
		host = ipStr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}

	if ip4 := ip.To4(); ip4 != nil {
		mask := net.CIDRMask(ipv4Mask, 32)
		network := ip4.Mask(mask)
		return fmt.Sprintf("%s/%d", network.String(), ipv4Mask)
	}

	mask := net.CIDRMask(ipv6Mask, 128)
	network := ip.Mask(mask)
	return fmt.Sprintf("%s/%d", network.String(), ipv6Mask)
}

// --- Password Breach Checking (HaveIBeenPwned k-Anonymity) ---

// BreachCheckConfig controls password breach detection.
type BreachCheckConfig struct {
	// Enabled controls whether breach checks are performed on registration/password change.
	Enabled bool `toml:"enabled"`

	// APIURL is the HaveIBeenPwned API endpoint. Defaults to the public API.
	APIURL string `toml:"api_url"`

	// Timeout is the maximum time to wait for the HIBP API response.
	Timeout time.Duration `toml:"timeout"`

	// MinBreachCount is the minimum number of breaches before blocking a password.
	// Setting this to 1 blocks any previously breached password.
	MinBreachCount int `toml:"min_breach_count"`
}

// DefaultBreachCheckConfig returns sensible defaults for password breach checking.
func DefaultBreachCheckConfig() BreachCheckConfig {
	return BreachCheckConfig{
		Enabled:        true,
		APIURL:         "https://api.pwnedpasswords.com/range/",
		Timeout:        5 * time.Second,
		MinBreachCount: 1,
	}
}

// BreachChecker checks passwords against the HaveIBeenPwned API using the
// k-anonymity model. Only the first 5 characters of the SHA-1 hash are sent
// to the API, preserving password privacy.
type BreachChecker struct {
	config     BreachCheckConfig
	httpClient *http.Client
	logger     *slog.Logger
}

// NewBreachChecker creates a new password breach checker with the given configuration.
func NewBreachChecker(cfg BreachCheckConfig, logger *slog.Logger) *BreachChecker {
	return &BreachChecker{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// IsBreached checks whether the given password appears in known data breaches.
// It uses the k-anonymity model: only the first 5 hex characters of the SHA-1
// hash are sent to the API. The full hash is compared locally against the
// returned suffix list. Returns the breach count and any error.
func (bc *BreachChecker) IsBreached(ctx context.Context, password string) (int, error) {
	if !bc.config.Enabled {
		return 0, nil
	}

	// SHA-1 is required by the HaveIBeenPwned k-anonymity API protocol.
	// This is NOT used for password storage (Argon2id handles that).
	// Only the first 5 hex chars of the SHA-1 hash are sent to the API;
	// the full hash is compared locally against the returned suffix list.
	hash := sha1.New()                 //nolint:gosec // HIBP protocol requires SHA-1
	hash.Write([]byte(password))       // codeql[go/weak-sensitive-data-hashing]: Required by HIBP k-anonymity protocol
	hashHex := strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))

	prefix := hashHex[:5]
	suffix := hashHex[5:]

	// Query the HIBP API with the prefix.
	url := bc.config.APIURL + prefix
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("creating HIBP request: %w", err)
	}
	req.Header.Set("User-Agent", "AmityVox-PasswordCheck/1.0")
	req.Header.Set("Add-Padding", "true") // Request padding to prevent response-length analysis.

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		// Network errors should not block registration — log and allow.
		bc.logger.Warn("HIBP API request failed, allowing password",
			slog.String("error", err.Error()),
		)
		return 0, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bc.logger.Warn("HIBP API returned non-200 status",
			slog.Int("status", resp.StatusCode),
		)
		return 0, nil
	}

	// Read response body (limit to 1MB for safety).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("reading HIBP response: %w", err)
	}

	// Parse the response: each line is "SUFFIX:COUNT".
	lines := strings.Split(string(body), "\r\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == suffix {
			var count int
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &count)
			if count >= bc.config.MinBreachCount {
				return count, nil
			}
		}
	}

	return 0, nil
}

// --- Enhanced Rate Limiting (Sliding Window) ---

// SlidingWindowConfig configures the sliding window rate limiter.
type SlidingWindowConfig struct {
	// WindowSize is the duration of the sliding window.
	WindowSize time.Duration

	// MaxRequests is the maximum number of requests allowed within the window.
	MaxRequests int

	// PerEndpoint enables per-endpoint rate limiting. When false, all endpoints
	// share a single rate limit per IP.
	PerEndpoint bool

	// CleanupInterval controls how often expired entries are purged.
	CleanupInterval time.Duration
}

// DefaultSlidingWindowConfig returns sensible defaults for the sliding window rate limiter.
func DefaultSlidingWindowConfig() SlidingWindowConfig {
	return SlidingWindowConfig{
		WindowSize:      time.Minute,
		MaxRequests:     60,
		PerEndpoint:     true,
		CleanupInterval: 5 * time.Minute,
	}
}

// EndpointRateConfig defines per-endpoint rate limit overrides.
type EndpointRateConfig struct {
	Pattern     string
	MaxRequests int
	WindowSize  time.Duration
}

// DefaultEndpointRates returns per-endpoint rate limit configurations for
// endpoints that need different limits than the global default.
func DefaultEndpointRates() []EndpointRateConfig {
	return []EndpointRateConfig{
		{Pattern: "/api/v1/auth/login", MaxRequests: 5, WindowSize: time.Minute},
		{Pattern: "/api/v1/auth/register", MaxRequests: 3, WindowSize: time.Minute},
		{Pattern: "/api/v1/files/upload", MaxRequests: 10, WindowSize: time.Minute},
		{Pattern: "/api/v1/search/", MaxRequests: 20, WindowSize: time.Minute},
		{Pattern: "/api/v1/channels/*/messages", MaxRequests: 30, WindowSize: time.Minute},
		{Pattern: "/api/v1/auth/totp/", MaxRequests: 5, WindowSize: 5 * time.Minute},
	}
}

// slidingWindowEntry tracks request timestamps for a single client+endpoint pair.
type slidingWindowEntry struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// SlidingWindowLimiter implements a per-IP sliding window rate limiter that
// supports per-endpoint overrides and automatic cleanup of expired entries.
type SlidingWindowLimiter struct {
	config    SlidingWindowConfig
	endpoints []EndpointRateConfig
	entries   sync.Map // map[string]*slidingWindowEntry
	logger    *slog.Logger
	stopCh    chan struct{}
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter.
func NewSlidingWindowLimiter(cfg SlidingWindowConfig, endpoints []EndpointRateConfig, logger *slog.Logger) *SlidingWindowLimiter {
	l := &SlidingWindowLimiter{
		config:    cfg,
		endpoints: endpoints,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow checks whether a request from the given IP to the given path should be
// allowed. Returns true if the request is within rate limits, false if it should
// be rejected.
func (l *SlidingWindowLimiter) Allow(ip, path string) bool {
	maxReqs, window := l.getLimits(path)
	key := l.buildKey(ip, path)
	now := time.Now()

	val, _ := l.entries.LoadOrStore(key, &slidingWindowEntry{})
	entry := val.(*slidingWindowEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Remove expired timestamps.
	cutoff := now.Add(-window)
	valid := entry.timestamps[:0]
	for _, ts := range entry.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	entry.timestamps = valid

	if len(entry.timestamps) >= maxReqs {
		return false
	}

	entry.timestamps = append(entry.timestamps, now)
	return true
}

// RemainingRequests returns how many requests the client has left in the current window.
func (l *SlidingWindowLimiter) RemainingRequests(ip, path string) int {
	maxReqs, window := l.getLimits(path)
	key := l.buildKey(ip, path)
	now := time.Now()

	val, ok := l.entries.Load(key)
	if !ok {
		return maxReqs
	}

	entry := val.(*slidingWindowEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	cutoff := now.Add(-window)
	count := 0
	for _, ts := range entry.timestamps {
		if ts.After(cutoff) {
			count++
		}
	}

	remaining := maxReqs - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RetryAfter returns the number of seconds until the client can make another request.
// Returns 0 if the client is not rate limited.
func (l *SlidingWindowLimiter) RetryAfter(ip, path string) int {
	_, window := l.getLimits(path)
	key := l.buildKey(ip, path)
	now := time.Now()

	val, ok := l.entries.Load(key)
	if !ok {
		return 0
	}

	entry := val.(*slidingWindowEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if len(entry.timestamps) == 0 {
		return 0
	}

	oldest := entry.timestamps[0]
	expiresAt := oldest.Add(window)
	if expiresAt.After(now) {
		return int(math.Ceil(expiresAt.Sub(now).Seconds()))
	}
	return 0
}

// getLimits returns the rate limit and window for the given path, checking
// per-endpoint overrides first.
func (l *SlidingWindowLimiter) getLimits(path string) (int, time.Duration) {
	for _, ep := range l.endpoints {
		if matchEndpointPattern(ep.Pattern, path) {
			return ep.MaxRequests, ep.WindowSize
		}
	}
	return l.config.MaxRequests, l.config.WindowSize
}

// buildKey creates a cache key from IP and path.
func (l *SlidingWindowLimiter) buildKey(ip, path string) string {
	if l.config.PerEndpoint {
		return ip + ":" + path
	}
	return ip
}

// cleanup periodically removes expired entries from the rate limiter.
func (l *SlidingWindowLimiter) cleanup() {
	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			l.entries.Range(func(key, val interface{}) bool {
				entry := val.(*slidingWindowEntry)
				entry.mu.Lock()
				cutoff := now.Add(-l.config.WindowSize)
				valid := entry.timestamps[:0]
				for _, ts := range entry.timestamps {
					if ts.After(cutoff) {
						valid = append(valid, ts)
					}
				}
				entry.timestamps = valid
				empty := len(entry.timestamps) == 0
				entry.mu.Unlock()

				if empty {
					l.entries.Delete(key)
				}
				return true
			})
		case <-l.stopCh:
			return
		}
	}
}

// Stop halts the cleanup goroutine.
func (l *SlidingWindowLimiter) Stop() {
	close(l.stopCh)
}

// matchEndpointPattern matches a URL path against a simple pattern where * is a wildcard
// for a single path segment and ** matches any number of segments.
func matchEndpointPattern(pattern, path string) bool {
	if pattern == path {
		return true
	}
	if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
		return true
	}

	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		// Check for prefix match with trailing wildcard.
		if len(patternParts) <= len(pathParts) {
			match := true
			for i, pp := range patternParts {
				if pp == "*" || pp == "**" {
					continue
				}
				if pp != pathParts[i] {
					match = false
					break
				}
			}
			return match
		}
		return false
	}

	for i, pp := range patternParts {
		if pp == "*" {
			continue
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}

// RateLimitMiddleware returns an HTTP middleware using the sliding window rate limiter.
// It sets standard rate limit response headers (X-RateLimit-Limit, X-RateLimit-Remaining,
// Retry-After) and responds with 429 Too Many Requests when the limit is exceeded.
func RateLimitMiddleware(limiter *SlidingWindowLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.Split(fwd, ",")[0]
				ip = strings.TrimSpace(ip)
			}

			path := r.URL.Path
			maxReqs, _ := limiter.getLimits(path)

			if !limiter.Allow(ip, path) {
				retryAfter := limiter.RetryAfter(ip, path)
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxReqs))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error":{"code":"rate_limited","message":"Too many requests. Retry after %d seconds."}}`, retryAfter)
				return
			}

			remaining := limiter.RemainingRequests(ip, path)
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxReqs))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

			next.ServeHTTP(w, r)
		})
	}
}
