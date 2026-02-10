// Package presence tracks user online/idle/offline status using DragonflyDB
// (Redis-compatible). It manages heartbeat-based presence detection, session
// caching, and provides a Cache type for generic key-value caching operations
// used throughout AmityVox.
package presence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Key prefix constants for organizing data in DragonflyDB.
const (
	PrefixSession  = "session:"
	PrefixPresence = "presence:"
	PrefixRateLimit = "ratelimit:"
	PrefixCache    = "cache:"
)

// Status constants for user presence.
const (
	StatusOnline    = "online"
	StatusIdle      = "idle"
	StatusFocus     = "focus"
	StatusBusy      = "busy"
	StatusInvisible = "invisible"
	StatusOffline   = "offline"
)

// Cache provides a DragonflyDB/Redis client for session storage, presence
// tracking, rate limiting, and general-purpose caching.
type Cache struct {
	client *redis.Client
	logger *slog.Logger
}

// New creates a new Cache connected to DragonflyDB/Redis at the given URL.
// The URL should be in the format redis://host:port or redis://host:port/db.
func New(cacheURL string, logger *slog.Logger) (*Cache, error) {
	opts, err := redis.ParseURL(cacheURL)
	if err != nil {
		return nil, fmt.Errorf("parsing cache URL %q: %w", cacheURL, err)
	}

	opts.PoolSize = 20
	opts.MinIdleConns = 5
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging cache at %s: %w", cacheURL, err)
	}

	logger.Info("cache connection established", slog.String("addr", opts.Addr))

	return &Cache{client: client, logger: logger}, nil
}

// HealthCheck verifies the cache connection is alive.
func (c *Cache) HealthCheck(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache health check: %w", err)
	}
	return nil
}

// Close shuts down the cache connection.
func (c *Cache) Close() error {
	c.logger.Info("closing cache connection")
	return c.client.Close()
}

// Client returns the underlying Redis client for direct access.
func (c *Cache) Client() *redis.Client {
	return c.client
}

// --- Session Operations ---

// SessionData holds cached session information for fast token validation
// without hitting PostgreSQL on every request.
type SessionData struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SetSession caches a session token with its associated user ID and expiry.
func (c *Cache) SetSession(ctx context.Context, sessionID string, data SessionData) error {
	ttl := time.Until(data.ExpiresAt)
	if ttl <= 0 {
		return nil
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling session data: %w", err)
	}

	if err := c.client.Set(ctx, PrefixSession+sessionID, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("caching session %s: %w", sessionID, err)
	}

	return nil
}

// GetSession retrieves cached session data for a token. Returns nil if the
// session is not cached (caller should fall back to database lookup).
func (c *Cache) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	val, err := c.client.Get(ctx, PrefixSession+sessionID).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting cached session %s: %w", sessionID, err)
	}

	var data SessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("unmarshaling session data: %w", err)
	}

	return &data, nil
}

// DeleteSession removes a session from the cache (e.g. on logout).
func (c *Cache) DeleteSession(ctx context.Context, sessionID string) error {
	if err := c.client.Del(ctx, PrefixSession+sessionID).Err(); err != nil {
		return fmt.Errorf("deleting cached session %s: %w", sessionID, err)
	}
	return nil
}

// --- Presence Operations ---

// SetPresence updates a user's presence status with a TTL. If the TTL expires
// without renewal (heartbeat), the user is considered offline.
func (c *Cache) SetPresence(ctx context.Context, userID, status string, ttl time.Duration) error {
	if err := c.client.Set(ctx, PrefixPresence+userID, status, ttl).Err(); err != nil {
		return fmt.Errorf("setting presence for user %s: %w", userID, err)
	}
	return nil
}

// GetPresence returns a user's current presence status. Returns StatusOffline
// if the user has no active presence entry.
func (c *Cache) GetPresence(ctx context.Context, userID string) (string, error) {
	val, err := c.client.Get(ctx, PrefixPresence+userID).Result()
	if err == redis.Nil {
		return StatusOffline, nil
	}
	if err != nil {
		return StatusOffline, fmt.Errorf("getting presence for user %s: %w", userID, err)
	}
	return val, nil
}

// GetBulkPresence returns presence statuses for multiple users at once.
func (c *Cache) GetBulkPresence(ctx context.Context, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}

	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = PrefixPresence + id
	}

	vals, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("getting bulk presence: %w", err)
	}

	result := make(map[string]string, len(userIDs))
	for i, userID := range userIDs {
		if vals[i] != nil {
			result[userID] = vals[i].(string)
		} else {
			result[userID] = StatusOffline
		}
	}

	return result, nil
}

// RemovePresence removes a user's presence entry (sets them offline).
func (c *Cache) RemovePresence(ctx context.Context, userID string) error {
	if err := c.client.Del(ctx, PrefixPresence+userID).Err(); err != nil {
		return fmt.Errorf("removing presence for user %s: %w", userID, err)
	}
	return nil
}

// --- Rate Limiting ---

// CheckRateLimit implements a sliding window rate limiter using Redis.
// Returns true if the request is allowed, false if rate limited.
func (c *Cache) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := PrefixRateLimit + key

	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return true, fmt.Errorf("checking rate limit for %s: %w", key, err)
	}

	count := incr.Val()
	return count <= int64(limit), nil
}

// --- Generic Cache Operations ---

// Set stores a value in the cache with an optional TTL.
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshaling cache value: %w", err)
	}

	if err := c.client.Set(ctx, PrefixCache+key, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("setting cache key %s: %w", key, err)
	}

	return nil
}

// Get retrieves a value from the cache and unmarshals it into dst.
// Returns false if the key does not exist.
func (c *Cache) Get(ctx context.Context, key string, dst interface{}) (bool, error) {
	val, err := c.client.Get(ctx, PrefixCache+key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getting cache key %s: %w", key, err)
	}

	if err := json.Unmarshal([]byte(val), dst); err != nil {
		return false, fmt.Errorf("unmarshaling cache value: %w", err)
	}

	return true, nil
}

// Delete removes a key from the cache.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, PrefixCache+key).Err(); err != nil {
		return fmt.Errorf("deleting cache key %s: %w", key, err)
	}
	return nil
}
