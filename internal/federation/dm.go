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
	ChannelID    string              `json:"channel_id"`     // remote channel ID
	ChannelType  string              `json:"channel_type"`   // "dm" or "group"
	Creator      federatedUserInfo   `json:"creator"`        // who initiated the DM
	RecipientIDs []string            `json:"recipient_ids"`  // all participant user IDs
	Recipients   []federatedUserInfo `json:"recipients"`     // full user info for stubs
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
	RemoteChannelID string                 `json:"remote_channel_id"`
	Message         federatedMessageData   `json:"message"`
}

// federatedMessageData carries the message content for federation.
type federatedMessageData struct {
	ID          string          `json:"id"`
	AuthorID    string          `json:"author_id"`
	Content     string          `json:"content"`
	Attachments json.RawMessage `json:"attachments,omitempty"`
	Embeds      json.RawMessage `json:"embeds,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
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

	// Create the local mirror channel.
	localChannelID := models.NewULID().String()
	now := time.Now()

	tx, err := ss.fed.pool.Begin(ctx)
	if err != nil {
		ss.logger.Error("failed to begin tx for federated DM", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	if req.ChannelType == "group" {
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, channel_type, name, owner_id, created_at) VALUES ($1, 'group', $2, $3, $4)`,
			localChannelID, req.GroupName, req.Creator.ID, now,
		)
	} else {
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, channel_type, created_at) VALUES ($1, 'dm', $2)`,
			localChannelID, now,
		)
	}
	if err != nil {
		ss.logger.Error("failed to create federated DM channel", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

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
		`INSERT INTO federation_channel_mirrors (local_channel_id, remote_channel_id, remote_instance_id, created_at)
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

	// Publish CHANNEL_CREATE for local WebSocket clients.
	channel := map[string]interface{}{
		"id":           localChannelID,
		"channel_type": req.ChannelType,
		"name":         req.GroupName,
		"created_at":   now,
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectChannelCreate, "CHANNEL_CREATE", localChannelID, channel)

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

	// Look up the local channel via mirror mapping.
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_channel_mirrors
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
		createdAt = time.Now()
	}

	tag, err := ss.fed.pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, author_id, content, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO NOTHING`,
		req.Message.ID, localChannelID, req.Message.AuthorID, req.Message.Content, createdAt,
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
		return
	}

	// Note: Attachments and embeds are stored in separate tables (attachments, embeds)
	// linked by message_id. For federated messages, the actual files live on the remote
	// instance's S3. We don't proxy or re-upload them — we just pass the metadata through
	// to WebSocket clients so they can render the remote URLs. Full attachment proxying
	// is a future enhancement.

	// Update channel's last_message_id.
	if _, err := ss.fed.pool.Exec(ctx,
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`,
		req.Message.ID, localChannelID); err != nil {
		ss.logger.Warn("failed to update last_message_id",
			slog.String("channel_id", localChannelID),
			slog.String("error", err.Error()),
		)
	}

	// Publish MESSAGE_CREATE for local WebSocket clients.
	// Include attachment/embed metadata so clients can render remote media.
	msg := map[string]interface{}{
		"id":         req.Message.ID,
		"channel_id": localChannelID,
		"author_id":  req.Message.AuthorID,
		"content":    req.Message.Content,
		"created_at": createdAt,
	}
	if req.Message.Attachments != nil {
		msg["attachments"] = req.Message.Attachments
	}
	if req.Message.Embeds != nil {
		msg["embeds"] = req.Message.Embeds
	}
	ss.bus.PublishChannelEvent(ctx, events.SubjectMessageCreate, "MESSAGE_CREATE", localChannelID, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
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
		`SELECT local_channel_id FROM federation_channel_mirrors
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
		`SELECT local_channel_id FROM federation_channel_mirrors
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
				`INSERT INTO federation_channel_mirrors (local_channel_id, remote_channel_id, remote_instance_id, created_at)
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
