package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"id": "user123"})

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
	if data["id"] != "user123" {
		t.Errorf("data[id] = %v, want %q", data["id"], "user123")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusNotFound, "user_not_found", "User not found")

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
	if errObj["code"] != "user_not_found" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "user_not_found")
	}
}

func TestUpdateSelfRequest_PartialFields(t *testing.T) {
	raw := `{"display_name": "Alice"}`
	var req updateSelfRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.DisplayName == nil || *req.DisplayName != "Alice" {
		t.Error("expected display_name to be 'Alice'")
	}
	if req.AvatarID != nil {
		t.Error("expected nil avatar_id")
	}
	if req.StatusText != nil {
		t.Error("expected nil status_text")
	}
	if req.Bio != nil {
		t.Error("expected nil bio")
	}
}

func TestUpdateSelfRequest_Empty(t *testing.T) {
	raw := `{}`
	var req updateSelfRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.DisplayName != nil {
		t.Error("expected nil display_name")
	}
}

func TestWriteJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, make([]map[string]string, 0))

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("expected 'data' to be an array")
	}
	if len(data) != 0 {
		t.Errorf("data length = %d, want 0", len(data))
	}
}

func TestWriteError_Various(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "invalid_body"},
		{http.StatusForbidden, "blocked"},
		{http.StatusConflict, "already_friends"},
		{http.StatusUnauthorized, "unauthorized"},
	}

	for _, tc := range tests {
		w := httptest.NewRecorder()
		writeError(w, tc.status, tc.code, "test")
		if w.Code != tc.status {
			t.Errorf("status = %d, want %d for code %s", w.Code, tc.status, tc.code)
		}
	}
}
