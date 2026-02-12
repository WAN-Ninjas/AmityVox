// Package config handles TOML configuration parsing for AmityVox. It loads
// configuration from amityvox.toml, applies environment variable overrides
// (prefixed with AMITYVOX_), validates required fields, and provides sane
// defaults for all settings.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Config is the top-level configuration for an AmityVox instance.
type Config struct {
	Instance  InstanceConfig  `toml:"instance"`
	Database  DatabaseConfig  `toml:"database"`
	NATS      NATSConfig      `toml:"nats"`
	Cache     CacheConfig     `toml:"cache"`
	Storage   StorageConfig   `toml:"storage"`
	LiveKit   LiveKitConfig   `toml:"livekit"`
	Search    SearchConfig    `toml:"search"`
	Auth      AuthConfig      `toml:"auth"`
	Media     MediaConfig     `toml:"media"`
	Push      PushConfig      `toml:"push"`
	HTTP      HTTPConfig      `toml:"http"`
	WebSocket WebSocketConfig `toml:"websocket"`
	Logging   LoggingConfig   `toml:"logging"`
	Metrics   MetricsConfig   `toml:"metrics"`
}

// InstanceConfig defines the identity of this AmityVox instance.
type InstanceConfig struct {
	Domain         string `toml:"domain"`
	Name           string `toml:"name"`
	Description    string `toml:"description"`
	FederationMode string `toml:"federation_mode"`
}

// DatabaseConfig defines PostgreSQL connection settings.
type DatabaseConfig struct {
	URL            string `toml:"url"`
	MaxConnections int    `toml:"max_connections"`
}

// NATSConfig defines NATS message broker connection settings.
type NATSConfig struct {
	URL string `toml:"url"`
}

// CacheConfig defines DragonflyDB/Redis connection settings.
type CacheConfig struct {
	URL string `toml:"url"`
}

// StorageConfig defines S3-compatible object storage settings.
type StorageConfig struct {
	Type      string `toml:"type"`
	Endpoint  string `toml:"endpoint"`
	Bucket    string `toml:"bucket"`
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
	Region    string `toml:"region"`
	UseSSL    bool   `toml:"use_ssl"`
}

// LiveKitConfig defines LiveKit voice/video server settings.
type LiveKitConfig struct {
	URL       string `toml:"url"`
	APIKey    string `toml:"api_key"`
	APISecret string `toml:"api_secret"`
}

// SearchConfig defines Meilisearch settings.
type SearchConfig struct {
	Enabled bool   `toml:"enabled"`
	URL     string `toml:"url"`
	APIKey  string `toml:"api_key"`
}

// AuthConfig defines authentication and registration settings.
type AuthConfig struct {
	SessionDuration     string         `toml:"session_duration"`
	RegistrationEnabled bool           `toml:"registration_enabled"`
	InviteOnly          bool           `toml:"invite_only"`
	RequireEmail        bool           `toml:"require_email"`
	WebAuthn            WebAuthnConfig `toml:"webauthn"`
}

// WebAuthnConfig defines WebAuthn/FIDO2 relying party settings.
type WebAuthnConfig struct {
	RPDisplayName string   `toml:"rp_display_name"`
	RPID          string   `toml:"rp_id"`
	RPOrigins     []string `toml:"rp_origins"`
}

// SessionDurationParsed returns the session duration as a time.Duration.
func (a AuthConfig) SessionDurationParsed() (time.Duration, error) {
	d, err := time.ParseDuration(a.SessionDuration)
	if err != nil {
		return 0, fmt.Errorf("parsing session_duration %q: %w", a.SessionDuration, err)
	}
	return d, nil
}

// MediaConfig defines file upload and processing settings.
type MediaConfig struct {
	MaxUploadSize       string `toml:"max_upload_size"`
	ImageThumbnailSizes []int  `toml:"image_thumbnail_sizes"`
	TranscodeVideo      bool   `toml:"transcode_video"`
	StripExif           bool   `toml:"strip_exif"`
}

// MaxUploadSizeBytes parses the MaxUploadSize string (e.g. "100MB") and returns bytes.
func (m MediaConfig) MaxUploadSizeBytes() (int64, error) {
	s := strings.TrimSpace(strings.ToUpper(m.MaxUploadSize))
	multiplier := int64(1)

	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}

	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing max_upload_size %q: %w", m.MaxUploadSize, err)
	}
	return n * multiplier, nil
}

// PushConfig defines WebPush notification settings.
type PushConfig struct {
	VAPIDPublicKey    string `toml:"vapid_public_key"`
	VAPIDPrivateKey   string `toml:"vapid_private_key"`
	VAPIDContactEmail string `toml:"vapid_contact_email"`
}

// HTTPConfig defines the REST API HTTP server settings.
type HTTPConfig struct {
	Listen      string   `toml:"listen"`
	CORSOrigins []string `toml:"cors_origins"`
}

// WebSocketConfig defines the WebSocket gateway settings.
type WebSocketConfig struct {
	Listen            string `toml:"listen"`
	HeartbeatInterval string `toml:"heartbeat_interval"`
	HeartbeatTimeout  string `toml:"heartbeat_timeout"`
}

// HeartbeatIntervalParsed returns the heartbeat interval as a time.Duration.
func (w WebSocketConfig) HeartbeatIntervalParsed() (time.Duration, error) {
	d, err := time.ParseDuration(w.HeartbeatInterval)
	if err != nil {
		return 0, fmt.Errorf("parsing heartbeat_interval %q: %w", w.HeartbeatInterval, err)
	}
	return d, nil
}

// HeartbeatTimeoutParsed returns the heartbeat timeout as a time.Duration.
func (w WebSocketConfig) HeartbeatTimeoutParsed() (time.Duration, error) {
	d, err := time.ParseDuration(w.HeartbeatTimeout)
	if err != nil {
		return 0, fmt.Errorf("parsing heartbeat_timeout %q: %w", w.HeartbeatTimeout, err)
	}
	return d, nil
}

// LoggingConfig defines structured logging settings.
type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// MetricsConfig defines Prometheus metrics endpoint settings.
type MetricsConfig struct {
	Enabled bool   `toml:"enabled"`
	Listen  string `toml:"listen"`
}

// defaults returns a Config with sane default values for all fields.
func defaults() Config {
	return Config{
		Instance: InstanceConfig{
			Domain:         "localhost",
			Name:           "AmityVox",
			FederationMode: "closed",
		},
		Database: DatabaseConfig{
			URL:            "postgres://amityvox:amityvox@localhost:5432/amityvox?sslmode=disable",
			MaxConnections: 25,
		},
		NATS: NATSConfig{
			URL: "nats://localhost:4222",
		},
		Cache: CacheConfig{
			URL: "redis://localhost:6379",
		},
		Storage: StorageConfig{
			Type:     "s3",
			Endpoint: "http://localhost:3900",
			Bucket:   "amityvox",
			Region:   "garage",
			UseSSL:   false,
		},
		LiveKit: LiveKitConfig{
			URL: "ws://localhost:7880",
		},
		Search: SearchConfig{
			Enabled: true,
			URL:     "http://localhost:7700",
		},
		Auth: AuthConfig{
			SessionDuration:     "720h",
			RegistrationEnabled: true,
			InviteOnly:          false,
			RequireEmail:        false,
		},
		Media: MediaConfig{
			MaxUploadSize:       "100MB",
			ImageThumbnailSizes: []int{128, 256, 512},
			TranscodeVideo:      true,
			StripExif:           true,
		},
		HTTP: HTTPConfig{
			Listen:      "0.0.0.0:8080",
			CORSOrigins: []string{"*"},
		},
		WebSocket: WebSocketConfig{
			Listen:            "0.0.0.0:8081",
			HeartbeatInterval: "30s",
			HeartbeatTimeout:  "90s",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Listen:  "0.0.0.0:9090",
		},
	}
}

// Load reads the configuration from the given TOML file path, applies defaults
// for missing values, and then applies environment variable overrides.
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file; use defaults + env overrides
			applyEnvOverrides(&cfg)
			deriveDefaults(&cfg)
			if err := validate(&cfg); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	applyEnvOverrides(&cfg)
	deriveDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyEnvOverrides overrides config fields with environment variables when set.
// Environment variables use the prefix AMITYVOX_ followed by the section and
// field name in uppercase with underscores (e.g. AMITYVOX_DATABASE_URL).
func applyEnvOverrides(cfg *Config) {
	// Instance
	if v := os.Getenv("AMITYVOX_INSTANCE_DOMAIN"); v != "" {
		cfg.Instance.Domain = v
	}
	if v := os.Getenv("AMITYVOX_INSTANCE_NAME"); v != "" {
		cfg.Instance.Name = v
	}
	if v := os.Getenv("AMITYVOX_INSTANCE_DESCRIPTION"); v != "" {
		cfg.Instance.Description = v
	}
	if v := os.Getenv("AMITYVOX_INSTANCE_FEDERATION_MODE"); v != "" {
		cfg.Instance.FederationMode = v
	}

	// Database
	if v := os.Getenv("AMITYVOX_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("AMITYVOX_DATABASE_MAX_CONNECTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.MaxConnections = n
		}
	}

	// NATS
	if v := os.Getenv("AMITYVOX_NATS_URL"); v != "" {
		cfg.NATS.URL = v
	}

	// Cache
	if v := os.Getenv("AMITYVOX_CACHE_URL"); v != "" {
		cfg.Cache.URL = v
	}

	// Storage
	if v := os.Getenv("AMITYVOX_STORAGE_TYPE"); v != "" {
		cfg.Storage.Type = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_ENDPOINT"); v != "" {
		cfg.Storage.Endpoint = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_BUCKET"); v != "" {
		cfg.Storage.Bucket = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_ACCESS_KEY"); v != "" {
		cfg.Storage.AccessKey = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_SECRET_KEY"); v != "" {
		cfg.Storage.SecretKey = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_REGION"); v != "" {
		cfg.Storage.Region = v
	}
	if v := os.Getenv("AMITYVOX_STORAGE_USE_SSL"); v != "" {
		cfg.Storage.UseSSL = v == "true" || v == "1"
	}

	// LiveKit
	if v := os.Getenv("AMITYVOX_LIVEKIT_URL"); v != "" {
		cfg.LiveKit.URL = v
	}
	if v := os.Getenv("AMITYVOX_LIVEKIT_API_KEY"); v != "" {
		cfg.LiveKit.APIKey = v
	}
	if v := os.Getenv("AMITYVOX_LIVEKIT_API_SECRET"); v != "" {
		cfg.LiveKit.APISecret = v
	}

	// Search
	if v := os.Getenv("AMITYVOX_SEARCH_ENABLED"); v != "" {
		cfg.Search.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AMITYVOX_SEARCH_URL"); v != "" {
		cfg.Search.URL = v
	}
	if v := os.Getenv("AMITYVOX_SEARCH_API_KEY"); v != "" {
		cfg.Search.APIKey = v
	}

	// Auth
	if v := os.Getenv("AMITYVOX_AUTH_SESSION_DURATION"); v != "" {
		cfg.Auth.SessionDuration = v
	}
	if v := os.Getenv("AMITYVOX_AUTH_REGISTRATION_ENABLED"); v != "" {
		cfg.Auth.RegistrationEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AMITYVOX_AUTH_INVITE_ONLY"); v != "" {
		cfg.Auth.InviteOnly = v == "true" || v == "1"
	}
	if v := os.Getenv("AMITYVOX_AUTH_REQUIRE_EMAIL"); v != "" {
		cfg.Auth.RequireEmail = v == "true" || v == "1"
	}

	// WebAuthn
	if v := os.Getenv("AMITYVOX_AUTH_WEBAUTHN_RP_DISPLAY_NAME"); v != "" {
		cfg.Auth.WebAuthn.RPDisplayName = v
	}
	if v := os.Getenv("AMITYVOX_AUTH_WEBAUTHN_RP_ID"); v != "" {
		cfg.Auth.WebAuthn.RPID = v
	}
	if v := os.Getenv("AMITYVOX_AUTH_WEBAUTHN_RP_ORIGINS"); v != "" {
		cfg.Auth.WebAuthn.RPOrigins = strings.Split(v, ",")
	}

	// Media
	if v := os.Getenv("AMITYVOX_MEDIA_MAX_UPLOAD_SIZE"); v != "" {
		cfg.Media.MaxUploadSize = v
	}
	if v := os.Getenv("AMITYVOX_MEDIA_TRANSCODE_VIDEO"); v != "" {
		cfg.Media.TranscodeVideo = v == "true" || v == "1"
	}
	if v := os.Getenv("AMITYVOX_MEDIA_STRIP_EXIF"); v != "" {
		cfg.Media.StripExif = v == "true" || v == "1"
	}

	// Push notifications
	if v := os.Getenv("AMITYVOX_PUSH_VAPID_PUBLIC_KEY"); v != "" {
		cfg.Push.VAPIDPublicKey = v
	}
	if v := os.Getenv("AMITYVOX_PUSH_VAPID_PRIVATE_KEY"); v != "" {
		cfg.Push.VAPIDPrivateKey = v
	}
	if v := os.Getenv("AMITYVOX_PUSH_VAPID_CONTACT_EMAIL"); v != "" {
		cfg.Push.VAPIDContactEmail = v
	}

	// HTTP
	if v := os.Getenv("AMITYVOX_HTTP_LISTEN"); v != "" {
		cfg.HTTP.Listen = v
	}

	// WebSocket
	if v := os.Getenv("AMITYVOX_WEBSOCKET_LISTEN"); v != "" {
		cfg.WebSocket.Listen = v
	}
	if v := os.Getenv("AMITYVOX_WEBSOCKET_HEARTBEAT_INTERVAL"); v != "" {
		cfg.WebSocket.HeartbeatInterval = v
	}
	if v := os.Getenv("AMITYVOX_WEBSOCKET_HEARTBEAT_TIMEOUT"); v != "" {
		cfg.WebSocket.HeartbeatTimeout = v
	}

	// Logging
	if v := os.Getenv("AMITYVOX_LOGGING_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("AMITYVOX_LOGGING_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}

	// Metrics
	if v := os.Getenv("AMITYVOX_METRICS_ENABLED"); v != "" {
		cfg.Metrics.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AMITYVOX_METRICS_LISTEN"); v != "" {
		cfg.Metrics.Listen = v
	}
}

// deriveDefaults fills in config values that can be inferred from other settings.
// Called after env overrides so that explicitly set values are not overwritten.
func deriveDefaults(cfg *Config) {
	if cfg.Auth.WebAuthn.RPID == "" || cfg.Auth.WebAuthn.RPID == "localhost" {
		if cfg.Instance.Domain != "" && cfg.Instance.Domain != "localhost" {
			cfg.Auth.WebAuthn.RPID = cfg.Instance.Domain
		}
	}
	if cfg.Auth.WebAuthn.RPDisplayName == "" {
		if cfg.Instance.Name != "" {
			cfg.Auth.WebAuthn.RPDisplayName = cfg.Instance.Name
		}
	}
	if len(cfg.Auth.WebAuthn.RPOrigins) == 0 ||
		(len(cfg.Auth.WebAuthn.RPOrigins) == 1 && cfg.Auth.WebAuthn.RPOrigins[0] == "http://localhost") {
		if cfg.Instance.Domain != "" && cfg.Instance.Domain != "localhost" {
			cfg.Auth.WebAuthn.RPOrigins = []string{"https://" + cfg.Instance.Domain}
		}
	}
}

// validate checks that required configuration fields are present and valid.
func validate(cfg *Config) error {
	if cfg.Instance.Domain == "" {
		return fmt.Errorf("config: instance.domain is required")
	}

	if cfg.Database.URL == "" {
		return fmt.Errorf("config: database.url is required")
	}

	if cfg.Database.MaxConnections < 1 {
		return fmt.Errorf("config: database.max_connections must be at least 1")
	}

	if cfg.NATS.URL == "" {
		return fmt.Errorf("config: nats.url is required")
	}

	if cfg.Cache.URL == "" {
		return fmt.Errorf("config: cache.url is required")
	}

	validFedModes := map[string]bool{"open": true, "allowlist": true, "closed": true}
	if !validFedModes[cfg.Instance.FederationMode] {
		return fmt.Errorf("config: instance.federation_mode must be one of: open, allowlist, closed (got %q)", cfg.Instance.FederationMode)
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[cfg.Logging.Level] {
		return fmt.Errorf("config: logging.level must be one of: debug, info, warn, error (got %q)", cfg.Logging.Level)
	}

	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[cfg.Logging.Format] {
		return fmt.Errorf("config: logging.format must be one of: json, text (got %q)", cfg.Logging.Format)
	}

	if _, err := cfg.Auth.SessionDurationParsed(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if _, err := cfg.Media.MaxUploadSizeBytes(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if cfg.HTTP.Listen == "" {
		return fmt.Errorf("config: http.listen is required")
	}

	return nil
}
