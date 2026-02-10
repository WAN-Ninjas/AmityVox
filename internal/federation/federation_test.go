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
