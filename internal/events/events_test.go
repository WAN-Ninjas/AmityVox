package events

import (
	"encoding/json"
	"testing"
)

func TestEventMarshal(t *testing.T) {
	data, _ := json.Marshal(map[string]string{"message": "hello"})
	event := Event{
		Type:      "MESSAGE_CREATE",
		GuildID:   "guild123",
		ChannelID: "channel456",
		UserID:    "user789",
		Data:      data,
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Type != "MESSAGE_CREATE" {
		t.Errorf("type = %q, want %q", decoded.Type, "MESSAGE_CREATE")
	}
	if decoded.GuildID != "guild123" {
		t.Errorf("guild_id = %q, want %q", decoded.GuildID, "guild123")
	}
	if decoded.ChannelID != "channel456" {
		t.Errorf("channel_id = %q, want %q", decoded.ChannelID, "channel456")
	}
	if decoded.UserID != "user789" {
		t.Errorf("user_id = %q, want %q", decoded.UserID, "user789")
	}

	var payload map[string]string
	if err := json.Unmarshal(decoded.Data, &payload); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if payload["message"] != "hello" {
		t.Errorf("data.message = %q, want %q", payload["message"], "hello")
	}
}

func TestEventMarshal_EmptyOptionals(t *testing.T) {
	data, _ := json.Marshal(nil)
	event := Event{
		Type: "PRESENCE_UPDATE",
		Data: data,
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// guild_id, channel_id, user_id should be omitted with omitempty.
	str := string(encoded)
	if containsKey(str, "guild_id") {
		t.Error("empty guild_id should be omitted")
	}
}

func TestSubjectConstants(t *testing.T) {
	// Verify all subjects follow the amityvox.* pattern.
	subjects := []string{
		SubjectMessageCreate, SubjectMessageUpdate, SubjectMessageDelete,
		SubjectChannelCreate, SubjectChannelUpdate, SubjectChannelDelete,
		SubjectGuildCreate, SubjectGuildUpdate, SubjectGuildDelete,
		SubjectGuildMemberAdd, SubjectGuildMemberRemove,
		SubjectPresenceUpdate, SubjectUserUpdate,
		SubjectVoiceStateUpdate, SubjectChannelAck,
	}

	for _, s := range subjects {
		if s == "" {
			t.Error("empty subject constant")
		}
		if len(s) < 10 {
			t.Errorf("subject %q seems too short", s)
		}
		if s[:9] != "amityvox." {
			t.Errorf("subject %q should start with 'amityvox.'", s)
		}
	}
}

func TestEventJSON_Tags(t *testing.T) {
	// Verify JSON tag names match the spec.
	data := []byte(`{"t":"TEST","guild_id":"g","channel_id":"c","user_id":"u","d":{"key":"val"}}`)
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event.Type != "TEST" {
		t.Errorf("Type = %q, want %q", event.Type, "TEST")
	}
	if event.GuildID != "g" {
		t.Errorf("GuildID = %q, want %q", event.GuildID, "g")
	}
	if event.ChannelID != "c" {
		t.Errorf("ChannelID = %q, want %q", event.ChannelID, "c")
	}
	if event.UserID != "u" {
		t.Errorf("UserID = %q, want %q", event.UserID, "u")
	}
}

func containsKey(jsonStr, key string) bool {
	return json.Valid([]byte(jsonStr)) && len(jsonStr) > 0 &&
		contains(jsonStr, `"`+key+`"`)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
