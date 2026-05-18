package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amityvox/amityvox/internal/federation"
)

func TestFederationSyncBackfillIsAuthorizedOrderedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	localID := "sync-local-" + time.Now().Format("150405.000000000")
	remoteID := "sync-remote-" + time.Now().Format("150405.000000000")
	guildID := "sync-guild-" + time.Now().Format("150405.000000000")
	channelID := "sync-channel-" + time.Now().Format("150405.000000000")
	ownerID := "sync-owner-" + time.Now().Format("150405.000000000")

	_, localPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("local keygen: %v", err)
	}
	remotePub, remotePriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("remote keygen: %v", err)
	}

	insertInstance(t, ctx, localID, "local-"+localID+".test", marshalPublicKeyPEM(t, localPriv.Public().(ed25519.PublicKey)))
	insertInstance(t, ctx, remoteID, "remote-"+remoteID+".test", marshalPublicKeyPEM(t, remotePub))
	_, err = testPool.Exec(ctx, `INSERT INTO users (id, instance_id, username, password_hash, created_at) VALUES ($1, $2, $3, 'hash', now())`, ownerID, localID, ownerID)
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO guilds (id, instance_id, owner_id, name, created_at) VALUES ($1, $2, $3, 'Sync Guild', now())`, guildID, localID, ownerID)
	if err != nil {
		t.Fatalf("insert guild: %v", err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO channels (id, guild_id, instance_id, channel_type, name, position, created_at) VALUES ($1, $2, $3, 'text', 'general', 0, now())`, channelID, guildID, localID)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO federation_channel_peers (channel_id, instance_id) VALUES ($1, $2)`, channelID, remoteID)
	if err != nil {
		t.Fatalf("insert channel peer: %v", err)
	}

	firstPayloadA := json.RawMessage(`{"id":"msg-1","content":"hello","nested":{"z":2,"a":1}}`)
	firstPayloadB := json.RawMessage(`{"nested":{"a":1,"z":2},"content":"hello","id":"msg-1"}`)
	eventID := federationTestStableEventID(remoteID, "MESSAGE_CREATE", guildID, channelID, 1000, 1, firstPayloadA)
	insertFederationEvent(t, ctx, eventID, remoteID, "MESSAGE_CREATE", guildID, channelID, 1000, 1, firstPayloadA)
	insertFederationEvent(t, ctx, eventID, remoteID, "MESSAGE_CREATE", guildID, channelID, 1000, 1, firstPayloadB)
	insertFederationEvent(t, ctx, "sync-event-2-"+channelID, remoteID, "MESSAGE_UPDATE", guildID, channelID, 1001, 0, json.RawMessage(`{"id":"msg-1","content":"edited"}`))

	localFed := federation.New(federation.Config{
		Pool:       testPool,
		InstanceID: localID,
		Domain:     "local.test",
		PrivateKey: localPriv,
		Logger:     testLogger,
	})
	syncService := federation.NewSyncService(localFed, testBus, testLogger, federation.SyncConfig{BackfillWindowDays: 7})
	remoteFed := federation.New(federation.Config{
		InstanceID: remoteID,
		Domain:     "remote.test",
		PrivateKey: remotePriv,
		Logger:     testLogger,
	})

	body := signSyncRequest(t, remoteFed, map[string]interface{}{
		"last_seen_hlc": map[string]interface{}{"wall_ms": 999, "counter": 0},
		"guild_ids":     []string{guildID},
	})
	req := httptest.NewRequest(http.MethodPost, "/federation/v1/sync", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	syncService.HandleSync(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("sync status = %d body=%s", rec.Code, rec.Body.String())
	}
	var decoded struct {
		Events []struct {
			ID      string `json:"id"`
			Type    string `json:"event_type"`
			GuildID string `json:"guild_id"`
			HLC     struct {
				WallMs  int64 `json:"wall_ms"`
				Counter int   `json:"counter"`
			} `json:"hlc"`
		} `json:"events"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if decoded.Truncated {
		t.Fatal("small backfill should not be truncated")
	}
	if len(decoded.Events) != 2 {
		t.Fatalf("sync returned %d events, want 2: %#v", len(decoded.Events), decoded.Events)
	}
	if decoded.Events[0].ID != eventID || decoded.Events[0].Type != "MESSAGE_CREATE" {
		t.Fatalf("first event = %#v, want canonical message create", decoded.Events[0])
	}
	if decoded.Events[1].Type != "MESSAGE_UPDATE" || decoded.Events[1].HLC.WallMs != 1001 {
		t.Fatalf("second event = %#v, want ordered message update", decoded.Events[1])
	}
}

func insertInstance(t *testing.T, ctx context.Context, id, domain, publicKey string) {
	t.Helper()
	_, err := testPool.Exec(ctx,
		`INSERT INTO instances (id, domain, public_key, name, software_version, federation_mode, created_at)
		 VALUES ($1, $2, $3, $4, 'test', 'open', now())`,
		id, domain, publicKey, id)
	if err != nil {
		t.Fatalf("insert instance %s: %v", id, err)
	}
}

func insertFederationEvent(t *testing.T, ctx context.Context, id, instanceID, eventType, guildID, channelID string, wallMs int64, counter int, payload json.RawMessage) {
	t.Helper()
	_, err := testPool.Exec(ctx,
		`INSERT INTO federation_events (id, instance_id, event_type, guild_id, channel_id, hlc_wall_ms, hlc_counter, payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO NOTHING`,
		id, instanceID, eventType, guildID, channelID, wallMs, counter, payload)
	if err != nil {
		t.Fatalf("insert federation event %s: %v", id, err)
	}
}

func signSyncRequest(t *testing.T, svc *federation.Service, payload interface{}) []byte {
	t.Helper()
	signed, err := svc.Sign(payload)
	if err != nil {
		t.Fatalf("sign sync request: %v", err)
	}
	body, err := json.Marshal(signed)
	if err != nil {
		t.Fatalf("marshal signed request: %v", err)
	}
	return body
}

func marshalPublicKeyPEM(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func federationTestStableEventID(instanceID, eventType, guildID, channelID string, wallMs int64, counter int, payload json.RawMessage) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00", instanceID, eventType, guildID, channelID, wallMs, counter)
	var decoded interface{}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err == nil {
		if canonical, err := json.Marshal(decoded); err == nil {
			h.Write(canonical)
			return hex.EncodeToString(h.Sum(nil))
		}
	}
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
