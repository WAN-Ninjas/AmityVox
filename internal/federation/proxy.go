package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// ForwardToHomeInstance sends a signed management request to the guild's home
// instance. It looks up the instance domain, builds a manageRequest envelope,
// signs it with Ed25519, and POSTs to the remote /federation/v1/guilds/{guildID}/manage
// endpoint. Returns the parsed manageResponse, or an error if the request failed.
func (ss *SyncService) ForwardToHomeInstance(
	ctx context.Context,
	guildID string,
	instanceID string,
	action string,
	userID string,
	data interface{},
) (*manageResponse, error) {
	// 1. Look up the home instance's domain.
	var domain string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, instanceID,
	).Scan(&domain)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("home instance %s not found", instanceID)
		}
		return nil, fmt.Errorf("looking up home instance %s: %w", instanceID, err)
	}

	// 2. Marshal the action data into json.RawMessage for the manage request.
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling action data: %w", err)
	}

	innerPayload := manageRequest{
		Action: action,
		UserID: userID,
		Data:   json.RawMessage(dataJSON),
	}

	// 3. Sign the inner payload.
	signed, err := ss.fed.Sign(innerPayload)
	if err != nil {
		return nil, fmt.Errorf("signing management request: %w", err)
	}

	// 4. POST to the remote manage endpoint with a 5-second timeout.
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://%s/federation/v1/guilds/%s/manage", domain, guildID)

	body, err := json.Marshal(signed)
	if err != nil {
		return nil, fmt.Errorf("marshaling signed payload: %w", err)
	}

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating management request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending management request to %s: %w", domain, err)
	}
	defer resp.Body.Close()

	// 5. Read and parse the response body.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", domain, err)
	}

	var mresp manageResponse
	if err := json.Unmarshal(respBody, &mresp); err != nil {
		return nil, fmt.Errorf("decoding management response from %s (status %d): %w", domain, resp.StatusCode, err)
	}

	return &mresp, nil
}

// ProxyToHomeInstance is a convenience wrapper for HTTP handlers. It checks if
// the guild is federated (instanceID is non-nil), and if so, forwards the
// management request to the home instance and writes the response back to the
// caller. Returns true if the request was forwarded (the handler should return
// immediately), or false if the guild is local (the handler should continue
// with its normal local logic).
func (ss *SyncService) ProxyToHomeInstance(
	w http.ResponseWriter,
	r *http.Request,
	guildID string,
	instanceID *string,
	action string,
	userID string,
	data interface{},
) bool {
	// Local guild — handler should continue with local logic.
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false
	}

	resp, err := ss.ForwardToHomeInstance(r.Context(), guildID, *instanceID, action, userID, data)
	if err != nil {
		ss.logger.Error("failed to forward management request to home instance",
			slog.String("guild_id", guildID),
			slog.String("instance_id", *instanceID),
			slog.String("action", action),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to forward request to home instance",
			},
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")

	if !resp.OK {
		// The remote instance returned an error — pass it through.
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_REMOTE_ERROR",
				"message": resp.Error,
			},
		})
		return true
	}

	// Success — write the remote response data as the response body.
	if resp.Data != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]json.RawMessage{
			"data": resp.Data,
		})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
	return true
}

// ProxyReadGuildMembers checks if the guild is federated and, if so, fetches
// the member list from the home instance. Returns true if proxied (handler
// should return), false if local (handler continues).
func (ss *SyncService) ProxyReadGuildMembers(w http.ResponseWriter, r *http.Request, guildID string) bool {
	ctx := r.Context()
	userID := auth.UserIDFromContext(ctx)

	// Check if the guild is federated.
	var instanceID *string
	var instanceDomain string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.instance_id, COALESCE(i.domain, '')
		 FROM guilds g LEFT JOIN instances i ON i.id = g.instance_id
		 WHERE g.id = $1`, guildID,
	).Scan(&instanceID, &instanceDomain)
	if err != nil {
		// Guild not found or DB error — let the handler deal with it.
		return false
	}
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false // local guild
	}

	// Fetch members from the home instance.
	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/members", instanceDomain, guildID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildMembersRequest{UserID: userID})
	if err != nil {
		ss.logger.Error("failed to proxy guild members read",
			slog.String("guild_id", guildID),
			slog.String("domain", instanceDomain),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to fetch members from home instance",
			},
		})
		return true
	}

	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return true
	}

	// Parse the flat remote response and transform into GuildMember shape.
	var remoteResp struct {
		Data []struct {
			UserID         string          `json:"user_id"`
			Username       string          `json:"username"`
			DisplayName    *string         `json:"display_name"`
			AvatarID       *string         `json:"avatar_id"`
			InstanceDomain string          `json:"instance_domain"`
			RoleIDs        json.RawMessage `json:"role_ids"`
			JoinedAt       time.Time       `json:"joined_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &remoteResp); err != nil {
		// Fallback: return raw response if parsing fails.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(respBody)
		return true
	}

	// Build domain → instance_id map for avatar proxy URLs.
	domainSet := map[string]bool{}
	for _, m := range remoteResp.Data {
		domain := m.InstanceDomain
		if domain == "" {
			domain = instanceDomain
		}
		domainSet[domain] = true
	}
	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}
	domainToID := map[string]string{}
	if len(domains) > 0 {
		rows, err := ss.fed.pool.Query(ctx,
			`SELECT domain, id FROM instances WHERE domain = ANY($1)`, domains)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var d, id string
				if rows.Scan(&d, &id) == nil {
					domainToID[d] = id
				}
			}
		}
	}

	members := make([]map[string]interface{}, 0, len(remoteResp.Data))
	for _, m := range remoteResp.Data {
		var roles []string
		if len(m.RoleIDs) > 0 {
			json.Unmarshal(m.RoleIDs, &roles)
		}
		if roles == nil {
			roles = []string{}
		}

		domain := m.InstanceDomain
		if domain == "" {
			domain = instanceDomain
		}

		user := map[string]interface{}{
			"id":              m.UserID,
			"username":        m.Username,
			"display_name":    m.DisplayName,
			"avatar_id":       m.AvatarID,
			"instance_domain": domain,
		}
		if id := domainToID[domain]; id != "" {
			user["instance_id"] = id
		}
		members = append(members, map[string]interface{}{
			"guild_id":  guildID,
			"user_id":   m.UserID,
			"nickname":  nil,
			"avatar_id": nil,
			"joined_at": m.JoinedAt,
			"roles":     roles,
			"user":      user,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": members})
	return true
}

// ProxyReadChannelMessages checks if the channel belongs to a federated guild
// and, if so, fetches messages from the home instance. Returns true if proxied
// (handler should return), false if local (handler continues).
func (ss *SyncService) ProxyReadChannelMessages(w http.ResponseWriter, r *http.Request, channelID string) bool {
	ctx := r.Context()
	userID := auth.UserIDFromContext(ctx)

	// Check if the channel's guild is federated.
	var guildID string
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.id, g.instance_id
		 FROM channels c JOIN guilds g ON g.id = c.guild_id
		 WHERE c.id = $1`, channelID,
	).Scan(&guildID, &instanceID)
	if err != nil {
		return false // channel not found or not in a guild — let handler deal with it
	}
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false // local guild
	}

	// Look up the home instance domain.
	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, *instanceID,
	).Scan(&instanceDomain); err != nil {
		return false
	}

	// Extract query params for pagination.
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	payload := federatedGuildMessagesRequest{
		UserID: userID,
		Before: r.URL.Query().Get("before"),
		After:  r.URL.Query().Get("after"),
		Limit:  limit,
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages",
		instanceDomain, guildID, channelID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		ss.logger.Error("failed to proxy channel messages read",
			slog.String("channel_id", channelID),
			slog.String("guild_id", guildID),
			slog.String("domain", instanceDomain),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to fetch messages from home instance",
			},
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
	return true
}

// ProxyCreateChannelMessage checks if the channel belongs to a federated guild
// and, if so, forwards message creation to the home instance. Returns true if
// proxied (handler should return), false if local (handler continues).
func (ss *SyncService) ProxyCreateChannelMessage(
	w http.ResponseWriter,
	r *http.Request,
	channelID string,
	userID string,
	content string,
	opts map[string]interface{},
) bool {
	ctx := r.Context()

	// Check if the channel's guild is federated.
	var guildID string
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.id, g.instance_id
		 FROM channels c JOIN guilds g ON g.id = c.guild_id
		 WHERE c.id = $1`, channelID,
	).Scan(&guildID, &instanceID)
	if err != nil {
		return false // channel not found or not in a guild
	}
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false // local guild
	}

	// Look up the home instance domain. Fail closed — if we can't resolve the
	// domain, return 502 rather than falling through to a local write.
	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, *instanceID,
	).Scan(&instanceDomain); err != nil {
		ss.logger.Error("failed to resolve home instance domain for message proxy",
			slog.String("instance_id", *instanceID),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to resolve home instance",
			},
		})
		return true
	}

	// Build the federation message create payload.
	payload := federatedGuildPostMessageRequest{
		UserID:  userID,
		Content: content,
	}
	if v, ok := opts["nonce"].(string); ok {
		payload.Nonce = v
	}
	if v, ok := opts["reply_to_ids"].([]string); ok {
		payload.ReplyToIDs = v
	}
	if v, ok := opts["attachment_ids"].([]string); ok && len(v) > 0 {
		attachments, err := ss.federatedAttachmentsForUploadIDs(ctx, userID, v)
		if err != nil {
			ss.logger.Warn("invalid attachments for federated message proxy",
				slog.String("channel_id", channelID),
				slog.String("user_id", userID),
				slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "INVALID_ATTACHMENTS",
					"message": "One or more attachments are invalid, already linked, or not owned by you",
				},
			})
			return true
		}
		payload.Attachments = attachments
	}
	if v, ok := opts["expires_in_seconds"].(int); ok {
		payload.ExpiresInSeconds = &v
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages/create",
		instanceDomain, guildID, channelID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		ss.logger.Error("failed to proxy message creation to home instance",
			slog.String("channel_id", channelID),
			slog.String("guild_id", guildID),
			slog.String("domain", instanceDomain),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to send message to home instance",
			},
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
	return true
}

// ProxyAckChannel checks if the channel belongs to a federated guild and, if
// so, forwards read acknowledgement to the home instance. Returns true if
// proxied (handler should return), false if local.
func (ss *SyncService) ProxyAckChannel(w http.ResponseWriter, r *http.Request, channelID string) bool {
	ctx := r.Context()
	userID := auth.UserIDFromContext(ctx)

	var guildID string
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.id, g.instance_id
		 FROM channels c JOIN guilds g ON g.id = c.guild_id
		 WHERE c.id = $1`, channelID,
	).Scan(&guildID, &instanceID)
	if err != nil {
		return false
	}
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false
	}

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, *instanceID,
	).Scan(&instanceDomain); err != nil {
		ss.logger.Error("failed to resolve home instance domain for ack proxy",
			slog.String("instance_id", *instanceID),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to resolve home instance",
			},
		})
		return true
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/ack",
		instanceDomain, guildID, channelID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, federatedGuildAckChannelRequest{UserID: userID})
	if err != nil {
		ss.logger.Error("failed to proxy channel ack to home instance",
			slog.String("channel_id", channelID),
			slog.String("guild_id", guildID),
			slog.String("domain", instanceDomain),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to acknowledge channel on home instance",
			},
		})
		return true
	}

	if statusCode >= 200 && statusCode < 300 {
		var lastMessageID *string
		if err := ss.fed.pool.QueryRow(ctx,
			`SELECT last_message_id FROM channels WHERE id = $1`, channelID,
		).Scan(&lastMessageID); err == nil && lastMessageID != nil {
			if _, err := ss.fed.pool.Exec(ctx,
				`INSERT INTO read_state (user_id, channel_id, last_read_id, mention_count)
				 VALUES ($1, $2, $3, 0)
				 ON CONFLICT (user_id, channel_id) DO UPDATE SET last_read_id = $3, mention_count = 0`,
				userID, channelID, lastMessageID,
			); err != nil {
				ss.logger.Warn("failed to mirror proxied channel ack locally",
					slog.String("channel_id", channelID),
					slog.String("user_id", userID),
					slog.String("error", err.Error()))
			}
		}
	}

	if statusCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
	return true
}

// ProxyTranslateChannelMessage checks if the channel belongs to a federated
// guild and, if so, forwards message translation to the home instance. Returns
// true if proxied (handler should return), false if local.
func (ss *SyncService) ProxyTranslateChannelMessage(w http.ResponseWriter, r *http.Request, channelID string, messageID string) bool {
	ctx := r.Context()
	userID := auth.UserIDFromContext(ctx)

	var guildID string
	var instanceID *string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT g.id, g.instance_id
		 FROM channels c JOIN guilds g ON g.id = c.guild_id
		 WHERE c.id = $1`, channelID,
	).Scan(&guildID, &instanceID)
	if err != nil {
		return false
	}
	if instanceID == nil || *instanceID == ss.fed.instanceID {
		return false
	}

	var instanceDomain string
	if err := ss.fed.pool.QueryRow(ctx,
		`SELECT domain FROM instances WHERE id = $1`, *instanceID,
	).Scan(&instanceDomain); err != nil {
		ss.logger.Error("failed to resolve home instance domain for translation proxy",
			slog.String("instance_id", *instanceID),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to resolve home instance",
			},
		})
		return true
	}

	var req struct {
		TargetLang string `json:"target_lang"`
		Force      bool   `json:"force,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "INVALID_JSON",
				"message": "Invalid request body",
			},
		})
		return true
	}

	payload := federatedGuildTranslateMessageRequest{
		UserID:     userID,
		TargetLang: req.TargetLang,
		Force:      req.Force,
	}

	remoteURL := fmt.Sprintf("https://%s/federation/v1/guilds/%s/channels/%s/messages/%s/translate",
		instanceDomain, guildID, channelID, messageID)
	respBody, statusCode, err := ss.signAndPost(ctx, remoteURL, payload)
	if err != nil {
		ss.logger.Error("failed to proxy message translation to home instance",
			slog.String("channel_id", channelID),
			slog.String("message_id", messageID),
			slog.String("guild_id", guildID),
			slog.String("domain", instanceDomain),
			slog.String("error", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "FEDERATION_PROXY_ERROR",
				"message": "Failed to translate message on home instance",
			},
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(respBody)
	return true
}

func (ss *SyncService) federatedAttachmentsForUploadIDs(ctx context.Context, userID string, attachmentIDs []string) ([]federatedAttachment, error) {
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash,
		        alt_text, nsfw, description, instance_id, created_at
		   FROM attachments
		  WHERE id = ANY($1) AND uploader_id = $2 AND message_id IS NULL`,
		attachmentIDs, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]models.Attachment, 0, len(attachmentIDs))
	for rows.Next() {
		var att models.Attachment
		if err := rows.Scan(
			&att.ID, &att.MessageID, &att.UploaderID, &att.Filename, &att.ContentType,
			&att.SizeBytes, &att.Width, &att.Height, &att.DurationSeconds, &att.S3Bucket,
			&att.S3Key, &att.Blurhash, &att.AltText, &att.NSFW, &att.Description,
			&att.InstanceID, &att.CreatedAt,
		); err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(attachments) != len(attachmentIDs) {
		return nil, fmt.Errorf("attachment ownership validation failed")
	}

	byID := make(map[string]models.Attachment, len(attachments))
	for _, att := range attachments {
		byID[att.ID] = att
	}
	ordered := make([]models.Attachment, 0, len(attachmentIDs))
	for _, id := range attachmentIDs {
		att, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("attachment %s not found", id)
		}
		ordered = append(ordered, att)
	}

	return federationAttachmentsFromModels(ordered), nil
}
