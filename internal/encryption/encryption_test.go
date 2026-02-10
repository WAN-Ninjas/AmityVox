package encryption

import (
	"encoding/json"
	"testing"
	"time"
)

func TestKeyPackage_JSON(t *testing.T) {
	kp := KeyPackage{
		ID:        "kp_001",
		UserID:    "user_001",
		DeviceID:  "device_abc",
		Data:      []byte{0xDE, 0xAD, 0xBE, 0xEF},
		ExpiresAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(kp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded KeyPackage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != kp.ID {
		t.Errorf("id = %q, want %q", decoded.ID, kp.ID)
	}
	if decoded.UserID != kp.UserID {
		t.Errorf("user_id = %q, want %q", decoded.UserID, kp.UserID)
	}
	if decoded.DeviceID != kp.DeviceID {
		t.Errorf("device_id = %q, want %q", decoded.DeviceID, kp.DeviceID)
	}
	if len(decoded.Data) != 4 || decoded.Data[0] != 0xDE {
		t.Errorf("data mismatch: got %v", decoded.Data)
	}
	if !decoded.ExpiresAt.Equal(kp.ExpiresAt) {
		t.Errorf("expires_at = %v, want %v", decoded.ExpiresAt, kp.ExpiresAt)
	}
}

func TestWelcomeMessage_JSON(t *testing.T) {
	wm := WelcomeMessage{
		ID:         "wm_001",
		ChannelID:  "ch_encrypted",
		ReceiverID: "user_002",
		Data:       []byte("welcome-bytes"),
		CreatedAt:  time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(wm)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded WelcomeMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != wm.ID {
		t.Errorf("id = %q, want %q", decoded.ID, wm.ID)
	}
	if decoded.ChannelID != wm.ChannelID {
		t.Errorf("channel_id = %q, want %q", decoded.ChannelID, wm.ChannelID)
	}
	if decoded.ReceiverID != wm.ReceiverID {
		t.Errorf("receiver_id = %q, want %q", decoded.ReceiverID, wm.ReceiverID)
	}
	if string(decoded.Data) != "welcome-bytes" {
		t.Errorf("data = %q, want %q", decoded.Data, "welcome-bytes")
	}
}

func TestGroupState_JSON(t *testing.T) {
	gs := GroupState{
		ChannelID: "ch_encrypted",
		Epoch:     42,
		TreeHash:  []byte{0x01, 0x02, 0x03},
		UpdatedAt: time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded GroupState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ChannelID != gs.ChannelID {
		t.Errorf("channel_id = %q, want %q", decoded.ChannelID, gs.ChannelID)
	}
	if decoded.Epoch != 42 {
		t.Errorf("epoch = %d, want 42", decoded.Epoch)
	}
	if len(decoded.TreeHash) != 3 {
		t.Errorf("tree_hash length = %d, want 3", len(decoded.TreeHash))
	}
}

func TestCommit_JSON(t *testing.T) {
	c := Commit{
		ID:        "commit_001",
		ChannelID: "ch_encrypted",
		SenderID:  "user_001",
		Epoch:     43,
		Data:      []byte("commit-data"),
		CreatedAt: time.Date(2026, 2, 10, 13, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Commit
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != c.ID {
		t.Errorf("id = %q, want %q", decoded.ID, c.ID)
	}
	if decoded.ChannelID != c.ChannelID {
		t.Errorf("channel_id = %q, want %q", decoded.ChannelID, c.ChannelID)
	}
	if decoded.SenderID != c.SenderID {
		t.Errorf("sender_id = %q, want %q", decoded.SenderID, c.SenderID)
	}
	if decoded.Epoch != 43 {
		t.Errorf("epoch = %d, want 43", decoded.Epoch)
	}
	if string(decoded.Data) != "commit-data" {
		t.Errorf("data = %q, want %q", decoded.Data, "commit-data")
	}
}

func TestKeyPackage_EmptyData(t *testing.T) {
	kp := KeyPackage{
		ID:     "kp_empty",
		UserID: "user_001",
	}

	data, err := json.Marshal(kp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded KeyPackage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Data != nil {
		t.Errorf("expected nil data, got %v", decoded.Data)
	}
}

func TestGroupState_ZeroEpoch(t *testing.T) {
	gs := GroupState{
		ChannelID: "ch_new",
		Epoch:     0,
	}

	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded GroupState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Epoch != 0 {
		t.Errorf("epoch = %d, want 0", decoded.Epoch)
	}
}
