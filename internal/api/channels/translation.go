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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// translateRequest is the body for HandleTranslateMessage.
type translateRequest struct {
	TargetLang string `json:"target_lang"`
	Force      bool   `json:"force,omitempty"`
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
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need VIEW_CHANNEL permission")
		return
	}

	// Check translation config.
	enabled, apiURL, defaultLang := getTranslationConfig()
	if !enabled {
		apiutil.WriteError(w, http.StatusBadRequest, "translation_disabled", "Translation is not enabled on this instance")
		return
	}

	// Parse request body.
	var req translateRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.TargetLang == "" {
		req.TargetLang = defaultLang
	}

	// Validate target language (basic check: 2-5 chars, lowercase).
	if len(req.TargetLang) < 2 || len(req.TargetLang) > 5 {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_lang", "Target language must be a 2-5 character language code")
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
			apiutil.WriteError(w, http.StatusNotFound, "message_not_found", "Message not found")
			return
		}
		apiutil.InternalError(w, h.Logger, "Failed to fetch message", err)
		return
	}
	if content == nil || *content == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "no_content", "Message has no text content to translate")
		return
	}

	// Check the translation cache first (skip if force=true for retry).
	if !req.Force {
		var cachedText string
		var cachedSourceLang string
		err = h.Pool.QueryRow(r.Context(),
			`SELECT translated_text, source_lang FROM translation_cache
			 WHERE message_id = $1 AND target_lang = $2`,
			messageID, req.TargetLang,
		).Scan(&cachedText, &cachedSourceLang)
		if err == nil {
			// Cache hit — return cached translation.
			apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"message_id":      messageID,
				"source_lang":     cachedSourceLang,
				"target_lang":     req.TargetLang,
				"translated_text": cachedText,
				"cached":          true,
			})
			return
		}
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
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to prepare translation request")
		return
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Post(apiURL+"/translate", "application/json", bytes.NewReader(body))
	if err != nil {
		h.Logger.Error("failed to call LibreTranslate", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusBadGateway, "translation_error", "Translation service is unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		h.Logger.Error("LibreTranslate returned error",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(respBody)),
		)
		apiutil.WriteError(w, http.StatusBadGateway, "translation_error",
			fmt.Sprintf("Translation service returned status %d", resp.StatusCode))
		return
	}

	var ltResp libreTranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ltResp); err != nil {
		h.Logger.Error("failed to decode LibreTranslate response", slog.String("error", err.Error()))
		apiutil.WriteError(w, http.StatusBadGateway, "translation_error", "Failed to parse translation response")
		return
	}

	// Validate translation output: detect garbage responses where a single word
	// is repeated many times (e.g. "MAINSTREAM MAINSTREAM MAINSTREAM...").
	// This happens when LibreTranslate language models fail to load or are corrupted.
	if isRepeatedWordGarbage(ltResp.TranslatedText) {
		h.Logger.Warn("LibreTranslate returned repeated-word garbage output",
			slog.String("output", ltResp.TranslatedText[:min(len(ltResp.TranslatedText), 100)]),
		)
		apiutil.WriteError(w, http.StatusBadGateway, "translation_error",
			"Translation service returned invalid output — check LibreTranslate configuration")
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

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message_id":      messageID,
		"source_lang":     sourceLang,
		"target_lang":     req.TargetLang,
		"translated_text": ltResp.TranslatedText,
		"cached":          false,
	})
}

// isRepeatedWordGarbage detects translation output that indicates a LibreTranslate
// model failure. Catches two patterns:
// 1. Space-separated: "MAINSTREAM MAINSTREAM MAINSTREAM"
// 2. Concatenated: "MAINSTREMAINSTREMAINSTRE..." (single long token with repeated substring)
func isRepeatedWordGarbage(text string) bool {
	// Pattern 1: all space-separated words are the same.
	words := strings.Fields(text)
	if len(words) >= 3 {
		first := strings.ToLower(words[0])
		allSame := true
		for _, w := range words[1:] {
			if strings.ToLower(w) != first {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}

	// Pattern 2: a short substring (3-20 chars) repeats 5+ times in the text.
	lower := strings.ToLower(text)
	if len(lower) < 30 {
		return false
	}
	for subLen := 3; subLen <= 20 && subLen <= len(lower)/5; subLen++ {
		sub := lower[:subLen]
		count := strings.Count(lower, sub)
		if count >= 5 && len(sub)*count >= len(lower)/2 {
			return true
		}
	}

	return false
}
