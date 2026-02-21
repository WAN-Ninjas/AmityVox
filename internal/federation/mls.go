package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/models"
)

// --- Request types for federated MLS endpoints ---

// mlsClaimKeyPackageRequest is the signed payload for claiming a key package.
type mlsClaimKeyPackageRequest struct {
	// No additional fields needed — userID comes from the URL path.
}

// mlsSendWelcomeRequest is the signed payload for sending an MLS Welcome message.
type mlsSendWelcomeRequest struct {
	ReceiverID string `json:"receiver_id"`
	Data       []byte `json:"data"` // Opaque MLS Welcome bytes
}

// mlsPublishCommitRequest is the signed payload for publishing an MLS Commit.
type mlsPublishCommitRequest struct {
	UserID string `json:"user_id"`
	Epoch  uint64 `json:"epoch"`
	Data   []byte `json:"data"` // Opaque MLS Commit bytes
}

// --- Response types ---

// mlsKeyPackageResponse represents a key package in federation responses.
type mlsKeyPackageResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Data      []byte    `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// mlsWelcomeResponse represents a welcome message in federation responses.
type mlsWelcomeResponse struct {
	ID         string    `json:"id"`
	ChannelID  string    `json:"channel_id"`
	ReceiverID string    `json:"receiver_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// mlsGroupStateResponse represents MLS group state in federation responses.
type mlsGroupStateResponse struct {
	ChannelID string    `json:"channel_id"`
	Epoch     uint64    `json:"epoch"`
	TreeHash  []byte    `json:"tree_hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

// mlsCommitResponse represents an MLS commit in federation responses.
type mlsCommitResponse struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	SenderID  string    `json:"sender_id"`
	Epoch     uint64    `json:"epoch"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Helper ---

// verifyChannelInLocalGuild verifies that a channel belongs to a guild and that
// the guild is local (instance_id IS NULL). Returns an error if the channel is
// not found, does not belong to the guild, or the guild is federated.
func (ss *SyncService) verifyChannelInLocalGuild(ctx context.Context, guildID, channelID string) error {
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.instance_id FROM channels c
		 JOIN guilds g ON c.guild_id = g.id
		 WHERE c.id = $1 AND c.guild_id = $2`, channelID, guildID,
	).Scan(&instanceID)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("channel %s not found in guild %s", channelID, guildID)
	}
	if err != nil {
		return fmt.Errorf("verifying channel ownership: %w", err)
	}
	if instanceID != nil {
		return fmt.Errorf("guild %s is not local to this instance", guildID)
	}
	return nil
}

// writeMLSError writes a JSON error response in the standard federation format.
func writeMLSError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}

// writeMLSJSON writes a JSON success response.
func writeMLSJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// --- Handlers ---

// HandleMLSKeyPackages handles GET /federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-packages/{userID}.
// A remote instance requests key packages for a local user so they can be added
// to a federated MLS group. The signed payload can be empty (just signature verification).
func (ss *SyncService) HandleMLSKeyPackages(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	userID := chi.URLParam(r, "userID")
	if guildID == "" || channelID == "" || userID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID, channelID, or userID")
		return
	}

	ctx := r.Context()

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS key-packages: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	// Query non-expired key packages for the user.
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT id, user_id, device_id, data, expires_at, created_at
		 FROM mls_key_packages
		 WHERE user_id = $1 AND expires_at > now()
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		ss.logger.Error("MLS key-packages: query failed",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to query key packages")
		return
	}
	defer rows.Close()

	var packages []mlsKeyPackageResponse
	for rows.Next() {
		var kp mlsKeyPackageResponse
		if err := rows.Scan(&kp.ID, &kp.UserID, &kp.DeviceID, &kp.Data, &kp.ExpiresAt, &kp.CreatedAt); err != nil {
			ss.logger.Warn("MLS key-packages: failed to scan row", slog.String("error", err.Error()))
			continue
		}
		packages = append(packages, kp)
	}
	if err := rows.Err(); err != nil {
		ss.logger.Error("MLS key-packages: rows iteration error", slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to read key packages")
		return
	}

	if packages == nil {
		packages = []mlsKeyPackageResponse{}
	}
	writeMLSJSON(w, http.StatusOK, packages)
}

// HandleMLSClaimKeyPackage handles POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/key-packages/{userID}/claim.
// A remote instance claims (consumes) one key package for a local user. The key
// package is deleted after claiming so it cannot be reused.
func (ss *SyncService) HandleMLSClaimKeyPackage(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	userID := chi.URLParam(r, "userID")
	if guildID == "" || channelID == "" || userID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID, channelID, or userID")
		return
	}

	ctx := r.Context()

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS claim-key-package: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	// Claim one non-expired key package via DELETE RETURNING.
	var kp mlsKeyPackageResponse
	err := ss.fed.pool.QueryRow(ctx,
		`DELETE FROM mls_key_packages
		 WHERE id = (
			 SELECT id FROM mls_key_packages
			 WHERE user_id = $1 AND expires_at > now()
			 ORDER BY created_at ASC LIMIT 1
		 )
		 RETURNING id, user_id, device_id, data, expires_at, created_at`,
		userID,
	).Scan(&kp.ID, &kp.UserID, &kp.DeviceID, &kp.Data, &kp.ExpiresAt, &kp.CreatedAt)

	if err == pgx.ErrNoRows {
		writeMLSError(w, http.StatusNotFound, "no_key_packages", "No available key packages for this user")
		return
	}
	if err != nil {
		ss.logger.Error("MLS claim-key-package: failed to claim",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to claim key package")
		return
	}

	writeMLSJSON(w, http.StatusOK, kp)
}

// HandleMLSSendWelcome handles POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/welcome.
// A remote instance sends an MLS Welcome message to a local user joining an
// encrypted channel. The Welcome message is stored so the local user can retrieve
// it via their regular MLS welcome endpoint.
func (ss *SyncService) HandleMLSSendWelcome(w http.ResponseWriter, r *http.Request) {
	signed, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID or channelID")
		return
	}

	var req mlsSendWelcomeRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		writeMLSError(w, http.StatusBadRequest, "invalid_payload", "Invalid request payload")
		return
	}
	if req.ReceiverID == "" || len(req.Data) == 0 {
		writeMLSError(w, http.StatusBadRequest, "missing_fields", "receiver_id and data are required")
		return
	}

	ctx := r.Context()

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS send-welcome: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	// Verify the receiver is a local user (belongs to our instance).
	var receiverInstanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT instance_id FROM users WHERE id = $1`, req.ReceiverID,
	).Scan(&receiverInstanceID)
	if err == pgx.ErrNoRows {
		writeMLSError(w, http.StatusNotFound, "user_not_found", "Receiver user not found")
		return
	}
	if err != nil {
		ss.logger.Error("MLS send-welcome: failed to look up receiver",
			slog.String("receiver_id", req.ReceiverID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to verify receiver")
		return
	}
	// Receiver must be a local user (instance_id IS NULL).
	if receiverInstanceID != nil {
		writeMLSError(w, http.StatusBadRequest, "not_local_user", "Receiver is not a local user on this instance")
		return
	}

	// Store the welcome message.
	id := models.NewULID().String()
	_, err = ss.fed.pool.Exec(ctx,
		`INSERT INTO mls_welcome_messages (id, channel_id, receiver_id, data, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		id, channelID, req.ReceiverID, req.Data,
	)
	if err != nil {
		ss.logger.Error("MLS send-welcome: failed to store welcome message",
			slog.String("channel_id", channelID),
			slog.String("receiver_id", req.ReceiverID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to store welcome message")
		return
	}

	ss.logger.Info("MLS welcome message stored via federation",
		slog.String("welcome_id", id),
		slog.String("channel_id", channelID),
		slog.String("receiver_id", req.ReceiverID))

	writeMLSJSON(w, http.StatusCreated, mlsWelcomeResponse{
		ID:         id,
		ChannelID:  channelID,
		ReceiverID: req.ReceiverID,
		CreatedAt:  time.Now().UTC(),
	})
}

// HandleMLSPublishCommit handles POST /federation/v1/guilds/{guildID}/channels/{channelID}/mls/commits.
// A remote instance publishes an MLS Commit message from one of their users.
// The commit is stored locally so all participants can catch up on the MLS group state.
func (ss *SyncService) HandleMLSPublishCommit(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID or channelID")
		return
	}

	var req mlsPublishCommitRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		writeMLSError(w, http.StatusBadRequest, "invalid_payload", "Invalid request payload")
		return
	}
	if req.UserID == "" || len(req.Data) == 0 {
		writeMLSError(w, http.StatusBadRequest, "missing_fields", "user_id and data are required")
		return
	}

	ctx := r.Context()

	// Verify the user belongs to the sender's instance.
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS publish-commit: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	// Verify the user is a member of the guild.
	var isMember bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, req.UserID,
	).Scan(&isMember); err != nil {
		ss.logger.Error("MLS publish-commit: failed to check membership",
			slog.String("guild_id", guildID),
			slog.String("user_id", req.UserID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to verify membership")
		return
	}
	if !isMember {
		writeMLSError(w, http.StatusForbidden, "not_member", "User is not a guild member")
		return
	}

	// Store the commit.
	id := models.NewULID().String()
	_, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO mls_commits (id, channel_id, sender_id, epoch, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		id, channelID, req.UserID, req.Epoch, req.Data,
	)
	if err != nil {
		ss.logger.Error("MLS publish-commit: failed to store commit",
			slog.String("channel_id", channelID),
			slog.String("sender_id", req.UserID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to store commit")
		return
	}

	// Update the group state epoch.
	_, err = ss.fed.pool.Exec(ctx,
		`INSERT INTO mls_group_states (channel_id, epoch, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (channel_id) DO UPDATE SET
			epoch = GREATEST(mls_group_states.epoch, EXCLUDED.epoch),
			updated_at = now()`,
		channelID, req.Epoch,
	)
	if err != nil {
		ss.logger.Warn("MLS publish-commit: failed to update group state epoch",
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		// Non-fatal — the commit itself was stored successfully.
	}

	ss.logger.Info("MLS commit stored via federation",
		slog.String("commit_id", id),
		slog.String("channel_id", channelID),
		slog.String("sender_id", req.UserID),
		slog.Uint64("epoch", req.Epoch))

	writeMLSJSON(w, http.StatusCreated, mlsCommitResponse{
		ID:        id,
		ChannelID: channelID,
		SenderID:  req.UserID,
		Epoch:     req.Epoch,
		Data:      req.Data,
		CreatedAt: time.Now().UTC(),
	})
}

// HandleMLSGetGroupState handles GET /federation/v1/guilds/{guildID}/channels/{channelID}/mls/group-state.
// A remote instance queries the MLS group state for a channel. Read-only — just
// needs signed request verification and channel/guild ownership check.
func (ss *SyncService) HandleMLSGetGroupState(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID or channelID")
		return
	}

	ctx := r.Context()

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS get-group-state: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	var gs mlsGroupStateResponse
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT channel_id, epoch, tree_hash, updated_at
		 FROM mls_group_states WHERE channel_id = $1`,
		channelID,
	).Scan(&gs.ChannelID, &gs.Epoch, &gs.TreeHash, &gs.UpdatedAt)

	if err == pgx.ErrNoRows {
		writeMLSError(w, http.StatusNotFound, "no_group", "No MLS group state for this channel")
		return
	}
	if err != nil {
		ss.logger.Error("MLS get-group-state: query failed",
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to query group state")
		return
	}

	writeMLSJSON(w, http.StatusOK, gs)
}

// HandleMLSGetCommits handles GET /federation/v1/guilds/{guildID}/channels/{channelID}/mls/commits?since_epoch=N.
// A remote instance catches up on MLS commits for a channel since a given epoch.
func (ss *SyncService) HandleMLSGetCommits(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	guildID := chi.URLParam(r, "guildID")
	channelID := chi.URLParam(r, "channelID")
	if guildID == "" || channelID == "" {
		writeMLSError(w, http.StatusBadRequest, "missing_params", "Missing guildID or channelID")
		return
	}

	ctx := r.Context()

	// Verify channel belongs to a local guild.
	if err := ss.verifyChannelInLocalGuild(ctx, guildID, channelID); err != nil {
		ss.logger.Warn("MLS get-commits: channel verification failed",
			slog.String("guild_id", guildID),
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusNotFound, "not_found", "Channel not found in guild")
		return
	}

	// Parse since_epoch from query params.
	sinceEpoch := int64(0)
	if s := r.URL.Query().Get("since_epoch"); s != "" {
		fmt.Sscanf(s, "%d", &sinceEpoch)
	}

	rows, err := ss.fed.pool.Query(ctx,
		`SELECT id, channel_id, sender_id, epoch, data, created_at
		 FROM mls_commits
		 WHERE channel_id = $1 AND epoch >= $2
		 ORDER BY epoch ASC, created_at ASC
		 LIMIT 100`,
		channelID, sinceEpoch,
	)
	if err != nil {
		ss.logger.Error("MLS get-commits: query failed",
			slog.String("channel_id", channelID),
			slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to query commits")
		return
	}
	defer rows.Close()

	var commits []mlsCommitResponse
	for rows.Next() {
		var c mlsCommitResponse
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.SenderID, &c.Epoch, &c.Data, &c.CreatedAt); err != nil {
			ss.logger.Warn("MLS get-commits: failed to scan row", slog.String("error", err.Error()))
			continue
		}
		commits = append(commits, c)
	}
	if err := rows.Err(); err != nil {
		ss.logger.Error("MLS get-commits: rows iteration error", slog.String("error", err.Error()))
		writeMLSError(w, http.StatusInternalServerError, "internal_error", "Failed to read commits")
		return
	}

	if commits == nil {
		commits = []mlsCommitResponse{}
	}
	writeMLSJSON(w, http.StatusOK, commits)
}
