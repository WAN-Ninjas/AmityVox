package admin

import (
	"net/http"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

type transcriptionConfig struct {
	EngineType     string `json:"engine_type"`
	EngineEndpoint string `json:"engine_endpoint"`
	SaveEnabled    bool   `json:"save_enabled"`
}

func (h *Handler) HandleGetTranscriptionConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	apiutil.WriteJSON(w, http.StatusOK, h.getTranscriptionConfig(r))
}

func (h *Handler) HandleUpdateTranscriptionConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		EngineType     *string `json:"engine_type"`
		EngineEndpoint *string `json:"engine_endpoint"`
		SaveEnabled    *bool   `json:"save_enabled"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.EngineType != nil {
		valid := map[string]bool{"none": true, "whisper": true, "llm": true, "custom": true}
		if !valid[*req.EngineType] {
			apiutil.WriteError(w, http.StatusBadRequest, "invalid_engine_type", "Engine type must be none, whisper, llm, or custom")
			return
		}
		if err := h.setInstanceSetting(r, "transcription_engine_type", *req.EngineType); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to save transcription engine type", err)
			return
		}
	}
	if req.EngineEndpoint != nil {
		if err := h.setInstanceSetting(r, "transcription_engine_endpoint", *req.EngineEndpoint); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to save transcription endpoint", err)
			return
		}
	}
	if req.SaveEnabled != nil {
		value := "false"
		if *req.SaveEnabled {
			value = "true"
		}
		if err := h.setInstanceSetting(r, "transcription_save_enabled", value); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to save transcription retention policy", err)
			return
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, h.getTranscriptionConfig(r))
}

func (h *Handler) getTranscriptionConfig(r *http.Request) transcriptionConfig {
	var engineType, endpoint, saveEnabled string
	_ = h.Pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'transcription_engine_type'), 'none')`).Scan(&engineType)
	_ = h.Pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'transcription_engine_endpoint'), '')`).Scan(&endpoint)
	_ = h.Pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'transcription_save_enabled'), 'false')`).Scan(&saveEnabled)
	return transcriptionConfig{
		EngineType:     engineType,
		EngineEndpoint: endpoint,
		SaveEnabled:    saveEnabled == "true",
	}
}

func (h *Handler) setInstanceSetting(r *http.Request, key, value string) error {
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO instance_settings (key, value, updated_at) VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}
