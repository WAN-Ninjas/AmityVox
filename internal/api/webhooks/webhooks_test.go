package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"id": "msg123"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'data' key in response")
	}
	if data["id"] != "msg123" {
		t.Errorf("data[id] = %v, want %q", data["id"], "msg123")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteError(w, http.StatusNotFound, "webhook_not_found", "Unknown webhook")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' key in response")
	}
	if errObj["code"] != "webhook_not_found" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "webhook_not_found")
	}
	if errObj["message"] != "Unknown webhook" {
		t.Errorf("error.message = %v, want %q", errObj["message"], "Unknown webhook")
	}
}

func TestExecuteWebhookRequest_Unmarshal(t *testing.T) {
	raw := `{"content": "Hello from webhook!", "username": "Bot", "avatar_url": "https://example.com/avatar.png"}`
	var req executeWebhookRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Content != "Hello from webhook!" {
		t.Errorf("content = %q, want %q", req.Content, "Hello from webhook!")
	}
	if req.Username == nil || *req.Username != "Bot" {
		t.Error("expected username to be 'Bot'")
	}
	if req.AvatarURL == nil || *req.AvatarURL != "https://example.com/avatar.png" {
		t.Error("expected avatar_url to be set")
	}
}

func TestExecuteWebhookRequest_MinimalFields(t *testing.T) {
	raw := `{"content": "test"}`
	var req executeWebhookRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Content != "test" {
		t.Errorf("content = %q, want %q", req.Content, "test")
	}
	if req.Username != nil {
		t.Error("expected nil username")
	}
	if req.AvatarURL != nil {
		t.Error("expected nil avatar_url")
	}
}

func TestWriteError_StatusCodes(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "invalid_token"},
		{http.StatusBadRequest, "wrong_type"},
		{http.StatusBadRequest, "empty_content"},
		{http.StatusBadRequest, "content_too_long"},
		{http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range tests {
		w := httptest.NewRecorder()
		apiutil.WriteError(w, tc.status, tc.code, "test")
		if w.Code != tc.status {
			t.Errorf("status = %d, want %d for code %s", w.Code, tc.status, tc.code)
		}
	}
}
