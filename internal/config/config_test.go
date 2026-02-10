package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := defaults()

	if cfg.Instance.Domain != "localhost" {
		t.Errorf("default domain = %q, want %q", cfg.Instance.Domain, "localhost")
	}
	if cfg.Instance.FederationMode != "closed" {
		t.Errorf("default federation_mode = %q, want %q", cfg.Instance.FederationMode, "closed")
	}
	if cfg.Database.MaxConnections != 25 {
		t.Errorf("default max_connections = %d, want 25", cfg.Database.MaxConnections)
	}
	if cfg.HTTP.Listen != "0.0.0.0:8080" {
		t.Errorf("default http.listen = %q, want %q", cfg.HTTP.Listen, "0.0.0.0:8080")
	}
	if !cfg.Auth.RegistrationEnabled {
		t.Error("default auth.registration_enabled should be true")
	}
	if !cfg.Search.Enabled {
		t.Error("default search.enabled should be true")
	}
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load("/nonexistent/amityvox.toml")
	if err != nil {
		t.Fatalf("Load non-existent file should use defaults, got error: %v", err)
	}
	if cfg.Instance.Domain != "localhost" {
		t.Errorf("domain = %q, want %q", cfg.Instance.Domain, "localhost")
	}
}

func TestLoad_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amityvox.toml")
	content := `
[instance]
domain = "test.example.com"
name = "Test Instance"
federation_mode = "open"

[database]
url = "postgres://test:test@localhost/test"
max_connections = 10

[http]
listen = "127.0.0.1:9090"
cors_origins = ["https://test.example.com"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Instance.Domain != "test.example.com" {
		t.Errorf("domain = %q, want %q", cfg.Instance.Domain, "test.example.com")
	}
	if cfg.Instance.FederationMode != "open" {
		t.Errorf("federation_mode = %q, want %q", cfg.Instance.FederationMode, "open")
	}
	if cfg.Database.MaxConnections != 10 {
		t.Errorf("max_connections = %d, want 10", cfg.Database.MaxConnections)
	}
	// Values not in TOML should retain defaults.
	if cfg.NATS.URL != "nats://localhost:4222" {
		t.Errorf("nats.url = %q, want default", cfg.NATS.URL)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amityvox.toml")
	if err := os.WriteFile(path, []byte("not valid toml [[["), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should fail on invalid TOML")
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			"invalid federation mode",
			`[instance]
domain = "test.com"
federation_mode = "invalid"`,
		},
		{
			"invalid log level",
			`[logging]
level = "trace"`,
		},
		{
			"invalid log format",
			`[logging]
format = "xml"`,
		},
		{
			"empty database URL",
			`[database]
url = ""`,
		},
		{
			"zero max connections",
			`[database]
max_connections = 0`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "amityvox.toml")
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestEnvOverrides(t *testing.T) {
	// Set env vars before loading.
	t.Setenv("AMITYVOX_INSTANCE_DOMAIN", "env.example.com")
	t.Setenv("AMITYVOX_DATABASE_MAX_CONNECTIONS", "50")
	t.Setenv("AMITYVOX_AUTH_REGISTRATION_ENABLED", "false")
	t.Setenv("AMITYVOX_SEARCH_ENABLED", "false")

	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Instance.Domain != "env.example.com" {
		t.Errorf("domain = %q, want %q", cfg.Instance.Domain, "env.example.com")
	}
	if cfg.Database.MaxConnections != 50 {
		t.Errorf("max_connections = %d, want 50", cfg.Database.MaxConnections)
	}
	if cfg.Auth.RegistrationEnabled {
		t.Error("registration should be disabled via env")
	}
	if cfg.Search.Enabled {
		t.Error("search should be disabled via env")
	}
}

func TestSessionDurationParsed(t *testing.T) {
	cfg := AuthConfig{SessionDuration: "720h"}
	d, err := cfg.SessionDurationParsed()
	if err != nil {
		t.Fatalf("SessionDurationParsed error: %v", err)
	}
	if d.Hours() != 720 {
		t.Errorf("duration = %v, want 720h", d)
	}
}

func TestSessionDurationParsed_Invalid(t *testing.T) {
	cfg := AuthConfig{SessionDuration: "not-a-duration"}
	_, err := cfg.SessionDurationParsed()
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestMaxUploadSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"100MB", 100 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"512KB", 512 * 1024},
		{"1024B", 1024},
		{"50mb", 50 * 1024 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			cfg := MediaConfig{MaxUploadSize: tc.input}
			got, err := cfg.MaxUploadSizeBytes()
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxUploadSizeBytes_Invalid(t *testing.T) {
	cfg := MediaConfig{MaxUploadSize: "abc"}
	_, err := cfg.MaxUploadSizeBytes()
	if err == nil {
		t.Fatal("expected error for invalid size")
	}
}
