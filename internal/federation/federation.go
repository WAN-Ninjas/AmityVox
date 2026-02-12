// Package federation implements the AmityVox instance-to-instance federation protocol.
// It handles instance discovery (.well-known endpoint), signed payload exchange using
// Ed25519 keys, and the framework for message synchronization between federated
// instances. See docs/architecture.md Section 7 for the full protocol specification.
package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/models"
)

// Version is the federation protocol version.
const Version = "amityvox-federation/1.0"

// VersionNext is the next protocol version with enhanced capabilities.
const VersionNext = "amityvox-federation/1.1"

// SupportedVersions lists all protocol versions this instance can negotiate.
var SupportedVersions = []string{Version, VersionNext}

// DefaultCapabilities lists the default capabilities advertised by this instance.
var DefaultCapabilities = []string{
	"messages", "presence", "profiles", "channels", "guilds",
	"reactions", "attachments", "embeds", "typing",
}

// CapabilityDeliveryReceipts is the capability flag for delivery receipt support.
const CapabilityDeliveryReceipts = "delivery_receipts"

// CapabilityFederatedSearch is the capability flag for federated search support.
const CapabilityFederatedSearch = "federated_search"

// CapabilityBridge is the capability flag for bridge attribution support.
const CapabilityBridge = "bridge_attribution"

// DiscoveryResponse is the payload returned by /.well-known/amityvox for
// instance discovery.
type DiscoveryResponse struct {
	InstanceID         string   `json:"instance_id"`
	Domain             string   `json:"domain"`
	Name               *string  `json:"name,omitempty"`
	PublicKey          string   `json:"public_key"`
	Software           string   `json:"software"`
	SoftwareVersion    string   `json:"software_version"`
	FederationMode     string   `json:"federation_mode"`
	APIEndpoint        string   `json:"api_endpoint"`
	SupportedProtocols []string `json:"supported_protocols"`
	Capabilities       []string `json:"capabilities,omitempty"`
}

// HandshakeRequest is sent by an initiating instance to establish a federation
// peering relationship. It includes the sender's protocol versions and
// capabilities so both sides can negotiate a common feature set.
type HandshakeRequest struct {
	SenderID           string   `json:"sender_id"`
	SenderDomain       string   `json:"sender_domain"`
	ProtocolVersion    string   `json:"protocol_version"`
	SupportedVersions  []string `json:"supported_versions"`
	Capabilities       []string `json:"capabilities"`
	Timestamp          time.Time `json:"timestamp"`
}

// HandshakeResponse is returned by the receiving instance. NegotiatedVersion
// is the highest common protocol version both peers support.
type HandshakeResponse struct {
	Accepted           bool     `json:"accepted"`
	NegotiatedVersion  string   `json:"negotiated_version"`
	Capabilities       []string `json:"capabilities"`
	Reason             string   `json:"reason,omitempty"`
}

// DeliveryReceipt confirms delivery (or failure) of a federated message.
type DeliveryReceipt struct {
	MessageID      string    `json:"message_id"`
	SourceInstance string    `json:"source_instance"`
	TargetInstance string    `json:"target_instance"`
	Status         string    `json:"status"` // delivered, failed
	Timestamp      time.Time `json:"timestamp"`
	Error          string    `json:"error,omitempty"`
}

// SignedPayload wraps a federation message with Ed25519 signature for
// authenticity verification.
type SignedPayload struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"` // hex-encoded Ed25519 signature
	SenderID  string          `json:"sender_id"` // instance ID of the sender
	Timestamp time.Time       `json:"timestamp"`
}

// Service provides federation operations.
type Service struct {
	pool       *pgxpool.Pool
	instanceID string
	domain     string
	privateKey ed25519.PrivateKey
	logger     *slog.Logger
}

// Config holds the configuration for the federation service.
type Config struct {
	Pool       *pgxpool.Pool
	InstanceID string
	Domain     string
	PrivateKey ed25519.PrivateKey // loaded from PEM at startup
	Logger     *slog.Logger
}

// New creates a new federation service.
func New(cfg Config) *Service {
	return &Service{
		pool:       cfg.Pool,
		instanceID: cfg.InstanceID,
		domain:     cfg.Domain,
		privateKey: cfg.PrivateKey,
		logger:     cfg.Logger,
	}
}

// HandleDiscovery handles GET /.well-known/amityvox — the federation discovery
// endpoint that other instances use to find this instance's public key, API
// endpoint, and federation capabilities.
func (s *Service) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	var inst models.Instance
	err := s.pool.QueryRow(r.Context(),
		`SELECT id, domain, public_key, name, software, software_version, federation_mode
		 FROM instances WHERE id = $1`, s.instanceID).Scan(
		&inst.ID, &inst.Domain, &inst.PublicKey, &inst.Name,
		&inst.Software, &inst.SoftwareVersion, &inst.FederationMode,
	)
	if err != nil {
		s.logger.Error("federation discovery: failed to query instance",
			slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	version := "0.1.0"
	if inst.SoftwareVersion != nil {
		version = *inst.SoftwareVersion
	}

	// Load capabilities from database.
	var capsJSON json.RawMessage
	capErr := s.pool.QueryRow(r.Context(),
		`SELECT COALESCE(capabilities, '[]'::jsonb) FROM instances WHERE id = $1`,
		s.instanceID).Scan(&capsJSON)
	var caps []string
	if capErr == nil {
		json.Unmarshal(capsJSON, &caps)
	}
	if len(caps) == 0 {
		caps = DefaultCapabilities
	}

	resp := DiscoveryResponse{
		InstanceID:         inst.ID,
		Domain:             inst.Domain,
		Name:               inst.Name,
		PublicKey:          inst.PublicKey,
		Software:           inst.Software,
		SoftwareVersion:    version,
		FederationMode:     inst.FederationMode,
		APIEndpoint:        fmt.Sprintf("https://%s/federation/v1", inst.Domain),
		SupportedProtocols: SupportedVersions,
		Capabilities:       caps,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(resp)
}

// Sign creates a signed payload from the given data using this instance's
// Ed25519 private key.
func (s *Service) Sign(data interface{}) (*SignedPayload, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}

	signature := ed25519.Sign(s.privateKey, payload)

	return &SignedPayload{
		Payload:   payload,
		Signature: fmt.Sprintf("%x", signature),
		SenderID:  s.instanceID,
		Timestamp: time.Now().UTC(),
	}, nil
}

// VerifySignature verifies an Ed25519 signature against a public key PEM.
func VerifySignature(publicKeyPEM string, payload []byte, signatureHex string) (bool, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return false, fmt.Errorf("failed to decode PEM block")
	}

	pubKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("parsing public key: %w", err)
	}

	pubKey, ok := pubKeyInterface.(ed25519.PublicKey)
	if !ok {
		return false, fmt.Errorf("key is not Ed25519")
	}

	var sig []byte
	fmt.Sscanf(signatureHex, "%x", &sig)

	return ed25519.Verify(pubKey, payload, sig), nil
}

// DiscoverInstance fetches the .well-known/amityvox endpoint of a remote
// instance and returns its discovery response.
func DiscoverInstance(ctx context.Context, domain string) (*DiscoveryResponse, error) {
	url := fmt.Sprintf("https://%s/.well-known/amityvox", domain)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AmityVox/0.1.0 (+federation)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("instance %s returned status %d", domain, resp.StatusCode)
	}

	var discovery DiscoveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("decoding response from %s: %w", domain, err)
	}

	return &discovery, nil
}

// RegisterRemoteInstance saves a discovered remote instance to the database.
func (s *Service) RegisterRemoteInstance(ctx context.Context, disc *DiscoveryResponse) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO instances (id, domain, public_key, name, software, software_version,
		                        federation_mode, created_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (domain) DO UPDATE SET
			public_key = EXCLUDED.public_key,
			name = EXCLUDED.name,
			software = EXCLUDED.software,
			software_version = EXCLUDED.software_version,
			federation_mode = EXCLUDED.federation_mode,
			last_seen_at = EXCLUDED.last_seen_at`,
		disc.InstanceID, disc.Domain, disc.PublicKey, disc.Name,
		disc.Software, disc.SoftwareVersion, disc.FederationMode, now, now,
	)
	if err != nil {
		return fmt.Errorf("registering remote instance %s: %w", disc.Domain, err)
	}
	return nil
}

// IsFederationAllowed checks whether a remote instance is allowed to federate
// with this instance based on the configured federation mode and per-peer
// controls (blocklist/allowlist).
func (s *Service) IsFederationAllowed(ctx context.Context, remoteInstanceID string) (bool, error) {
	// Check per-peer controls first — explicit block always wins.
	var peerAction string
	err := s.pool.QueryRow(ctx,
		`SELECT action FROM federation_peer_controls
		 WHERE instance_id = $1 AND peer_id = $2`,
		s.instanceID, remoteInstanceID).Scan(&peerAction)
	if err == nil {
		// Explicit per-peer control exists.
		if peerAction == "block" {
			return false, nil
		}
		if peerAction == "allow" {
			return true, nil
		}
		// "mute" falls through to federation mode check.
	}

	// Get local federation mode.
	var mode string
	err = s.pool.QueryRow(ctx,
		`SELECT federation_mode FROM instances WHERE id = $1`, s.instanceID).Scan(&mode)
	if err != nil {
		return false, fmt.Errorf("getting federation mode: %w", err)
	}

	switch mode {
	case "open":
		return true, nil
	case "closed":
		return false, nil
	case "allowlist":
		// Check if remote instance is in our federation peers with active status.
		var exists bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM federation_peers
				WHERE instance_id = $1 AND peer_id = $2 AND status = 'active'
			)`, s.instanceID, remoteInstanceID).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("checking allowlist: %w", err)
		}
		return exists, nil
	default:
		return false, nil
	}
}

// NegotiateProtocol performs version negotiation between this instance and a
// remote peer. It returns the highest mutually supported protocol version.
func NegotiateProtocol(localVersions, remoteVersions []string) string {
	// Build a set of remote versions for O(1) lookup.
	remoteSet := make(map[string]bool, len(remoteVersions))
	for _, v := range remoteVersions {
		remoteSet[v] = true
	}

	// Walk local versions from highest to lowest; return the first match.
	// SupportedVersions is ordered lowest-first, so iterate in reverse.
	for i := len(localVersions) - 1; i >= 0; i-- {
		if remoteSet[localVersions[i]] {
			return localVersions[i]
		}
	}

	// Fallback to the base version if no common version is found.
	return Version
}

// NegotiateCapabilities returns the intersection of local and remote capabilities.
func NegotiateCapabilities(local, remote []string) []string {
	remoteSet := make(map[string]bool, len(remote))
	for _, c := range remote {
		remoteSet[c] = true
	}

	result := make([]string, 0)
	for _, c := range local {
		if remoteSet[c] {
			result = append(result, c)
		}
	}
	return result
}

// HandleHandshake handles POST /federation/v1/handshake — the endpoint a
// remote instance calls to initiate or refresh a federation peering relationship.
func (s *Service) HandleHandshake(w http.ResponseWriter, r *http.Request) {
	var req HandshakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HandshakeResponse{
			Accepted: false,
			Reason:   "invalid request body",
		})
		return
	}

	// Check if federation is allowed for this peer.
	allowed, err := s.IsFederationAllowed(r.Context(), req.SenderID)
	if err != nil {
		s.logger.Error("handshake: federation check failed",
			slog.String("error", err.Error()),
			slog.String("sender", req.SenderDomain))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(HandshakeResponse{
			Accepted: false,
			Reason:   "internal error checking federation policy",
		})
		return
	}

	if !allowed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(HandshakeResponse{
			Accepted: false,
			Reason:   "federation not allowed by instance policy",
		})
		return
	}

	// Negotiate protocol version.
	negotiatedVersion := NegotiateProtocol(SupportedVersions, req.SupportedVersions)

	// Load local capabilities from the database.
	var capsJSON json.RawMessage
	err = s.pool.QueryRow(r.Context(),
		`SELECT COALESCE(capabilities, '[]'::jsonb) FROM instances WHERE id = $1`,
		s.instanceID).Scan(&capsJSON)
	if err != nil {
		capsJSON = []byte("[]")
	}

	var localCaps []string
	if err := json.Unmarshal(capsJSON, &localCaps); err != nil || len(localCaps) == 0 {
		localCaps = DefaultCapabilities
	}

	negotiatedCaps := NegotiateCapabilities(localCaps, req.Capabilities)

	// Update the peer status with the negotiated version and capabilities.
	negotiatedCapsJSON, _ := json.Marshal(negotiatedCaps)
	s.pool.Exec(r.Context(),
		`INSERT INTO federation_peer_status (peer_id, instance_id, status, version, capabilities, last_check_at, updated_at)
		 VALUES ($1, $2, 'healthy', $3, $4, now(), now())
		 ON CONFLICT (peer_id) DO UPDATE SET
			status = 'healthy', version = $3, capabilities = $4,
			last_check_at = now(), updated_at = now()`,
		req.SenderID, s.instanceID, negotiatedVersion, negotiatedCapsJSON)

	resp := HandshakeResponse{
		Accepted:          true,
		NegotiatedVersion: negotiatedVersion,
		Capabilities:      negotiatedCaps,
	}

	s.logger.Info("federation handshake accepted",
		slog.String("peer", req.SenderDomain),
		slog.String("version", negotiatedVersion),
		slog.Int("capabilities", len(negotiatedCaps)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RecordDeliveryReceipt stores a delivery receipt for a federated message.
func (s *Service) RecordDeliveryReceipt(ctx context.Context, receipt DeliveryReceipt) error {
	id := models.NewULID().String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO federation_delivery_receipts
		 (id, message_id, source_instance, target_instance, status, attempts, last_attempt_at, delivered_at, error_message)
		 VALUES ($1, $2, $3, $4, $5, 1, now(), CASE WHEN $5 = 'delivered' THEN now() ELSE NULL END, $6)
		 ON CONFLICT (id) DO NOTHING`,
		id, receipt.MessageID, receipt.SourceInstance, receipt.TargetInstance,
		receipt.Status, receipt.Error)
	if err != nil {
		return fmt.Errorf("recording delivery receipt: %w", err)
	}
	return nil
}

// HandleDeliveryReceipt handles POST /federation/v1/delivery-receipt — accepts
// delivery confirmations from remote instances.
func (s *Service) HandleDeliveryReceipt(w http.ResponseWriter, r *http.Request) {
	var receipt DeliveryReceipt
	if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if receipt.MessageID == "" || receipt.SourceInstance == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	receipt.TargetInstance = s.instanceID

	if err := s.RecordDeliveryReceipt(r.Context(), receipt); err != nil {
		s.logger.Error("failed to record delivery receipt",
			slog.String("error", err.Error()),
			slog.String("message_id", receipt.MessageID))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

// UpdatePeerHealth updates the health status of a federation peer after a
// successful or failed event exchange.
func (s *Service) UpdatePeerHealth(ctx context.Context, peerID string, healthy bool, eventLagMs int) {
	status := "healthy"
	if !healthy {
		status = "degraded"
	}

	s.pool.Exec(ctx,
		`INSERT INTO federation_peer_status (peer_id, instance_id, status, event_lag_ms, last_event_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (peer_id) DO UPDATE SET
			status = $3, event_lag_ms = $4, last_event_at = now(), updated_at = now()`,
		peerID, s.instanceID, status, eventLagMs)
}

// IncrementPeerEventCount increments the sent/received event counters for a peer.
func (s *Service) IncrementPeerEventCount(ctx context.Context, peerID string, sent bool) {
	if sent {
		s.pool.Exec(ctx,
			`UPDATE federation_peer_status SET events_sent = events_sent + 1, updated_at = now()
			 WHERE peer_id = $1`, peerID)
	} else {
		s.pool.Exec(ctx,
			`UPDATE federation_peer_status SET events_received = events_received + 1, updated_at = now()
			 WHERE peer_id = $1`, peerID)
	}
}

// IncrementPeerErrors increments the 24h error counter for a peer.
func (s *Service) IncrementPeerErrors(ctx context.Context, peerID string) {
	s.pool.Exec(ctx,
		`UPDATE federation_peer_status SET errors_24h = errors_24h + 1, updated_at = now()
		 WHERE peer_id = $1`, peerID)
}

// GetPeerCapabilities returns the negotiated capabilities for a given peer.
func (s *Service) GetPeerCapabilities(ctx context.Context, peerID string) ([]string, error) {
	var capsJSON json.RawMessage
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(capabilities, '[]'::jsonb) FROM federation_peer_status WHERE peer_id = $1`,
		peerID).Scan(&capsJSON)
	if err != nil {
		return DefaultCapabilities, nil
	}

	var caps []string
	if err := json.Unmarshal(capsJSON, &caps); err != nil {
		return DefaultCapabilities, nil
	}
	return caps, nil
}

// PeerSupportsCapability checks if a given peer supports a specific capability.
func (s *Service) PeerSupportsCapability(ctx context.Context, peerID, capability string) bool {
	caps, _ := s.GetPeerCapabilities(ctx, peerID)
	for _, c := range caps {
		if c == capability {
			return true
		}
	}
	return false
}
