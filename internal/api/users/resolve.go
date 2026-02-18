package users

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/federation"
	"github.com/amityvox/amityvox/internal/models"
)

// errRemoteUserNotFound indicates the remote instance explicitly returned 404.
var errRemoteUserNotFound = errors.New("remote user not found")


// HandleResolveHandle resolves a user handle (@username or @username@domain) to a user.
// GET /api/v1/users/resolve?handle=...
func (h *Handler) HandleResolveHandle(w http.ResponseWriter, r *http.Request) {
	handle := r.URL.Query().Get("handle")
	if handle == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_handle", "The handle query parameter is required")
		return
	}

	// Strip leading @.
	handle = strings.TrimPrefix(handle, "@")

	// Split on @ to get username and optional domain.
	parts := strings.SplitN(handle, "@", 2)
	username := parts[0]
	var domain string
	if len(parts) == 2 {
		domain = parts[1]
	}

	if !federation.UsernameRegex.MatchString(username) {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_handle", "Invalid username format")
		return
	}

	// Local lookup if no domain or domain matches our instance.
	if domain == "" || strings.EqualFold(domain, h.InstanceDomain) {
		user, err := h.resolveLocalUser(r, username)
		if err != nil {
			if err == pgx.ErrNoRows {
				apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "No user found with that handle")
				return
			}
			apiutil.InternalError(w, h.Logger, "Failed to resolve handle", err)
			return
		}
		h.computeHandle(r.Context(), user)
		apiutil.WriteJSON(w, http.StatusOK, user)
		return
	}

	// Check that our local instance allows federation before making remote requests.
	var localMode string
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT federation_mode FROM instances WHERE id = $1`, h.InstanceID).Scan(&localMode); err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to check federation mode", err)
		return
	}
	if localMode == "closed" {
		apiutil.WriteError(w, http.StatusForbidden, "federation_disabled", "Federated lookups are not enabled on this instance")
		return
	}

	// Validate the remote domain to prevent SSRF.
	if err := federation.ValidateFederationDomain(domain); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_domain", "Invalid federation domain")
		return
	}

	// In allowlist mode, verify the remote domain is an approved peer.
	if localMode == "allowlist" {
		var peerExists bool
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(
				SELECT 1 FROM federation_peers fp
				JOIN instances i ON fp.peer_id = i.id
				WHERE LOWER(i.domain) = LOWER($1) AND fp.instance_id = $2 AND fp.status = 'active'
			)`, domain, h.InstanceID).Scan(&peerExists); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to check federation allowlist", err)
			return
		}
		if !peerExists {
			apiutil.WriteError(w, http.StatusForbidden, "peer_not_allowed", "This instance is not an approved federation peer")
			return
		}
	}

	// Remote lookup via federation.
	user, err := h.resolveRemoteUser(r, username, domain)
	if err != nil {
		h.Logger.Error("failed to resolve remote user",
			slog.String("error", err.Error()),
			slog.String("domain", domain))
		if errors.Is(err, errRemoteUserNotFound) {
			apiutil.WriteError(w, http.StatusNotFound, "user_not_found", "No user found with that handle")
		} else {
			apiutil.WriteError(w, http.StatusBadGateway, "remote_lookup_failed", "Failed to look up user on remote instance")
		}
		return
	}
	h.computeHandle(r.Context(), user)
	apiutil.WriteJSON(w, http.StatusOK, user)
}

func (h *Handler) resolveLocalUser(r *http.Request, username string) (*models.User, error) {
	var user models.User
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, instance_id, username, display_name, avatar_id, status_text,
		        status_emoji, status_presence, status_expires_at, bio,
		        banner_id, accent_color, pronouns,
		        bot_owner_id, flags, created_at
		 FROM users
		 WHERE LOWER(username) = LOWER($1) AND instance_id = $2`,
		username, h.InstanceID,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Flags, &user.CreatedAt,
	)
	return &user, err
}

// federatedUserLookupResponse is the response from a remote instance's user lookup endpoint.
type federatedUserLookupResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	AvatarID    *string `json:"avatar_id"`
	Bio         *string `json:"bio"`
	CreatedAt   string  `json:"created_at"`
}

func (h *Handler) resolveRemoteUser(r *http.Request, username, domain string) (*models.User, error) {
	// First check if we already have this user cached locally.
	var user models.User
	err := h.Pool.QueryRow(r.Context(),
		`SELECT u.id, u.instance_id, u.username, u.display_name, u.avatar_id,
		        u.status_text, u.status_emoji, u.status_presence, u.status_expires_at,
		        u.bio, u.banner_id, u.accent_color, u.pronouns,
		        u.bot_owner_id, u.flags, u.created_at
		 FROM users u
		 JOIN instances i ON u.instance_id = i.id
		 WHERE LOWER(u.username) = LOWER($1) AND LOWER(i.domain) = LOWER($2)`,
		username, domain,
	).Scan(
		&user.ID, &user.InstanceID, &user.Username, &user.DisplayName,
		&user.AvatarID, &user.StatusText, &user.StatusEmoji, &user.StatusPresence,
		&user.StatusExpiresAt, &user.Bio, &user.BannerID, &user.AccentColor,
		&user.Pronouns, &user.BotOwnerID, &user.Flags, &user.CreatedAt,
	)
	if err == nil {
		return &user, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("checking cached remote user: %w", err)
	}

	// Discover the remote instance.
	disc, err := federation.DiscoverInstance(r.Context(), domain)
	if err != nil {
		return nil, fmt.Errorf("discovering instance %s: %w", domain, err)
	}

	if disc.FederationMode == "closed" {
		return nil, fmt.Errorf("remote instance %s has federation closed", domain)
	}

	// Look up the user on the remote instance.
	lookupURL := fmt.Sprintf("https://%s/federation/v1/users/lookup?username=%s", domain, username)
	req, err := http.NewRequestWithContext(r.Context(), "GET", lookupURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating lookup request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			if req.URL.Scheme != "https" {
				return errors.New("redirects must use https")
			}
			if err := federation.ValidateFederationDomain(req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("looking up user on %s: %w", domain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errRemoteUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote instance returned status %d", resp.StatusCode)
	}

	var remoteUser federatedUserLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&remoteUser); err != nil {
		return nil, fmt.Errorf("decoding remote user response: %w", err)
	}

	// Ensure the remote instance is registered locally.
	now := time.Now().UTC()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO instances (id, domain, public_key, name, software, federation_mode, created_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (domain) DO UPDATE SET last_seen_at = $8`,
		disc.InstanceID, disc.Domain, disc.PublicKey, disc.Name,
		disc.Software, disc.FederationMode, now, now,
	)
	if err != nil {
		h.Logger.Warn("failed to upsert remote instance", slog.String("error", err.Error()))
	}

	// Create a stub user record for the remote user.
	createdAt, _ := time.Parse(time.RFC3339, remoteUser.CreatedAt)
	if createdAt.IsZero() {
		createdAt = now
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO users (id, instance_id, username, display_name, avatar_id, bio, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (instance_id, username) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			avatar_id = EXCLUDED.avatar_id,
			bio = EXCLUDED.bio`,
		remoteUser.ID, disc.InstanceID, remoteUser.Username,
		remoteUser.DisplayName, remoteUser.AvatarID, remoteUser.Bio, createdAt,
	)
	if err != nil {
		h.Logger.Warn("failed to upsert remote user stub", slog.String("error", err.Error()))
	}

	// Build the user model to return.
	return &models.User{
		ID:             remoteUser.ID,
		InstanceID:     disc.InstanceID,
		Username:       remoteUser.Username,
		DisplayName:    remoteUser.DisplayName,
		AvatarID:       remoteUser.AvatarID,
		Bio:            remoteUser.Bio,
		StatusPresence: "offline",
		CreatedAt:      createdAt,
	}, nil
}
