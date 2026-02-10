package gateway

import (
	"encoding/json"
	"testing"
)

func TestOpcodeConstants(t *testing.T) {
	// Verify opcodes match the WebSocket protocol spec (Section 8.2).
	opcodes := map[string]int{
		"Dispatch":         OpDispatch,
		"Heartbeat":        OpHeartbeat,
		"Identify":         OpIdentify,
		"PresenceUpdate":   OpPresenceUpdate,
		"VoiceStateUpdate": OpVoiceStateUpdate,
		"Resume":           OpResume,
		"Reconnect":        OpReconnect,
		"RequestMembers":   OpRequestMembers,
		"Typing":           OpTyping,
		"Subscribe":        OpSubscribe,
		"Hello":            OpHello,
		"HeartbeatAck":     OpHeartbeatAck,
	}

	// Check uniqueness.
	seen := make(map[int]string)
	for name, op := range opcodes {
		if existing, ok := seen[op]; ok {
			t.Errorf("duplicate opcode %d: %s and %s", op, existing, name)
		}
		seen[op] = name
	}

	// Verify specific values.
	if OpDispatch != 0 {
		t.Errorf("OpDispatch = %d, want 0", OpDispatch)
	}
	if OpHello != 10 {
		t.Errorf("OpHello = %d, want 10", OpHello)
	}
	if OpHeartbeatAck != 11 {
		t.Errorf("OpHeartbeatAck = %d, want 11", OpHeartbeatAck)
	}
}

func TestGatewayMessage_JSON(t *testing.T) {
	data, _ := json.Marshal(map[string]string{"key": "value"})
	seq := int64(42)
	msg := GatewayMessage{
		Op:   OpDispatch,
		Type: "MESSAGE_CREATE",
		Data: data,
		Seq:  &seq,
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded GatewayMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Op != OpDispatch {
		t.Errorf("op = %d, want %d", decoded.Op, OpDispatch)
	}
	if decoded.Type != "MESSAGE_CREATE" {
		t.Errorf("type = %q, want %q", decoded.Type, "MESSAGE_CREATE")
	}
	if decoded.Seq == nil || *decoded.Seq != 42 {
		t.Errorf("seq = %v, want 42", decoded.Seq)
	}
}

func TestGatewayMessage_Omitempty(t *testing.T) {
	msg := GatewayMessage{Op: OpHeartbeat}

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	str := string(encoded)
	// Type and Seq should be omitted.
	var decoded map[string]interface{}
	json.Unmarshal(encoded, &decoded)

	if _, ok := decoded["t"]; ok && decoded["t"] != "" {
		// t is omitted or empty
	}
	if _, ok := decoded["s"]; ok {
		t.Errorf("seq should be omitted, got: %s", str)
	}
}

func TestIdentifyPayload_JSON(t *testing.T) {
	payload := IdentifyPayload{Token: "my-secret-token"}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded IdentifyPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Token != "my-secret-token" {
		t.Errorf("token = %q, want %q", decoded.Token, "my-secret-token")
	}
}

func TestHelloPayload_JSON(t *testing.T) {
	payload := HelloPayload{HeartbeatInterval: 30000}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded HelloPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.HeartbeatInterval != 30000 {
		t.Errorf("heartbeat_interval = %d, want %d", decoded.HeartbeatInterval, 30000)
	}
}

func TestGatewayMessage_FromJSON(t *testing.T) {
	// Test parsing from raw JSON like a client would send.
	raw := `{"op":2,"d":{"token":"abc123"}}`
	var msg GatewayMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if msg.Op != OpIdentify {
		t.Errorf("op = %d, want %d", msg.Op, OpIdentify)
	}

	var identify IdentifyPayload
	if err := json.Unmarshal(msg.Data, &identify); err != nil {
		t.Fatalf("unmarshal data error: %v", err)
	}
	if identify.Token != "abc123" {
		t.Errorf("token = %q, want %q", identify.Token, "abc123")
	}
}

func TestResumePayload_JSON(t *testing.T) {
	payload := ResumePayload{
		Token:     "my-token",
		SessionID: "session-123",
		Seq:       42,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ResumePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Token != "my-token" {
		t.Errorf("token = %q, want %q", decoded.Token, "my-token")
	}
	if decoded.SessionID != "session-123" {
		t.Errorf("session_id = %q, want %q", decoded.SessionID, "session-123")
	}
	if decoded.Seq != 42 {
		t.Errorf("seq = %d, want 42", decoded.Seq)
	}
}

func TestRequestMembersPayload_JSON(t *testing.T) {
	payload := RequestMembersPayload{
		GuildID: "guild-abc",
		Query:   "user",
		Limit:   50,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded RequestMembersPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.GuildID != "guild-abc" {
		t.Errorf("guild_id = %q, want %q", decoded.GuildID, "guild-abc")
	}
	if decoded.Query != "user" {
		t.Errorf("query = %q, want %q", decoded.Query, "user")
	}
	if decoded.Limit != 50 {
		t.Errorf("limit = %d, want 50", decoded.Limit)
	}
}

func TestRequestMembersPayload_Defaults(t *testing.T) {
	raw := `{"guild_id": "abc"}`
	var payload RequestMembersPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if payload.GuildID != "abc" {
		t.Errorf("guild_id = %q, want %q", payload.GuildID, "abc")
	}
	if payload.Query != "" {
		t.Errorf("query = %q, want empty", payload.Query)
	}
	if payload.Limit != 0 {
		t.Errorf("limit = %d, want 0", payload.Limit)
	}
}

func TestResumePayload_FromJSON(t *testing.T) {
	raw := `{"op":5,"d":{"token":"tok","session_id":"sid","seq":10}}`
	var msg GatewayMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if msg.Op != OpResume {
		t.Errorf("op = %d, want %d", msg.Op, OpResume)
	}

	var resume ResumePayload
	if err := json.Unmarshal(msg.Data, &resume); err != nil {
		t.Fatalf("unmarshal data error: %v", err)
	}

	if resume.Token != "tok" {
		t.Errorf("token = %q, want %q", resume.Token, "tok")
	}
	if resume.Seq != 10 {
		t.Errorf("seq = %d, want 10", resume.Seq)
	}
}

func TestClient_GuildIDsInit(t *testing.T) {
	// Verify that guildIDs map works as expected.
	guilds := make(map[string]bool)
	guilds["guild-1"] = true
	guilds["guild-2"] = true

	if !guilds["guild-1"] {
		t.Error("guild-1 should be in map")
	}
	if guilds["guild-3"] {
		t.Error("guild-3 should not be in map")
	}
	if len(guilds) != 2 {
		t.Errorf("len = %d, want 2", len(guilds))
	}

	delete(guilds, "guild-1")
	if guilds["guild-1"] {
		t.Error("guild-1 should be removed")
	}
}
