// Package channels — translation.go implements the message translation endpoint.
// Uses LibreTranslate (self-hosted) for translation with a PostgreSQL-backed cache.
package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// translateRequest is the body for HandleTranslateMessage.
type translateRequest struct {
	TargetLang string `json:"target_lang"`
}

// libreTranslateRequest is the request body sent to the LibreTranslate API.
type libreTranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	Format string `json:"format"`
}

// libreTranslateResponse is the response from the LibreTranslate API.
type libreTranslateResponse struct {
	TranslatedText string `json:"translatedText"`
	DetectedLanguage struct {
		Confidence float64 `json:"confidence"`
		Language   string  `json:"language"`
	} `json:"detectedLanguage"`
}

// getTranslationConfig reads translation config from environment variables.
// Returns (enabled, apiURL, defaultTargetLang).
func getTranslationConfig() (bool, string, string) {
	enabled := os.Getenv("AMITYVOX_TRANSLATION_ENABLED")
	if enabled != "true" && enabled != "1" {
		return false, "", ""
	}
	apiURL := os.Getenv("AMITYVOX_TRANSLATION_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:5000"
	}
	defaultLang := os.Getenv("AMITYVOX_TRANSLATION_DEFAULT_LANG")
	if defaultLang == "" {
		defaultLang = "en"
	}
	return true, apiURL, defaultLang
}

// HandleTranslateMessage translates a message's content to the requested target language.
// POST /api/v1/channels/{channelID}/messages/{messageID}/translate
//
// Request body: {"target_lang": "es"}
// Response: {"data": {"message_id": "...", "source_lang": "en", "target_lang": "es", "translated_text": "..."}}
func (h *Handler) HandleTranslateMessage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")

	// Check that the user can view this channel.
	if !h.hasChannelPermission(r.Context(), channelID, userID, permissions.ViewChannel) {
		writeError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Check translation config.
	enabled, apiURL, defaultLang := getTranslationConfig()
	if !enabled {
		writeError(w, http.StatusBadRequest, "translation_disabled", "Translation is not enabled on this instance")
		return
	}

	// Parse request body.
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.TargetLang == "" {
		req.TargetLang = defaultLang
	}

	// Validate target language (basic check: 2-5 chars, lowercase).
	if len(req.TargetLang) < 2 || len(req.TargetLang) > 5 {
		writeError(w, http.StatusBadRequest, "invalid_lang", "Target language must be a 2-5 character language code")
		return
	}

	// Fetch the message content from the database.
	var content *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT content FROM messages WHERE id = $1 AND channel_id = $2`,
		messageID, channelID,
	).Scan(&content)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "message_not_found", "Message not found")
			return
		}
		h.Logger.Error("failed to fetch message for translation", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch message")
		return
	}
	if content == nil || *content == "" {
		writeError(w, http.StatusBadRequest, "no_content", "Message has no text content to translate")
		return
	}

	// Check the translation cache first.
	var cachedText string
	var cachedSourceLang string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT translated_text, source_lang FROM translation_cache
		 WHERE message_id = $1 AND target_lang = $2`,
		messageID, req.TargetLang,
	).Scan(&cachedText, &cachedSourceLang)
	if err == nil {
		// Cache hit — return cached translation.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message_id":      messageID,
			"source_lang":     cachedSourceLang,
			"target_lang":     req.TargetLang,
			"translated_text": cachedText,
			"cached":          true,
		})
		return
	}

	// Call LibreTranslate API.
	ltReq := libreTranslateRequest{
		Q:      *content,
		Source: "auto",
		Target: req.TargetLang,
		Format: "text",
	}
	body, err := json.Marshal(ltReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to prepare translation request")
		return
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Post(apiURL+"/translate", "application/json", bytes.NewReader(body))
	if err != nil {
		h.Logger.Error("failed to call LibreTranslate", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "translation_error", "Translation service is unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		h.Logger.Error("LibreTranslate returned error",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(respBody)),
		)
		writeError(w, http.StatusBadGateway, "translation_error",
			fmt.Sprintf("Translation service returned status %d", resp.StatusCode))
		return
	}

	var ltResp libreTranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ltResp); err != nil {
		h.Logger.Error("failed to decode LibreTranslate response", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "translation_error", "Failed to parse translation response")
		return
	}

	sourceLang := ltResp.DetectedLanguage.Language
	if sourceLang == "" {
		sourceLang = "auto"
	}

	// Cache the translation.
	cacheID := models.NewULID().String()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO translation_cache (id, message_id, source_lang, target_lang, translated_text, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (message_id, target_lang) DO UPDATE SET
		   translated_text = EXCLUDED.translated_text,
		   source_lang = EXCLUDED.source_lang`,
		cacheID, messageID, sourceLang, req.TargetLang, ltResp.TranslatedText, time.Now(),
	)
	if err != nil {
		// Log but do not fail — translation still succeeded.
		h.Logger.Warn("failed to cache translation", slog.String("error", err.Error()))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message_id":      messageID,
		"source_lang":     sourceLang,
		"target_lang":     req.TargetLang,
		"translated_text": ltResp.TranslatedText,
		"cached":          false,
	})
}
