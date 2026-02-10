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

	resp := DiscoveryResponse{
		InstanceID:         inst.ID,
		Domain:             inst.Domain,
		Name:               inst.Name,
		PublicKey:          inst.PublicKey,
		Software:           inst.Software,
		SoftwareVersion:    version,
		FederationMode:     inst.FederationMode,
		APIEndpoint:        fmt.Sprintf("https://%s/federation/v1", inst.Domain),
		SupportedProtocols: []string{Version},
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
// with this instance based on the configured federation mode.
func (s *Service) IsFederationAllowed(ctx context.Context, remoteInstanceID string) (bool, error) {
	// Get local federation mode.
	var mode string
	err := s.pool.QueryRow(ctx,
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
