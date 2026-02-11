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

	"github.com/amityvox/amityvox/internal/events"
)

// FederatedMessage is the envelope for messages sent between federated instances.
type FederatedMessage struct {
	Type       string       `json:"type"`                 // Event type (e.g., MESSAGE_CREATE)
	OriginID   string       `json:"origin_id"`            // Originating instance ID
	Timestamp  HLCTimestamp `json:"timestamp"`             // HLC timestamp for causal ordering
	GuildID    string       `json:"guild_id,omitempty"`
	ChannelID  string       `json:"channel_id,omitempty"`
	Data       interface{}  `json:"data"`                  // Event payload
}

// InboxRequest is the incoming request body at /federation/v1/inbox.
type InboxRequest struct {
	SignedPayload
}

// SyncService handles federation message routing and delivery.
type SyncService struct {
	fed    *Service
	bus    *events.Bus
	hlc    *HLC
	logger *slog.Logger
}

// NewSyncService creates a new federation sync service.
func NewSyncService(fed *Service, bus *events.Bus, logger *slog.Logger) *SyncService {
	return &SyncService{
		fed:    fed,
		bus:    bus,
		hlc:    NewHLC(),
		logger: logger,
	}
}

// HandleInbox handles POST /federation/v1/inbox — receives signed messages from
// remote instances, verifies the signature, checks federation permissions, and
// dispatches the event to the local event bus.
func (ss *SyncService) HandleInbox(w http.ResponseWriter, r *http.Request) {
	// Read request body.
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
		// Unknown instance — try to discover it.
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
		ss.logger.Warn("invalid federation signature",
			slog.String("sender_id", signed.SenderID),
		)
		http.Error(w, "Invalid signature", http.StatusForbidden)
		return
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

	// Dispatch to local event bus for gateway and workers.
	eventData, err := json.Marshal(msg.Data)
	if err != nil {
		http.Error(w, "Failed to process event", http.StatusInternalServerError)
		return
	}

	event := events.Event{
		Type:      msg.Type,
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

	ss.logger.Debug("received federated event",
		slog.String("type", msg.Type),
		slog.String("sender", signed.SenderID),
	)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
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

	// Get all active peers.
	rows, err := ss.fed.pool.Query(ctx,
		`SELECT i.domain FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.status = 'active'`,
		ss.fed.instanceID)
	if err != nil {
		ss.logger.Error("failed to query federation peers", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err == nil {
			domains = append(domains, domain)
		}
	}

	for _, domain := range domains {
		go ss.deliverToPeer(ctx, domain, signed)
	}
}

// deliverToPeer sends a signed payload to a specific peer instance.
func (ss *SyncService) deliverToPeer(ctx context.Context, domain string, signed *SignedPayload) {
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
	req.Header.Set("User-Agent", "AmityVox/0.2.0 (+federation)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ss.logger.Warn("federation delivery failed",
			slog.String("domain", domain),
			slog.String("error", err.Error()),
		)
		// Queue for retry via NATS JetStream.
		ss.queueForRetry(domain, signed)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		ss.logger.Warn("federation delivery rejected",
			slog.String("domain", domain),
			slog.Int("status", resp.StatusCode),
		)
		if resp.StatusCode >= 500 {
			ss.queueForRetry(domain, signed)
		}
		return
	}

	ss.logger.Debug("federated event delivered",
		slog.String("domain", domain),
	)
}

// queueForRetry publishes a failed delivery to the federation JetStream for retry.
func (ss *SyncService) queueForRetry(domain string, signed *SignedPayload) {
	retryMsg := map[string]interface{}{
		"domain": domain,
		"signed": signed,
	}
	data, err := json.Marshal(retryMsg)
	if err != nil {
		return
	}

	ss.bus.Publish(context.Background(), "amityvox.federation.retry", events.Event{
		Type: "FEDERATION_RETRY",
		Data: data,
	})
}

// StartRouter subscribes to local events that should be federated to peers.
// It watches message, guild, and channel events and forwards them.
func (ss *SyncService) StartRouter(ctx context.Context) {
	// Federated event types and their subjects.
	subjects := []string{
		events.SubjectMessageCreate,
		events.SubjectMessageUpdate,
		events.SubjectMessageDelete,
		events.SubjectGuildCreate,
		events.SubjectGuildUpdate,
		events.SubjectGuildMemberAdd,
		events.SubjectGuildMemberRemove,
		events.SubjectChannelCreate,
		events.SubjectChannelUpdate,
		events.SubjectChannelDelete,
	}

	for _, subject := range subjects {
		sub := subject // capture for closure
		ss.bus.QueueSubscribe(sub, "federation-router", func(event events.Event) {
			ss.routeEvent(ctx, event)
		})
	}

	ss.logger.Info("federation router started", slog.Int("subjects", len(subjects)))
}

// routeEvent converts a local event to a FederatedMessage and delivers it to peers.
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

	ss.DeliverToAllPeers(ctx, msg)
}

// ProcessRetryQueue processes the federation retry queue, attempting to redeliver
// failed messages. Called periodically by the worker manager.
func (ss *SyncService) ProcessRetryQueue(ctx context.Context) error {
	// Retry queue is consumed via NATS JetStream WorkQueue policy.
	// Messages are automatically redelivered after the ack wait period.
	// This method is a placeholder for any additional retry processing logic.
	return nil
}

// eventTypeToSubject maps event type strings to NATS subjects for dispatching
// received federated events into the local event bus.
func eventTypeToSubject(eventType string) string {
	mapping := map[string]string{
		"MESSAGE_CREATE":     events.SubjectMessageCreate,
		"MESSAGE_UPDATE":     events.SubjectMessageUpdate,
		"MESSAGE_DELETE":     events.SubjectMessageDelete,
		"GUILD_CREATE":       events.SubjectGuildCreate,
		"GUILD_UPDATE":       events.SubjectGuildUpdate,
		"GUILD_MEMBER_ADD":   events.SubjectGuildMemberAdd,
		"GUILD_MEMBER_REMOVE": events.SubjectGuildMemberRemove,
		"CHANNEL_CREATE":     events.SubjectChannelCreate,
		"CHANNEL_UPDATE":     events.SubjectChannelUpdate,
		"CHANNEL_DELETE":     events.SubjectChannelDelete,
	}
	return mapping[eventType]
}
