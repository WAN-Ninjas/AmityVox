package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// maxRetryAttempts is the maximum number of delivery attempts before moving
// a message to the dead letter queue.
const maxRetryAttempts = 10

// FederatedMessage is the envelope for messages sent between federated instances.
type FederatedMessage struct {
	Type      string       `json:"type"`                  // Event type (e.g., MESSAGE_CREATE)
	OriginID  string       `json:"origin_id"`             // Originating instance ID
	Timestamp HLCTimestamp `json:"timestamp"`              // HLC timestamp for causal ordering
	GuildID   string       `json:"guild_id,omitempty"`
	ChannelID string       `json:"channel_id,omitempty"`
	Data      interface{}  `json:"data"`                   // Event payload
}

// InboxRequest is the incoming request body at /federation/v1/inbox.
type InboxRequest struct {
	SignedPayload
}

// retryMessage is the format for the federation retry queue.
type retryMessage struct {
	Domain   string         `json:"domain"`
	PeerID   string         `json:"peer_id,omitempty"`
	Signed   *SignedPayload `json:"signed"`
	Attempts int            `json:"attempts"`
}

// peerTarget holds a peer's domain and ID for delivery.
type peerTarget struct {
	domain string
	peerID string
}

// SyncService handles federation message routing and delivery.
type SyncService struct {
	fed        *Service
	bus        *events.Bus
	hlc        *HLC
	logger     *slog.Logger
	client     *http.Client
	voiceSvc   VoiceTokenGenerator // optional, for federated voice
	liveKitURL string              // public LiveKit URL for this instance
}

// VoiceTokenGenerator is the subset of voice.Service that federation needs.
type VoiceTokenGenerator interface {
	GenerateToken(userID, channelID string, canPublish, canSubscribe, canVideo bool, metadata string) (string, error)
	EnsureRoom(ctx context.Context, channelID string) error
}

// NewSyncService creates a new federation sync service.
func NewSyncService(fed *Service, bus *events.Bus, logger *slog.Logger) *SyncService {
	return &SyncService{
		fed:    fed,
		bus:    bus,
		hlc:    NewHLC(),
		logger: logger,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// SetVoiceService configures the voice service for federated voice token generation.
func (ss *SyncService) SetVoiceService(voiceSvc VoiceTokenGenerator, liveKitPublicURL string) {
	ss.voiceSvc = voiceSvc
	ss.liveKitURL = liveKitPublicURL
}

// HandleInbox handles POST /federation/v1/inbox — receives signed messages from
// remote instances, verifies the signature, checks federation permissions,
// persists message events to the local database, and dispatches to the event bus.
func (ss *SyncService) HandleInbox(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var signed SignedPayload
	if err := json.Unmarshal(body, &signed); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Look up sender's public key.
	var publicKeyPEM string
	err = ss.fed.pool.QueryRow(r.Context(),
		`SELECT public_key FROM instances WHERE id = $1`, signed.SenderID,
	).Scan(&publicKeyPEM)
	if err == pgx.ErrNoRows {
		ss.logger.Info("unknown sender, attempting discovery", slog.String("sender_id", signed.SenderID))
		http.Error(w, "Unknown sender instance", http.StatusForbidden)
		return
	}
	if err != nil {
		ss.logger.Error("failed to look up sender", slog.String("error", err.Error()))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Verify signature.
	valid, err := VerifySignature(publicKeyPEM, signed.Payload, signed.Signature)
	if err != nil || !valid {
		ss.logger.Warn("invalid federation signature", slog.String("sender_id", signed.SenderID))
		http.Error(w, "Invalid signature", http.StatusForbidden)
		return
	}

	// Check timestamp freshness.
	if msg := validateTimestamp(signed.Timestamp); msg != "" {
		ss.logger.Warn("inbox rejected: stale timestamp",
			slog.String("sender_id", signed.SenderID),
			slog.String("detail", msg))
		http.Error(w, "Stale or future timestamp", http.StatusBadRequest)
		return
	}

	// Verify source IP.
	if ipMsg := ss.fed.verifySourceIP(r, signed.SenderID); ipMsg != "" {
		ss.logger.Warn("inbox source IP mismatch",
			slog.String("sender_id", signed.SenderID),
			slog.String("detail", ipMsg))
		if ss.fed.enforceIPCheck {
			http.Error(w, "Source IP mismatch", http.StatusForbidden)
			return
		}
	}

	// Check federation is allowed from this sender.
	allowed, err := ss.fed.IsFederationAllowed(r.Context(), signed.SenderID)
	if err != nil || !allowed {
		http.Error(w, "Federation not allowed", http.StatusForbidden)
		return
	}

	// Decode the federated message.
	var msg FederatedMessage
	if err := json.Unmarshal(signed.Payload, &msg); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Update HLC with remote timestamp.
	ss.hlc.Update(msg.Timestamp)

	// Update last_seen_at for the remote instance.
	ss.fed.pool.Exec(r.Context(),
		`UPDATE instances SET last_seen_at = now() WHERE id = $1`, signed.SenderID)

	// Update last_synced_at for the federation peer.
	ss.fed.pool.Exec(r.Context(),
		`UPDATE federation_peers SET last_synced_at = now()
		 WHERE instance_id = $1 AND peer_id = $2`,
		ss.fed.instanceID, signed.SenderID)

	// Persist inbound message events to the local database.
	if msg.ChannelID != "" {
		eventData, err := json.Marshal(msg.Data)
		if err != nil {
			ss.logger.Warn("failed to marshal inbound event data",
				slog.String("sender_id", signed.SenderID),
				slog.String("type", msg.Type),
				slog.String("error", err.Error()),
			)
		} else {
			ss.persistInboundMessage(r.Context(), signed.SenderID, msg.Type, msg.ChannelID, eventData)
		}
	}

	// Dispatch to local event bus for gateway and workers.
	eventData, err := json.Marshal(msg.Data)
	if err != nil {
		http.Error(w, "Failed to process event", http.StatusInternalServerError)
		return
	}

	event := events.Event{
		Type:      federationToGatewayType(msg.Type),
		GuildID:   msg.GuildID,
		ChannelID: msg.ChannelID,
		Data:      eventData,
	}

	subject := eventTypeToSubject(msg.Type)
	if subject != "" {
		if err := ss.bus.Publish(r.Context(), subject, event); err != nil {
			ss.logger.Error("failed to publish federated event",
				slog.String("type", msg.Type),
				slog.String("error", err.Error()),
			)
		}
	}

	// Update federation guild cache for guild-level events from remote instances.
	if msg.GuildID != "" {
		ss.updateGuildCacheFromEvent(r.Context(), signed.SenderID, msg.Type, msg.GuildID, eventData)
	}

	// Track inbound event count for the sender peer.
	ss.fed.IncrementPeerEventCount(r.Context(), signed.SenderID, false)

	ss.logger.Debug("received federated event",
		slog.String("type", msg.Type),
		slog.String("sender", signed.SenderID),
	)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// persistInboundMessage writes inbound federated message events to the local
// database using federation_channel_mirrors to map remote channel IDs to local ones.
func (ss *SyncService) persistInboundMessage(ctx context.Context, remoteInstanceID, eventType, remoteChannelID string, data json.RawMessage) {
	// Look up the local channel ID via mirror mapping, scoped to the sender instance.
	var localChannelID string
	err := ss.fed.pool.QueryRow(ctx,
		`SELECT local_channel_id FROM federation_channel_mirrors
		 WHERE remote_channel_id = $1 AND remote_instance_id = $2 LIMIT 1`,
		remoteChannelID, remoteInstanceID,
	).Scan(&localChannelID)
	if err != nil {
		if err != pgx.ErrNoRows {
			ss.logger.Warn("failed to lookup channel mirror",
				slog.String("remote_channel_id", remoteChannelID),
				slog.String("remote_instance_id", remoteInstanceID),
				slog.String("error", err.Error()),
			)
		}
		// No mirror for this channel — skip persistence (channel setup happens in DM/guild PRs).
		return
	}

	switch eventType {
	case "MESSAGE_CREATE":
		var msgData struct {
			ID          string          `json:"id"`
			AuthorID    string          `json:"author_id"`
			Content     string          `json:"content"`
			Attachments json.RawMessage `json:"attachments,omitempty"`
			Embeds      json.RawMessage `json:"embeds,omitempty"`
			CreatedAt   *time.Time      `json:"created_at,omitempty"`
		}
		if err := json.Unmarshal(data, &msgData); err != nil {
			ss.logger.Warn("failed to unmarshal inbound message", slog.String("error", err.Error()))
			return
		}
		createdAt := time.Now().UTC()
		if msgData.CreatedAt != nil {
			createdAt = *msgData.CreatedAt
		}
		_, err := ss.fed.pool.Exec(ctx,
			`INSERT INTO messages (id, channel_id, author_id, content, created_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (id) DO NOTHING`,
			msgData.ID, localChannelID, msgData.AuthorID, msgData.Content, createdAt)
		if err != nil {
			ss.logger.Warn("failed to persist inbound message",
				slog.String("message_id", msgData.ID),
				slog.String("error", err.Error()))
		}

	case "MESSAGE_UPDATE":
		var msgData struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(data, &msgData); err != nil {
			ss.logger.Warn("failed to unmarshal inbound message update", slog.String("error", err.Error()))
			return
		}
		if _, err := ss.fed.pool.Exec(ctx,
			`UPDATE messages SET content = $1, edited_at = now() WHERE id = $2 AND channel_id = $3`,
			msgData.Content, msgData.ID, localChannelID); err != nil {
			ss.logger.Warn("failed to persist inbound message update",
				slog.String("message_id", msgData.ID),
				slog.String("error", err.Error()))
		}

	case "MESSAGE_DELETE":
		var msgData struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &msgData); err != nil {
			ss.logger.Warn("failed to unmarshal inbound message delete", slog.String("error", err.Error()))
			return
		}
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM messages WHERE id = $1 AND channel_id = $2`,
			msgData.ID, localChannelID); err != nil {
			ss.logger.Warn("failed to persist inbound message delete",
				slog.String("message_id", msgData.ID),
				slog.String("error", err.Error()))
		}

	case "TYPING_START":
		// No DB persistence needed — just let the event flow through NATS to the gateway.

	case "REACTION_ADD":
		var rxData struct {
			MessageID string `json:"message_id"`
			UserID    string `json:"user_id"`
			Emoji     string `json:"emoji"`
		}
		if err := json.Unmarshal(data, &rxData); err != nil {
			ss.logger.Warn("failed to unmarshal inbound reaction add", slog.String("error", err.Error()))
			return
		}
		if _, err := ss.fed.pool.Exec(ctx,
			`INSERT INTO message_reactions (message_id, user_id, emoji, created_at)
			 VALUES ($1, $2, $3, now()) ON CONFLICT DO NOTHING`,
			rxData.MessageID, rxData.UserID, rxData.Emoji); err != nil {
			ss.logger.Warn("failed to persist inbound reaction add",
				slog.String("message_id", rxData.MessageID),
				slog.String("error", err.Error()))
		}

	case "REACTION_REMOVE":
		var rxData struct {
			MessageID string `json:"message_id"`
			UserID    string `json:"user_id"`
			Emoji     string `json:"emoji"`
		}
		if err := json.Unmarshal(data, &rxData); err != nil {
			ss.logger.Warn("failed to unmarshal inbound reaction remove", slog.String("error", err.Error()))
			return
		}
		if _, err := ss.fed.pool.Exec(ctx,
			`DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
			rxData.MessageID, rxData.UserID, rxData.Emoji); err != nil {
			ss.logger.Warn("failed to persist inbound reaction remove",
				slog.String("message_id", rxData.MessageID),
				slog.String("error", err.Error()))
		}

	case "CHANNEL_PINS_UPDATE":
		// Pins update — let the event flow through to clients via NATS.
		// No local persistence needed since pin state is managed by the remote instance.
	}
}

// DeliverToAllPeers sends a signed event to all active federation peers.
func (ss *SyncService) DeliverToAllPeers(ctx context.Context, msg FederatedMessage) {
	msg.OriginID = ss.fed.instanceID
	msg.Timestamp = ss.hlc.Now()

	signed, err := ss.fed.Sign(msg)
	if err != nil {
		ss.logger.Error("failed to sign federation message",
			slog.String("type", msg.Type),
			slog.String("error", err.Error()),
		)
		return
	}

	// Get all active peers with their IDs and domains.
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT fp.peer_id, i.domain FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.status = 'active'`,
		ss.fed.instanceID)
	if err != nil {
		ss.logger.Error("failed to query federation peers", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	var peers []peerTarget
	for rows.Next() {
		var p peerTarget
		if err := rows.Scan(&p.peerID, &p.domain); err == nil {
			peers = append(peers, p)
		}
	}
	if err := rows.Err(); err != nil {
		ss.logger.Error("failed to iterate federation peers", slog.String("error", err.Error()))
	}

	for _, peer := range peers {
		go ss.deliverToPeer(ctx, peer.domain, peer.peerID, signed)
	}
}

// DeliverToChannelPeers sends a signed event only to instances that have
// registered interest in a specific channel via federation_channel_peers.
func (ss *SyncService) DeliverToChannelPeers(ctx context.Context, msg FederatedMessage) {
	msg.OriginID = ss.fed.instanceID
	msg.Timestamp = ss.hlc.Now()

	signed, err := ss.fed.Sign(msg)
	if err != nil {
		ss.logger.Error("failed to sign federation message",
			slog.String("type", msg.Type),
			slog.String("error", err.Error()),
		)
		return
	}

	rows, err := ss.fed.pool.Query(ctx,
		`SELECT fp.peer_id, i.domain
		 FROM federation_channel_peers fcp
		 JOIN federation_peers fp
		   ON fp.peer_id = fcp.instance_id
		  AND fp.instance_id = $1
		  AND fp.status = 'active'
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fcp.channel_id = $2`,
		ss.fed.instanceID, msg.ChannelID)
	if err != nil {
		ss.logger.Error("failed to query channel peers",
			slog.String("channel_id", msg.ChannelID),
			slog.String("error", err.Error()),
		)
		return
	}
	defer rows.Close()

	var peers []peerTarget
	for rows.Next() {
		var p peerTarget
		if err := rows.Scan(&p.peerID, &p.domain); err == nil {
			peers = append(peers, p)
		}
	}
	if err := rows.Err(); err != nil {
		ss.logger.Error("failed to iterate channel peers", slog.String("error", err.Error()))
	}

	if len(peers) == 0 {
		// No channel-specific peers — fall through to broadcast.
		ss.DeliverToAllPeers(ctx, msg)
		return
	}

	for _, peer := range peers {
		go ss.deliverToPeer(ctx, peer.domain, peer.peerID, signed)
	}
}

// deliverToPeer sends a signed payload to a specific peer instance.
func (ss *SyncService) deliverToPeer(ctx context.Context, domain, peerID string, signed *SignedPayload) {
	url := fmt.Sprintf("https://%s/federation/v1/inbox", domain)

	body, err := json.Marshal(signed)
	if err != nil {
		ss.logger.Error("failed to marshal signed payload", slog.String("error", err.Error()))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		ss.logger.Error("failed to create request", slog.String("domain", domain), slog.String("error", err.Error()))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	resp, err := ss.client.Do(req)
	if err != nil {
		ss.logger.Warn("federation delivery failed",
			slog.String("domain", domain),
			slog.String("error", err.Error()),
		)
		ss.fed.IncrementPeerErrors(ctx, peerID)
		ss.queueForRetry(domain, peerID, signed, 0)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		ss.logger.Warn("federation delivery rejected",
			slog.String("domain", domain),
			slog.Int("status", resp.StatusCode),
		)
		ss.fed.IncrementPeerErrors(ctx, peerID)
		if resp.StatusCode >= 500 {
			ss.queueForRetry(domain, peerID, signed, 0)
		}
		return
	}

	// Delivery succeeded — update health tracking.
	ss.fed.IncrementPeerEventCount(ctx, peerID, true)
	ss.fed.UpdatePeerHealth(ctx, peerID, true, 0)

	ss.logger.Debug("federated event delivered", slog.String("domain", domain))
}

// queueForRetry publishes a failed delivery to the federation JetStream for retry.
func (ss *SyncService) queueForRetry(domain, peerID string, signed *SignedPayload, attempts int) {
	msg := retryMessage{
		Domain:   domain,
		PeerID:   peerID,
		Signed:   signed,
		Attempts: attempts + 1,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		ss.logger.Error("failed to marshal retry message",
			slog.String("domain", domain),
			slog.String("error", err.Error()),
		)
		return
	}

	if err := ss.bus.Publish(context.Background(), events.SubjectFederationRetry, events.Event{
		Type: "FEDERATION_RETRY",
		Data: data,
	}); err != nil {
		ss.logger.Error("failed to enqueue federation retry",
			slog.String("domain", domain),
			slog.String("error", err.Error()),
		)
	}
}

// StartRouter subscribes to local events that should be federated to peers
// and starts the JetStream retry queue consumer.
func (ss *SyncService) StartRouter(ctx context.Context) {
	// Federated event types and their subjects.
	subjects := []string{
		events.SubjectMessageCreate,
		events.SubjectMessageUpdate,
		events.SubjectMessageDelete,
		events.SubjectMessageReactionAdd,
		events.SubjectMessageReactionDel,
		events.SubjectGuildCreate,
		events.SubjectGuildUpdate,
		events.SubjectGuildMemberAdd,
		events.SubjectGuildMemberRemove,
		events.SubjectChannelCreate,
		events.SubjectChannelUpdate,
		events.SubjectChannelDelete,
		events.SubjectChannelPinsUpdate,
		events.SubjectTypingStart,
		events.SubjectVoiceStateUpdate,
		events.SubjectCallRing,
	}

	for _, subject := range subjects {
		sub := subject // capture for closure
		ss.bus.QueueSubscribe(sub, "federation-router", func(event events.Event) {
			ss.routeEvent(ctx, event)
		})
	}

	// Start the JetStream retry queue consumer.
	ss.startRetryConsumer(ctx)

	ss.logger.Info("federation router started", slog.Int("subjects", len(subjects)))
}

// startRetryConsumer subscribes to the federation retry JetStream subject and
// processes failed deliveries with exponential backoff.
func (ss *SyncService) startRetryConsumer(ctx context.Context) {
	js := ss.bus.JetStream()

	sub, err := js.QueueSubscribe(events.SubjectFederationRetry, "federation-retry", func(natsMsg *nats.Msg) {
		var evt events.Event
		if err := json.Unmarshal(natsMsg.Data, &evt); err != nil {
			ss.logger.Error("failed to unmarshal retry event", slog.String("error", err.Error()))
			natsMsg.Ack()
			return
		}

		var retry retryMessage
		if err := json.Unmarshal(evt.Data, &retry); err != nil {
			ss.logger.Error("failed to unmarshal retry message", slog.String("error", err.Error()))
			natsMsg.Ack()
			return
		}

		// Use JetStream NumDelivered for accurate attempt tracking across redeliveries.
		attempt := retry.Attempts
		if md, err := natsMsg.Metadata(); err == nil {
			attempt = int(md.NumDelivered) - 1
		}

		if attempt >= maxRetryAttempts {
			// Move to dead letter queue.
			retry.Attempts = attempt
			ss.insertDeadLetter(ctx, retry)
			natsMsg.Ack()
			return
		}

		// Attempt redelivery.
		ss.logger.Info("retrying federation delivery",
			slog.String("domain", retry.Domain),
			slog.Int("attempt", attempt),
		)

		deliverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		url := fmt.Sprintf("https://%s/federation/v1/inbox", retry.Domain)
		body, err := json.Marshal(retry.Signed)
		if err != nil {
			ss.logger.Error("failed to marshal retry payload, dropping message",
				slog.String("domain", retry.Domain),
				slog.String("error", err.Error()),
			)
			natsMsg.Ack()
			return
		}

		req, err := http.NewRequestWithContext(deliverCtx, "POST", url, bytes.NewReader(body))
		if err != nil {
			ss.logger.Error("failed to create retry request",
				slog.String("domain", retry.Domain),
				slog.String("error", err.Error()),
			)
			natsMsg.NakWithDelay(retryDelay(attempt))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

		resp, err := ss.client.Do(req)
		if err != nil {
			delay := retryDelay(attempt)
			ss.logger.Warn("federation retry failed",
				slog.String("domain", retry.Domain),
				slog.Int("attempt", attempt),
				slog.String("error", err.Error()),
				slog.Duration("next_retry", delay),
			)
			natsMsg.NakWithDelay(delay)
			return
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
			// Delivery succeeded.
			if retry.PeerID != "" {
				ss.fed.IncrementPeerEventCount(ctx, retry.PeerID, true)
				ss.fed.UpdatePeerHealth(ctx, retry.PeerID, true, 0)
			}
			ss.logger.Info("federation retry succeeded",
				slog.String("domain", retry.Domain),
				slog.Int("attempt", attempt),
			)
			natsMsg.Ack()
			return
		}

		if resp.StatusCode >= 500 {
			natsMsg.NakWithDelay(retryDelay(attempt))
			return
		}

		// 4xx — permanent failure, dead letter it.
		retry.Attempts = attempt
		ss.insertDeadLetter(ctx, retry)
		natsMsg.Ack()
	}, nats.Durable("federation-retry-consumer"), nats.ManualAck(),
		nats.AckWait(30*time.Second), nats.MaxDeliver(maxRetryAttempts+5))
	if err != nil {
		ss.logger.Error("failed to subscribe to federation retry queue", slog.String("error", err.Error()))
		return
	}

	ss.logger.Info("federation retry consumer started", slog.String("subject", events.SubjectFederationRetry))
	_ = sub // subscription managed by NATS connection lifecycle
}

// insertDeadLetter inserts a permanently failed delivery into the dead letter table.
func (ss *SyncService) insertDeadLetter(ctx context.Context, retry retryMessage) {
	payloadJSON, marshalErr := json.Marshal(retry.Signed)
	errorMsg := fmt.Sprintf("exhausted %d retry attempts", retry.Attempts)
	if marshalErr != nil {
		ss.logger.Error("failed to marshal dead letter payload",
			slog.String("domain", retry.Domain),
			slog.String("error", marshalErr.Error()),
		)
		payloadJSON = []byte(`{"error":"payload marshal failed"}`)
		errorMsg = fmt.Sprintf("exhausted %d retry attempts; payload marshal error: %s", retry.Attempts, marshalErr.Error())
	}
	id := models.NewULID().String()

	_, execErr := ss.fed.pool.Exec(ctx,
		`INSERT INTO federation_dead_letters (id, target_domain, payload, error_message, attempts, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		id, retry.Domain, payloadJSON,
		errorMsg,
		retry.Attempts)
	if execErr != nil {
		ss.logger.Error("failed to insert dead letter",
			slog.String("domain", retry.Domain),
			slog.String("error", execErr.Error()),
		)
		return
	}

	ss.logger.Warn("federation delivery moved to dead letters",
		slog.String("domain", retry.Domain),
		slog.Int("attempts", retry.Attempts),
	)
}

// retryDelay returns the backoff delay for a given attempt number.
// Schedule: 5s, 30s, 2m, 10m, 1h (capped).
func retryDelay(attempt int) time.Duration {
	delays := []time.Duration{
		5 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		1 * time.Hour,
	}
	if attempt < len(delays) {
		return delays[attempt]
	}
	return delays[len(delays)-1]
}

// routeEvent converts a local event to a FederatedMessage and delivers it to
// the appropriate peers. If the event has a ChannelID, it uses targeted delivery
// to only reach instances with members in that channel.
func (ss *SyncService) routeEvent(ctx context.Context, event events.Event) {
	var data interface{}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	msg := FederatedMessage{
		Type:      event.Type,
		GuildID:   event.GuildID,
		ChannelID: event.ChannelID,
		Data:      data,
	}

	if event.ChannelID != "" {
		ss.DeliverToChannelPeers(ctx, msg)
	} else {
		ss.DeliverToAllPeers(ctx, msg)
	}
}

// ProcessRetryQueue is a no-op — retry processing is handled by the JetStream
// consumer started in StartRouter.
func (ss *SyncService) ProcessRetryQueue(_ context.Context) error {
	return nil
}

// federationToGatewayType translates federation wire protocol event type names
// to the gateway event type names that clients expect. Only types that differ
// between the federation protocol and the gateway are mapped; all others pass through.
func federationToGatewayType(fedType string) string {
	switch fedType {
	case "REACTION_ADD":
		return "MESSAGE_REACTION_ADD"
	case "REACTION_REMOVE":
		return "MESSAGE_REACTION_REMOVE"
	default:
		return fedType
	}
}

// eventTypeToSubject maps event type strings to NATS subjects for dispatching
// received federated events into the local event bus.
func eventTypeToSubject(eventType string) string {
	mapping := map[string]string{
		"MESSAGE_CREATE":       events.SubjectMessageCreate,
		"MESSAGE_UPDATE":       events.SubjectMessageUpdate,
		"MESSAGE_DELETE":       events.SubjectMessageDelete,
		"REACTION_ADD":         events.SubjectMessageReactionAdd,
		"REACTION_REMOVE":      events.SubjectMessageReactionDel,
		"GUILD_CREATE":         events.SubjectGuildCreate,
		"GUILD_UPDATE":         events.SubjectGuildUpdate,
		"GUILD_MEMBER_ADD":     events.SubjectGuildMemberAdd,
		"GUILD_MEMBER_REMOVE":  events.SubjectGuildMemberRemove,
		"CHANNEL_CREATE":       events.SubjectChannelCreate,
		"CHANNEL_UPDATE":       events.SubjectChannelUpdate,
		"CHANNEL_DELETE":       events.SubjectChannelDelete,
		"CHANNEL_PINS_UPDATE":  events.SubjectChannelPinsUpdate,
		"TYPING_START":         events.SubjectTypingStart,
		"VOICE_STATE_UPDATE":   events.SubjectVoiceStateUpdate,
		"CALL_RING":            events.SubjectCallRing,
	}
	return mapping[eventType]
}

// RetryDelay is exported for testing.
func RetryDelay(attempt int) time.Duration {
	return retryDelay(attempt)
}

