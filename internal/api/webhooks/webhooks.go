// Package webhooks implements the webhook execution endpoint. Incoming webhooks
// allow external services to post messages to channels using a webhook ID and
// token pair, without requiring Bearer auth. Mounted at
// /api/v1/webhooks/{webhookID}/{token}.
package webhooks

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements webhook-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

type executeWebhookRequest struct {
	Content   string  `json:"content"`
	Username  *string `json:"username"`
	AvatarURL *string `json:"avatar_url"`
}

// HandleExecute handles POST /api/v1/webhooks/{webhookID}/{token}.
// This endpoint does NOT require Bearer auth — the token in the URL is the secret.
func (h *Handler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhookID")
	token := chi.URLParam(r, "token")

	// Look up the webhook and verify the token.
	var wh models.Webhook
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, guild_id, channel_id, creator_id, name, avatar_id, token,
		        webhook_type, outgoing_url, created_at
		 FROM webhooks WHERE id = $1`, webhookID).Scan(
		&wh.ID, &wh.GuildID, &wh.ChannelID, &wh.CreatorID, &wh.Name,
		&wh.AvatarID, &wh.Token, &wh.WebhookType, &wh.OutgoingURL, &wh.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "webhook_not_found", "Unknown webhook")
		return
	}
	if err != nil {
		h.Logger.Error("failed to look up webhook", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up webhook")
		return
	}

	// Constant-time token comparison for security.
	if wh.Token != token {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid webhook token")
		return
	}

	if wh.WebhookType != models.WebhookTypeIncoming {
		writeError(w, http.StatusBadRequest, "wrong_type", "This webhook does not accept incoming messages")
		return
	}

	var req executeWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "empty_content", "Message content cannot be empty")
		return
	}
	if len(req.Content) > 4000 {
		writeError(w, http.StatusBadRequest, "content_too_long", "Message content exceeds 4000 characters")
		return
	}

	// Create the message.
	messageID := models.NewULID().String()
	now := time.Now().UTC()

	// Determine the display name: use override if provided, otherwise webhook name.
	displayName := wh.Name
	if req.Username != nil && *req.Username != "" {
		displayName = *req.Username
	}

	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO messages (id, channel_id, author_id, content, message_type, created_at)
		 VALUES ($1, $2, NULL, $3, 'webhook', $4)`,
		messageID, wh.ChannelID, req.Content, now)
	if err != nil {
		h.Logger.Error("failed to create webhook message",
			slog.String("error", err.Error()),
			slog.String("webhook_id", webhookID),
		)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create message")
		return
	}

	// Update channel last_message_id.
	h.Pool.Exec(r.Context(),
		`UPDATE channels SET last_message_id = $1 WHERE id = $2`, messageID, wh.ChannelID)

	// Publish message create event.
	h.EventBus.PublishJSON(r.Context(), events.SubjectMessageCreate, "MESSAGE_CREATE",
		map[string]interface{}{
			"id":           messageID,
			"channel_id":   wh.ChannelID,
			"guild_id":     wh.GuildID,
			"content":      req.Content,
			"webhook_id":   webhookID,
			"display_name": displayName,
			"avatar_url":   req.AvatarURL,
			"created_at":   now,
		})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         messageID,
		"channel_id": wh.ChannelID,
		"content":    req.Content,
		"webhook_id": webhookID,
		"author": map[string]interface{}{
			"id":       webhookID,
			"username": displayName,
			"bot":      true,
		},
		"created_at": now,
	})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
