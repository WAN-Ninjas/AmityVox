package federation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
)

// --- Request/Response types for federated invite endpoints ---

// inviteResolveResponse is the guild preview returned when resolving an invite code.
type inviteResolveResponse struct {
	GuildID     string  `json:"guild_id"`
	GuildName   string  `json:"guild_name"`
	IconID      *string `json:"icon_id,omitempty"`
	Description *string `json:"description,omitempty"`
	MemberCount int     `json:"member_count"`
}

// inviteAcceptRequest is the signed payload for accepting an invite.
type inviteAcceptRequest struct {
	UserID         string  `json:"user_id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	InstanceDomain string  `json:"instance_domain"`
}

// proxyResolveInviteRequest is the body for the local proxy resolve endpoint.
type proxyResolveInviteRequest struct {
	InstanceDomain string `json:"instance_domain"`
	Code           string `json:"code"`
}

// ============================================================
// Federation-facing invite handlers
// ============================================================

// HandleInviteResolve returns a guild preview for an invite code.
// GET /federation/v1/invites/{code}
// This is a public endpoint (like guild preview) — no signature verification.
func (ss *SyncService) HandleInviteResolve(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing invite code"}}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the invite.
	var guildID string
	var maxUses, uses int
	var expiresAt *time.Time
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, max_uses, uses, expires_at FROM invites WHERE code = $1`,
		code,
	).Scan(&guildID, &maxUses, &uses, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, `{"error":{"code":"not_found","message":"Invite not found"}}`, http.StatusNotFound)
		} else {
			ss.logger.Error("failed to look up invite", slog.String("code", code), slog.String("error", err.Error()))
			http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		}
		return
	}

	// Check if invite is expired.
	if expiresAt != nil && time.Now().After(*expiresAt) {
		http.Error(w, `{"error":{"code":"gone","message":"Invite has expired"}}`, http.StatusGone)
		return
	}

	// Check if invite is exhausted.
	if maxUses > 0 && uses >= maxUses {
		http.Error(w, `{"error":{"code":"gone","message":"Invite has been exhausted"}}`, http.StatusGone)
		return
	}

	// Look up guild preview info.
	var resp inviteResolveResponse
	err = ss.fed.pool.QueryRow(ctx,
		`SELECT id, name, icon_id, description, member_count FROM guilds WHERE id = $1`,
		guildID,
	).Scan(&resp.GuildID, &resp.GuildName, &resp.IconID, &resp.Description, &resp.MemberCount)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, `{"error":{"code":"not_found","message":"Guild not found"}}`, http.StatusNotFound)
		} else {
			ss.logger.Error("failed to look up guild for invite",
				slog.String("guild_id", guildID), slog.String("error", err.Error()))
			http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// HandleInviteAccept handles a remote user accepting a guild invite.
// POST /federation/v1/invites/{code}/accept
func (ss *SyncService) HandleInviteAccept(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing invite code"}}`, http.StatusBadRequest)
		return
	}

	var req inviteAcceptRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, `{"error":{"code":"bad_request","message":"Invalid payload"}}`, http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.Username == "" || req.InstanceDomain == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing required fields: user_id, username, instance_domain"}}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the invite.
	var guildID string
	var maxUses, uses int
	var expiresAt *time.Time
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT guild_id, max_uses, uses, expires_at FROM invites WHERE code = $1`,
		code,
	).Scan(&guildID, &maxUses, &uses, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, `{"error":{"code":"not_found","message":"Invalid invite code"}}`, http.StatusNotFound)
		} else {
			ss.logger.Error("failed to look up invite for accept",
				slog.String("code", code), slog.String("error", err.Error()))
			http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		}
		return
	}

	// Check if invite is expired.
	if expiresAt != nil && time.Now().After(*expiresAt) {
		http.Error(w, `{"error":{"code":"gone","message":"Invite has expired"}}`, http.StatusGone)
		return
	}

	// Check if invite is exhausted.
	if maxUses > 0 && uses >= maxUses {
		http.Error(w, `{"error":{"code":"gone","message":"Invite has been exhausted"}}`, http.StatusGone)
		return
	}

	// Check if the user is banned from this guild (fail closed on query error).
	var banned bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_bans WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&banned); err != nil {
		ss.logger.Error("failed to check guild ban",
			slog.String("guild_id", guildID), slog.String("user_id", req.UserID),
			slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		return
	}
	if banned {
		http.Error(w, `{"error":{"code":"forbidden","message":"User is banned from this guild"}}`, http.StatusForbidden)
		return
	}

	// Validate that the claimed domain matches the signed sender.
	instanceID, ok := ss.validateSenderDomain(ctx, w, senderID, req.InstanceDomain)
	if !ok {
		return
	}

	// Create or update the remote user stub.
	ss.ensureRemoteUserStub(ctx, instanceID, federatedUserInfo{
		ID:             req.UserID,
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		AvatarID:       req.AvatarID,
		InstanceDomain: req.InstanceDomain,
	})

	// Add to guild_members (idempotent).
	tag, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO guild_members (guild_id, user_id, joined_at)
		 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		guildID, req.UserID,
	)
	if err != nil {
		ss.logger.Error("failed to add federated guild member via invite",
			slog.String("guild_id", guildID), slog.String("user_id", req.UserID),
			slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		return
	}

	// Only update counts and register peers if a new row was inserted.
	if tag.RowsAffected() > 0 {
		// Increment invite uses.
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE invites SET uses = uses + 1 WHERE code = $1`, code); err != nil {
			ss.logger.Warn("failed to increment invite uses",
				slog.String("code", code), slog.String("error", err.Error()))
		}

		// Increment guild member count.
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE guilds SET member_count = member_count + 1 WHERE id = $1`, guildID); err != nil {
			ss.logger.Warn("failed to increment guild member count",
				slog.String("guild_id", guildID), slog.String("error", err.Error()))
		}

		// Register channel peers so federation events flow to the remote instance.
		ss.addInstanceToGuildChannelPeers(ctx, guildID, instanceID)

		// Publish GUILD_MEMBER_ADD event.
		ss.bus.PublishGuildEvent(ctx, events.SubjectGuildMemberAdd, "GUILD_MEMBER_ADD", guildID, map[string]interface{}{
			"guild_id": guildID,
			"user_id":  req.UserID,
			"username": req.Username,
		})
	}

	// Build full guild structure response.
	resp, err := ss.buildGuildJoinResponse(ctx, guildID)
	if err != nil {
		ss.logger.Error("failed to build guild join response for invite accept",
			slog.String("guild_id", guildID), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// ============================================================
// Local user-facing invite proxy handler
// ============================================================

// HandleProxyResolveInvite resolves a cross-instance invite code for a local user.
// POST /api/v1/federation/invites/resolve
func (ss *SyncService) HandleProxyResolveInvite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, `{"error":{"code":"unauthorized","message":"Unauthorized"}}`, http.StatusUnauthorized)
		return
	}

	var req proxyResolveInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"code":"bad_request","message":"Invalid request body"}}`, http.StatusBadRequest)
		return
	}
	if req.InstanceDomain == "" || req.Code == "" {
		http.Error(w, `{"error":{"code":"bad_request","message":"Missing instance_domain and code"}}`, http.StatusBadRequest)
		return
	}
	if err := ValidateFederationDomain(req.InstanceDomain); err != nil {
		http.Error(w, `{"error":{"code":"bad_request","message":"Invalid instance domain"}}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Discover the remote instance to verify it exists and is reachable.
	discovery, err := DiscoverInstance(ctx, req.InstanceDomain)
	if err != nil {
		ss.logger.Warn("failed to discover remote instance for invite resolve",
			slog.String("domain", req.InstanceDomain), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"bad_gateway","message":"Failed to discover remote instance"}}`, http.StatusBadGateway)
		return
	}

	// Validate that the discovered domain matches an active federation peer
	// to prevent SSRF via arbitrary host redirection in discovery responses.
	var peerExists bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM federation_peers fp
			JOIN instances i ON i.id = fp.peer_id
			WHERE i.domain = $1 AND fp.instance_id = $2 AND fp.status = 'active'
		)`, discovery.Domain, ss.fed.instanceID,
	).Scan(&peerExists); err != nil {
		ss.logger.Error("failed to validate discovery domain against peers",
			slog.String("domain", discovery.Domain), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		return
	}
	if !peerExists {
		ss.logger.Warn("discovery domain does not match any active federation peer",
			slog.String("requested_domain", req.InstanceDomain),
			slog.String("discovered_domain", discovery.Domain))
		http.Error(w, `{"error":{"code":"bad_gateway","message":"Remote instance is not a known federation peer"}}`, http.StatusBadGateway)
		return
	}

	// Fetch the invite preview from the remote instance.
	remoteURL := fmt.Sprintf("https://%s/federation/v1/invites/%s", discovery.Domain, req.Code)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", remoteURL, nil)
	if err != nil {
		ss.logger.Error("failed to create invite resolve request",
			slog.String("url", remoteURL), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"internal","message":"Internal error"}}`, http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(httpReq)
	if err != nil {
		ss.logger.Warn("failed to contact remote instance for invite resolve",
			slog.String("url", remoteURL), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"bad_gateway","message":"Failed to contact remote instance"}}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Forward the error response from the remote instance.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		var buf [4096]byte
		n, _ := resp.Body.Read(buf[:])
		if n > 0 {
			w.Write(buf[:n])
		} else {
			fmt.Fprintf(w, `{"error":{"code":"remote_error","message":"Remote instance returned status %d"}}`, resp.StatusCode)
		}
		return
	}

	// Parse the remote response and enrich with instance_domain.
	var remoteResp struct {
		Data inviteResolveResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&remoteResp); err != nil {
		ss.logger.Warn("failed to decode invite resolve response",
			slog.String("domain", req.InstanceDomain), slog.String("error", err.Error()))
		http.Error(w, `{"error":{"code":"bad_gateway","message":"Invalid response from remote instance"}}`, http.StatusBadGateway)
		return
	}

	// Return the preview enriched with the instance domain.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"guild_id":        remoteResp.Data.GuildID,
			"guild_name":      remoteResp.Data.GuildName,
			"icon_id":         remoteResp.Data.IconID,
			"description":     remoteResp.Data.Description,
			"member_count":    remoteResp.Data.MemberCount,
			"instance_domain": req.InstanceDomain,
		},
	})
}
