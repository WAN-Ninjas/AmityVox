package channels

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"hello": "world"})

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
	if data["hello"] != "world" {
		t.Errorf("data[hello] = %v, want %q", data["hello"], "world")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteError(w, http.StatusBadRequest, "test_code", "Test message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' key in response")
	}
	if errObj["code"] != "test_code" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "test_code")
	}
	if errObj["message"] != "Test message" {
		t.Errorf("error.message = %v, want %q", errObj["message"], "Test message")
	}
}

func TestWriteJSON_StatusCreated(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusCreated, []string{"a", "b"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("expected 'data' to be an array")
	}
	if len(data) != 2 {
		t.Errorf("data length = %d, want 2", len(data))
	}
}

func TestWriteError_InternalServer(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestWriteJSON_EmptySlice(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, make([]string, 0))

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

func TestCreateMessageRequest_Defaults(t *testing.T) {
	raw := `{"content": "hello"}`
	var req createMessageRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Content == nil || *req.Content != "hello" {
		t.Error("expected content to be 'hello'")
	}
	if req.MentionEveryone {
		t.Error("expected mention_everyone to default to false")
	}
	if len(req.AttachmentIDs) != 0 {
		t.Error("expected empty attachment_ids")
	}
	if len(req.ReplyToIDs) != 0 {
		t.Error("expected empty reply_to_ids")
	}
}

func TestUpdateChannelRequest_NilFields(t *testing.T) {
	raw := `{}`
	var req updateChannelRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != nil {
		t.Error("expected nil Name")
	}
	if req.Topic != nil {
		t.Error("expected nil Topic")
	}
	if req.Position != nil {
		t.Error("expected nil Position")
	}
	if req.NSFW != nil {
		t.Error("expected nil NSFW")
	}
	if req.SlowmodeSeconds != nil {
		t.Error("expected nil SlowmodeSeconds")
	}
}

func TestPermissionOverrideRequest(t *testing.T) {
	raw := `{"target_type":"role","permissions_allow":1024,"permissions_deny":2048}`
	var req permissionOverrideRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.TargetType != "role" {
		t.Errorf("target_type = %q, want %q", req.TargetType, "role")
	}
	if req.PermissionsAllow != 1024 {
		t.Errorf("permissions_allow = %d, want 1024", req.PermissionsAllow)
	}
	if req.PermissionsDeny != 2048 {
		t.Errorf("permissions_deny = %d, want 2048", req.PermissionsDeny)
	}
}
