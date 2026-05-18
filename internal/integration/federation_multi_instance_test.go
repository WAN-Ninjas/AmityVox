package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/federation"
)

func TestFederatedGuildJoinCreatesRemoteMemberAndPeersIdempotently(t *testing.T) {
	ctx := context.Background()
	fixture := newFederationFixture(t, "guild-join")
	guildID := fixture.id("guild")
	channelID := fixture.id("channel")
	ownerID := fixture.id("owner")
	remoteUserID := fixture.id("remote-user")

	insertUser(t, ctx, ownerID, fixture.localID, "owner-"+ownerID)
	insertGuild(t, ctx, guildID, fixture.localID, ownerID, "Join Guild", true)
	insertChannel(t, ctx, channelID, guildID, fixture.localID, "general")

	payload := map[string]interface{}{
		"user_id":         remoteUserID,
		"username":        "remotejoiner",
		"display_name":    "Remote Joiner",
		"instance_domain": fixture.remoteDomain,
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := signedJSONRequest(t, fixture.remoteFed, http.MethodPost, "/federation/v1/guilds/"+guildID+"/join", payload)
		req = withChiParam(req, "guildID", guildID)

		fixture.sync.HandleFederatedGuildJoin(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("join attempt %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}

	var memberInstanceID, userInstanceID string
	if err := testPool.QueryRow(ctx,
		`SELECT gm.instance_id, u.instance_id
		   FROM guild_members gm
		   JOIN users u ON u.id = gm.user_id
		  WHERE gm.guild_id = $1 AND gm.user_id = $2`,
		guildID, remoteUserID).Scan(&memberInstanceID, &userInstanceID); err != nil {
		t.Fatalf("query federated member: %v", err)
	}
	if memberInstanceID != fixture.remoteID || userInstanceID != fixture.remoteID {
		t.Fatalf("remote ownership = member:%s user:%s, want %s", memberInstanceID, userInstanceID, fixture.remoteID)
	}

	var peerCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_channel_peers WHERE channel_id = $1 AND instance_id = $2`,
		channelID, fixture.remoteID).Scan(&peerCount); err != nil {
		t.Fatalf("query channel peers: %v", err)
	}
	if peerCount != 1 {
		t.Fatalf("channel peer count = %d, want 1", peerCount)
	}

	var memberRows int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guild_members WHERE guild_id = $1 AND user_id = $2`,
		guildID, remoteUserID).Scan(&memberRows); err != nil {
		t.Fatalf("query member rows: %v", err)
	}
	if memberRows != 1 {
		t.Fatalf("member rows = %d, want 1", memberRows)
	}
}

func TestFederatedDMCreateAndMessageWithMediaAreIdempotent(t *testing.T) {
	ctx := context.Background()
	fixture := newFederationFixture(t, "dm-media")
	localUserID := fixture.id("local-user")
	remoteUserID := fixture.id("remote-user")
	remoteChannelID := fixture.id("remote-dm")
	messageID := fixture.id("message")
	attachmentID := fixture.id("attachment")

	insertUser(t, ctx, localUserID, fixture.localID, "local-"+localUserID)

	createPayload := map[string]interface{}{
		"channel_id":    remoteChannelID,
		"channel_type":  "dm",
		"recipient_ids": []string{localUserID, remoteUserID},
		"creator": map[string]interface{}{
			"id":              remoteUserID,
			"username":        "remoteauthor",
			"instance_domain": fixture.remoteDomain,
		},
		"recipients": []map[string]interface{}{
			{
				"id":              localUserID,
				"username":        "localuser",
				"instance_domain": fixture.localDomain,
			},
			{
				"id":              remoteUserID,
				"username":        "remoteauthor",
				"instance_domain": fixture.remoteDomain,
			},
		},
	}

	var localChannelID string
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := signedJSONRequest(t, fixture.remoteFed, http.MethodPost, "/federation/v1/dm/create", createPayload)

		fixture.sync.HandleFederatedDMCreate(rec, req)

		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("dm create attempt %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
		var body struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode dm create response: %v", err)
		}
		if localChannelID == "" {
			localChannelID = body.ChannelID
		} else if localChannelID != body.ChannelID {
			t.Fatalf("duplicate dm create returned channel %s, want %s", body.ChannelID, localChannelID)
		}
	}

	var recipientCount int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM channel_recipients WHERE channel_id = $1 AND user_id = ANY($2)`,
		localChannelID, []string{localUserID, remoteUserID}).Scan(&recipientCount); err != nil {
		t.Fatalf("query channel recipients: %v", err)
	}
	if recipientCount != 2 {
		t.Fatalf("recipient count = %d, want 2", recipientCount)
	}

	messagePayload := map[string]interface{}{
		"remote_channel_id": remoteChannelID,
		"message": map[string]interface{}{
			"id":           messageID,
			"author_id":    remoteUserID,
			"content":      "federated media",
			"message_type": "default",
			"created_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"attachments": []map[string]interface{}{
				{
					"id":           attachmentID,
					"uploader_id":  remoteUserID,
					"filename":     "remote.png",
					"content_type": "image/png",
					"size_bytes":   1234,
					"s3_bucket":    "remote-bucket",
					"s3_key":       "remote/key.png",
				},
			},
		},
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := signedJSONRequest(t, fixture.remoteFed, http.MethodPost, "/federation/v1/dm/message", messagePayload)

		fixture.sync.HandleFederatedDMMessage(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("dm message attempt %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}

	var messageRows int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM messages WHERE id = $1 AND channel_id = $2 AND instance_id = $3`,
		messageID, localChannelID, fixture.remoteID).Scan(&messageRows); err != nil {
		t.Fatalf("query federated message: %v", err)
	}
	if messageRows != 1 {
		t.Fatalf("message rows = %d, want 1", messageRows)
	}

	var attachmentRows int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM attachments WHERE id = $1 AND message_id = $2 AND instance_id = $3`,
		attachmentID, messageID, fixture.remoteID).Scan(&attachmentRows); err != nil {
		t.Fatalf("query federated attachment: %v", err)
	}
	if attachmentRows != 1 {
		t.Fatalf("attachment rows = %d, want 1", attachmentRows)
	}
}

type federationFixture struct {
	localID      string
	localDomain  string
	remoteID     string
	remoteDomain string
	localFed     *federation.Service
	remoteFed    *federation.Service
	sync         *federation.SyncService
	prefix       string
}

func newFederationFixture(t *testing.T, name string) federationFixture {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().Format("150405000000000")
	localID := name + "-local-" + suffix
	remoteID := name + "-remote-" + suffix
	localDomain := localID + ".example"
	remoteDomain := remoteID + ".example"

	_, localPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("local keygen: %v", err)
	}
	remotePub, remotePriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("remote keygen: %v", err)
	}

	insertInstance(t, ctx, localID, localDomain, marshalPublicKeyPEM(t, localPriv.Public().(ed25519.PublicKey)))
	insertInstance(t, ctx, remoteID, remoteDomain, marshalPublicKeyPEM(t, remotePub))

	localFed := federation.New(federation.Config{
		Pool:       testPool,
		InstanceID: localID,
		Domain:     localDomain,
		PrivateKey: localPriv,
		Logger:     testLogger,
	})
	remoteFed := federation.New(federation.Config{
		Pool:       testPool,
		InstanceID: remoteID,
		Domain:     remoteDomain,
		PrivateKey: remotePriv,
		Logger:     testLogger,
	})

	return federationFixture{
		localID:      localID,
		localDomain:  localDomain,
		remoteID:     remoteID,
		remoteDomain: remoteDomain,
		localFed:     localFed,
		remoteFed:    remoteFed,
		sync:         federation.NewSyncService(localFed, testBus, testLogger, federation.SyncConfig{BackfillWindowDays: 7}),
		prefix:       name + "-" + suffix,
	}
}

func (f federationFixture) id(kind string) string {
	return f.prefix + "-" + kind
}

func signedJSONRequest(t *testing.T, svc *federation.Service, method, path string, payload interface{}) *http.Request {
	t.Helper()
	body := signSyncRequest(t, svc, payload)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

func withChiParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func insertUser(t *testing.T, ctx context.Context, id, instanceID, username string) {
	t.Helper()
	_, err := testPool.Exec(ctx,
		`INSERT INTO users (id, instance_id, username, password_hash, created_at)
		 VALUES ($1, $2, $3, 'hash', now())`,
		id, instanceID, username)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func insertGuild(t *testing.T, ctx context.Context, id, instanceID, ownerID, name string, discoverable bool) {
	t.Helper()
	_, err := testPool.Exec(ctx,
		`INSERT INTO guilds (id, instance_id, owner_id, name, discoverable, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		id, instanceID, ownerID, name, discoverable)
	if err != nil {
		t.Fatalf("insert guild %s: %v", id, err)
	}
}

func insertChannel(t *testing.T, ctx context.Context, id, guildID, instanceID, name string) {
	t.Helper()
	_, err := testPool.Exec(ctx,
		`INSERT INTO channels (id, guild_id, instance_id, channel_type, name, position, created_at)
		 VALUES ($1, $2, $3, 'text', $4, 0, now())`,
		id, guildID, instanceID, name)
	if err != nil {
		t.Fatalf("insert channel %s: %v", id, err)
	}
}
