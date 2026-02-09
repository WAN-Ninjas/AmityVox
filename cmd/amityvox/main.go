// Package main is the CLI entrypoint for AmityVox. It provides subcommands for
// running the server (serve), managing database migrations (migrate), and
// printing version information (version). The serve command loads configuration,
// connects to PostgreSQL, runs pending migrations, starts the HTTP API server,
// and handles graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/amityvox/amityvox/internal/api"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
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

// runServe starts the full AmityVox server: loads config, connects to the
// database, runs migrations, and starts the HTTP API server with graceful
// shutdown on SIGINT/SIGTERM.
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

	// Connect to database.
	ctx := context.Background()
	db, err := database.New(ctx, cfg.Database.URL, cfg.Database.MaxConnections, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	// Run migrations.
	if err := database.MigrateUp(cfg.Database.URL, logger); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Create and start HTTP server.
	srv := api.NewServer(db, cfg, logger)

	// Graceful shutdown handler.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case sig := <-shutdownCh:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	// Graceful shutdown with timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("AmityVox stopped")
	return nil
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
