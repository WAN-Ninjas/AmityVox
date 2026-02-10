package federation

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"testing"
)

func TestSign_And_Verify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen error: %v", err)
	}

	svc := New(Config{
		InstanceID: "test-instance",
		Domain:     "test.example.com",
		PrivateKey: priv,
		Logger:     slog.Default(),
	})

	data := map[string]string{"message": "hello federation"}

	signed, err := svc.Sign(data)
	if err != nil {
		t.Fatalf("Sign error: %v", err)
	}

	if signed.SenderID != "test-instance" {
		t.Errorf("SenderID = %q, want %q", signed.SenderID, "test-instance")
	}
	if signed.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if signed.Signature == "" {
		t.Error("Signature should not be empty")
	}
	if len(signed.Payload) == 0 {
		t.Error("Payload should not be empty")
	}

	// Verify the signature.
	pubKeyPEM := marshalPublicKeyPEM(t, pub)
	valid, err := VerifySignature(pubKeyPEM, signed.Payload, signed.Signature)
	if err != nil {
		t.Fatalf("VerifySignature error: %v", err)
	}
	if !valid {
		t.Error("signature should be valid")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	pubKeyPEM := marshalPublicKeyPEM(t, pub)

	// Tampered payload.
	valid, err := VerifySignature(pubKeyPEM, []byte("tampered"), "deadbeef")
	if err != nil {
		t.Fatalf("VerifySignature error: %v", err)
	}
	if valid {
		t.Error("tampered signature should be invalid")
	}
}

func TestVerifySignature_BadPEM(t *testing.T) {
	_, err := VerifySignature("not-a-pem", []byte("data"), "sig")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestDiscoveryResponse_JSON(t *testing.T) {
	name := "Test Instance"
	resp := DiscoveryResponse{
		InstanceID:         "inst-123",
		Domain:             "test.example.com",
		Name:               &name,
		PublicKey:          "PEM-KEY-HERE",
		Software:           "AmityVox",
		SoftwareVersion:    "0.1.0",
		FederationMode:     "open",
		APIEndpoint:        "https://test.example.com/federation/v1",
		SupportedProtocols: []string{Version},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded DiscoveryResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.InstanceID != "inst-123" {
		t.Errorf("InstanceID = %q, want %q", decoded.InstanceID, "inst-123")
	}
	if decoded.FederationMode != "open" {
		t.Errorf("FederationMode = %q, want %q", decoded.FederationMode, "open")
	}
	if len(decoded.SupportedProtocols) != 1 || decoded.SupportedProtocols[0] != Version {
		t.Errorf("SupportedProtocols = %v, want [%s]", decoded.SupportedProtocols, Version)
	}
}

func TestSignedPayload_JSON(t *testing.T) {
	sp := SignedPayload{
		Payload:   json.RawMessage(`{"test":"data"}`),
		Signature: "abcdef",
		SenderID:  "sender-123",
	}

	data, err := json.Marshal(sp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SignedPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.SenderID != "sender-123" {
		t.Errorf("SenderID = %q, want %q", decoded.SenderID, "sender-123")
	}
	if decoded.Signature != "abcdef" {
		t.Errorf("Signature = %q, want %q", decoded.Signature, "abcdef")
	}
}

func TestVersionConstant(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Version != "amityvox-federation/1.0" {
		t.Errorf("Version = %q, want %q", Version, "amityvox-federation/1.0")
	}
}

func marshalPublicKeyPEM(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

// --- HLC Tests ---

func TestHLC_Now_Monotonic(t *testing.T) {
	hlc := NewHLC()
	prev := hlc.Now()
	for i := 0; i < 100; i++ {
		next := hlc.Now()
		if !prev.Before(next) {
			t.Errorf("HLC not monotonic: prev=%+v next=%+v", prev, next)
		}
		prev = next
	}
}

func TestHLC_Update_RemoteAhead(t *testing.T) {
	hlc := NewHLC()
	local := hlc.Now()

	// Simulate a remote timestamp 1 second in the future.
	remote := HLCTimestamp{
		WallMs:  local.WallMs + 1000,
		Counter: 5,
	}

	updated := hlc.Update(remote)
	if updated.WallMs < remote.WallMs {
		t.Errorf("after Update, wall should be >= remote: got %d, remote %d", updated.WallMs, remote.WallMs)
	}
}

func TestHLC_Update_SameWall(t *testing.T) {
	hlc := NewHLC()
	local := hlc.Now()

	remote := HLCTimestamp{
		WallMs:  local.WallMs,
		Counter: local.Counter + 5,
	}

	updated := hlc.Update(remote)
	if updated.Counter <= remote.Counter {
		t.Errorf("counter should advance past remote: got %d, remote %d", updated.Counter, remote.Counter)
	}
}

func TestHLCTimestamp_Before(t *testing.T) {
	a := HLCTimestamp{WallMs: 1000, Counter: 0}
	b := HLCTimestamp{WallMs: 2000, Counter: 0}
	c := HLCTimestamp{WallMs: 1000, Counter: 1}

	if !a.Before(b) {
		t.Error("a should be before b (different wall)")
	}
	if b.Before(a) {
		t.Error("b should not be before a")
	}
	if !a.Before(c) {
		t.Error("a should be before c (same wall, different counter)")
	}
	if c.Before(a) {
		t.Error("c should not be before a")
	}
}

func TestFederatedMessage_JSON(t *testing.T) {
	msg := FederatedMessage{
		Type:      "MESSAGE_CREATE",
		OriginID:  "origin-123",
		Timestamp: HLCTimestamp{WallMs: 1700000000000, Counter: 3},
		GuildID:   "guild-1",
		ChannelID: "channel-1",
		Data:      map[string]string{"content": "hello"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded FederatedMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Type != "MESSAGE_CREATE" {
		t.Errorf("Type = %q, want %q", decoded.Type, "MESSAGE_CREATE")
	}
	if decoded.Timestamp.WallMs != 1700000000000 {
		t.Errorf("WallMs = %d, want %d", decoded.Timestamp.WallMs, 1700000000000)
	}
	if decoded.Timestamp.Counter != 3 {
		t.Errorf("Counter = %d, want %d", decoded.Timestamp.Counter, 3)
	}
}

func TestEventTypeToSubject(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		{"MESSAGE_CREATE", "amityvox.message.create"},
		{"MESSAGE_UPDATE", "amityvox.message.update"},
		{"GUILD_CREATE", "amityvox.guild.create"},
		{"CHANNEL_DELETE", "amityvox.channel.delete"},
		{"UNKNOWN_EVENT", ""},
	}

	for _, tt := range tests {
		got := eventTypeToSubject(tt.eventType)
		if got != tt.want {
			t.Errorf("eventTypeToSubject(%q) = %q, want %q", tt.eventType, got, tt.want)
		}
	}
}
