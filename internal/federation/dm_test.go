package federation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFederatedDMCreateRequest_JSON(t *testing.T) {
	name := "Test Group"
	req := federatedDMCreateRequest{
		ChannelID:   "ch-123",
		ChannelType: "group",
		Creator: federatedUserInfo{
			ID:             "user-1",
			Username:       "alice",
			InstanceDomain: "instance-a.com",
		},
		RecipientIDs: []string{"user-2", "user-3"},
		Recipients: []federatedUserInfo{
			{ID: "user-2", Username: "bob", InstanceDomain: "instance-b.com"},
			{ID: "user-3", Username: "charlie", InstanceDomain: "instance-a.com"},
		},
		GroupName: &name,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedDMCreateRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ChannelID != "ch-123" {
		t.Errorf("ChannelID = %q, want %q", decoded.ChannelID, "ch-123")
	}
	if decoded.ChannelType != "group" {
		t.Errorf("ChannelType = %q, want %q", decoded.ChannelType, "group")
	}
	if decoded.Creator.Username != "alice" {
		t.Errorf("Creator.Username = %q, want %q", decoded.Creator.Username, "alice")
	}
	if len(decoded.RecipientIDs) != 2 {
		t.Errorf("RecipientIDs length = %d, want 2", len(decoded.RecipientIDs))
	}
	if len(decoded.Recipients) != 2 {
		t.Errorf("Recipients length = %d, want 2", len(decoded.Recipients))
	}
	if decoded.GroupName == nil || *decoded.GroupName != "Test Group" {
		t.Errorf("GroupName = %v, want %q", decoded.GroupName, "Test Group")
	}
}

func TestFederatedDMCreateRequest_DM(t *testing.T) {
	req := federatedDMCreateRequest{
		ChannelID:   "ch-456",
		ChannelType: "dm",
		Creator: federatedUserInfo{
			ID:             "user-1",
			Username:       "alice",
			InstanceDomain: "instance-a.com",
		},
		RecipientIDs: []string{"user-2"},
		Recipients: []federatedUserInfo{
			{ID: "user-2", Username: "bob", InstanceDomain: "instance-a.com"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedDMCreateRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ChannelType != "dm" {
		t.Errorf("ChannelType = %q, want %q", decoded.ChannelType, "dm")
	}
	if decoded.GroupName != nil {
		t.Errorf("GroupName should be nil for DM, got %v", decoded.GroupName)
	}
}

func TestFederatedDMMessageRequest_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	req := federatedDMMessageRequest{
		RemoteChannelID: "ch-789",
		Message: federatedMessageData{
			ID:        "msg-1",
			AuthorID:  "user-1",
			Content:   "Hello from federation!",
			CreatedAt: now,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedDMMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RemoteChannelID != "ch-789" {
		t.Errorf("RemoteChannelID = %q, want %q", decoded.RemoteChannelID, "ch-789")
	}
	if decoded.Message.ID != "msg-1" {
		t.Errorf("Message.ID = %q, want %q", decoded.Message.ID, "msg-1")
	}
	if decoded.Message.Content != "Hello from federation!" {
		t.Errorf("Message.Content = %q, want %q", decoded.Message.Content, "Hello from federation!")
	}
	if decoded.Message.AuthorID != "user-1" {
		t.Errorf("Message.AuthorID = %q, want %q", decoded.Message.AuthorID, "user-1")
	}
}

func TestFederatedDMMessageRequest_WithAttachments(t *testing.T) {
	req := federatedDMMessageRequest{
		RemoteChannelID: "ch-100",
		Message: federatedMessageData{
			ID:          "msg-2",
			AuthorID:    "user-2",
			Content:     "Check this out",
			Attachments: json.RawMessage(`[{"id":"att-1","filename":"test.png"}]`),
			CreatedAt:   time.Now(),
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedDMMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Message.Attachments == nil {
		t.Error("Message.Attachments should not be nil")
	}
}

func TestFederatedDMRecipientRequest_JSON(t *testing.T) {
	req := federatedDMRecipientRequest{
		RemoteChannelID: "ch-200",
		User: federatedUserInfo{
			ID:             "user-5",
			Username:       "eve",
			InstanceDomain: "remote.example.com",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedDMRecipientRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RemoteChannelID != "ch-200" {
		t.Errorf("RemoteChannelID = %q, want %q", decoded.RemoteChannelID, "ch-200")
	}
	if decoded.User.ID != "user-5" {
		t.Errorf("User.ID = %q, want %q", decoded.User.ID, "user-5")
	}
	if decoded.User.Username != "eve" {
		t.Errorf("User.Username = %q, want %q", decoded.User.Username, "eve")
	}
	if decoded.User.InstanceDomain != "remote.example.com" {
		t.Errorf("User.InstanceDomain = %q, want %q", decoded.User.InstanceDomain, "remote.example.com")
	}
}

func TestFederatedUserInfo_JSON(t *testing.T) {
	displayName := "Alice Wonderland"
	avatarID := "avatar-123"

	u := federatedUserInfo{
		ID:             "user-1",
		Username:       "alice",
		DisplayName:    &displayName,
		AvatarID:       &avatarID,
		InstanceDomain: "instance-a.com",
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedUserInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
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

func TestFederatedUserInfo_MinimalFields(t *testing.T) {
	u := federatedUserInfo{
		ID:             "user-2",
		Username:       "bob",
		InstanceDomain: "instance-b.com",
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded federatedUserInfo
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
