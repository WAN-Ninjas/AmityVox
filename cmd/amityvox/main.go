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

	"github.com/alexedwards/argon2id"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/api"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/automod"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/encryption"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/federation"
	"github.com/amityvox/amityvox/internal/gateway"
	"github.com/amityvox/amityvox/internal/media"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/notifications"
	"github.com/amityvox/amityvox/internal/presence"
	"github.com/amityvox/amityvox/internal/search"
	"github.com/amityvox/amityvox/internal/voice"
	"github.com/amityvox/amityvox/internal/workers"
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
	case "admin":
		if err := runAdmin(); err != nil {
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
	fmt.Println("  admin     Manage users and instance settings")
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

	// Bootstrap local instance record (generates/loads Ed25519 keypair for federation).
	instanceID, federationKey, err := ensureLocalInstance(ctx, db, cfg)
	if err != nil {
		return fmt.Errorf("bootstrapping local instance: %w", err)
	}
	logger.Info("local instance ready", slog.String("instance_id", instanceID),
		slog.Bool("federation_key_loaded", len(federationKey) > 0))

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

	// Create media/S3 storage service.
	var mediaSvc *media.Service
	if cfg.Storage.Endpoint != "" {
		maxBytes, _ := cfg.Media.MaxUploadSizeBytes()
		if maxBytes <= 0 {
			maxBytes = 100 * 1024 * 1024
		}
		logger.Info("media upload limit configured", slog.Int64("max_bytes", maxBytes), slog.String("max_upload_size", cfg.Media.MaxUploadSize))
		svc, err := media.New(media.Config{
			Endpoint:       cfg.Storage.Endpoint,
			Bucket:         cfg.Storage.Bucket,
			AccessKey:      cfg.Storage.AccessKey,
			SecretKey:      cfg.Storage.SecretKey,
			Region:         cfg.Storage.Region,
			UseSSL:         cfg.Storage.UseSSL,
			MaxUploadMB:    maxBytes / (1024 * 1024),
			ThumbnailSizes: cfg.Media.ImageThumbnailSizes,
			StripExif:      cfg.Media.StripExif,
			Pool:           db.Pool,
			Logger:         logger,
		})
		if err != nil {
			logger.Warn("media service unavailable, file uploads disabled", slog.String("error", err.Error()))
		} else {
			if err := svc.EnsureBucket(ctx); err != nil {
				logger.Warn("could not ensure S3 bucket", slog.String("error", err.Error()))
			}
			mediaSvc = svc
			logger.Info("media service ready", slog.String("endpoint", cfg.Storage.Endpoint))
		}
	}

	// Create Meilisearch search service (optional).
	var searchSvc *search.Service
	if cfg.Search.Enabled && cfg.Search.URL != "" {
		svc, err := search.New(search.Config{
			URL:    cfg.Search.URL,
			APIKey: cfg.Search.APIKey,
			Pool:   db.Pool,
			Logger: logger,
		})
		if err != nil {
			logger.Warn("search service unavailable", slog.String("error", err.Error()))
		} else {
			if err := svc.EnsureIndexes(ctx); err != nil {
				logger.Warn("could not ensure search indexes", slog.String("error", err.Error()))
			}
			searchSvc = svc
			logger.Info("search service ready", slog.String("url", cfg.Search.URL))
		}
	}

	// Create federation service.
	fedSvc := federation.New(federation.Config{
		Pool:           db.Pool,
		InstanceID:     instanceID,
		Domain:         cfg.Instance.Domain,
		PrivateKey:     federationKey,
		EnforceIPCheck: cfg.Federation.EnforceIPCheck,
		Logger:         logger,
	})

	// Refresh federation peer public keys on startup to handle key rotations.
	fedSvc.RefreshPeerKeys(ctx)

	// Create AutoMod service.
	automodSvc := automod.NewService(automod.Config{
		Pool:   db.Pool,
		Bus:    bus,
		Logger: logger,
	})

	// Create notification service (always — handles preferences; push is optional).
	notifSvc := notifications.NewService(notifications.Config{
		Pool:              db.Pool,
		Logger:            logger,
		VAPIDPublicKey:    cfg.Push.VAPIDPublicKey,
		VAPIDPrivateKey:   cfg.Push.VAPIDPrivateKey,
		VAPIDContactEmail: cfg.Push.VAPIDContactEmail,
		Bus:               bus,
	})
	if cfg.Push.VAPIDPublicKey != "" && cfg.Push.VAPIDPrivateKey != "" {
		logger.Info("push notifications enabled")
	}

	// Start background workers.
	workerMgr := workers.New(workers.Config{
		Pool:          db.Pool,
		Bus:           bus,
		Search:        searchSvc,
		Media:         mediaSvc,
		AutoMod:       automodSvc,
		Notifications: notifSvc,
		Logger:        logger,
	})
	workerMgr.Start(ctx)

	// Create voice service (optional — only when LiveKit is configured).
	var voiceSvc *voice.Service
	if cfg.LiveKit.URL != "" && cfg.LiveKit.APIKey != "" && cfg.LiveKit.APISecret != "" {
		svc, err := voice.New(voice.Config{
			URL:       cfg.LiveKit.URL,
			APIKey:    cfg.LiveKit.APIKey,
			APISecret: cfg.LiveKit.APISecret,
			Pool:      db.Pool,
			Logger:    logger,
		})
		if err != nil {
			logger.Warn("voice service unavailable", slog.String("error", err.Error()))
		} else {
			voiceSvc = svc
			logger.Info("voice service ready", slog.String("url", cfg.LiveKit.URL))
		}
	}

	// Create MLS encryption delivery service.
	encryptionSvc := encryption.NewService(encryption.Config{
		Pool:   db.Pool,
		Logger: logger,
	})

	// Create and start HTTP API server.
	srv := api.NewServer(db, cfg, authSvc, bus, cache, mediaSvc, searchSvc, voiceSvc, instanceID, logger)
	srv.Encryption = encryptionSvc
	srv.AutoMod = automodSvc
	srv.Notifications = notifSvc
	srv.FedSvc = fedSvc
	srv.Version = version

	// Register API routes after all optional services are set.
	srv.RegisterRoutes()

	// Public federation discovery and handshake (rate limited — no signature verification).
	fedRL := srv.RateLimitGlobal()
	srv.Router.With(fedRL).Get("/.well-known/amityvox", fedSvc.HandleDiscovery)
	srv.Router.With(fedRL).Post("/federation/v1/handshake", fedSvc.HandleHandshake)

	// Create and start federation sync service (message routing between instances).
	syncSvc := federation.NewSyncService(fedSvc, bus, logger)

	// Wire voice service into federation sync for federated voice token generation.
	if voiceSvc != nil {
		syncSvc.SetVoiceService(voiceSvc, srv.Config.LiveKit.PublicURL)
	}

	// Signed federation endpoints — no rate limit. These verify Ed25519 signatures
	// from authenticated peers, so IP-based rate limiting is unnecessary and causes
	// 429 errors that break real-time event delivery between instances.
	srv.Router.Post("/federation/v1/inbox", syncSvc.HandleInbox)
	srv.Router.Get("/federation/v1/users/lookup", fedSvc.HandleUserLookup)

	// Wire federation DM notifier into the users handler.
	if cfg.Instance.FederationMode != "closed" && srv.UserHandler != nil {
		srv.UserHandler.NotifyFederatedDM = syncSvc.NotifyFederatedDM
	}

	// Federation DM endpoints (signed, no rate limit).
	srv.Router.Post("/federation/v1/dm/create", syncSvc.HandleFederatedDMCreate)
	srv.Router.Post("/federation/v1/dm/message", syncSvc.HandleFederatedDMMessage)
	srv.Router.Post("/federation/v1/dm/recipient-add", syncSvc.HandleFederatedDMRecipientAdd)
	srv.Router.Post("/federation/v1/dm/recipient-remove", syncSvc.HandleFederatedDMRecipientRemove)

	// Federation guild endpoints (signed, no rate limit).
	srv.Router.Get("/federation/v1/guilds/{guildID}/preview", syncSvc.HandleFederatedGuildPreview)
	srv.Router.Post("/federation/v1/guilds/{guildID}/join", syncSvc.HandleFederatedGuildJoin)
	srv.Router.Post("/federation/v1/guilds/{guildID}/leave", syncSvc.HandleFederatedGuildLeave)
	srv.Router.Post("/federation/v1/guilds/invite-accept", syncSvc.HandleFederatedGuildInviteAccept)
	srv.Router.Post("/federation/v1/guilds/{guildID}/channels/{channelID}/messages", syncSvc.HandleFederatedGuildMessages)
	srv.Router.Post("/federation/v1/guilds/{guildID}/channels/{channelID}/messages/create", syncSvc.HandleFederatedGuildPostMessage)
	srv.Router.Post("/federation/v1/guilds/{guildID}/members", syncSvc.HandleFederatedGuildMembers)
	srv.Router.Post("/federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions", syncSvc.HandleFederatedGuildReactionAdd)
	srv.Router.Post("/federation/v1/guilds/{guildID}/channels/{channelID}/messages/{messageID}/reactions/remove", syncSvc.HandleFederatedGuildReactionRemove)
	srv.Router.Post("/federation/v1/guilds/{guildID}/channels/{channelID}/typing", syncSvc.HandleFederatedGuildTyping)

	// Federation guild proxy endpoints (authenticated, for local users accessing remote guilds).
	srv.Router.Route("/api/v1/federation/guilds", func(r chi.Router) {
		r.Use(auth.RequireAuth(authSvc))
		r.Use(srv.RateLimitGlobal())
		r.Post("/join", syncSvc.HandleProxyJoinFederatedGuild)
		r.Post("/{guildID}/leave", syncSvc.HandleProxyLeaveFederatedGuild)
		r.Get("/{guildID}/channels/{channelID}/messages", syncSvc.HandleProxyGetFederatedGuildMessages)
		r.Post("/{guildID}/channels/{channelID}/messages", syncSvc.HandleProxyPostFederatedGuildMessage)
		r.Get("/{guildID}/members", syncSvc.HandleProxyGetFederatedGuildMembers)
		r.Put("/{guildID}/channels/{channelID}/messages/{messageID}/reactions/{emoji}", syncSvc.HandleProxyAddFederatedReaction)
		r.Delete("/{guildID}/channels/{channelID}/messages/{messageID}/reactions/{emoji}", syncSvc.HandleProxyRemoveFederatedReaction)
		r.Post("/{guildID}/channels/{channelID}/typing", syncSvc.HandleProxyFederatedTyping)
	})

	// Federation user proxy endpoints (authenticated, for local users managing remote user stubs).
	srv.Router.Route("/api/v1/federation/users", func(r chi.Router) {
		r.Use(auth.RequireAuth(authSvc))
		r.Use(srv.RateLimitGlobal())
		r.Post("/ensure", syncSvc.HandleProxyEnsureFederatedUser)
	})

	// Federation peers and discovery proxy endpoints (authenticated, for local users).
	srv.Router.Route("/api/v1/federation/peers", func(r chi.Router) {
		r.Use(auth.RequireAuth(authSvc))
		r.Use(srv.RateLimitGlobal())
		r.Get("/public", syncSvc.HandleGetPublicFederationPeers)
		r.Get("/{peerID}/guilds", syncSvc.HandleProxyDiscoverRemoteGuilds)
	})

	// Federation guild discovery (signed, no rate limit).
	srv.Router.Post("/federation/v1/guilds/discover", syncSvc.HandleFederatedGuildDiscover)

	// Federation voice endpoint (signed, no rate limit).
	srv.Router.Post("/federation/v1/voice/token", syncSvc.HandleFederatedVoiceToken)

	// Federation voice proxy endpoints (authenticated, for local users joining remote voice channels).
	srv.Router.Route("/api/v1/federation/voice", func(r chi.Router) {
		r.Use(auth.RequireAuth(authSvc))
		r.Use(srv.RateLimitGlobal())
		r.Post("/join", syncSvc.HandleProxyFederatedVoiceJoin)
		r.Post("/guild-join", syncSvc.HandleProxyFederatedVoiceJoinByGuild)
	})

	if cfg.Instance.FederationMode != "closed" {
		syncSvc.StartRouter(ctx)
		logger.Info("federation sync router started", slog.String("mode", cfg.Instance.FederationMode))
	}

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
		Pool:              db.Pool,
		Voice:             voiceSvc,
		HeartbeatInterval: heartbeatInterval,
		HeartbeatTimeout:  heartbeatTimeout,
		ListenAddr:        cfg.WebSocket.Listen,
		BuildVersion:      version + "-" + commit + "-" + buildDate,
		Logger:            logger,
		CORSOrigins:       cfg.HTTP.CORSOrigins,
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

	// Stop background workers.
	workerMgr.Stop()

	logger.Info("AmityVox stopped")
	return nil
}

// ensureLocalInstance checks if the local instance record exists in the database
// (matched by domain). If not, it creates one with a generated Ed25519 key pair
// for federation signing. Returns the instance ID and the Ed25519 private key.
// If the instance exists but has no private key (legacy bootstrap), a new keypair
// is generated and both keys are updated.
func ensureLocalInstance(ctx context.Context, db *database.DB, cfg *config.Config) (string, ed25519.PrivateKey, error) {
	var id string
	var privKeyPEM *string

	// Try loading with private_key_pem (requires migration 061).
	// Fall back to id-only query if the column doesn't exist yet.
	err := db.Pool.QueryRow(ctx,
		`SELECT id, private_key_pem FROM instances WHERE domain = $1`,
		cfg.Instance.Domain,
	).Scan(&id, &privKeyPEM)
	if err != nil && strings.Contains(err.Error(), "private_key_pem") {
		// Column doesn't exist — migration 061 not yet applied. Fall back.
		err = db.Pool.QueryRow(ctx,
			`SELECT id FROM instances WHERE domain = $1`,
			cfg.Instance.Domain,
		).Scan(&id)
	}

	if err == nil && privKeyPEM != nil && *privKeyPEM != "" {
		// Instance exists and has a stored private key — parse and return it.
		block, _ := pem.Decode([]byte(*privKeyPEM))
		if block == nil {
			return "", nil, fmt.Errorf("invalid private key PEM for instance %s", id)
		}
		privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", nil, fmt.Errorf("parsing stored private key: %w", err)
		}
		edKey, ok := privKey.(ed25519.PrivateKey)
		if !ok {
			return "", nil, fmt.Errorf("stored private key is not Ed25519")
		}
		return id, edKey, nil
	}

	// Either instance doesn't exist or it exists without a private key.
	// Generate a new Ed25519 keypair.
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", nil, fmt.Errorf("generating Ed25519 key pair: %w", err)
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling public key: %w", err)
	}
	pubKeyPEMStr := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}))

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling private key: %w", err)
	}
	privKeyPEMStr := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}))

	if id != "" {
		// Instance exists but had no private key (legacy bootstrap) — update both keys.
		// Try with private_key_pem first; fall back if column doesn't exist.
		_, err = db.Pool.Exec(ctx,
			`UPDATE instances SET public_key = $1, private_key_pem = $2 WHERE id = $3`,
			pubKeyPEMStr, privKeyPEMStr, id,
		)
		if err != nil && strings.Contains(err.Error(), "private_key_pem") {
			_, err = db.Pool.Exec(ctx,
				`UPDATE instances SET public_key = $1 WHERE id = $2`,
				pubKeyPEMStr, id,
			)
		}
		if err != nil {
			return "", nil, fmt.Errorf("updating instance keypair: %w", err)
		}
		return id, privKey, nil
	}

	// Instance doesn't exist — create it.
	id = models.NewULID().String()
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO instances (id, domain, public_key, private_key_pem, name, description, software_version, federation_mode, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 ON CONFLICT (domain) DO NOTHING`,
		id, cfg.Instance.Domain, pubKeyPEMStr, privKeyPEMStr, cfg.Instance.Name, cfg.Instance.Description, version, cfg.Instance.FederationMode,
	)
	if err != nil && strings.Contains(err.Error(), "private_key_pem") {
		_, err = db.Pool.Exec(ctx,
			`INSERT INTO instances (id, domain, public_key, name, description, software_version, federation_mode, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			 ON CONFLICT (domain) DO NOTHING`,
			id, cfg.Instance.Domain, pubKeyPEMStr, cfg.Instance.Name, cfg.Instance.Description, version, cfg.Instance.FederationMode,
		)
	}
	if err != nil {
		return "", nil, fmt.Errorf("creating local instance record: %w", err)
	}

	// Re-read in case of race (ON CONFLICT DO NOTHING).
	err = db.Pool.QueryRow(ctx,
		`SELECT id FROM instances WHERE domain = $1`,
		cfg.Instance.Domain,
	).Scan(&id)
	if err != nil {
		return "", nil, fmt.Errorf("reading instance ID after insert: %w", err)
	}

	return id, privKey, nil
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

// runAdmin handles admin subcommands for user and instance management.
func runAdmin() error {
	if len(os.Args) < 3 {
		fmt.Println("Usage: amityvox admin <action>")
		fmt.Println()
		fmt.Println("Actions:")
		fmt.Println("  create-user  Create a new user account")
		fmt.Println("  suspend      Suspend a user account")
		fmt.Println("  unsuspend    Unsuspend a user account")
		fmt.Println("  set-admin    Grant admin flag to a user")
		fmt.Println("  unset-admin  Remove admin flag from a user")
		fmt.Println("  list-users   List all user accounts")
		return nil
	}

	logger := setupLogger("info", "text")

	cfgPath := configPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()
	db, err := database.New(ctx, cfg.Database.URL, cfg.Database.MaxConnections, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	switch os.Args[2] {
	case "create-user":
		if len(os.Args) < 5 {
			return fmt.Errorf("usage: amityvox admin create-user <username> <password>")
		}
		username, password := os.Args[3], os.Args[4]

		// Get local instance ID.
		var instanceID string
		if err := db.Pool.QueryRow(ctx, `SELECT id FROM instances WHERE domain = $1`, cfg.Instance.Domain).Scan(&instanceID); err != nil {
			return fmt.Errorf("instance not found — run 'amityvox serve' first to bootstrap")
		}

		// Hash password.
		hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
		if err != nil {
			return fmt.Errorf("hashing password: %w", err)
		}

		userID := models.NewULID().String()
		_, err = db.Pool.Exec(ctx,
			`INSERT INTO users (id, instance_id, username, password_hash, created_at) VALUES ($1, $2, $3, $4, now())`,
			userID, instanceID, username, hash)
		if err != nil {
			return fmt.Errorf("creating user: %w", err)
		}
		fmt.Printf("Created user %s (ID: %s)\n", username, userID)

	case "suspend":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: amityvox admin suspend <username>")
		}
		tag, err := db.Pool.Exec(ctx,
			`UPDATE users SET flags = flags | $1 WHERE username = $2`,
			models.UserFlagSuspended, os.Args[3])
		if err != nil {
			return fmt.Errorf("suspending user: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("user %q not found", os.Args[3])
		}
		fmt.Printf("Suspended user %s\n", os.Args[3])

	case "unsuspend":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: amityvox admin unsuspend <username>")
		}
		tag, err := db.Pool.Exec(ctx,
			`UPDATE users SET flags = flags & ~$1 WHERE username = $2`,
			models.UserFlagSuspended, os.Args[3])
		if err != nil {
			return fmt.Errorf("unsuspending user: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("user %q not found", os.Args[3])
		}
		fmt.Printf("Unsuspended user %s\n", os.Args[3])

	case "set-admin":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: amityvox admin set-admin <username>")
		}
		tag, err := db.Pool.Exec(ctx,
			`UPDATE users SET flags = flags | $1 WHERE username = $2`,
			models.UserFlagAdmin, os.Args[3])
		if err != nil {
			return fmt.Errorf("setting admin: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("user %q not found", os.Args[3])
		}
		fmt.Printf("Granted admin to %s\n", os.Args[3])

	case "unset-admin":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: amityvox admin unset-admin <username>")
		}
		tag, err := db.Pool.Exec(ctx,
			`UPDATE users SET flags = flags & ~$1 WHERE username = $2`,
			models.UserFlagAdmin, os.Args[3])
		if err != nil {
			return fmt.Errorf("unsetting admin: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("user %q not found", os.Args[3])
		}
		fmt.Printf("Removed admin from %s\n", os.Args[3])

	case "list-users":
		rows, err := db.Pool.Query(ctx,
			`SELECT id, username, display_name, email, flags, created_at FROM users ORDER BY created_at`)
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}
		defer rows.Close()

		fmt.Printf("%-28s %-20s %-20s %-30s %6s %s\n", "ID", "Username", "DisplayName", "Email", "Flags", "Created")
		fmt.Println(strings.Repeat("-", 130))
		for rows.Next() {
			var id, username string
			var displayName, email *string
			var flags int
			var createdAt time.Time
			if err := rows.Scan(&id, &username, &displayName, &email, &flags, &createdAt); err != nil {
				return fmt.Errorf("scanning user: %w", err)
			}
			dn := ""
			if displayName != nil {
				dn = *displayName
			}
			em := ""
			if email != nil {
				em = *email
			}
			fmt.Printf("%-28s %-20s %-20s %-30s %6d %s\n", id, username, dn, em, flags, createdAt.Format(time.RFC3339))
		}

	default:
		return fmt.Errorf("unknown admin action: %s", os.Args[2])
	}

	return nil
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
