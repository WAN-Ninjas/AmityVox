package invites

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"code": "abc123"})

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
	if data["code"] != "abc123" {
		t.Errorf("data[code] = %v, want %q", data["code"], "abc123")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")

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
	if errObj["code"] != "invite_not_found" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "invite_not_found")
	}
	if errObj["message"] != "Invite not found" {
		t.Errorf("error.message = %v, want %q", errObj["message"], "Invite not found")
	}
}

func TestWriteError_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
	}{
		{"forbidden", http.StatusForbidden, "banned"},
		{"gone", http.StatusGone, "invite_expired"},
		{"conflict", http.StatusConflict, "already_member"},
		{"internal", http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			apiutil.WriteError(w, tc.status, tc.code, "test")
			if w.Code != tc.status {
				t.Errorf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestWriteJSON_NestedData(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"invite":       map[string]string{"code": "abc"},
		"guild_name":   "Test Guild",
		"member_count": 42,
	})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'data' key in response")
	}
	if data["guild_name"] != "Test Guild" {
		t.Errorf("guild_name = %v, want %q", data["guild_name"], "Test Guild")
	}
}
