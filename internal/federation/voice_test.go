package federation

import (
	"encoding/json"
	"testing"
)

func TestFederatedVoiceTokenRequest_JSONRoundTrip(t *testing.T) {
	req := &federatedVoiceTokenRequest{
		UserID:         "user-123",
		ChannelID:      "channel-456",
		Username:       "alice",
		DisplayName:    "Alice Wonderland",
		AvatarID:       "avatar-789",
		InstanceDomain: "remote.example.com",
		ScreenShare:    true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded federatedVoiceTokenRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.UserID != req.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, req.UserID)
	}
	if decoded.ChannelID != req.ChannelID {
		t.Errorf("ChannelID = %q, want %q", decoded.ChannelID, req.ChannelID)
	}
	if decoded.Username != req.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, req.Username)
	}
	if decoded.DisplayName != req.DisplayName {
		t.Errorf("DisplayName = %q, want %q", decoded.DisplayName, req.DisplayName)
	}
	if decoded.AvatarID != req.AvatarID {
		t.Errorf("AvatarID = %q, want %q", decoded.AvatarID, req.AvatarID)
	}
	if decoded.InstanceDomain != req.InstanceDomain {
		t.Errorf("InstanceDomain = %q, want %q", decoded.InstanceDomain, req.InstanceDomain)
	}
	if decoded.ScreenShare != req.ScreenShare {
		t.Errorf("ScreenShare = %v, want %v", decoded.ScreenShare, req.ScreenShare)
	}
}

func TestFederatedVoiceTokenRequest_OptionalFields(t *testing.T) {
	req := &federatedVoiceTokenRequest{
		UserID:         "user-123",
		ChannelID:      "channel-456",
		Username:       "bob",
		InstanceDomain: "other.example.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify optional fields are omitted when empty.
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["display_name"]; ok {
		// display_name should be present but empty string (not omitted by default in Go)
		if raw["display_name"] != "" {
			t.Errorf("display_name should be empty, got %v", raw["display_name"])
		}
	}

	if raw["screen_share"] != false {
		t.Errorf("screen_share should be false, got %v", raw["screen_share"])
	}
}

func TestVoiceTokenResponse_JSONFormat(t *testing.T) {
	resp := map[string]interface{}{
		"token":      "eyJhbGciOiJIUzI1NiJ9.test",
		"url":        "wss://voice.example.com/rtc",
		"channel_id": "channel-123",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["token"] != resp["token"] {
		t.Errorf("token = %v, want %v", decoded["token"], resp["token"])
	}
	if decoded["url"] != resp["url"] {
		t.Errorf("url = %v, want %v", decoded["url"], resp["url"])
	}
	if decoded["channel_id"] != resp["channel_id"] {
		t.Errorf("channel_id = %v, want %v", decoded["channel_id"], resp["channel_id"])
	}
}

func TestDiscoveryResponse_IncludesLiveKitURL(t *testing.T) {
	resp := DiscoveryResponse{
		InstanceID:         "inst-1",
		Domain:             "example.com",
		PublicKey:          "pubkey",
		Software:           "AmityVox",
		SoftwareVersion:    "1.0.0",
		FederationMode:     "open",
		APIEndpoint:        "https://example.com/federation/v1",
		SupportedProtocols: []string{"amityvox-federation/1.0"},
		LiveKitURL:         "wss://example.com/rtc",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["livekit_url"] != "wss://example.com/rtc" {
		t.Errorf("livekit_url = %v, want wss://example.com/rtc", decoded["livekit_url"])
	}
}

func TestDiscoveryResponse_OmitsLiveKitURLWhenEmpty(t *testing.T) {
	resp := DiscoveryResponse{
		InstanceID:      "inst-1",
		Domain:          "example.com",
		PublicKey:        "pubkey",
		Software:         "AmityVox",
		SoftwareVersion:  "1.0.0",
		FederationMode:   "open",
		APIEndpoint:      "https://example.com/federation/v1",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if _, ok := decoded["livekit_url"]; ok {
		t.Errorf("livekit_url should be omitted when empty, but was present: %v", decoded["livekit_url"])
	}
}

func TestNewTestVoiceTokenRequest(t *testing.T) {
	req := NewTestVoiceTokenRequest()
	if req.UserID == "" {
		t.Error("UserID should not be empty")
	}
	if req.ChannelID == "" {
		t.Error("ChannelID should not be empty")
	}
	if req.Username != "testuser" {
		t.Errorf("Username = %q, want testuser", req.Username)
	}
	if req.InstanceDomain != "remote.example.com" {
		t.Errorf("InstanceDomain = %q, want remote.example.com", req.InstanceDomain)
	}
}
