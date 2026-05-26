package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// --- Request/Response types for federated DM endpoints ---

// federatedDMCreateRequest is the signed payload for creating a DM mirror.
type federatedDMCreateRequest struct {
	ChannelID    string              `json:"channel_id"`    // remote channel ID
	ChannelType  string              `json:"channel_type"`  // "dm" or "group"
	Creator      federatedUserInfo   `json:"creator"`       // who initiated the DM
	RecipientIDs []string            `json:"recipient_ids"` // all participant user IDs
	Recipients   []federatedUserInfo `json:"recipients"`    // full user info for stubs
	GroupName    *string             `json:"group_name,omitempty"`
}

// federatedUserInfo carries the minimum user data for creating stub records.
type federatedUserInfo struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	InstanceDomain string  `json:"instance_domain"`
}

// federatedDMMessageRequest is the signed payload for delivering a DM message.
type federatedDMMessageRequest struct {
	RemoteChannelID string               `json:"remote_channel_id"`
	Message         federatedMessageData `json:"message"`
}

// federatedDMMessageDeleteRequest is the signed payload for deleting a DM message.
type federatedDMMessageDeleteRequest struct {
	RemoteChannelID string `json:"remote_channel_id"`
	MessageID       string `json:"message_id"`
}

// federatedDMReactionRequest is the signed payload for mutating a DM reaction.
type federatedDMReactionRequest struct {
	RemoteChannelID string `json:"remote_channel_id"`
	MessageID       string `json:"message_id"`
	UserID          string `json:"user_id"`
	Emoji           string `json:"emoji"`
}

// federatedMessageData carries the message content for federation.
type federatedMessageData struct {
	ID                  string                `json:"id"`
	AuthorID            string                `json:"author_id"`
	Content             string                `json:"content"`
	Nonce               *string               `json:"nonce,omitempty"`
	MessageType         string                `json:"message_type,omitempty"`
	Flags               int                   `json:"flags,omitempty"`
	ReplyToIDs          []string              `json:"reply_to_ids,omitempty"`
	MentionUserIDs      []string              `json:"mention_user_ids,omitempty"`
	MentionRoleIDs      []string              `json:"mention_role_ids,omitempty"`
	MentionHere         bool                  `json:"mention_here,omitempty"`
	ThreadID            *string               `json:"thread_id,omitempty"`
	MasqueradeName      *string               `json:"masquerade_name,omitempty"`
	MasqueradeAvatar    *string               `json:"masquerade_avatar,omitempty"`
	MasqueradeColor     *string               `json:"masquerade_color,omitempty"`
	Encrypted           bool                  `json:"encrypted,omitempty"`
	EncryptionSessionID *string               `json:"encryption_session_id,omitempty"`
	VoiceDurationMs     *int                  `json:"voice_duration_ms,omitempty"`
	VoiceWaveform       json.RawMessage       `json:"voice_waveform,omitempty"`
	Attachments         []federatedAttachment `json:"attachments,omitempty"`
	Embeds              []federatedEmbed      `json:"embeds,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	EditedAt            *time.Time            `json:"edited_at,omitempty"`
}

// federatedDMRecipientRequest is the signed payload for adding/removing recipients.
type federatedDMRecipientRequest struct {
	RemoteChannelID string            `json:"remote_channel_id"`
	User            federatedUserInfo `json:"user"`
}

// HandleFederatedDMCreate handles POST /federation/v1/dm/create — creates a
// local mirror of a DM channel initiated on a remote instance.
func (ss *SyncService) HandleFederatedDMCreate(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMCreateRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.ChannelID == "" || req.ChannelType == "" || len(req.RecipientIDs) == 0 || req.Creator.ID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}
	if req.ChannelType != "dm" && req.ChannelType != "group" {
		http.Error(w, "Invalid channel_type", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check at least one recipient is a local user.
	var localCount int
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE id = ANY($1) AND instance_id = $2`,
		req.RecipientIDs, ss.fed.instanceID,
	).Scan(&localCount)
	if err != nil || localCount == 0 {
		http.Error(w, "No local recipients", http.StatusBadRequest)
		return
	}

	// Ensure remote user stubs exist for all non-local participants.
	allUsers := make([]federatedUserInfo, 0, len(req.Recipients)+1)
	allUsers = append(allUsers, req.Recipients...)
	allUsers = append(allUsers, req.Creator)
	for _, u := range allUsers {
		if u.InstanceDomain == "" || u.InstanceDomain == ss.fed.domain {
			// Skip local users — never overwrite local user data from remote claims.
			continue
		}
		// Resolve the correct instance ID for this user's domain.
		var instanceID string
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE domain = $1`, u.InstanceDomain,
		).Scan(&instanceID); err != nil {
			ss.logger.Warn("unknown instance for federated user stub",
				slog.String("domain", u.InstanceDomain),
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
			continue
		}
		ss.ensureRemoteUserStub(ctx, instanceID, u)
	}

	// Create the local mirror channel inside a transaction with duplicate check.
	localChannelID := models.NewULID().String()
	now := time.Now()
	created := false

	tx, err := ss.fed.pool.Begin(ctx)
	if err != nil {
		ss.logger.Error("failed to begin tx for federated DM", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Check if we already have a local mirror for this remote channel.
	var existingLocalID string
	err = tx.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_dm_channel_map
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2
		 FOR UPDATE`,
		req.ChannelID, senderID,
	).Scan(&existingLocalID)
	if err == nil {
		// Already mirrored — return existing channel.
		if err := tx.Commit(ctx); err != nil {
			ss.logger.Error("failed to commit (mirror lookup)", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"channel_id": existingLocalID})
		return
	}
	if err != pgx.ErrNoRows {
		ss.logger.Error("failed to check DM mirror", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// For 1:1 DMs, also check if a DM already exists between the same pair.
	if req.ChannelType == "dm" && len(req.RecipientIDs) > 0 {
		var existingDM string
		err = tx.QueryRow(ctx,
			`SELECT c.id FROM channels c
			 JOIN channel_recipients cr1 ON c.id = cr1.channel_id AND cr1.user_id = $1
			 JOIN channel_recipients cr2 ON c.id = cr2.channel_id AND cr2.user_id = $2
			 WHERE c.channel_type = 'dm'
			 LIMIT 1
			 FOR UPDATE OF c`,
			req.Creator.ID, req.RecipientIDs[0],
		).Scan(&existingDM)
		if err == nil {
			// DM already exists — map the remote channel to this local one and register peer.
			tx.Exec(ctx,
				`INSERT INTO federation_dm_channel_map (local_channel_id, remote_channel_id, remote_instance_id, created_at)
				 VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
				existingDM, req.ChannelID, senderID, now,
			)
			tx.Exec(ctx,
				`INSERT INTO federation_channel_peers (channel_id, instance_id)
				 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				existingDM, senderID,
			)
			if err := tx.Commit(ctx); err != nil {
				ss.logger.Error("failed to commit (DM pair reuse)", slog.String("error", err.Error()))
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"channel_id": existingDM})
			return
		}
		if err != pgx.ErrNoRows {
			ss.logger.Error("failed to check existing DM pair", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	if req.ChannelType == "group" {
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, instance_id, channel_type, name, owner_id, created_at)
			 VALUES ($1, $2, 'group', $3, $4, $5)`,
			localChannelID, senderID, req.GroupName, req.Creator.ID, now,
		)
	} else {
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, instance_id, channel_type, created_at) VALUES ($1, $2, 'dm', $3)`,
			localChannelID, senderID, now,
		)
	}
	if err != nil {
		ss.logger.Error("failed to create federated DM channel", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	created = true

	// Add all participants as channel recipients.
	allIDs := append([]string{req.Creator.ID}, req.RecipientIDs...)
	for _, uid := range allIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO channel_recipients (channel_id, user_id, joined_at)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			localChannelID, uid, now,
		)
		if err != nil {
			ss.logger.Error("failed to add federated DM recipient",
				slog.String("user_id", uid), slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	// Store channel mirror mapping.
	_, err = tx.Exec(ctx,
		`INSERT INTO federation_dm_channel_map (local_channel_id, remote_channel_id, remote_instance_id, created_at)
		 VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		localChannelID, req.ChannelID, senderID, now,
	)
	if err != nil {
		ss.logger.Error("failed to store channel mirror", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Register the sender instance as a channel peer.
	_, err = tx.Exec(ctx,
		`INSERT INTO federation_channel_peers (channel_id, instance_id)
		 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		localChannelID, senderID,
	)
	if err != nil {
		ss.logger.Error("failed to register channel peer", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		ss.logger.Error("failed to commit federated DM", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if created {
		// Publish CHANNEL_CREATE for local WebSocket clients.
		channel := map[string]interface{}{
			"id":           localChannelID,
			"channel_type": req.ChannelType,
			"name":         req.GroupName,
			"created_at":   now,
		}
		ss.bus.PublishChannelEvent(ctx, events.SubjectChannelCreate, "CHANNEL_CREATE", localChannelID, channel)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"channel_id": localChannelID})
}

// HandleFederatedDMMessage handles POST /federation/v1/dm/message — receives
// a message sent in a DM on the remote instance and persists it locally.
func (ss *SyncService) HandleFederatedDMMessage(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMMessageRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.RemoteChannelID == "" || req.Message.ID == "" || req.Message.AuthorID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if len(req.Message.Attachments) > 0 && !ss.federationFeatureEnabled(ctx, "federated_attachments") {
		http.Error(w, "Federated attachments disabled", http.StatusForbidden)
		return
	}

	// Look up the local channel via mirror mapping.
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_dm_channel_map
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2 LIMIT 1`,
		req.RemoteChannelID, senderID,
	).Scan(&localChannelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Unknown channel", http.StatusNotFound)
		} else {
			ss.logger.Error("failed to lookup channel mirror", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	// Insert the message into the local database.
	createdAt := req.Message.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if req.Message.MessageType == "" {
		req.Message.MessageType = models.MessageTypeDefault
	}

	tx, err := ss.fed.pool.Begin(ctx)
	if err != nil {
		ss.logger.Error("failed to begin federated DM message tx",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, instance_id, content, nonce, message_type, flags,
		                       reply_to_ids, mention_user_ids, mention_role_ids, mention_here,
		                       thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		                       encrypted, encryption_session_id, voice_duration_ms, voice_waveform, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		 ON CONFLICT (id) DO NOTHING`,
		req.Message.ID, localChannelID, req.Message.AuthorID, senderID, req.Message.Content, req.Message.Nonce,
		req.Message.MessageType, req.Message.Flags, req.Message.ReplyToIDs, req.Message.MentionUserIDs,
		req.Message.MentionRoleIDs, req.Message.MentionHere, req.Message.ThreadID, req.Message.MasqueradeName,
		req.Message.MasqueradeAvatar, req.Message.MasqueradeColor, req.Message.Encrypted,
		req.Message.EncryptionSessionID, req.Message.VoiceDurationMs, req.Message.VoiceWaveform, createdAt,
	)
	if err != nil {
		ss.logger.Error("failed to persist federated DM message",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// If the message already existed (retry), skip duplicate broadcast.
	if tag.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			ss.logger.Error("failed to commit duplicate federated DM message tx",
				slog.String("message_id", req.Message.ID),
				slog.String("error", err.Error()),
			)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
		return
	}

	if err := ss.upsertFederatedAttachments(ctx, tx, senderID, req.Message.ID, req.Message.Attachments); err != nil {
		ss.logger.Error("failed to persist federated DM attachments",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if err := ss.upsertFederatedEmbeds(ctx, tx, senderID, req.Message.ID, req.Message.Embeds); err != nil {
		ss.logger.Error("failed to persist federated DM embeds",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Update channel's last_message_id.
	if _, err := tx.Exec(ctx,
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`,
		req.Message.ID, localChannelID); err != nil {
		ss.logger.Warn("failed to update last_message_id",
			slog.String("channel_id", localChannelID),
			slog.String("error", err.Error()),
		)
	}
	if err := tx.Commit(ctx); err != nil {
		ss.logger.Error("failed to commit federated DM message tx",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Publish MESSAGE_CREATE for local WebSocket clients.
	// Include attachment/embed metadata so clients can render remote media.
	msg := map[string]interface{}{
		"id":           req.Message.ID,
		"channel_id":   localChannelID,
		"author_id":    req.Message.AuthorID,
		"content":      req.Message.Content,
		"instance_id":  senderID,
		"message_type": req.Message.MessageType,
		"flags":        req.Message.Flags,
		"created_at":   createdAt,
	}
	if len(req.Message.Attachments) > 0 {
		msg["attachments"] = req.Message.Attachments
	}
	if len(req.Message.Embeds) > 0 {
		msg["embeds"] = req.Message.Embeds
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", localChannelID, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// HandleFederatedDMMessageUpdate handles POST /federation/v1/dm/message/update.
func (ss *SyncService) HandleFederatedDMMessageUpdate(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMMessageRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.RemoteChannelID == "" || req.Message.ID == "" || req.Message.AuthorID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if len(req.Message.Attachments) > 0 && !ss.federationFeatureEnabled(ctx, "federated_attachments") {
		http.Error(w, "Federated attachments disabled", http.StatusForbidden)
		return
	}
	if !ss.validateSenderUser(ctx, w, senderID, req.Message.AuthorID) {
		return
	}
	localChannelID, ok := ss.lookupFederatedDMLocalChannel(ctx, w, req.RemoteChannelID, senderID)
	if !ok {
		return
	}

	editedAt := time.Now().UTC()
	if req.Message.EditedAt != nil && !req.Message.EditedAt.IsZero() {
		editedAt = *req.Message.EditedAt
	}

	tx, err := ss.fed.pool.Begin(ctx)
	if err != nil {
		ss.logger.Error("failed to begin federated DM message update tx",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE messages
		    SET content = $1, edited_at = $2, flags = $3,
		        mention_user_ids = $4, mention_role_ids = $5, mention_here = $6,
		        masquerade_name = $7, masquerade_avatar = $8, masquerade_color = $9,
		        encrypted = $10, encryption_session_id = $11,
		        voice_duration_ms = $12, voice_waveform = $13
		  WHERE id = $14 AND channel_id = $15`,
		req.Message.Content, editedAt, req.Message.Flags,
		req.Message.MentionUserIDs, req.Message.MentionRoleIDs, req.Message.MentionHere,
		req.Message.MasqueradeName, req.Message.MasqueradeAvatar, req.Message.MasqueradeColor,
		req.Message.Encrypted, req.Message.EncryptionSessionID,
		req.Message.VoiceDurationMs, req.Message.VoiceWaveform,
		req.Message.ID, localChannelID,
	)
	if err != nil {
		ss.logger.Error("failed to persist federated DM message update",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}
	if len(req.Message.Attachments) > 0 {
		if err := ss.upsertFederatedAttachments(ctx, tx, senderID, req.Message.ID, req.Message.Attachments); err != nil {
			ss.logger.Error("failed to persist federated DM update attachments",
				slog.String("message_id", req.Message.ID),
				slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}
	if len(req.Message.Embeds) > 0 {
		if err := ss.upsertFederatedEmbeds(ctx, tx, senderID, req.Message.ID, req.Message.Embeds); err != nil {
			ss.logger.Error("failed to persist federated DM update embeds",
				slog.String("message_id", req.Message.ID),
				slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		ss.logger.Error("failed to commit federated DM message update tx",
			slog.String("message_id", req.Message.ID),
			slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	msg := map[string]interface{}{
		"id":               req.Message.ID,
		"channel_id":       localChannelID,
		"author_id":        req.Message.AuthorID,
		"content":          req.Message.Content,
		"instance_id":      senderID,
		"message_type":     req.Message.MessageType,
		"flags":            req.Message.Flags,
		"mention_user_ids": req.Message.MentionUserIDs,
		"mention_role_ids": req.Message.MentionRoleIDs,
		"mention_here":     req.Message.MentionHere,
		"edited_at":        editedAt,
	}
	if len(req.Message.Attachments) > 0 {
		msg["attachments"] = req.Message.Attachments
	}
	if len(req.Message.Embeds) > 0 {
		msg["embeds"] = req.Message.Embeds
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageUpdate, "MESSAGE_UPDATE", localChannelID, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// HandleFederatedDMMessageDelete handles POST /federation/v1/dm/message/delete.
func (ss *SyncService) HandleFederatedDMMessageDelete(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMMessageDeleteRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.RemoteChannelID == "" || req.MessageID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	localChannelID, ok := ss.lookupFederatedDMLocalChannel(ctx, w, req.RemoteChannelID, senderID)
	if !ok {
		return
	}

	if _, err := ss.fed.pool.Exec(ctx,
		`DELETE FROM messages WHERE id = $1 AND channel_id = $2`,
		req.MessageID, localChannelID); err != nil {
		ss.logger.Error("failed to delete federated DM message",
			slog.String("message_id", req.MessageID),
			slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageDelete, "MESSAGE_DELETE", localChannelID, map[string]string{
		"id":          req.MessageID,
		"channel_id":  localChannelID,
		"instance_id": senderID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedDMReactionAdd handles POST /federation/v1/dm/reaction/add.
func (ss *SyncService) HandleFederatedDMReactionAdd(w http.ResponseWriter, r *http.Request) {
	ss.handleFederatedDMReaction(w, r, true)
}

// HandleFederatedDMReactionRemove handles POST /federation/v1/dm/reaction/remove.
func (ss *SyncService) HandleFederatedDMReactionRemove(w http.ResponseWriter, r *http.Request) {
	ss.handleFederatedDMReaction(w, r, false)
}

func (ss *SyncService) handleFederatedDMReaction(w http.ResponseWriter, r *http.Request, add bool) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMReactionRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.RemoteChannelID == "" || req.MessageID == "" || req.UserID == "" || req.Emoji == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if !ss.validateSenderUser(ctx, w, senderID, req.UserID) {
		return
	}
	localChannelID, ok := ss.lookupFederatedDMLocalChannel(ctx, w, req.RemoteChannelID, senderID)
	if !ok {
		return
	}

	var exists bool
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1 AND channel_id = $2)`,
		req.MessageID, localChannelID).Scan(&exists); err != nil || !exists {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if add {
		if _, err := ss.fed.pool.Exec(ctx,
			`INSERT INTO reactions (message_id, user_id, emoji, instance_id, created_at)
			 VALUES ($1, $2, $3, $4, now()) ON CONFLICT DO NOTHING`,
			req.MessageID, req.UserID, req.Emoji, senderID); err != nil {
			ss.logger.Error("failed to add federated DM reaction", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionAdd, "MESSAGE_REACTION_ADD", localChannelID, map[string]string{
			"message_id":  req.MessageID,
			"channel_id":  localChannelID,
			"user_id":     req.UserID,
			"emoji":       req.Emoji,
			"instance_id": senderID,
		})
	} else {
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
			req.MessageID, req.UserID, req.Emoji); err != nil {
			ss.logger.Error("failed to remove federated DM reaction", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		ss.bus.PublishChannelEvent(ctx, events.SubjectMessageReactionDel, "MESSAGE_REACTION_REMOVE", localChannelID, map[string]string{
			"message_id":  req.MessageID,
			"channel_id":  localChannelID,
			"user_id":     req.UserID,
			"emoji":       req.Emoji,
			"instance_id": senderID,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedDMRecipientAdd handles POST /federation/v1/dm/recipient-add —
// adds a user to a local mirror of a group DM.
func (ss *SyncService) HandleFederatedDMRecipientAdd(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMRecipientRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.RemoteChannelID == "" || req.User.ID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the local channel.
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_dm_channel_map
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2 LIMIT 1`,
		req.RemoteChannelID, senderID,
	).Scan(&localChannelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Unknown channel", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	// Ensure the user stub exists (only for remote domains).
	if req.User.InstanceDomain != "" && req.User.InstanceDomain != ss.fed.domain {
		var instanceID string
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE domain = $1`, req.User.InstanceDomain,
		).Scan(&instanceID); err != nil {
			ss.logger.Warn("unknown instance for recipient stub",
				slog.String("domain", req.User.InstanceDomain),
				slog.String("user_id", req.User.ID),
				slog.String("error", err.Error()),
			)
		} else {
			ss.ensureRemoteUserStub(ctx, instanceID, req.User)
		}
	}

	// Add the recipient.
	_, err = ss.fed.pool.Exec(ctx,
		`INSERT INTO channel_recipients (channel_id, user_id, joined_at)
		 VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`,
		localChannelID, req.User.ID,
	)
	if err != nil {
		ss.logger.Error("failed to add federated DM recipient", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleFederatedDMRecipientRemove handles POST /federation/v1/dm/recipient-remove —
// removes a user from a local mirror of a group DM.
func (ss *SyncService) HandleFederatedDMRecipientRemove(w http.ResponseWriter, r *http.Request) {
	signed, senderID, ok := ss.verifyFederationRequest(w, r)
	if !ok {
		return
	}

	var req federatedDMRecipientRequest
	if err := json.Unmarshal(signed.Payload, &req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if req.RemoteChannelID == "" || req.User.ID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the local channel.
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_dm_channel_map
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2 LIMIT 1`,
		req.RemoteChannelID, senderID,
	).Scan(&localChannelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Unknown channel", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	// Remove the recipient.
	_, err = ss.fed.pool.Exec(ctx,
		`DELETE FROM channel_recipients WHERE channel_id = $1 AND user_id = $2`,
		localChannelID, req.User.ID,
	)
	if err != nil {
		ss.logger.Error("failed to remove federated DM recipient", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helper methods ---

// verifyFederationRequest reads the request body, verifies the signature,
// checks federation permissions, and returns the signed payload and sender ID.
// Returns false if verification failed (response already written).
func (ss *SyncService) verifyFederationRequest(w http.ResponseWriter, r *http.Request) (*SignedPayload, string, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return nil, "", false
	}

	var signed SignedPayload
	if err := json.Unmarshal(body, &signed); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return nil, "", false
	}

	// Look up sender's public key.
	var publicKeyPEM string
	err = ss.fed.pool.QueryRow(r.Context(),
		`SELECT public_key FROM instances WHERE id = $1`, signed.SenderID,
	).Scan(&publicKeyPEM)
	if err != nil {
		if err == pgx.ErrNoRows {
			ss.logger.Warn("federation: unknown sender instance",
				slog.String("sender_id", signed.SenderID),
				slog.String("remote", r.RemoteAddr))
			http.Error(w, "Unknown sender instance", http.StatusForbidden)
		} else {
			ss.logger.Error("failed to look up sender", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return nil, "", false
	}

	// Verify signature.
	valid, err := VerifySignature(publicKeyPEM, signed.Payload, signed.Signature)
	if err != nil || !valid {
		ss.logger.Warn("federation: invalid signature",
			slog.String("sender_id", signed.SenderID),
			slog.String("remote", r.RemoteAddr),
			slog.String("path", r.URL.Path))
		http.Error(w, "Invalid signature", http.StatusForbidden)
		return nil, "", false
	}

	// Check timestamp freshness.
	if msg := validateTimestamp(signed.Timestamp); msg != "" {
		ss.logger.Warn("federation request rejected: stale timestamp",
			slog.String("sender_id", signed.SenderID),
			slog.String("detail", msg))
		http.Error(w, "Stale or future timestamp", http.StatusBadRequest)
		return nil, "", false
	}

	// Verify source IP.
	if ipMsg := ss.fed.verifySourceIP(r, signed.SenderID); ipMsg != "" {
		ss.logger.Warn("federation source IP mismatch",
			slog.String("sender_id", signed.SenderID),
			slog.String("detail", ipMsg))
		if ss.fed.enforceIPCheck {
			http.Error(w, "Source IP mismatch", http.StatusForbidden)
			return nil, "", false
		}
	}

	// Check federation is allowed.
	allowed, err := ss.fed.IsFederationAllowed(r.Context(), signed.SenderID)
	if err != nil || !allowed {
		ss.logger.Warn("federation: not allowed",
			slog.String("sender_id", signed.SenderID),
			slog.String("remote", r.RemoteAddr),
			slog.String("path", r.URL.Path))
		http.Error(w, "Federation not allowed", http.StatusForbidden)
		return nil, "", false
	}

	return &signed, signed.SenderID, true
}

func (ss *SyncService) lookupFederatedDMLocalChannel(ctx context.Context, w http.ResponseWriter, remoteChannelID, remoteInstanceID string) (string, bool) {
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_dm_channel_map
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2 LIMIT 1`,
		remoteChannelID, remoteInstanceID,
	).Scan(&localChannelID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Unknown channel", http.StatusNotFound)
		} else {
			ss.logger.Error("failed to lookup channel mirror", slog.String("error", err.Error()))
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return "", false
	}
	return localChannelID, true
}

// NotifyFederatedDM sends a DM creation notification to a remote instance.
// This is called by the users handler when a local user creates a DM with a remote user.
func (ss *SyncService) NotifyFederatedDM(ctx context.Context, remoteDomain, localChannelID, channelType, creatorID string, recipientIDs []string, groupName *string) error {
	// Validate remote domain before constructing URL.
	if err := ValidateFederationDomain(remoteDomain); err != nil {
		return fmt.Errorf("invalid remote domain %q: %w", remoteDomain, err)
	}

	// Look up creator user info.
	var creator federatedUserInfo
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT id, username, display_name, avatar_id FROM users WHERE id = $1`, creatorID,
	).Scan(&creator.ID, &creator.Username, &creator.DisplayName, &creator.AvatarID)
	if err != nil {
		return fmt.Errorf("looking up creator: %w", err)
	}
	creator.InstanceDomain = ss.fed.domain

	// Look up all recipient user info.
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT id, username, display_name, avatar_id, instance_id FROM users WHERE id = ANY($1)`,
		recipientIDs,
	)
	if err != nil {
		return fmt.Errorf("looking up recipients: %w", err)
	}
	defer rows.Close()

	var recipients []federatedUserInfo
	for rows.Next() {
		var u federatedUserInfo
		var instanceID string
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarID, &instanceID); err != nil {
			ss.logger.Warn("failed to scan recipient row",
				slog.String("error", err.Error()),
			)
			continue
		}
		if instanceID == ss.fed.instanceID {
			u.InstanceDomain = ss.fed.domain
		} else {
			// Look up the domain for remote users.
			var domain string
			if err := ss.fed.pool.QueryRow(ctx,
				`SELECT domain FROM instances WHERE id = $1`, instanceID,
			).Scan(&domain); err != nil {
				ss.logger.Warn("failed to look up recipient instance domain",
					slog.String("instance_id", instanceID),
					slog.String("user_id", u.ID),
					slog.String("error", err.Error()),
				)
			}
			u.InstanceDomain = domain
		}
		recipients = append(recipients, u)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating recipients: %w", err)
	}

	// Build the federation request.
	req := federatedDMCreateRequest{
		ChannelID:    localChannelID,
		ChannelType:  channelType,
		Creator:      creator,
		RecipientIDs: recipientIDs,
		Recipients:   recipients,
		GroupName:    groupName,
	}

	signed, err := ss.fed.Sign(req)
	if err != nil {
		return fmt.Errorf("signing DM create request: %w", err)
	}

	body, err := json.Marshal(signed)
	if err != nil {
		return fmt.Errorf("marshaling signed payload: %w", err)
	}

	url := fmt.Sprintf("https://%s/federation/v1/dm/create", remoteDomain)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending DM create request to %s: %w", remoteDomain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("remote instance returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Store channel mirror mapping (remote channel ID from response).
	var result struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.ChannelID != "" {
		// Look up the remote instance ID.
		var remoteInstanceID string
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE domain = $1`, remoteDomain,
		).Scan(&remoteInstanceID); err != nil {
			ss.logger.Warn("failed to look up remote instance ID for mirror mapping",
				slog.String("domain", remoteDomain),
				slog.String("error", err.Error()),
			)
		}

		if remoteInstanceID != "" {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO federation_dm_channel_map (local_channel_id, remote_channel_id, remote_instance_id, created_at)
				 VALUES ($1, $2, $3, now()) ON CONFLICT DO NOTHING`,
				localChannelID, result.ChannelID, remoteInstanceID,
			); err != nil {
				ss.logger.Warn("failed to store channel mirror mapping",
					slog.String("channel_id", localChannelID),
					slog.String("error", err.Error()),
				)
			}
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO federation_channel_peers (channel_id, instance_id)
				 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				localChannelID, remoteInstanceID,
			); err != nil {
				ss.logger.Warn("failed to store channel peer mapping",
					slog.String("channel_id", localChannelID),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	ss.logger.Info("notified remote instance of federated DM",
		slog.String("domain", remoteDomain),
		slog.String("channel_id", localChannelID),
	)

	return nil
}

// ensureRemoteUserStub creates or updates a user stub for a remote user.
// Only updates users that belong to the expected instance to prevent cross-instance overwrites.
func (ss *SyncService) ensureRemoteUserStub(ctx context.Context, instanceID string, u federatedUserInfo) {
	// Check if user already exists and which instance it belongs to.
	var existingInstanceID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT instance_id FROM users WHERE id = $1`, u.ID,
	).Scan(&existingInstanceID)
	if err == nil {
		// User exists — only update if it belongs to the expected instance.
		if existingInstanceID != instanceID {
			ss.logger.Warn("refusing to update user stub: instance mismatch",
				slog.String("user_id", u.ID),
				slog.String("expected_instance", instanceID),
				slog.String("actual_instance", existingInstanceID),
			)
			return
		}
		// Safe to update display_name and avatar.
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE users SET display_name = $1, avatar_id = $2 WHERE id = $3 AND instance_id = $4`,
			u.DisplayName, u.AvatarID, u.ID, instanceID,
		); err != nil {
			ss.logger.Warn("failed to update remote user stub",
				slog.String("user_id", u.ID),
				slog.String("error", err.Error()),
			)
		}
		return
	}
	if err != pgx.ErrNoRows {
		// Real database error — log and bail out, don't mask with an INSERT.
		ss.logger.Warn("failed to look up user stub",
			slog.String("user_id", u.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	// User doesn't exist — create stub (race-safe with ON CONFLICT).
	_, err = ss.fed.pool.Exec(ctx,
		`INSERT INTO users (id, instance_id, username, display_name, avatar_id, status_presence, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'offline', now())
		 ON CONFLICT (id) DO UPDATE SET
		   display_name = EXCLUDED.display_name,
		   avatar_id = EXCLUDED.avatar_id
		 WHERE users.instance_id = EXCLUDED.instance_id`,
		u.ID, instanceID, u.Username, u.DisplayName, u.AvatarID,
	)
	if err != nil {
		ss.logger.Warn("failed to create remote user stub",
			slog.String("user_id", u.ID),
			slog.String("username", u.Username),
			slog.String("error", err.Error()),
		)
	}
}
