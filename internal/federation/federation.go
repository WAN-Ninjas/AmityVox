// Package federation implements the AmityVox instance-to-instance federation protocol.
// It handles instance discovery (.well-known endpoint), signed payload exchange using
// Ed25519 keys, and the framework for message synchronization between federated
// instances. See docs/architecture.md Section 7 for the full protocol specification.
package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/models"
)

// Version is the federation protocol version.
const Version = "amityvox-federation/1.0"

// VersionNext is the next protocol version with enhanced capabilities.
const VersionNext = "amityvox-federation/1.1"

// SupportedVersions lists all protocol versions this instance can negotiate.
var SupportedVersions = []string{Version, VersionNext}

// UsernameRegex validates usernames for federation and user lookup requests.
var UsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{2,32}$`)

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
	LiveKitURL         string   `json:"livekit_url,omitempty"`
	Shorthand          *string  `json:"shorthand,omitempty"`
	VoiceMode          string   `json:"voice_mode,omitempty"`
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
	pool            *pgxpool.Pool
	instanceID      string
	domain          string
	privateKey      ed25519.PrivateKey
	enforceIPCheck  bool
	logger          *slog.Logger
	onPeerRecovered func(ctx context.Context, peerID string) // called when a peer transitions to healthy

	// Caches to eliminate hot-path DB queries on the inbox path.
	allowedCache *TTLCache[bool]   // remoteInstanceID -> allowed
	pubKeyCache  *TTLCache[string] // instanceID -> public_key PEM
	fedModeCache *TTLCache[string] // "__local__" -> federation_mode

	// Batched counter increments — flushed every 5s by StartCounterFlusher.
	counterMu       sync.Mutex
	pendingCounters map[string]*counterEntry

	// onInstanceRegistered is called after RegisterRemoteInstance successfully
	// registers or updates an instance, so SyncService can clear its negative cache.
	onInstanceRegistered func(instanceID string)
}

// Config holds the configuration for the federation service.
type Config struct {
	Pool           *pgxpool.Pool
	InstanceID     string
	Domain         string
	PrivateKey     ed25519.PrivateKey // loaded from PEM at startup
	EnforceIPCheck bool
	Logger         *slog.Logger
}

// counterEntry accumulates sent/received event counts per peer for batch flushing.
type counterEntry struct {
	sent     int64
	received int64
}

// New creates a new federation service.
func New(cfg Config) *Service {
	s := &Service{
		pool:            cfg.Pool,
		instanceID:      cfg.InstanceID,
		domain:          cfg.Domain,
		privateKey:      cfg.PrivateKey,
		enforceIPCheck:  cfg.EnforceIPCheck,
		logger:          cfg.Logger,
		allowedCache:    NewTTLCache[bool](60*time.Second, 500),
		pubKeyCache:     NewTTLCache[string](5*time.Minute, 500),
		fedModeCache:    NewTTLCache[string](60*time.Second, 1),
		pendingCounters: make(map[string]*counterEntry),
	}

	// Pre-load federation mode cache at startup.
	var mode string
	if err := cfg.Pool.QueryRow(context.Background(),
		`SELECT federation_mode FROM instances WHERE id = $1`, cfg.InstanceID,
	).Scan(&mode); err == nil {
		s.fedModeCache.Set("__local__", mode)
	}

	return s
}

// SetOnPeerRecovered sets a callback that fires when a peer transitions from
// non-healthy to healthy status. Used by SyncService to trigger backfill.
func (s *Service) SetOnPeerRecovered(fn func(ctx context.Context, peerID string)) {
	s.onPeerRecovered = fn
}

// SetOnInstanceRegistered sets a callback that fires after RegisterRemoteInstance
// successfully registers or updates a remote instance. Used by SyncService to
// clear its negative cache for the new instance ID.
func (s *Service) SetOnInstanceRegistered(fn func(instanceID string)) {
	s.onInstanceRegistered = fn
}

// InvalidateAllowedCache removes a peer from the allowed cache, forcing
// the next IsFederationAllowed call to re-query the database. Called by
// admin peer control handlers when block/allow/mute actions change.
func (s *Service) InvalidateAllowedCache(peerID string) {
	s.allowedCache.Invalidate(peerID)
}

// InvalidateFedModeCache clears the cached federation_mode, forcing
// IsFederationAllowed to re-read it from DB on next call.
func (s *Service) InvalidateFedModeCache() {
	s.fedModeCache.InvalidateAll()
}

// HandleDiscovery handles GET /.well-known/amityvox — the federation discovery
// endpoint that other instances use to find this instance's public key, API
// endpoint, and federation capabilities.
func (s *Service) HandleDiscovery(w http.ResponseWriter, r *http.Request) {
	var inst models.Instance
	err := s.pool.QueryRow(r.Context(),
		`SELECT id, domain, public_key, name, description, software, software_version,
		        federation_mode, protocol_version, capabilities, livekit_url, private_key_pem,
		        resolved_ips, key_fingerprint, shorthand, voice_mode, created_at, last_seen_at
		 FROM instances WHERE id = $1`, s.instanceID).Scan(
		&inst.ID, &inst.Domain, &inst.PublicKey, &inst.Name, &inst.Description,
		&inst.Software, &inst.SoftwareVersion, &inst.FederationMode,
		&inst.ProtocolVersion, &inst.Capabilities, &inst.LiveKitURL, &inst.PrivateKeyPEM,
		&inst.ResolvedIPs, &inst.KeyFingerprint, &inst.Shorthand, &inst.VoiceMode,
		&inst.CreatedAt, &inst.LastSeenAt,
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

	// Parse capabilities from the instance record.
	var caps []string
	if len(inst.Capabilities) > 0 {
		json.Unmarshal(inst.Capabilities, &caps)
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
		Shorthand:          inst.Shorthand,
		VoiceMode:          inst.VoiceMode,
	}
	if inst.LiveKitURL != nil {
		resp.LiveKitURL = *inst.LiveKitURL
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(resp)
}

// Sign creates a signed payload from the given data using this instance's
// Ed25519 private key.
func (s *Service) Sign(data interface{}) (*SignedPayload, error) {
	if len(s.privateKey) == 0 {
		return nil, fmt.Errorf("federation private key not configured")
	}

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

// ValidateFederationDomain checks that a domain is a valid public hostname and
// not an internal/private address to prevent SSRF attacks.
func ValidateFederationDomain(domain string) error {
	// Block obviously internal domains.
	lower := strings.ToLower(domain)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("internal domain not allowed for federation")
	}

	// Resolve the domain and block private/loopback IPs.
	ips, err := net.LookupHost(domain)
	if err != nil {
		return fmt.Errorf("domain does not resolve: %w", err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("domain %s resolves to private/loopback address", domain)
		}
	}
	return nil
}

// DiscoverInstance fetches the .well-known/amityvox endpoint of a remote
// instance and returns its discovery response.
func DiscoverInstance(ctx context.Context, domain string) (*DiscoveryResponse, error) {
	if err := ValidateFederationDomain(domain); err != nil {
		return nil, fmt.Errorf("domain validation failed: %w", err)
	}

	url := fmt.Sprintf("https://%s/.well-known/amityvox", domain)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AmityVox/0.1.0 (+federation)")

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			if r.URL.Scheme != "https" {
				return errors.New("redirects must use https")
			}
			if err := ValidateFederationDomain(r.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
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
// It also computes a key fingerprint and logs a warning if the key changed.
func (s *Service) RegisterRemoteInstance(ctx context.Context, disc *DiscoveryResponse) error {
	now := time.Now().UTC()

	// Compute fingerprint for the new key.
	newFingerprint, fpErr := ComputeKeyFingerprint(disc.PublicKey)
	if fpErr != nil {
		s.logger.Warn("could not compute key fingerprint for remote instance",
			slog.String("domain", disc.Domain), slog.String("error", fpErr.Error()))
	}

	// Check for key change (only if instance already exists).
	var oldPublicKey, oldFingerprint string
	err := s.pool.QueryRow(ctx,
		`SELECT public_key, COALESCE(key_fingerprint, '') FROM instances WHERE id = $1`,
		disc.InstanceID,
	).Scan(&oldPublicKey, &oldFingerprint)
	if err == nil && oldPublicKey != "" && oldPublicKey != disc.PublicKey && fpErr == nil {
		// Key changed — record in audit log.
		if oldFingerprint == "" {
			oldFingerprint, _ = ComputeKeyFingerprint(oldPublicKey)
		}
		auditID := models.NewULID().String()
		if _, aErr := s.pool.Exec(ctx,
			`INSERT INTO federation_key_audit (id, instance_id, old_fingerprint, new_fingerprint, old_public_key, detected_at)
			 VALUES ($1, $2, $3, $4, $5, now())`,
			auditID, disc.InstanceID, oldFingerprint, newFingerprint, oldPublicKey,
		); aErr != nil {
			s.logger.Warn("failed to record key audit", slog.String("error", aErr.Error()))
		}
		s.logger.Warn("federation key change detected",
			slog.String("instance", disc.Domain),
			slog.String("old_fingerprint", oldFingerprint),
			slog.String("new_fingerprint", newFingerprint))
	}

	// Resolve shorthand collisions: if the remote instance advertises a shorthand
	// that already belongs to a different instance, append a numeric suffix.
	remoteShorthand := disc.Shorthand
	if remoteShorthand != nil && *remoteShorthand != "" {
		resolved, shErr := s.resolveShorthandCollision(ctx, disc.InstanceID, *remoteShorthand)
		if shErr != nil {
			s.logger.Warn("shorthand collision resolution failed, clearing shorthand",
				slog.String("domain", disc.Domain), slog.String("error", shErr.Error()))
			remoteShorthand = nil
		} else {
			remoteShorthand = &resolved
		}
	}

	voiceMode := disc.VoiceMode
	if voiceMode == "" {
		voiceMode = "direct"
	}

	// Check if domain already exists with a different ID (rebuilt peer).
	migrated := false
	var existingID string
	idErr := s.pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`, disc.Domain,
	).Scan(&existingID)
	if idErr == nil && existingID != disc.InstanceID {
		// Instance rebuilt with new ID — migrate all FK references atomically.
		s.logger.Warn("instance ID change detected, migrating references",
			slog.String("domain", disc.Domain),
			slog.String("old_id", existingID),
			slog.String("new_id", disc.InstanceID))

		if err := s.migrateInstanceID(ctx, existingID, disc.InstanceID); err != nil {
			return fmt.Errorf("migrating instance ID from %s to %s: %w", existingID, disc.InstanceID, err)
		}
		migrated = true

		// Invalidate caches for both old and new IDs.
		s.allowedCache.Invalidate(existingID)
		s.allowedCache.Invalidate(disc.InstanceID)
		s.pubKeyCache.Invalidate(existingID)
		s.pubKeyCache.Invalidate(disc.InstanceID)

		if s.onInstanceRegistered != nil {
			s.onInstanceRegistered(disc.InstanceID)
			s.onInstanceRegistered(existingID)
		}
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO instances (id, domain, public_key, name, software, software_version,
		                        federation_mode, key_fingerprint, shorthand, voice_mode, created_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (domain) DO UPDATE SET
			public_key = EXCLUDED.public_key,
			name = EXCLUDED.name,
			software = EXCLUDED.software,
			software_version = EXCLUDED.software_version,
			federation_mode = EXCLUDED.federation_mode,
			key_fingerprint = EXCLUDED.key_fingerprint,
			shorthand = EXCLUDED.shorthand,
			voice_mode = EXCLUDED.voice_mode,
			last_seen_at = EXCLUDED.last_seen_at`,
		disc.InstanceID, disc.Domain, disc.PublicKey, disc.Name,
		disc.Software, disc.SoftwareVersion, disc.FederationMode, newFingerprint,
		remoteShorthand, voiceMode, now, now,
	)
	if err != nil {
		return fmt.Errorf("registering remote instance %s: %w", disc.Domain, err)
	}

	if !migrated && s.onInstanceRegistered != nil {
		s.onInstanceRegistered(disc.InstanceID)
	}
	return nil
}

// migrateInstanceID updates all FK references from oldID to newID in a single
// transaction, then updates the instances PK itself. This handles rebuilt peers
// that generate a new ULID but keep the same domain.
func (s *Service) migrateInstanceID(ctx context.Context, oldID, newID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting ID migration tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update all child tables that reference instances(id).
	fkUpdates := []string{
		`UPDATE federation_peers SET peer_id = $1 WHERE peer_id = $2`,
		`UPDATE federation_peers SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE federation_peer_status SET peer_id = $1 WHERE peer_id = $2`,
		`UPDATE federation_peer_status SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE federation_peer_controls SET peer_id = $1 WHERE peer_id = $2`,
		`UPDATE federation_peer_controls SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE federation_channel_peers SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE federation_channel_mirrors SET remote_instance_id = $1 WHERE remote_instance_id = $2`,
		`UPDATE federation_events SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE guilds SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE users SET instance_id = $1 WHERE instance_id = $2`,
		`UPDATE federation_key_audit SET instance_id = $1 WHERE instance_id = $2`,
	}
	for _, q := range fkUpdates {
		if _, err := tx.Exec(ctx, q, newID, oldID); err != nil {
			return fmt.Errorf("updating FK: %w", err)
		}
	}

	// Update the PK itself now that all FKs point to the new ID.
	if _, err := tx.Exec(ctx, `UPDATE instances SET id = $1 WHERE id = $2`, newID, oldID); err != nil {
		return fmt.Errorf("updating instances PK: %w", err)
	}

	return tx.Commit(ctx)
}

// resolveShorthandCollision checks if a shorthand is already used by a different
// instance and, if so, appends numeric suffixes (1, 2, 3, ...) until a unique
// shorthand is found. Returns the resolved (possibly suffixed) shorthand.
func (s *Service) resolveShorthandCollision(ctx context.Context, instanceID, shorthand string) (string, error) {
	// Check if this exact shorthand is already used by a different instance.
	var existingID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE shorthand = $1 AND id <> $2 LIMIT 1`,
		shorthand, instanceID,
	).Scan(&existingID)
	if err != nil {
		// No conflict (ErrNoRows) or query error.
		if err == pgx.ErrNoRows {
			return shorthand, nil
		}
		return "", fmt.Errorf("checking shorthand collision: %w", err)
	}

	// Collision found — try numeric suffixes.
	for i := 1; i <= 99; i++ {
		candidate := fmt.Sprintf("%s%d", shorthand, i)
		if len(candidate) > 5 {
			// Truncate the base to fit within 5 chars.
			maxBase := 5 - len(fmt.Sprintf("%d", i))
			if maxBase < 1 {
				return "", fmt.Errorf("shorthand %q cannot be resolved within 5-char limit", shorthand)
			}
			candidate = fmt.Sprintf("%s%d", shorthand[:maxBase], i)
		}

		err = s.pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE shorthand = $1 AND id <> $2 LIMIT 1`,
			candidate, instanceID,
		).Scan(&existingID)
		if err == pgx.ErrNoRows {
			s.logger.Info("resolved shorthand collision",
				slog.String("original", shorthand),
				slog.String("resolved", candidate),
				slog.String("instance", instanceID))
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking shorthand collision for %q: %w", candidate, err)
		}
	}

	return "", fmt.Errorf("could not resolve shorthand collision for %q after 99 attempts", shorthand)
}

// RefreshPeerKeys re-discovers all active federation peers and updates their
// public keys. This handles the case where peers regenerated their keys (e.g.,
// after a migration that stores private keys in the DB for the first time).
func (s *Service) RefreshPeerKeys(ctx context.Context) {
	rows, err := s.pool.Query(ctx,
		`SELECT i.domain FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1 AND fp.status = 'active'`,
		s.instanceID)
	if err != nil {
		s.logger.Warn("failed to query federation peers for key refresh", slog.String("error", err.Error()))
		return
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			s.logger.Warn("failed to scan federation peer domain", slog.String("error", err.Error()))
			continue
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		s.logger.Warn("error iterating federation peers", slog.String("error", err.Error()))
		return
	}

	for _, domain := range domains {
		disc, err := DiscoverInstance(ctx, domain)
		if err != nil {
			s.logger.Warn("failed to refresh peer key",
				slog.String("peer", domain), slog.String("error", err.Error()))
			continue
		}
		if err := s.RegisterRemoteInstance(ctx, disc); err != nil {
			s.logger.Warn("failed to update peer key",
				slog.String("peer", domain), slog.String("error", err.Error()))
			continue
		}
		// Invalidate caches for the refreshed peer.
		s.pubKeyCache.Invalidate(disc.InstanceID)
		s.allowedCache.Invalidate(disc.InstanceID)
		s.logger.Info("refreshed federation peer key", slog.String("peer", domain))
	}
}

// IsFederationAllowed checks whether a remote instance is allowed to federate
// with this instance based on the configured federation mode and per-peer
// controls (blocklist/allowlist).
func (s *Service) IsFederationAllowed(ctx context.Context, remoteInstanceID string) (bool, error) {
	// Check cache first — eliminates 2-3 DB queries per inbox request.
	if allowed, ok := s.allowedCache.Get(remoteInstanceID); ok {
		return allowed, nil
	}

	// Check per-peer controls first — explicit block always wins.
	var peerAction string
	err := s.pool.QueryRow(ctx,
		`SELECT action FROM federation_peer_controls
		 WHERE instance_id = $1 AND peer_id = $2`,
		s.instanceID, remoteInstanceID).Scan(&peerAction)
	if err == nil {
		if peerAction == "block" {
			s.allowedCache.Set(remoteInstanceID, false)
			return false, nil
		}
		if peerAction == "allow" {
			s.allowedCache.Set(remoteInstanceID, true)
			return true, nil
		}
		// "mute" falls through to federation mode check.
	}

	// Get local federation mode — use cache, fall back to DB on miss.
	mode, ok := s.fedModeCache.Get("__local__")
	if !ok {
		err = s.pool.QueryRow(ctx,
			`SELECT federation_mode FROM instances WHERE id = $1`, s.instanceID).Scan(&mode)
		if err != nil {
			return false, fmt.Errorf("getting federation mode: %w", err)
		}
		s.fedModeCache.Set("__local__", mode)
	}

	var result bool
	switch mode {
	case "open":
		result = true
	case "closed":
		result = false
	case "allowlist":
		var exists bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM federation_peers
				WHERE instance_id = $1 AND peer_id = $2 AND status = 'active'
			)`, s.instanceID, remoteInstanceID).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("checking allowlist: %w", err)
		}
		result = exists
	default:
		result = false
	}

	s.allowedCache.Set(remoteInstanceID, result)
	return result, nil
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
// Creates a reverse peer record so both instances are aware of the peering.
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

	// Verify timestamp freshness.
	if msg := validateTimestamp(req.Timestamp); msg != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(HandshakeResponse{
			Accepted: false,
			Reason:   msg,
		})
		return
	}

	// Discover sender to ensure instances row is current (handles rebuilt peers).
	if req.SenderDomain != "" {
		disc, discErr := DiscoverInstance(r.Context(), req.SenderDomain)
		if discErr == nil && disc != nil {
			if disc.InstanceID != "" && disc.InstanceID != req.SenderID {
				s.logger.Warn("handshake sender mismatch",
					slog.String("domain", req.SenderDomain),
					slog.String("request_id", req.SenderID),
					slog.String("discovery_id", disc.InstanceID))
				http.Error(w, "sender_id does not match discovery response", http.StatusBadRequest)
				return
			}
			if regErr := s.RegisterRemoteInstance(r.Context(), disc); regErr != nil {
				s.logger.Warn("handshake: failed to register/update sender instance",
					slog.String("domain", req.SenderDomain),
					slog.String("error", regErr.Error()))
			}
		}
	}

	// Verify source IP matches sender domain.
	if ipMsg := s.verifySourceIP(r, req.SenderID); ipMsg != "" {
		s.logger.Warn("handshake IP mismatch",
			slog.String("sender", req.SenderDomain),
			slog.String("detail", ipMsg))
		if s.enforceIPCheck {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(HandshakeResponse{
				Accepted: false,
				Reason:   "source IP does not match sender domain",
			})
			return
		}
	}

	// Resolve and store sender's IPs.
	if req.SenderDomain != "" {
		if ips, err := net.LookupHost(req.SenderDomain); err == nil && len(ips) > 0 {
			s.pool.Exec(r.Context(),
				`UPDATE instances SET resolved_ips = $1 WHERE id = $2`,
				ips, req.SenderID)
		}
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

	// Get local federation mode.
	var localMode string
	s.pool.QueryRow(r.Context(),
		`SELECT federation_mode FROM instances WHERE id = $1`, s.instanceID).Scan(&localMode)

	// Create or update reverse peer record.
	if allowed {
		// For open mode: auto-approve; for allowlist mode: leave as pending.
		peerStatus := models.FederationPeerPending
		var handshakeCompletedAt *time.Time
		if localMode == "open" {
			peerStatus = models.FederationPeerActive
			now := time.Now().UTC()
			handshakeCompletedAt = &now
		}
		s.pool.Exec(r.Context(),
			`INSERT INTO federation_peers (instance_id, peer_id, status, established_at, initiated_by, handshake_completed_at)
			 VALUES ($1, $2, $3, now(), 'remote', $4)
			 ON CONFLICT (instance_id, peer_id) DO UPDATE SET
				handshake_completed_at = COALESCE(federation_peers.handshake_completed_at, $4),
				initiated_by = COALESCE(federation_peers.initiated_by, 'remote')`,
			s.instanceID, req.SenderID, peerStatus, handshakeCompletedAt)
	}

	if !allowed {
		// For allowlist mode, create pending peer but reject the handshake.
		if localMode == "allowlist" {
			s.pool.Exec(r.Context(),
				`INSERT INTO federation_peers (instance_id, peer_id, status, established_at, initiated_by)
				 VALUES ($1, $2, 'pending', now(), 'remote')
				 ON CONFLICT (instance_id, peer_id) DO NOTHING`,
				s.instanceID, req.SenderID)
		}
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
// successful or failed event exchange. If the peer transitions from a
// non-healthy status (degraded, unreachable, unknown) to healthy, it triggers
// a backfill request via the onPeerRecovered callback.
func (s *Service) UpdatePeerHealth(ctx context.Context, peerID string, healthy bool, eventLagMs int) {
	status := "healthy"
	if !healthy {
		status = "degraded"
	}

	// Check previous status to detect recovery transitions.
	var previousStatus string
	err := s.pool.QueryRow(ctx,
		`SELECT status FROM federation_peer_status WHERE peer_id = $1`, peerID,
	).Scan(&previousStatus)
	wasUnhealthy := err != nil || (previousStatus != "healthy")

	s.pool.Exec(ctx,
		`INSERT INTO federation_peer_status (peer_id, instance_id, status, event_lag_ms, last_event_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (peer_id) DO UPDATE SET
			status = $3, event_lag_ms = $4, last_event_at = now(), updated_at = now()`,
		peerID, s.instanceID, status, eventLagMs)

	// Trigger backfill when a peer recovers to healthy from a non-healthy state.
	if healthy && wasUnhealthy && s.onPeerRecovered != nil {
		go s.onPeerRecovered(context.Background(), peerID)
	}
}

// IncrementPeerEventCount accumulates sent/received event counts in memory.
// Counts are flushed to the database in batch every 5 seconds by StartCounterFlusher.
func (s *Service) IncrementPeerEventCount(_ context.Context, peerID string, sent bool) {
	s.counterMu.Lock()
	defer s.counterMu.Unlock()
	entry, ok := s.pendingCounters[peerID]
	if !ok {
		entry = &counterEntry{}
		s.pendingCounters[peerID] = entry
	}
	if sent {
		entry.sent++
	} else {
		entry.received++
	}
}

// StartCounterFlusher starts a background goroutine that flushes accumulated
// event counters to the database every 5 seconds. On context cancellation it
// performs a final flush before returning.
func (s *Service) StartCounterFlusher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.flushCounters(context.Background())
			case <-ctx.Done():
				s.flushCounters(context.Background())
				return
			}
		}
	}()
}

// flushCounters swaps the pending counters map and writes accumulated deltas
// to federation_peer_status in a single transaction.
func (s *Service) flushCounters(ctx context.Context) {
	s.counterMu.Lock()
	batch := s.pendingCounters
	s.pendingCounters = make(map[string]*counterEntry)
	s.counterMu.Unlock()

	if len(batch) == 0 {
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.logger.Error("counter flush: failed to begin tx", slog.String("error", err.Error()))
		// Put counters back so they're not lost.
		s.counterMu.Lock()
		for peerID, entry := range batch {
			if existing, ok := s.pendingCounters[peerID]; ok {
				existing.sent += entry.sent
				existing.received += entry.received
			} else {
				s.pendingCounters[peerID] = entry
			}
		}
		s.counterMu.Unlock()
		return
	}
	defer tx.Rollback(ctx)

	for peerID, entry := range batch {
		if _, err := tx.Exec(ctx,
			`INSERT INTO federation_peer_status (peer_id, instance_id, events_sent, events_received, updated_at)
			 VALUES ($3, $4, $1, $2, now())
			 ON CONFLICT (peer_id) DO UPDATE SET
			   events_sent = federation_peer_status.events_sent + EXCLUDED.events_sent,
			   events_received = federation_peer_status.events_received + EXCLUDED.events_received,
			   updated_at = now()`,
			entry.sent, entry.received, peerID, s.instanceID); err != nil {
			s.logger.Warn("counter flush: failed to update peer",
				slog.String("peer_id", peerID), slog.String("error", err.Error()))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("counter flush: failed to commit", slog.String("error", err.Error()))
		// Re-add counters so they're not lost.
		s.counterMu.Lock()
		for peerID, entry := range batch {
			if existing, ok := s.pendingCounters[peerID]; ok {
				existing.sent += entry.sent
				existing.received += entry.received
			} else {
				s.pendingCounters[peerID] = entry
			}
		}
		s.counterMu.Unlock()
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

// HandleUserLookup handles GET /federation/v1/users/lookup?username=... — a public
// endpoint that allows remote instances to look up a local user by username.
// Rate-limited. Returns 403 if the instance's federation_mode is not "open".
func (s *Service) HandleUserLookup(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "missing username parameter", http.StatusBadRequest)
		return
	}

	// Validate username format.
	if !UsernameRegex.MatchString(username) {
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}

	// Check federation mode.
	var mode string
	err := s.pool.QueryRow(r.Context(),
		`SELECT federation_mode FROM instances WHERE id = $1`, s.instanceID).Scan(&mode)
	if err != nil {
		s.logger.Error("user lookup: failed to get federation mode", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if mode != "open" {
		http.Error(w, "federation lookups are not enabled on this instance", http.StatusForbidden)
		return
	}

	// Look up the user.
	var user struct {
		ID          string
		Username    string
		DisplayName *string
		AvatarID    *string
		Bio         *string
		CreatedAt   time.Time
	}
	err = s.pool.QueryRow(r.Context(),
		`SELECT id, username, display_name, avatar_id, bio, created_at
		 FROM users
		 WHERE LOWER(username) = LOWER($1) AND instance_id = $2`,
		username, s.instanceID,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarID, &user.Bio, &user.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "user not found", http.StatusNotFound)
		} else {
			s.logger.Error("federation user lookup failed", slog.String("error", err.Error()))
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           user.ID,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"avatar_id":    user.AvatarID,
		"bio":          user.Bio,
		"created_at":   user.CreatedAt.Format(time.RFC3339),
	})
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

// ComputeKeyFingerprint computes the SHA-256 fingerprint of a PEM-encoded public key.
func ComputeKeyFingerprint(publicKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}
	hash := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(hash[:]), nil
}

// validateTimestamp checks that a federation payload timestamp is fresh.
// Rejects payloads older than 5 minutes or more than 30 seconds in the future.
func validateTimestamp(ts time.Time) string {
	now := time.Now().UTC()
	age := now.Sub(ts)
	if age > 5*time.Minute {
		return fmt.Sprintf("timestamp too old: %s ago", age.Truncate(time.Second))
	}
	if age < -30*time.Second {
		return fmt.Sprintf("timestamp too far in the future: %s ahead", (-age).Truncate(time.Second))
	}
	return ""
}

// verifySourceIP checks that the connecting IP matches the stored resolved IPs
// for a sender instance. Returns an empty string if valid, or a warning message.
func (s *Service) verifySourceIP(r *http.Request, senderID string) string {
	// Extract connecting IP (strip port).
	connectIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(connectIP); err == nil {
		connectIP = host
	}

	// Look up stored resolved IPs.
	var resolvedIPs []string
	err := s.pool.QueryRow(r.Context(),
		`SELECT resolved_ips FROM instances WHERE id = $1`, senderID,
	).Scan(&resolvedIPs)
	if err != nil || len(resolvedIPs) == 0 {
		// No stored IPs — fall back to live DNS resolution.
		var domain string
		if dErr := s.pool.QueryRow(r.Context(),
			`SELECT domain FROM instances WHERE id = $1`, senderID,
		).Scan(&domain); dErr != nil {
			return "could not resolve sender domain"
		}
		ips, lookupErr := net.LookupHost(domain)
		if lookupErr != nil {
			return fmt.Sprintf("DNS lookup failed for %s: %s", domain, lookupErr)
		}
		resolvedIPs = ips
	}

	for _, ip := range resolvedIPs {
		if ip == connectIP {
			return ""
		}
	}

	return fmt.Sprintf("connecting IP %s not in resolved IPs for sender", connectIP)
}

// SendHandshake sends a handshake request to a remote instance and returns the response.
func (s *Service) SendHandshake(ctx context.Context, remoteDomain string) (*HandshakeResponse, error) {
	if err := ValidateFederationDomain(remoteDomain); err != nil {
		return nil, fmt.Errorf("domain validation: %w", err)
	}

	req := HandshakeRequest{
		SenderID:          s.instanceID,
		SenderDomain:      s.domain,
		ProtocolVersion:   Version,
		SupportedVersions: SupportedVersions,
		Capabilities:      DefaultCapabilities,
		Timestamp:         time.Now().UTC(),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling handshake: %w", err)
	}

	target := (&url.URL{Scheme: "https", Host: remoteDomain, Path: "/federation/v1/handshake"}).String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", target, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "AmityVox/1.0 (+federation)")

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("stopped after 3 redirects")
			}
			if r.URL.Scheme != "https" {
				return errors.New("redirects must use https")
			}
			if err := ValidateFederationDomain(r.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
	resp, err := client.Do(httpReq) // SSRF validated: domain checked by ValidateFederationDomain above
	if err != nil {
		return nil, fmt.Errorf("sending handshake to %s: %w", remoteDomain, err)
	}
	defer resp.Body.Close()

	var hsResp HandshakeResponse
	if err := json.NewDecoder(resp.Body).Decode(&hsResp); err != nil {
		return nil, fmt.Errorf("decoding handshake response: %w", err)
	}

	return &hsResp, nil
}
