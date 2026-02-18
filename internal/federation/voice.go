package federation

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// federatedVoiceTokenRequest is the signed payload for requesting a LiveKit token
// from a remote instance that hosts the voice channel.
type federatedVoiceTokenRequest struct {
	UserID         string `json:"user_id"`
	ChannelID      string `json:"channel_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name,omitempty"`
	AvatarID       string `json:"avatar_id,omitempty"`
	InstanceDomain string `json:"instance_domain"`
	ScreenShare    bool   `json:"screen_share"`
}

// HandleFederatedVoiceToken generates a LiveKit token for a remote federated user.
// The remote user's home instance signs this request on behalf of the user.
// POST /federation/v1/voice/token
func (ss *SyncService) HandleFederatedVoiceToken(w http.ResponseWriter, r *http.Request) {
	if ss.voiceSvc == nil {
		http.Error(w, "Voice is not enabled on this instance", http.StatusServiceUnavailable)
		return
	}

	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedVoiceTokenRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.ChannelID == "" {
		http.Error(w, "Missing user_id or channel_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Determine channel type and guild.
	var channelType *string
	var guildID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT channel_type, guild_id FROM channels WHERE id = $1`, req.ChannelID,
	).Scan(&channelType, &guildID); err != nil {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}

	// Enforce voice-capable channel type.
	if channelType == nil || (*channelType != models.ChannelTypeVoice &&
		*channelType != models.ChannelTypeStage &&
		*channelType != models.ChannelTypeDM &&
		*channelType != models.ChannelTypeGroup) {
		http.Error(w, "Voice is not supported in this channel type", http.StatusBadRequest)
		return
	}

	// Authorization: verify the user has access to this channel.
	canPublish := true
	if guildID != nil {
		// Guild channel — verify membership.
		var isMember bool
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
			*guildID, req.UserID,
		).Scan(&isMember); err != nil {
			ss.logger.Error("failed to check guild membership for voice", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if !isMember {
			http.Error(w, "Not a guild member", http.StatusForbidden)
			return
		}

		// Compute permissions once for all checks.
		perms, ok := computeFederatedGuildPerms(ctx, ss.fed.pool, *guildID, req.UserID)
		if !ok {
			http.Error(w, "Failed to compute permissions", http.StatusInternalServerError)
			return
		}

		// Check CONNECT permission.
		if perms&permissions.Connect == 0 && perms&permissions.Administrator == 0 {
			http.Error(w, "Missing CONNECT permission", http.StatusForbidden)
			return
		}

		// Derive publish from SPEAK permission.
		canPublish = perms&permissions.Speak != 0 || perms&permissions.Administrator != 0

		// Check STREAM permission for screen share.
		if req.ScreenShare {
			hasStream := perms&permissions.Stream != 0 || perms&permissions.Administrator != 0
			if !hasStream {
				http.Error(w, "Missing STREAM permission", http.StatusForbidden)
				return
			}
			// Screen share requires publish capability even without Speak.
			canPublish = true
		}
	} else {
		// DM/Group channel — verify user is a recipient.
		var isRecipient bool
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
			req.ChannelID, req.UserID,
		).Scan(&isRecipient); err != nil {
			ss.logger.Error("failed to check channel recipient for voice", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if !isRecipient {
			http.Error(w, "Not a channel participant", http.StatusForbidden)
			return
		}
	}

	// Ensure the LiveKit room exists.
	if err := ss.voiceSvc.EnsureRoom(ctx, req.ChannelID); err != nil {
		ss.logger.Error("failed to ensure voice room for federation", slog.String("error", err.Error()))
		http.Error(w, "Failed to ensure voice room", http.StatusServiceUnavailable)
		return
	}

	// Build participant metadata.
	metaMap := map[string]interface{}{
		"userId":         req.UserID,
		"username":       req.Username,
		"instanceDomain": req.InstanceDomain,
	}
	if req.DisplayName != "" {
		metaMap["displayName"] = req.DisplayName
	}
	if req.AvatarID != "" {
		metaMap["avatarId"] = req.AvatarID
	}
	metaBytes, _ := json.Marshal(metaMap)

	canSubscribe := true

	// Note: GenerateToken uses canPublish for both audio and video grants.
	// Pass canPublish for canVideo to match the local voice handler pattern.
	token, err := ss.voiceSvc.GenerateToken(req.UserID, req.ChannelID, canPublish, canSubscribe, canPublish, string(metaBytes))
	if err != nil {
		ss.logger.Error("failed to generate federated voice token", slog.String("error", err.Error()))
		http.Error(w, "Failed to generate voice token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"url":        ss.liveKitURL,
		"channel_id": req.ChannelID,
	})
}

// HandleProxyFederatedVoiceJoin proxies a voice join request from a local user
// to a remote instance that hosts the voice channel.
// POST /api/v1/federation/voice/join
func (ss *SyncService) HandleProxyFederatedVoiceJoin(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		InstanceDomain string `json:"instance_domain"`
		ChannelID      string `json:"channel_id"`
		ScreenShare    bool   `json:"screen_share"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.InstanceDomain == "" || req.ChannelID == "" {
		http.Error(w, "Missing instance_domain or channel_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Validate remote domain.
	if err := ValidateFederationDomain(req.InstanceDomain); err != nil {
		http.Error(w, "Invalid instance domain", http.StatusBadRequest)
		return
	}

	// Get user profile for the voice token request.
	var username string
	var displayName, avatarID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT username, display_name, avatar_id FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &avatarID); err != nil {
		http.Error(w, "Failed to fetch user profile", http.StatusInternalServerError)
		return
	}

	// Build federation voice token request.
	voiceReq := federatedVoiceTokenRequest{
		UserID:         userID,
		ChannelID:      req.ChannelID,
		Username:       username,
		InstanceDomain: ss.fed.domain,
		ScreenShare:    req.ScreenShare,
	}
	if displayName != nil {
		voiceReq.DisplayName = *displayName
	}
	if avatarID != nil {
		voiceReq.AvatarID = *avatarID
	}

	// Discover remote instance and sign + POST.
	disc, err := DiscoverInstance(ctx, req.InstanceDomain)
	if err != nil {
		ss.logger.Error("failed to discover remote instance for voice",
			slog.String("domain", req.InstanceDomain), slog.String("error", err.Error()))
		http.Error(w, "Failed to discover remote instance", http.StatusBadGateway)
		return
	}

	targetURL := disc.APIEndpoint + "/voice/token"
	respBody, statusCode, err := ss.signAndPost(ctx, targetURL, voiceReq)
	if err != nil {
		ss.logger.Error("failed to request federated voice token",
			slog.String("error", err.Error()))
		http.Error(w, "Failed to request voice token from remote instance", http.StatusBadGateway)
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		body := string(respBody)
		if len(body) > 512 {
			body = body[:512] + "..."
		}
		ss.logger.Warn("remote instance rejected voice token request",
			slog.Int("status", statusCode), slog.String("body", body))
		http.Error(w, "Remote instance rejected voice request", statusCode)
		return
	}

	// Parse and return the voice token response.
	var voiceResp map[string]interface{}
	if err := json.Unmarshal(respBody, &voiceResp); err != nil {
		http.Error(w, "Invalid response from remote instance", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": voiceResp})
}

// HandleProxyFederatedVoiceJoinByGuild proxies a voice join request for a user
// who is in a federated guild. It looks up the remote instance from the guild cache.
// POST /api/v1/federation/voice/guild-join
func (ss *SyncService) HandleProxyFederatedVoiceJoinByGuild(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		GuildID     string `json:"guild_id"`
		ChannelID   string `json:"channel_id"`
		ScreenShare bool   `json:"screen_share"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.GuildID == "" || req.ChannelID == "" {
		http.Error(w, "Missing guild_id or channel_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the remote instance from federation_guild_cache.
	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT i.domain FROM federation_guild_cache fgc
		 JOIN instances i ON i.id = fgc.instance_id
		 WHERE fgc.guild_id = $1 AND fgc.user_id = $2 LIMIT 1`,
		req.GuildID, userID,
	).Scan(&instanceDomain); err != nil {
		http.Error(w, "Guild not found in federation cache", http.StatusNotFound)
		return
	}

	// Get user profile.
	var username string
	var displayName, avatarID *string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT username, display_name, avatar_id FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &avatarID); err != nil {
		http.Error(w, "Failed to fetch user profile", http.StatusInternalServerError)
		return
	}

	voiceReq := federatedVoiceTokenRequest{
		UserID:         userID,
		ChannelID:      req.ChannelID,
		Username:       username,
		InstanceDomain: ss.fed.domain,
		ScreenShare:    req.ScreenShare,
	}
	if displayName != nil {
		voiceReq.DisplayName = *displayName
	}
	if avatarID != nil {
		voiceReq.AvatarID = *avatarID
	}

	disc, err := DiscoverInstance(ctx, instanceDomain)
	if err != nil {
		ss.logger.Error("failed to discover remote instance for guild voice",
			slog.String("domain", instanceDomain), slog.String("error", err.Error()))
		http.Error(w, "Failed to discover remote instance", http.StatusBadGateway)
		return
	}

	targetURL := disc.APIEndpoint + "/voice/token"
	respBody, statusCode, err := ss.signAndPost(ctx, targetURL, voiceReq)
	if err != nil {
		ss.logger.Error("failed to request federated voice token",
			slog.String("error", err.Error()))
		http.Error(w, "Failed to request voice token from remote instance", http.StatusBadGateway)
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		http.Error(w, "Remote instance rejected voice request", statusCode)
		return
	}

	var voiceResp map[string]interface{}
	if err := json.Unmarshal(respBody, &voiceResp); err != nil {
		http.Error(w, "Invalid response from remote instance", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": voiceResp})
}

// computeFederatedGuildPerms computes the effective permission bitfield for a user
// in a guild. Returns the computed permissions and true on success, or (0, false)
// if any DB query fails (fail-closed). This mirrors the logic in
// internal/api/voice_handlers.go:checkGuildPerm but lives in the federation
// package to avoid circular imports.
func computeFederatedGuildPerms(ctx context.Context, pool *pgxpool.Pool, guildID, userID string) (uint64, bool) {
	// Owner has all permissions.
	var ownerID string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return 0, false
	}
	if userID == ownerID {
		return permissions.Administrator, true
	}

	// Admin flag.
	var userFlags int
	if err := pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags); err != nil {
		return 0, false
	}
	if userFlags&models.UserFlagAdmin != 0 {
		return permissions.Administrator, true
	}

	// Compute from default + role permissions.
	var defaultPerms int64
	if err := pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms); err != nil {
		return 0, false
	}
	computed := uint64(defaultPerms)

	rows, err := pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, userID,
	)
	if err != nil {
		return 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var allow, deny int64
		if err := rows.Scan(&allow, &deny); err != nil {
			return 0, false
		}
		computed |= uint64(allow)
		computed &^= uint64(deny)
	}
	if err := rows.Err(); err != nil {
		return 0, false
	}

	return computed, true
}

// voiceTokenTestRequest is used by tests.
type voiceTokenTestRequest = federatedVoiceTokenRequest

// NewTestVoiceTokenRequest creates a test voice token request for JSON serialization tests.
func NewTestVoiceTokenRequest() *federatedVoiceTokenRequest {
	return &federatedVoiceTokenRequest{
		UserID:         models.NewULID().String(),
		ChannelID:      models.NewULID().String(),
		Username:       "testuser",
		DisplayName:    "Test User",
		AvatarID:       "avatar-123",
		InstanceDomain: "remote.example.com",
		ScreenShare:    false,
	}
}
