package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
)

// profileCacheTTL is how long a remote user profile is considered fresh before
// re-fetching from their home instance.
const profileCacheTTL = 5 * time.Minute

// userProfileResponse is the response type for the user profile federation endpoint.
type userProfileResponse struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	Bio            *string `json:"bio,omitempty"`
	StatusText     *string `json:"status_text,omitempty"`
	StatusEmoji    *string `json:"status_emoji,omitempty"`
	BannerID       *string `json:"banner_id,omitempty"`
	AccentColor    *string `json:"accent_color,omitempty"`
	Pronouns       *string `json:"pronouns,omitempty"`
	Flags          int     `json:"flags"`
	InstanceID     *string `json:"instance_id,omitempty"`
}

// profileFetchTime tracks when each remote user's profile was last fetched.
// Key: userID, Value: time.Time of last fetch.
var profileFetchCache = struct {
	sync.RWMutex
	m map[string]time.Time
}{m: make(map[string]time.Time)}

// HandleUserProfile handles POST /federation/v1/users/{userID}/profile — a
// signed federation endpoint that returns the full profile of a local user.
// Remote instances call this to fetch up-to-date profile data for user stubs.
func (ss *SyncService) HandleUserProfile(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	userID := chi.URLParam(r, "userID")
	if userID == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing user ID"}}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Query the local user — must belong to this instance (instance_id IS NULL for local users).
	var profile userProfileResponse
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, bio,
		        status_text, status_emoji, banner_id, accent_color, pronouns, flags
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID, &instanceID, &profile.Username, &profile.DisplayName,
		&profile.AvatarID, &profile.Bio, &profile.StatusText, &profile.StatusEmoji,
		&profile.BannerID, &profile.AccentColor, &profile.Pronouns, &profile.Flags,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, `{"error":{"code":"not_found","message":"User not found"}}`, http.StatusNotFound)
		} else {
			ss.logger.Error("federation user profile: failed to query user",
				slog.String("user_id", userID), slog.String("error", err.Error()))
			http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		}
		return
	}

	// Verify the user belongs to this instance (local user has NULL instance_id,
	// or instance_id matching our instance ID).
	if instanceID != nil && *instanceID != ss.fed.instanceID {
		http.Error(w, `{"error":{"code":"not_found","message":"User does not belong to this instance"}}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": profile}); err != nil {
		ss.logger.Error("federation user profile: failed to encode response",
			slog.String("user_id", userID), slog.String("error", err.Error()))
	}
}

// FetchRemoteProfile fetches a remote user's full profile from their home instance.
// If the local stub was fetched recently (within profileCacheTTL), the cached
// local data is returned. Otherwise, the profile is fetched from the remote
// instance and the local user stub is updated.
func (ss *SyncService) FetchRemoteProfile(ctx context.Context, userID string, instanceID string) (*userProfileResponse, error) {
	// 1. Check if local stub was recently synced.
	profileFetchCache.RLock()
	lastFetch, hasCached := profileFetchCache.m[userID]
	profileFetchCache.RUnlock()

	if hasCached && time.Since(lastFetch) < profileCacheTTL {
		// Return cached data from local users table.
		profile, err := ss.queryLocalUserProfile(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("querying cached profile for user %s: %w", userID, err)
		}
		return profile, nil
	}

	// 2. Look up the instance domain.
	var domain string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, instanceID,
	).Scan(&domain)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("instance %s not found", instanceID)
		}
		return nil, fmt.Errorf("looking up instance %s: %w", instanceID, err)
	}

	// 3. Sign and POST to the remote /federation/v1/users/{userID}/profile endpoint.
	targetURL := fmt.Sprintf("https://%s/federation/v1/users/%s/profile", domain, userID)

	// Use a minimal payload for the signed request — the user ID is in the URL.
	payload := map[string]string{"user_id": userID}

	respBody, statusCode, err := ss.signAndPost(ctx, targetURL, payload)
	if err != nil {
		return nil, fmt.Errorf("fetching profile from %s: %w", domain, err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("remote instance %s returned status %d for user profile", domain, statusCode)
	}

	// 4. Parse the response.
	var resp struct {
		Data userProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decoding profile response from %s: %w", domain, err)
	}

	// 5. Update the local user stub with fresh profile data.
	if _, err := ss.fed.pool.Exec(ctx,
		`UPDATE users SET
			display_name = $2,
			avatar_id = $3,
			bio = $4,
			status_text = $5,
			status_emoji = $6,
			banner_id = $7,
			accent_color = $8,
			pronouns = $9
		 WHERE id = $1 AND instance_id = $10`,
		userID,
		resp.Data.DisplayName,
		resp.Data.AvatarID,
		resp.Data.Bio,
		resp.Data.StatusText,
		resp.Data.StatusEmoji,
		resp.Data.BannerID,
		resp.Data.AccentColor,
		resp.Data.Pronouns,
		instanceID,
	); err != nil {
		ss.logger.Warn("failed to update local user stub after profile fetch",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
		// Continue — we still have the response data to return.
	}

	// 6. Update the cache timestamp.
	profileFetchCache.Lock()
	// Evict the entire cache if it exceeds the max size to prevent unbounded growth.
	if len(profileFetchCache.m) >= 10000 {
		profileFetchCache.m = make(map[string]time.Time)
	}
	profileFetchCache.m[userID] = time.Now()
	profileFetchCache.Unlock()

	return &resp.Data, nil
}

// queryLocalUserProfile queries the local users table for a user's profile.
func (ss *SyncService) queryLocalUserProfile(ctx context.Context, userID string) (*userProfileResponse, error) {
	var profile userProfileResponse
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id, bio,
		        status_text, status_emoji, banner_id, accent_color, pronouns, flags
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&profile.ID, &instanceID, &profile.Username, &profile.DisplayName,
		&profile.AvatarID, &profile.Bio, &profile.StatusText, &profile.StatusEmoji,
		&profile.BannerID, &profile.AccentColor, &profile.Pronouns, &profile.Flags,
	)
	if err != nil {
		return nil, err
	}
	profile.InstanceID = instanceID
	return &profile, nil
}

// HandleProxyUserProfile handles GET /api/v1/federation/users/{instanceID}/{userID}/profile
// — an authenticated endpoint for local users to fetch a remote user's full profile.
// Calls FetchRemoteProfile to get (and cache) the profile from the remote instance.
func (ss *SyncService) HandleProxyUserProfile(w http.ResponseWriter, r *http.Request) {
	callerID := auth.UserIDFromContext(r.Context())
	if callerID == "" {
		http.Error(w, `{"error":{"code":"unauthorized","message":"Unauthorized"}}`, http.StatusUnauthorized)
		return
	}

	instanceID := chi.URLParam(r, "instanceID")
	userID := chi.URLParam(r, "userID")
	if instanceID == "" || userID == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing instanceID or userID"}}`, http.StatusBadRequest)
		return
	}

	profile, err := ss.FetchRemoteProfile(r.Context(), userID, instanceID)
	if err != nil {
		ss.logger.Warn("failed to fetch remote user profile",
			slog.String("user_id", userID),
			slog.String("instance_id", instanceID),
			slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"bad_gateway","message":"Failed to fetch remote user profile"}}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": profile}); err != nil {
		ss.logger.Warn("failed to encode user profile response",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
	}
}
