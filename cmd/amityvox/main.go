// Package main is the CLI entrypoint for AmityVox. It provides subcommands for
// running the server (serve), managing database migrations (migrate), and
// printing version information (version). The serve command loads configuration,
// connects to PostgreSQL, NATS, and DragonflyDB, runs pending migrations, starts
// the HTTP API server and WebSocket gateway, and handles graceful shutdown on
// SIGINT/SIGTERM.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/amityvox/amityvox/internal/api"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/gateway"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/presence"
)

// Build-time variables set via ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "migrate":
		if err := runMigrate(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		runVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints the CLI usage information.
func printUsage() {
	fmt.Println("AmityVox — Federated Communication Platform")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  amityvox <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  serve     Start the AmityVox server")
	fmt.Println("  migrate   Run database migrations")
	fmt.Println("  version   Print version information")
	fmt.Println("  help      Show this help message")
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Println("  Config file:  amityvox.toml (or set AMITYVOX_CONFIG_PATH)")
	fmt.Println("  Env prefix:   AMITYVOX_ (e.g. AMITYVOX_DATABASE_URL)")
}

// runServe starts the full AmityVox server: loads config, connects to all
// services (PostgreSQL, NATS, DragonflyDB), runs migrations, bootstraps the
// local instance, creates the auth service, starts the HTTP API server and
// WebSocket gateway, and handles graceful shutdown on SIGINT/SIGTERM.
func runServe() error {
	logger := setupLogger("info", "json")

	logger.Info("starting AmityVox",
		slog.String("version", version),
		slog.String("commit", commit),
	)

	// Load configuration.
	cfgPath := configPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Reconfigure logger with loaded settings.
	logger = setupLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info("configuration loaded", slog.String("path", cfgPath))

	ctx := context.Background()

	// Connect to database.
	db, err := database.New(ctx, cfg.Database.URL, cfg.Database.MaxConnections, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	// Run migrations.
	if err := database.MigrateUp(cfg.Database.URL, logger); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Bootstrap local instance record.
	instanceID, err := ensureLocalInstance(ctx, db, cfg)
	if err != nil {
		return fmt.Errorf("bootstrapping local instance: %w", err)
	}
	logger.Info("local instance ready", slog.String("instance_id", instanceID))

	// Connect to NATS event bus.
	bus, err := events.New(cfg.NATS.URL, logger)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer bus.Close()

	// Ensure JetStream streams exist.
	if err := bus.EnsureStreams(); err != nil {
		return fmt.Errorf("ensuring NATS streams: %w", err)
	}

	// Connect to DragonflyDB/Redis cache.
	cache, err := presence.New(cfg.Cache.URL, logger)
	if err != nil {
		return fmt.Errorf("connecting to cache: %w", err)
	}
	defer cache.Close()

	// Parse auth settings.
	sessionDuration, err := cfg.Auth.SessionDurationParsed()
	if err != nil {
		return fmt.Errorf("parsing session duration: %w", err)
	}

	// Create auth service.
	authSvc := auth.NewService(auth.Config{
		Pool:            db.Pool,
		Cache:           cache,
		InstanceID:      instanceID,
		SessionDuration: sessionDuration,
		RegEnabled:      cfg.Auth.RegistrationEnabled,
		InviteOnly:      cfg.Auth.InviteOnly,
		RequireEmail:    cfg.Auth.RequireEmail,
		Logger:          logger,
	})

	// Create and start HTTP API server.
	srv := api.NewServer(db, cfg, authSvc, bus, cache, instanceID, logger)

	// Parse WebSocket settings.
	heartbeatInterval, err := cfg.WebSocket.HeartbeatIntervalParsed()
	if err != nil {
		return fmt.Errorf("parsing heartbeat interval: %w", err)
	}
	heartbeatTimeout, err := cfg.WebSocket.HeartbeatTimeoutParsed()
	if err != nil {
		return fmt.Errorf("parsing heartbeat timeout: %w", err)
	}

	// Create WebSocket gateway server.
	gw := gateway.NewServer(gateway.ServerConfig{
		AuthService:       authSvc,
		EventBus:          bus,
		Cache:             cache,
		HeartbeatInterval: heartbeatInterval,
		HeartbeatTimeout:  heartbeatTimeout,
		ListenAddr:        cfg.WebSocket.Listen,
		Logger:            logger,
	})

	// Graceful shutdown handler.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 2)

	// Start HTTP API server.
	go func() {
		if err := srv.Start(); err != nil {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Start WebSocket gateway.
	go func() {
		if err := gw.Start(); err != nil {
			errCh <- fmt.Errorf("WebSocket gateway: %w", err)
		}
	}()

	// Wait for shutdown signal or server error.
	select {
	case err := <-errCh:
		return err
	case sig := <-shutdownCh:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	// Graceful shutdown with timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Shut down gateway first (sends RECONNECT to clients).
	if err := gw.Shutdown(shutdownCtx); err != nil {
		logger.Error("gateway shutdown error", slog.String("error", err.Error()))
	}

	// Then shut down HTTP server.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("AmityVox stopped")
	return nil
}

// ensureLocalInstance checks if the local instance record exists in the database
// (matched by domain). If not, it creates one with a generated Ed25519 key pair
// for federation signing. Returns the instance ID.
func ensureLocalInstance(ctx context.Context, db *database.DB, cfg *config.Config) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`,
		cfg.Instance.Domain,
	).Scan(&id)

	if err == nil {
		return id, nil
	}

	// Instance doesn't exist yet — generate an Ed25519 key pair and create it.
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", fmt.Errorf("generating Ed25519 key pair: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("marshaling public key: %w", err)
	}

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	id = models.NewULID().String()
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO instances (id, domain, public_key, name, description, software_version, federation_mode, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT (domain) DO NOTHING`,
		id, cfg.Instance.Domain, string(pubKeyPEM), cfg.Instance.Name, cfg.Instance.Description, version, cfg.Instance.FederationMode,
	)
	if err != nil {
		return "", fmt.Errorf("creating local instance record: %w", err)
	}

	// Re-read in case of race (ON CONFLICT DO NOTHING).
	err = db.Pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`,
		cfg.Instance.Domain,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("reading instance ID after insert: %w", err)
	}

	return id, nil
}

// runMigrate handles the migrate subcommand with up/down/status operations.
func runMigrate() error {
	logger := setupLogger("info", "text")

	cfgPath := configPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Parse migrate subcommand.
	action := "up"
	if len(os.Args) >= 3 {
		action = os.Args[2]
	}

	switch action {
	case "up":
		return database.MigrateUp(cfg.Database.URL, logger)
	case "down":
		return database.MigrateDown(cfg.Database.URL, logger)
	case "status":
		v, dirty, err := database.MigrateStatus(cfg.Database.URL)
		if err != nil {
			return err
		}
		fmt.Printf("Migration version: %d\n", v)
		fmt.Printf("Dirty: %v\n", dirty)
		return nil
	default:
		return fmt.Errorf("unknown migrate action: %s (use: up, down, status)", action)
	}
}

// runVersion prints version information and exits.
func runVersion() {
	fmt.Printf("AmityVox %s\n", version)
	fmt.Printf("  commit:     %s\n", commit)
	fmt.Printf("  built:      %s\n", buildDate)
}

// configPath returns the config file path from AMITYVOX_CONFIG_PATH env var
// or the default "amityvox.toml".
func configPath() string {
	if p := os.Getenv("AMITYVOX_CONFIG_PATH"); p != "" {
		return p
	}
	return "amityvox.toml"
}

// setupLogger creates a slog.Logger with the given level and format.
func setupLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
