package guilds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"name": "test-guild"})

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
	if data["name"] != "test-guild" {
		t.Errorf("data[name] = %v, want %q", data["name"], "test-guild")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteError(w, http.StatusForbidden, "not_owner", "Only the owner can do this")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' key in response")
	}
	if errObj["code"] != "not_owner" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "not_owner")
	}
}

func TestGenerateInviteCode(t *testing.T) {
	code1 := generateInviteCode()
	code2 := generateInviteCode()

	if len(code1) != 12 {
		t.Errorf("invite code length = %d, want 12", len(code1))
	}
	if code1 == code2 {
		t.Error("two generated codes should not be identical")
	}

	// Verify hex encoding (only hex chars).
	for _, c := range code1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("unexpected character %c in invite code", c)
		}
	}
}

func TestCreateGuildRequest_Unmarshal(t *testing.T) {
	raw := `{"name": "My Guild", "description": "A test guild"}`
	var req createGuildRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "My Guild" {
		t.Errorf("name = %q, want %q", req.Name, "My Guild")
	}
	if req.Description == nil || *req.Description != "A test guild" {
		t.Error("expected description to be 'A test guild'")
	}
}

func TestCreateGuildRequest_NilDescription(t *testing.T) {
	raw := `{"name": "My Guild"}`
	var req createGuildRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Description != nil {
		t.Error("expected nil description")
	}
}

func TestUpdateGuildRequest_PartialFields(t *testing.T) {
	raw := `{"name": "New Name"}`
	var req updateGuildRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name == nil || *req.Name != "New Name" {
		t.Error("expected name to be 'New Name'")
	}
	if req.Description != nil {
		t.Error("expected nil description")
	}
	if req.IconID != nil {
		t.Error("expected nil icon_id")
	}
	if req.NSFW != nil {
		t.Error("expected nil nsfw")
	}
}

func TestCreateChannelRequest_AllFields(t *testing.T) {
	categoryID := "cat123"
	topic := "General chat"
	pos := 5
	nsfw := true
	raw := `{"name":"general","channel_type":"text","category_id":"cat123","topic":"General chat","position":5,"nsfw":true}`
	var req createChannelRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "general" {
		t.Errorf("name = %q, want %q", req.Name, "general")
	}
	if req.ChannelType != "text" {
		t.Errorf("channel_type = %q, want %q", req.ChannelType, "text")
	}
	if req.CategoryID == nil || *req.CategoryID != categoryID {
		t.Error("expected category_id to be 'cat123'")
	}
	if req.Topic == nil || *req.Topic != topic {
		t.Error("expected topic to be 'General chat'")
	}
	if req.Position == nil || *req.Position != pos {
		t.Error("expected position to be 5")
	}
	if req.NSFW == nil || *req.NSFW != nsfw {
		t.Error("expected nsfw to be true")
	}
}

func TestUpdateMemberRequest_Defaults(t *testing.T) {
	raw := `{}`
	var req updateMemberRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Nickname != nil {
		t.Error("expected nil nickname")
	}
	if req.Deaf != nil {
		t.Error("expected nil deaf")
	}
	if req.Mute != nil {
		t.Error("expected nil mute")
	}
	if req.TimeoutUntil != nil {
		t.Error("expected nil timeout_until")
	}
	if req.Roles != nil {
		t.Error("expected nil roles")
	}
}

func TestCreateRoleRequest(t *testing.T) {
	raw := `{"name":"Moderator","color":"#FF0000","hoist":true,"mentionable":false,"position":3,"permissions_allow":1024}`
	var req createRoleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "Moderator" {
		t.Errorf("name = %q, want %q", req.Name, "Moderator")
	}
	if req.Color == nil || *req.Color != "#FF0000" {
		t.Error("expected color to be '#FF0000'")
	}
	if req.Hoist == nil || !*req.Hoist {
		t.Error("expected hoist to be true")
	}
	if req.Mentionable == nil || *req.Mentionable {
		t.Error("expected mentionable to be false")
	}
	if req.Position == nil || *req.Position != 3 {
		t.Error("expected position to be 3")
	}
	if !req.PermissionsAllow.Set || req.PermissionsAllow.Value != 1024 {
		t.Error("expected permissions_allow to be 1024")
	}
}

func TestBanRequest(t *testing.T) {
	raw := `{"reason": "Spam"}`
	var req banRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Reason == nil || *req.Reason != "Spam" {
		t.Error("expected reason to be 'Spam'")
	}
}

func TestWriteJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	apiutil.WriteJSON(w, http.StatusOK, nil)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, exists := resp["data"]; !exists {
		t.Error("expected 'data' key even with nil data")
	}
}
