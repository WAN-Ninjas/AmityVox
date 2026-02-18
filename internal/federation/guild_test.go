package federation

import (
	"encoding/json"
	"testing"
)

func TestFederatedGuildJoinRequest_JSON(t *testing.T) {
	displayName := "Alice Wonderland"
	avatarID := "avatar-123"

	req := federatedGuildJoinRequest{
		UserID:         "user-1",
		Username:       "alice",
		DisplayName:    &displayName,
		AvatarID:       &avatarID,
		InstanceDomain: "instance-a.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildJoinRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", decoded.UserID, "user-1")
	}
	if decoded.Username != "alice" {
		t.Errorf("Username = %q, want %q", decoded.Username, "alice")
	}
	if decoded.DisplayName == nil || *decoded.DisplayName != "Alice Wonderland" {
		t.Errorf("DisplayName = %v, want %q", decoded.DisplayName, "Alice Wonderland")
	}
	if decoded.AvatarID == nil || *decoded.AvatarID != "avatar-123" {
		t.Errorf("AvatarID = %v, want %q", decoded.AvatarID, "avatar-123")
	}
	if decoded.InstanceDomain != "instance-a.com" {
		t.Errorf("InstanceDomain = %q, want %q", decoded.InstanceDomain, "instance-a.com")
	}
}

func TestFederatedGuildJoinRequest_MinimalFields(t *testing.T) {
	req := federatedGuildJoinRequest{
		UserID:         "user-2",
		Username:       "bob",
		InstanceDomain: "instance-b.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildJoinRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.DisplayName != nil {
		t.Errorf("DisplayName should be nil, got %v", decoded.DisplayName)
	}
	if decoded.AvatarID != nil {
		t.Errorf("AvatarID should be nil, got %v", decoded.AvatarID)
	}
}

func TestFederatedGuildInviteRequest_JSON(t *testing.T) {
	displayName := "Charlie"

	req := federatedGuildInviteRequest{
		InviteCode:     "abc123",
		UserID:         "user-3",
		Username:       "charlie",
		DisplayName:    &displayName,
		InstanceDomain: "remote.example.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildInviteRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.InviteCode != "abc123" {
		t.Errorf("InviteCode = %q, want %q", decoded.InviteCode, "abc123")
	}
	if decoded.UserID != "user-3" {
		t.Errorf("UserID = %q, want %q", decoded.UserID, "user-3")
	}
	if decoded.Username != "charlie" {
		t.Errorf("Username = %q, want %q", decoded.Username, "charlie")
	}
	if decoded.DisplayName == nil || *decoded.DisplayName != "Charlie" {
		t.Errorf("DisplayName = %v, want %q", decoded.DisplayName, "Charlie")
	}
	if decoded.InstanceDomain != "remote.example.com" {
		t.Errorf("InstanceDomain = %q, want %q", decoded.InstanceDomain, "remote.example.com")
	}
}

func TestFederatedGuildLeaveRequest_JSON(t *testing.T) {
	req := federatedGuildLeaveRequest{
		UserID: "user-4",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildLeaveRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "user-4" {
		t.Errorf("UserID = %q, want %q", decoded.UserID, "user-4")
	}
}

func TestFederatedGuildMessagesRequest_JSON(t *testing.T) {
	req := federatedGuildMessagesRequest{
		UserID: "user-5",
		Before: "msg-100",
		Limit:  50,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildMessagesRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "user-5" {
		t.Errorf("UserID = %q, want %q", decoded.UserID, "user-5")
	}
	if decoded.Before != "msg-100" {
		t.Errorf("Before = %q, want %q", decoded.Before, "msg-100")
	}
	if decoded.After != "" {
		t.Errorf("After = %q, want empty", decoded.After)
	}
	if decoded.Limit != 50 {
		t.Errorf("Limit = %d, want 50", decoded.Limit)
	}
}

func TestFederatedGuildMessagesRequest_AfterPagination(t *testing.T) {
	req := federatedGuildMessagesRequest{
		UserID: "user-6",
		After:  "msg-200",
		Limit:  25,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildMessagesRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.After != "msg-200" {
		t.Errorf("After = %q, want %q", decoded.After, "msg-200")
	}
	if decoded.Before != "" {
		t.Errorf("Before = %q, want empty", decoded.Before)
	}
}

func TestFederatedGuildPostMessageRequest_JSON(t *testing.T) {
	req := federatedGuildPostMessageRequest{
		UserID:  "user-7",
		Content: "Hello from a remote guild!",
		Nonce:   "nonce-123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildPostMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "user-7" {
		t.Errorf("UserID = %q, want %q", decoded.UserID, "user-7")
	}
	if decoded.Content != "Hello from a remote guild!" {
		t.Errorf("Content = %q, want %q", decoded.Content, "Hello from a remote guild!")
	}
	if decoded.Nonce != "nonce-123" {
		t.Errorf("Nonce = %q, want %q", decoded.Nonce, "nonce-123")
	}
}

func TestFederatedGuildPostMessageRequest_NoNonce(t *testing.T) {
	req := federatedGuildPostMessageRequest{
		UserID:  "user-8",
		Content: "No nonce message",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedGuildPostMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Nonce != "" {
		t.Errorf("Nonce = %q, want empty", decoded.Nonce)
	}
}

func TestGuildPreviewResponse_JSON(t *testing.T) {
	desc := "A cool guild"
	iconID := "icon-456"

	resp := guildPreviewResponse{
		ID:           "guild-1",
		Name:         "Test Guild",
		Description:  &desc,
		IconID:       &iconID,
		MemberCount:  42,
		OnlineCount:  10,
		Discoverable: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded guildPreviewResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != "guild-1" {
		t.Errorf("ID = %q, want %q", decoded.ID, "guild-1")
	}
	if decoded.Name != "Test Guild" {
		t.Errorf("Name = %q, want %q", decoded.Name, "Test Guild")
	}
	if decoded.Description == nil || *decoded.Description != "A cool guild" {
		t.Errorf("Description = %v, want %q", decoded.Description, "A cool guild")
	}
	if decoded.MemberCount != 42 {
		t.Errorf("MemberCount = %d, want 42", decoded.MemberCount)
	}
	if !decoded.Discoverable {
		t.Error("Discoverable should be true")
	}
}

func TestGuildJoinResponse_JSON(t *testing.T) {
	iconID := "icon-789"

	resp := guildJoinResponse{
		GuildID:      "guild-2",
		Name:         "Federation Guild",
		IconID:       &iconID,
		MemberCount:  100,
		ChannelsJSON: json.RawMessage(`[{"id":"ch-1","name":"general"}]`),
		RolesJSON:    json.RawMessage(`[{"id":"role-1","name":"@everyone"}]`),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded guildJoinResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.GuildID != "guild-2" {
		t.Errorf("GuildID = %q, want %q", decoded.GuildID, "guild-2")
	}
	if decoded.Name != "Federation Guild" {
		t.Errorf("Name = %q, want %q", decoded.Name, "Federation Guild")
	}
	if decoded.IconID == nil || *decoded.IconID != "icon-789" {
		t.Errorf("IconID = %v, want %q", decoded.IconID, "icon-789")
	}
	if decoded.MemberCount != 100 {
		t.Errorf("MemberCount = %d, want 100", decoded.MemberCount)
	}

	// Verify channels JSON preserved.
	var channels []map[string]interface{}
	if err := json.Unmarshal(decoded.ChannelsJSON, &channels); err != nil {
		t.Fatalf("unmarshal channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels length = %d, want 1", len(channels))
	}
	if channels[0]["id"] != "ch-1" {
		t.Errorf("channels[0].id = %v, want %q", channels[0]["id"], "ch-1")
	}

	// Verify roles JSON preserved.
	var roles []map[string]interface{}
	if err := json.Unmarshal(decoded.RolesJSON, &roles); err != nil {
		t.Fatalf("unmarshal roles: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("roles length = %d, want 1", len(roles))
	}
	if roles[0]["name"] != "@everyone" {
		t.Errorf("roles[0].name = %v, want %q", roles[0]["name"], "@everyone")
	}
}

func TestGuildJoinResponse_NilOptionalFields(t *testing.T) {
	resp := guildJoinResponse{
		GuildID:      "guild-3",
		Name:         "Minimal Guild",
		MemberCount:  5,
		ChannelsJSON: json.RawMessage("[]"),
		RolesJSON:    json.RawMessage("[]"),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded guildJoinResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.IconID != nil {
		t.Errorf("IconID should be nil, got %v", decoded.IconID)
	}
	if decoded.Description != nil {
		t.Errorf("Description should be nil, got %v", decoded.Description)
	}
}
