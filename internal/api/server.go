// Package api implements the AmityVox REST API server using the chi router.
// It registers all route groups under /api/v1/, provides middleware for logging,
// recovery, CORS, and request IDs, and exposes JSON response helpers for
// consistent API envelope formatting.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/activities"
	"github.com/amityvox/amityvox/internal/api/admin"
	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/api/bookmarks"
	"github.com/amityvox/amityvox/internal/api/bots"
	"github.com/amityvox/amityvox/internal/api/channels"
	"github.com/amityvox/amityvox/internal/api/experimental"
	"github.com/amityvox/amityvox/internal/api/guildevents"
	"github.com/amityvox/amityvox/internal/api/guilds"
	"github.com/amityvox/amityvox/internal/api/integrations"
	"github.com/amityvox/amityvox/internal/api/invites"
	"github.com/amityvox/amityvox/internal/api/moderation"
	"github.com/amityvox/amityvox/internal/api/onboarding"
	"github.com/amityvox/amityvox/internal/api/polls"
	"github.com/amityvox/amityvox/internal/api/social"
	"github.com/amityvox/amityvox/internal/api/stickers"
	"github.com/amityvox/amityvox/internal/api/themes"
	"github.com/amityvox/amityvox/internal/api/users"
	"github.com/amityvox/amityvox/internal/api/webhooks"
	"github.com/amityvox/amityvox/internal/api/widgets"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/automod"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/encryption"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/features"
	"github.com/amityvox/amityvox/internal/federation"
	"github.com/amityvox/amityvox/internal/media"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/notifications"
	"github.com/amityvox/amityvox/internal/presence"
	"github.com/amityvox/amityvox/internal/search"
	"github.com/amityvox/amityvox/internal/voice"
)

// Server is the HTTP API server for AmityVox. It holds the chi router, database
// reference, services, configuration, and logger.
type Server struct {
	Router        *chi.Mux
	DB            *database.DB
	Config        *config.Config
	AuthService   *auth.Service
	EventBus      *events.Bus
	Cache         *presence.Cache
	Media         *media.Service
	Search        *search.Service
	Voice         *voice.Service
	Encryption    *encryption.Service
	AutoMod       *automod.Service
	Notifications *notifications.Service
	WebAuthn      *webauthn.WebAuthn
	InstanceID    string
	Version       string
	BuildVersion  string
	Logger        *slog.Logger
	FedSvc        *federation.Service     // exposed for admin federation handlers
	FedProxy      apiutil.FederationProxy // optional, set after sync service creation
	UserHandler   *users.Handler          // exposed for federation wiring
	server        *http.Server
}

// NewServer creates a new API server with all routes and middleware registered.
func NewServer(db *database.DB, cfg *config.Config, authSvc *auth.Service, bus *events.Bus, cache *presence.Cache, mediaSvc *media.Service, searchSvc *search.Service, voiceSvc *voice.Service, instanceID string, logger *slog.Logger) *Server {
	s := &Server{
		Router:      chi.NewRouter(),
		DB:          db,
		Config:      cfg,
		AuthService: authSvc,
		EventBus:    bus,
		Cache:       cache,
		Media:       mediaSvc,
		Search:      searchSvc,
		Voice:       voiceSvc,
		InstanceID:  instanceID,
		Logger:      logger,
	}

	// Initialize WebAuthn if configured.
	if cfg.Auth.WebAuthn.RPID != "" && len(cfg.Auth.WebAuthn.RPOrigins) > 0 {
		displayName := cfg.Auth.WebAuthn.RPDisplayName
		if displayName == "" {
			displayName = cfg.Instance.Name
		}
		wa, err := webauthn.New(&webauthn.Config{
			RPDisplayName: displayName,
			RPID:          cfg.Auth.WebAuthn.RPID,
			RPOrigins:     cfg.Auth.WebAuthn.RPOrigins,
		})
		if err != nil {
			logger.Warn("WebAuthn initialization failed", slog.String("error", err.Error()))
		} else {
			s.WebAuthn = wa
			logger.Info("WebAuthn enabled", slog.String("rp_id", cfg.Auth.WebAuthn.RPID))
		}
	}

	s.registerMiddleware()

	return s
}

// RegisterRoutes mounts all API route groups. Must be called after all optional
// services (Notifications, Encryption, AutoMod, etc.) are set on the Server.
func (s *Server) RegisterRoutes() {
	s.registerRoutes()
}

// RequireAdmin returns middleware that checks the user's flags for admin
// privilege. It must be mounted AFTER auth.RequireAuth so UserIDFromContext is
// populated. Returns 403 if the user is not an admin.
func RequireAdmin(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := auth.UserIDFromContext(r.Context())
			if userID == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}
			var flags int
			err := pool.QueryRow(r.Context(), `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
			if err != nil || flags&models.UserFlagAdmin == 0 {
				WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireInstanceFeature(featureKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.featureEnabled(w, r, featureKey, "") {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireGuildFeature(featureKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			guildID := chi.URLParam(r, "guildID")
			if !s.featureEnabled(w, r, featureKey, guildID) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireChannelFeature(featureKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			channelID := chi.URLParam(r, "channelID")
			if channelID == "" {
				if !s.featureEnabled(w, r, featureKey, "") {
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			guildID, ok := s.guildIDForChannel(w, r, channelID)
			if !ok {
				return
			}
			if !s.featureEnabled(w, r, featureKey, guildID) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireMessageFeature(featureKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			messageID := chi.URLParam(r, "messageID")
			if messageID == "" {
				if !s.featureEnabled(w, r, featureKey, "") {
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			var guildID *string
			err := s.DB.Pool.QueryRow(r.Context(), `
				SELECT c.guild_id
				FROM messages m
				JOIN channels c ON c.id = m.channel_id
				WHERE m.id = $1
			`, messageID).Scan(&guildID)
			if err != nil {
				if err == pgx.ErrNoRows {
					next.ServeHTTP(w, r)
					return
				}
				apiutil.InternalError(w, s.Logger, "Failed to load message feature scope", err)
				return
			}
			if !s.featureEnabled(w, r, featureKey, stringValue(guildID)) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) guildIDForChannel(w http.ResponseWriter, r *http.Request, channelID string) (string, bool) {
	var guildID *string
	err := s.DB.Pool.QueryRow(r.Context(), `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiutil.WriteError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
			return "", false
		}
		apiutil.InternalError(w, s.Logger, "Failed to load channel feature scope", err)
		return "", false
	}
	return stringValue(guildID), true
}

func (s *Server) featureEnabled(w http.ResponseWriter, r *http.Request, featureKey string, guildID string) bool {
	if !features.IsKnown(featureKey) {
		apiutil.WriteError(w, http.StatusInternalServerError, "unknown_feature", "Feature gate is not configured")
		return false
	}
	states, err := features.Resolve(r.Context(), s.DB.Pool, guildID)
	if err != nil {
		apiutil.InternalError(w, s.Logger, "Failed to load feature flags", err)
		return false
	}
	state := states[featureKey]
	if !state.Enabled {
		apiutil.WriteError(w, http.StatusForbidden, "feature_disabled", state.Name+" is disabled")
		return false
	}
	return true
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// registerMiddleware adds global middleware to the router.
func (s *Server) registerMiddleware() {
	s.Router.Use(middleware.RequestID)
	s.Router.Use(middleware.RealIP)
	s.Router.Use(slogMiddleware(s.Logger))
	s.Router.Use(middleware.Recoverer)
	s.Router.Use(corsMiddleware(s.Config.HTTP.CORSOrigins))
	s.Router.Use(middleware.Compress(5))
	s.Router.Use(middleware.Timeout(30 * time.Second))
	s.Router.Use(maxBodySize(1 << 20)) // 1MB default body limit
	// Rate limiting is applied per-route group in registerRoutes so that
	// authenticated routes run AFTER auth.RequireAuth, allowing the middleware
	// to key on userID (6000 req/min) instead of falling back to IP (1200 req/min).
}

// registerRoutes mounts all API route groups on the router.
func (s *Server) registerRoutes() {
	// Create domain handlers.
	userH := &users.Handler{
		Pool:           s.DB.Pool,
		EventBus:       s.EventBus,
		InstanceID:     s.InstanceID,
		InstanceDomain: s.Config.Instance.Domain,
		Logger:         s.Logger,
	}
	s.UserHandler = userH
	guildH := &guilds.Handler{
		Pool:       s.DB.Pool,
		EventBus:   s.EventBus,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
		FedProxy:   s.FedProxy,
	}
	channelH := &channels.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
		FedProxy: s.FedProxy,
	}
	inviteH := &invites.Handler{
		Pool:       s.DB.Pool,
		EventBus:   s.EventBus,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
		FedProxy:   s.FedProxy,
	}
	adminH := &admin.Handler{
		Pool:       s.DB.Pool,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
		Media:      s.Media,
		EventBus:   s.EventBus,
		Cache:      s.Cache,
		FedSvc:     s.FedSvc,
	}
	webhookH := &webhooks.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	pollH := &polls.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	bookmarkH := &bookmarks.Handler{
		Pool:   s.DB.Pool,
		Logger: s.Logger,
	}
	guildEventH := &guildevents.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	modH := &moderation.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	stickerH := &stickers.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	onboardH := &onboarding.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	botH := &bots.Handler{
		Pool:        s.DB.Pool,
		AuthService: s.AuthService,
		EventBus:    s.EventBus,
		Logger:      s.Logger,
	}
	themeH := &themes.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	channelEmojiH := &channels.EmojiHandler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	widgetH := &widgets.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	experimentalH := &experimental.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	activityH := &activities.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	socialH := &social.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	integrationH := &integrations.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}

	// Health check — outside versioned API prefix, no rate limit (used by Docker healthcheck).
	s.Router.Get("/health", s.handleHealthCheck)
	s.Router.Get("/health/deep", s.handleDeepHealthCheck)

	// Prometheus metrics endpoint.
	s.Router.With(s.RateLimitGlobal()).Get("/metrics", s.handleMetrics)

	// API v1 routes.
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Get("/client-config", s.handleClientConfig)

		// Auth routes.
		r.Route("/auth", func(r chi.Router) {
			// Public auth endpoints (login/register) — IP-based rate limiting.
			r.Group(func(r chi.Router) {
				r.Use(s.RateLimitGlobal())
				r.Get("/registration", s.handleGetPublicRegistrationConfig)
				r.Post("/register", s.handleRegister)
				r.Post("/login", s.handleLogin)
			})

			// Authenticated auth-management endpoints — user-based rate limiting.
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(s.AuthService))
				r.Use(s.RateLimitGlobal())
				r.Post("/logout", s.handleLogout)
				r.Post("/password", s.handleChangePassword)
				r.Post("/email", s.handleChangeEmail)
				r.Post("/totp/enable", s.handleTOTPEnable)
				r.Post("/totp/verify", s.handleTOTPVerify)
				r.Delete("/totp", s.handleTOTPDisable)
				r.Post("/backup-codes", s.handleGenerateBackupCodes)
				r.Post("/backup-codes/verify", s.handleConsumeBackupCode)
				r.Post("/webauthn/register/begin", s.handleWebAuthnRegisterBegin)
				r.Post("/webauthn/register/finish", s.handleWebAuthnRegisterFinish)
				r.Post("/webauthn/login/begin", s.handleWebAuthnLoginBegin)
				r.Post("/webauthn/login/finish", s.handleWebAuthnLoginFinish)
			})
		})

		// Authenticated routes — require Bearer token.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.AuthService))
			r.Use(s.RateLimitGlobal()) // Runs after auth: keys on userID (6000 req/min).

			// User routes.
			r.Route("/users", func(r chi.Router) {
				r.Get("/@me", userH.HandleGetSelf)
				r.Patch("/@me", userH.HandleUpdateSelf)
				r.Delete("/@me", userH.HandleDeleteSelf)
				r.Get("/@me/guilds", userH.HandleGetSelfGuilds)
				r.Get("/@me/dms", userH.HandleGetSelfDMs)
				r.Get("/@me/relationships", userH.HandleGetRelationships)
				r.Get("/@me/read-state", userH.HandleGetSelfReadState)
				r.Get("/@me/sessions", userH.HandleGetSelfSessions)
				r.Delete("/@me/sessions/{sessionID}", userH.HandleDeleteSelfSession)
				r.Get("/@me/settings", userH.HandleGetUserSettings)
				r.Patch("/@me/settings", userH.HandleUpdateUserSettings)
				r.Get("/@me/relationships", userH.HandleGetRelationships)
				r.Get("/@me/blocked", userH.HandleGetBlockedUsers)
				r.With(s.requireInstanceFeature("message_bookmarks")).Get("/@me/bookmarks", bookmarkH.HandleListBookmarks)
				r.Get("/@me/bots", botH.HandleListMyBots)
				r.Post("/@me/bots", botH.HandleCreateBot)
				r.Get("/@me/export", userH.HandleExportUserData)
				r.Get("/@me/export-account", userH.HandleExportAccount)
				r.Post("/@me/import-account", userH.HandleImportAccount)
				r.Get("/@me/instance-profiles", adminH.HandleGetInstanceProfiles)
				r.Post("/@me/instance-profiles", adminH.HandleAddInstanceProfile)
				r.Delete("/@me/instance-profiles/{profileID}", adminH.HandleRemoveInstanceProfile)
				r.Put("/@me/activity", userH.HandleUpdateActivity)
				r.Get("/@me/activity", userH.HandleGetActivity)
				r.With(s.requireInstanceFeature("threads_and_replies")).Get("/@me/hidden-threads", channelH.HandleGetHiddenThreads)
				r.With(s.requireInstanceFeature("custom_emoji")).Get("/@me/emoji", userH.HandleGetUserEmoji)
				r.With(s.requireInstanceFeature("custom_emoji")).Post("/@me/emoji", userH.HandleCreateUserEmoji)
				r.With(s.requireInstanceFeature("custom_emoji")).Delete("/@me/emoji/{emojiID}", userH.HandleDeleteUserEmoji)

				// Profile links.
				r.Get("/@me/links", userH.HandleGetMyLinks)
				r.Post("/@me/links", userH.HandleCreateLink)
				r.Patch("/@me/links/{linkID}", userH.HandleUpdateLink)
				r.Delete("/@me/links/{linkID}", userH.HandleDeleteLink)

				// User's own issues.
				r.With(s.requireInstanceFeature("moderation_reports")).Get("/@me/issues", modH.HandleGetMyIssues)

				// Group DMs.
				r.Post("/@me/group-dms", userH.HandleCreateGroupDM)

				// User guild positions (drag reordering).
				r.Put("/@me/guild-positions", userH.HandleUpdateGuildPositions)

				// Handle resolution must be before /{userID} to avoid conflicts.
				r.Get("/resolve", userH.HandleResolveHandle)

				r.Get("/{userID}", userH.HandleGetUser)
				r.Get("/{userID}/note", userH.HandleGetUserNote)
				r.Put("/{userID}/note", userH.HandleSetUserNote)
				r.Post("/{userID}/dm", userH.HandleCreateDM)
				r.Put("/{userID}/friend", userH.HandleAddFriend)
				r.Delete("/{userID}/friend", userH.HandleRemoveFriend)
				r.Put("/{userID}/block", userH.HandleBlockUser)
				r.Patch("/{userID}/block", userH.HandleUpdateBlockLevel)
				r.Delete("/{userID}/block", userH.HandleUnblockUser)
				r.Get("/{userID}/mutual-friends", userH.HandleGetMutualFriends)
				r.Get("/{userID}/mutual-guilds", userH.HandleGetMutualGuilds)
				r.Get("/{userID}/badges", userH.HandleGetUserBadges)
				r.Get("/{userID}/links", userH.HandleGetUserLinks)
				r.With(s.requireInstanceFeature("moderation_reports")).Post("/{userID}/report", modH.HandleReportUser)
			})

			// Bot management routes.
			r.Route("/bots/{botID}", func(r chi.Router) {
				r.Get("/", botH.HandleGetBot)
				r.Patch("/", botH.HandleUpdateBot)
				r.Delete("/", botH.HandleDeleteBot)
				r.Route("/tokens", func(r chi.Router) {
					r.Get("/", botH.HandleListTokens)
					r.Post("/", botH.HandleCreateToken)
					r.Delete("/{tokenID}", botH.HandleDeleteToken)
				})
				r.Route("/commands", func(r chi.Router) {
					r.Get("/", botH.HandleListCommands)
					r.Post("/", botH.HandleRegisterCommand)
					r.Patch("/{commandID}", botH.HandleUpdateCommand)
					r.Delete("/{commandID}", botH.HandleDeleteCommand)
				})
				r.Get("/guilds/{guildID}/permissions", botH.HandleGetBotGuildPermissions)
				r.Put("/guilds/{guildID}/permissions", botH.HandleUpdateBotGuildPermissions)
				r.Get("/presence", botH.HandleGetBotPresence)
				r.Put("/presence", botH.HandleUpdateBotPresence)
				r.Get("/rate-limit", botH.HandleGetBotRateLimit)
				r.Put("/rate-limit", botH.HandleUpdateBotRateLimit)
				r.Route("/subscriptions", func(r chi.Router) {
					r.Post("/", botH.HandleCreateEventSubscription)
					r.Get("/", botH.HandleListEventSubscriptions)
					r.Delete("/{subscriptionID}", botH.HandleDeleteEventSubscription)
				})
			})
			// Guild routes.
			r.Route("/guilds", func(r chi.Router) {
				r.Post("/", guildH.HandleCreateGuild)
				r.Get("/discover", guildH.HandleDiscoverGuilds)
				r.Get("/vanity/{code}", guildH.HandleResolveVanityURL)
				r.Get("/{guildID}/preview", guildH.HandleGetGuildPreview)
				r.Post("/{guildID}/join", guildH.HandleJoinDiscoverableGuild)
				r.Get("/{guildID}", guildH.HandleGetGuild)
				r.Patch("/{guildID}", guildH.HandleUpdateGuild)
				r.Delete("/{guildID}", guildH.HandleDeleteGuild)
				r.Post("/{guildID}/leave", guildH.HandleLeaveGuild)
				r.Post("/{guildID}/transfer", guildH.HandleTransferGuildOwnership)
				r.Get("/{guildID}/channels", guildH.HandleGetGuildChannels)
				r.Patch("/{guildID}/channels", guildH.HandleReorderGuildChannels)
				r.Post("/{guildID}/channels", guildH.HandleCreateGuildChannel)
				r.Post("/{guildID}/channels/{channelID}/clone", guildH.HandleCloneChannel)
				r.With(s.requireGuildFeature("guild_onboarding")).Get("/{guildID}/guide", guildH.HandleGetServerGuide)
				r.With(s.requireGuildFeature("guild_onboarding")).Put("/{guildID}/guide", guildH.HandleUpdateServerGuide)
				r.Get("/{guildID}/bump", guildH.HandleGetBumpStatus)
				r.Post("/{guildID}/bump", guildH.HandleBumpGuild)
				r.With(s.requireGuildFeature("widgets")).Get("/{guildID}/plugins", widgetH.HandleGetGuildPlugins)
				r.With(s.requireGuildFeature("widgets")).Post("/{guildID}/plugins", widgetH.HandleInstallPlugin)
				r.With(s.requireGuildFeature("widgets")).Patch("/{guildID}/plugins/{installID}", widgetH.HandleUpdateGuildPlugin)
				r.With(s.requireGuildFeature("widgets")).Delete("/{guildID}/plugins/{installID}", widgetH.HandleUninstallPlugin)
				r.Get("/{guildID}/channel-templates", channelH.HandleGetChannelTemplates)
				r.Post("/{guildID}/channel-templates", channelH.HandleCreateChannelTemplate)
				r.Delete("/{guildID}/channel-templates/{templateID}", channelH.HandleDeleteChannelTemplate)
				r.Post("/{guildID}/channel-templates/{templateID}/apply", channelH.HandleApplyChannelTemplate)
				r.Route("/{guildID}/templates", func(r chi.Router) {
					r.Post("/", guildH.HandleCreateGuildTemplate)
					r.Get("/", guildH.HandleGetGuildTemplates)
					r.Get("/{templateID}", guildH.HandleGetGuildTemplate)
					r.Delete("/{templateID}", guildH.HandleDeleteGuildTemplate)
					r.Post("/{templateID}/apply", guildH.HandleApplyGuildTemplate)
				})
				r.Get("/{guildID}/members/@me/permissions", guildH.HandleGetMyPermissions)
				r.Get("/{guildID}/members", guildH.HandleGetGuildMembers)
				r.Get("/{guildID}/members/search", guildH.HandleSearchGuildMembers)
				r.Get("/{guildID}/members/{memberID}", guildH.HandleGetGuildMember)
				r.Patch("/{guildID}/members/{memberID}", guildH.HandleUpdateGuildMember)
				r.Delete("/{guildID}/members/{memberID}", guildH.HandleRemoveGuildMember)
				r.Post("/{guildID}/members/{memberID}/warn", modH.HandleWarnMember)
				r.Get("/{guildID}/members/{memberID}/warnings", modH.HandleGetWarnings)
				r.Get("/{guildID}/members/{memberID}/roles", guildH.HandleGetMemberRoles)
				r.Put("/{guildID}/members/{memberID}/roles/{roleID}", guildH.HandleAddMemberRole)
				r.Delete("/{guildID}/members/{memberID}/roles/{roleID}", guildH.HandleRemoveMemberRole)
				r.Get("/{guildID}/prune", guildH.HandleGetGuildPruneCount)
				r.Post("/{guildID}/prune", guildH.HandleGuildPrune)
				r.Get("/{guildID}/bans", guildH.HandleGetGuildBans)
				r.Put("/{guildID}/bans/{userID}", guildH.HandleCreateGuildBan)
				r.Delete("/{guildID}/bans/{userID}", guildH.HandleRemoveGuildBan)
				r.Get("/{guildID}/roles", guildH.HandleGetGuildRoles)
				r.Patch("/{guildID}/roles", guildH.HandleReorderGuildRoles)
				r.Post("/{guildID}/roles", guildH.HandleCreateGuildRole)
				r.Patch("/{guildID}/roles/{roleID}", guildH.HandleUpdateGuildRole)
				r.Delete("/{guildID}/roles/{roleID}", guildH.HandleDeleteGuildRole)
				r.Get("/{guildID}/invites", guildH.HandleGetGuildInvites)
				r.Post("/{guildID}/invites", guildH.HandleCreateGuildInvite)
				r.Get("/{guildID}/categories", guildH.HandleGetGuildCategories)
				r.Post("/{guildID}/categories", guildH.HandleCreateGuildCategory)
				r.Patch("/{guildID}/categories/{categoryID}", guildH.HandleUpdateGuildCategory)
				r.Delete("/{guildID}/categories/{categoryID}", guildH.HandleDeleteGuildCategory)
				r.With(s.requireGuildFeature("audit_logs")).Get("/{guildID}/audit-log", guildH.HandleGetGuildAuditLog)
				r.With(s.requireGuildFeature("custom_emoji")).Get("/{guildID}/emoji", guildH.HandleGetGuildEmoji)
				r.With(s.requireGuildFeature("custom_emoji")).Post("/{guildID}/emoji", guildH.HandleCreateGuildEmoji)
				r.With(s.requireGuildFeature("custom_emoji")).Patch("/{guildID}/emoji/{emojiID}", guildH.HandleUpdateGuildEmoji)
				r.With(s.requireGuildFeature("custom_emoji")).Delete("/{guildID}/emoji/{emojiID}", guildH.HandleDeleteGuildEmoji)
				r.With(s.requireGuildFeature("webhooks")).Get("/{guildID}/webhooks", guildH.HandleGetGuildWebhooks)
				r.With(s.requireGuildFeature("webhooks")).Post("/{guildID}/webhooks", guildH.HandleCreateGuildWebhook)
				r.With(s.requireGuildFeature("webhooks")).Patch("/{guildID}/webhooks/{webhookID}", guildH.HandleUpdateGuildWebhook)
				r.With(s.requireGuildFeature("webhooks")).Delete("/{guildID}/webhooks/{webhookID}", guildH.HandleDeleteGuildWebhook)
				r.With(s.requireGuildFeature("widgets")).Get("/{guildID}/widget", widgetH.HandleGetGuildWidget)
				r.With(s.requireGuildFeature("widgets")).Patch("/{guildID}/widget", widgetH.HandleUpdateGuildWidget)
				r.With(s.requireGuildFeature("voice_broadcasts")).Get("/{guildID}/soundboard/config", s.handleGetSoundboardConfig)
				r.With(s.requireGuildFeature("voice_broadcasts")).Patch("/{guildID}/soundboard/config", s.handleUpdateSoundboardConfig)
				r.With(s.requireGuildFeature("voice_broadcasts")).Get("/{guildID}/soundboard/sounds", s.handleGetSoundboardSounds)
				r.With(s.requireGuildFeature("voice_broadcasts")).Post("/{guildID}/soundboard/sounds", s.handleCreateSoundboardSound)
				r.With(s.requireGuildFeature("voice_broadcasts")).Delete("/{guildID}/soundboard/sounds/{soundID}", s.handleDeleteSoundboardSound)
				r.With(s.requireGuildFeature("voice_broadcasts")).Post("/{guildID}/soundboard/sounds/{soundID}/play", s.handlePlaySoundboardSound)
				r.With(s.requireGuildFeature("webhooks")).Get("/{guildID}/webhooks/{webhookID}/logs", webhookH.HandleGetWebhookLogs)
				r.Get("/{guildID}/vanity-url", guildH.HandleGetGuildVanityURL)
				r.Patch("/{guildID}/vanity-url", guildH.HandleSetGuildVanityURL)
				r.Delete("/{guildID}/warnings/{warningID}", modH.HandleDeleteWarning)
				r.With(s.requireGuildFeature("moderation_reports")).Get("/{guildID}/reports", modH.HandleGetReports)
				r.With(s.requireGuildFeature("moderation_reports")).Patch("/{guildID}/reports/{reportID}", modH.HandleResolveReport)
				r.Get("/{guildID}/raid-config", modH.HandleGetRaidConfig)
				r.Patch("/{guildID}/raid-config", modH.HandleUpdateRaidConfig)
				r.Get("/{guildID}/features", guildH.HandleGetFeatureFlags)
				r.Patch("/{guildID}/features/{featureKey}", guildH.HandleUpdateFeatureFlag)

				// Ban list routes.
				r.Route("/{guildID}/ban-lists", func(r chi.Router) {
					r.Post("/", modH.HandleCreateBanList)
					r.Get("/", modH.HandleGetBanLists)
					r.Delete("/{listID}", modH.HandleDeleteBanList)
					r.Get("/{listID}/entries", modH.HandleGetBanListEntries)
					r.Post("/{listID}/entries", modH.HandleAddBanListEntry)
					r.Delete("/{listID}/entries/{entryID}", modH.HandleRemoveBanListEntry)
					r.Get("/{listID}/export", modH.HandleExportBanList)
					r.Post("/{listID}/import", modH.HandleImportBanList)
				})
				r.Get("/{guildID}/ban-list-subscriptions", modH.HandleGetBanListSubscriptions)
				r.Post("/{guildID}/ban-list-subscriptions", modH.HandleSubscribeBanList)
				r.Delete("/{guildID}/ban-list-subscriptions/{subID}", modH.HandleUnsubscribeBanList)

				// Guild sticker pack routes.
				r.Route("/{guildID}/sticker-packs", func(r chi.Router) {
					r.Use(s.requireGuildFeature("sticker_packs"))
					r.Post("/", stickerH.HandleCreateGuildPack)
					r.Get("/", stickerH.HandleGetGuildPacks)
					r.Delete("/{packID}", stickerH.HandleDeletePack)
					r.Get("/{packID}/stickers", stickerH.HandleGetPackStickers)
					r.Post("/{packID}/stickers", stickerH.HandleAddSticker)
					r.Delete("/{packID}/stickers/{stickerID}", stickerH.HandleDeleteSticker)
				})

				// Guild onboarding routes.
				r.Route("/{guildID}/onboarding", func(r chi.Router) {
					r.Use(s.requireGuildFeature("guild_onboarding"))
					r.Get("/", onboardH.HandleGetOnboarding)
					r.Put("/", onboardH.HandleUpdateOnboarding)
					r.Post("/prompts", onboardH.HandleCreatePrompt)
					r.Put("/prompts/{promptID}", onboardH.HandleUpdatePrompt)
					r.Delete("/prompts/{promptID}", onboardH.HandleDeletePrompt)
					r.Post("/complete", onboardH.HandleCompleteOnboarding)
					r.Get("/status", onboardH.HandleGetOnboardingStatus)
				})

				// Guild event routes.
				r.Route("/{guildID}/events", func(r chi.Router) {
					r.Post("/", guildEventH.HandleCreateEvent)
					r.Get("/", guildEventH.HandleListEvents)
					r.Get("/{eventID}", guildEventH.HandleGetEvent)
					r.Patch("/{eventID}", guildEventH.HandleUpdateEvent)
					r.Delete("/{eventID}", guildEventH.HandleDeleteEvent)
					r.Post("/{eventID}/rsvp", guildEventH.HandleRSVP)
					r.Delete("/{eventID}/rsvp", guildEventH.HandleDeleteRSVP)
					r.Get("/{eventID}/rsvps", guildEventH.HandleListRSVPs)
				})

				// Guild retention policy routes.
				r.Route("/{guildID}/retention", func(r chi.Router) {
					r.Get("/", guildH.HandleGetGuildRetentionPolicies)
					r.Post("/", guildH.HandleCreateGuildRetentionPolicy)
					r.Patch("/{policyID}", guildH.HandleUpdateGuildRetentionPolicy)
					r.Delete("/{policyID}", guildH.HandleDeleteGuildRetentionPolicy)
				})

				// Guild channel group routes (admin-managed).
				r.Route("/{guildID}/channel-groups", func(r chi.Router) {
					r.Use(s.requireGuildFeature("channel_groups"))
					r.Get("/", guildH.HandleGetChannelGroups)
					r.Post("/", guildH.HandleCreateChannelGroup)
					r.Patch("/{groupID}", guildH.HandleUpdateChannelGroup)
					r.Delete("/{groupID}", guildH.HandleDeleteChannelGroup)
					r.Put("/{groupID}/channels", guildH.HandleSetGroupChannels)
					r.Delete("/{groupID}/channels/{channelID}", guildH.HandleRemoveChannelFromGroup)
				})

				// Media gallery and tag routes.
				r.With(s.requireGuildFeature("gallery_media")).Get("/{guildID}/gallery", guildH.HandleGetGuildGallery)
				r.With(s.requireGuildFeature("gallery_media")).Get("/{guildID}/media-tags", guildH.HandleGetMediaTags)
				r.With(s.requireGuildFeature("gallery_media")).Post("/{guildID}/media-tags", guildH.HandleCreateMediaTag)
				r.With(s.requireGuildFeature("gallery_media")).Delete("/{guildID}/media-tags/{tagID}", guildH.HandleDeleteMediaTag)

				// AutoMod rules management.
				if s.AutoMod != nil {
					r.With(s.requireGuildFeature("automod")).Route("/{guildID}/automod", func(r chi.Router) {
						r.Get("/rules", s.AutoMod.HandleListRules)
						r.Post("/rules", s.AutoMod.HandleCreateRule)
						r.Post("/rules/test", s.AutoMod.HandleTestRule)
						r.Get("/rules/{ruleID}", s.AutoMod.HandleGetRule)
						r.Patch("/rules/{ruleID}", s.AutoMod.HandleUpdateRule)
						r.Delete("/rules/{ruleID}", s.AutoMod.HandleDeleteRule)
						r.Get("/actions", s.AutoMod.HandleGetActions)
					})
				}
			})

			// Channel routes.
			r.Route("/channels", func(r chi.Router) {
				r.Get("/{channelID}", channelH.HandleGetChannel)
				r.Patch("/{channelID}", channelH.HandleUpdateChannel)
				r.Delete("/{channelID}", channelH.HandleDeleteChannel)
				r.Get("/{channelID}/messages", channelH.HandleGetMessages)
				r.With(s.RateLimitMessages).Post("/{channelID}/messages", channelH.HandleCreateMessage)
				r.Post("/{channelID}/messages/bulk-delete", channelH.HandleBulkDeleteMessages)
				r.Get("/{channelID}/messages/{messageID}", channelH.HandleGetMessage)
				r.Patch("/{channelID}/messages/{messageID}", channelH.HandleUpdateMessage)
				r.Delete("/{channelID}/messages/{messageID}", channelH.HandleDeleteMessage)
				r.Get("/{channelID}/messages/{messageID}/edits", channelH.HandleGetMessageEdits)
				r.With(s.requireChannelFeature("announcement_channels")).Post("/{channelID}/messages/{messageID}/crosspost", channelH.HandleCrosspostMessage)
				r.Post("/{channelID}/messages/{messageID}/components/{componentID}/interact", botH.HandleComponentInteraction)
				r.Get("/{channelID}/messages/{messageID}/reactions", channelH.HandleGetReactions)
				r.Put("/{channelID}/messages/{messageID}/reactions/{emoji}", channelH.HandleAddReaction)
				r.Delete("/{channelID}/messages/{messageID}/reactions/{emoji}", channelH.HandleRemoveReaction)
				r.Delete("/{channelID}/messages/{messageID}/reactions/{emoji}/{targetUserID}", channelH.HandleRemoveUserReaction)
				r.With(s.requireChannelFeature("pins")).Get("/{channelID}/pins", channelH.HandleGetPins)
				r.With(s.requireChannelFeature("pins")).Put("/{channelID}/pins/{messageID}", channelH.HandlePinMessage)
				r.With(s.requireChannelFeature("pins")).Delete("/{channelID}/pins/{messageID}", channelH.HandleUnpinMessage)
				r.Post("/{channelID}/typing", channelH.HandleTriggerTyping)
				r.Post("/{channelID}/decrypt-messages", channelH.HandleBatchDecryptMessages)
				r.Post("/{channelID}/ack", channelH.HandleAckChannel)
				r.Put("/{channelID}/permissions/{overrideID}", channelH.HandleSetChannelPermission)
				r.Delete("/{channelID}/permissions/{overrideID}", channelH.HandleDeleteChannelPermission)
				r.With(s.requireChannelFeature("threads_and_replies")).Post("/{channelID}/messages/{messageID}/threads", channelH.HandleCreateThread)
				r.With(s.requireChannelFeature("moderation_reports")).Post("/{channelID}/messages/{messageID}/report", modH.HandleReportMessage)
				r.With(s.requireChannelFeature("moderation_reports")).Post("/{channelID}/messages/{messageID}/report-admin", modH.HandleReportToAdmin)
				r.With(s.requireChannelFeature("translation")).Post("/{channelID}/messages/{messageID}/translate", channelH.HandleTranslateMessage)
				r.With(s.requireChannelFeature("threads_and_replies")).Get("/{channelID}/threads", channelH.HandleGetThreads)
				r.With(s.requireChannelFeature("threads_and_replies")).Post("/{channelID}/threads/{threadID}/hide", channelH.HandleHideThread)
				r.With(s.requireChannelFeature("threads_and_replies")).Delete("/{channelID}/threads/{threadID}/hide", channelH.HandleUnhideThread)
				r.Post("/{channelID}/lock", modH.HandleLockChannel)
				r.Post("/{channelID}/unlock", modH.HandleUnlockChannel)
				r.With(s.requireChannelFeature("webhooks")).Get("/{channelID}/webhooks", channelH.HandleGetChannelWebhooks)
				r.Get("/{channelID}/export", userH.HandleExportChannelMessages)
				r.With(s.requireChannelFeature("gallery_media")).Get("/{channelID}/gallery", channelH.HandleGetChannelGallery)

				// Forum tag routes.
				r.Get("/{channelID}/tags", channelH.HandleGetForumTags)
				r.Post("/{channelID}/tags", channelH.HandleCreateForumTag)
				r.Patch("/{channelID}/tags/{tagID}", channelH.HandleUpdateForumTag)
				r.Delete("/{channelID}/tags/{tagID}", channelH.HandleDeleteForumTag)

				// Forum post routes.
				r.Get("/{channelID}/posts", channelH.HandleGetForumPosts)
				r.Post("/{channelID}/posts", channelH.HandleCreateForumPost)
				r.Post("/{channelID}/posts/{postID}/pin", channelH.HandlePinForumPost)
				r.Post("/{channelID}/posts/{postID}/close", channelH.HandleCloseForumPost)

				// Gallery tag routes.
				r.With(s.requireChannelFeature("gallery_media")).Get("/{channelID}/gallery-tags", channelH.HandleGetGalleryTags)
				r.With(s.requireChannelFeature("gallery_media")).Post("/{channelID}/gallery-tags", channelH.HandleCreateGalleryTag)
				r.With(s.requireChannelFeature("gallery_media")).Patch("/{channelID}/gallery-tags/{tagID}", channelH.HandleUpdateGalleryTag)
				r.With(s.requireChannelFeature("gallery_media")).Delete("/{channelID}/gallery-tags/{tagID}", channelH.HandleDeleteGalleryTag)

				// Gallery post routes.
				r.With(s.requireChannelFeature("gallery_media")).Get("/{channelID}/gallery-posts", channelH.HandleGetGalleryPosts)
				r.With(s.requireChannelFeature("gallery_media")).Post("/{channelID}/gallery-posts", channelH.HandleCreateGalleryPost)
				r.With(s.requireChannelFeature("gallery_media")).Post("/{channelID}/gallery-posts/{postID}/pin", channelH.HandlePinGalleryPost)
				r.With(s.requireChannelFeature("gallery_media")).Post("/{channelID}/gallery-posts/{postID}/close", channelH.HandleCloseGalleryPost)

				// Channel template routes.
				r.Route("/{channelID}/templates", func(r chi.Router) {
					r.Post("/", channelH.HandleCreateChannelTemplate)
					r.Get("/", channelH.HandleGetChannelTemplates)
					r.Delete("/{templateID}", channelH.HandleDeleteChannelTemplate)
					r.Post("/{templateID}/apply", channelH.HandleApplyChannelTemplate)
				})

				// Channel emoji routes.
				r.With(s.requireChannelFeature("custom_emoji")).Get("/{channelID}/emoji", channelEmojiH.HandleGetChannelEmoji)
				r.With(s.requireChannelFeature("custom_emoji")).Post("/{channelID}/emoji", channelEmojiH.HandleCreateChannelEmoji)
				r.With(s.requireChannelFeature("custom_emoji")).Delete("/{channelID}/emoji/{emojiID}", channelEmojiH.HandleDeleteChannelEmoji)

				// Announcement channel follower routes.
				r.With(s.requireChannelFeature("announcement_channels")).Post("/{channelID}/followers", channelH.HandleFollowChannel)
				r.With(s.requireChannelFeature("announcement_channels")).Get("/{channelID}/followers", channelH.HandleGetChannelFollowers)
				r.With(s.requireChannelFeature("announcement_channels")).Delete("/{channelID}/followers/{followerID}", channelH.HandleUnfollowChannel)
				r.With(s.requireChannelFeature("announcement_channels")).Post("/{channelID}/messages/{messageID}/publish", channelH.HandlePublishMessage)

				// Scheduled message routes.
				r.With(s.requireChannelFeature("scheduled_messages")).Post("/{channelID}/scheduled-messages", channelH.HandleScheduleMessage)
				r.With(s.requireChannelFeature("scheduled_messages")).Get("/{channelID}/scheduled-messages", channelH.HandleGetScheduledMessages)
				r.With(s.requireChannelFeature("scheduled_messages")).Delete("/{channelID}/scheduled-messages/{messageID}", channelH.HandleDeleteScheduledMessage)

				// Group DM recipient routes.
				r.Put("/{channelID}/recipients/{userID}", channelH.HandleAddGroupDMRecipient)
				r.Delete("/{channelID}/recipients/{userID}", channelH.HandleRemoveGroupDMRecipient)

				// Poll routes.
				r.With(s.requireChannelFeature("polls")).Post("/{channelID}/polls", pollH.HandleCreatePoll)
				r.With(s.requireChannelFeature("polls")).Get("/{channelID}/polls/{pollID}", pollH.HandleGetPoll)
				r.With(s.requireChannelFeature("polls")).Post("/{channelID}/polls/{pollID}/votes", pollH.HandleVotePoll)
				r.With(s.requireChannelFeature("polls")).Post("/{channelID}/polls/{pollID}/close", pollH.HandleClosePoll)
				r.With(s.requireChannelFeature("polls")).Delete("/{channelID}/polls/{pollID}", pollH.HandleDeletePoll)
			})

			// Message bookmark routes (top-level, not channel-scoped).
			r.Route("/messages", func(r chi.Router) {
				r.With(s.requireMessageFeature("message_bookmarks")).Put("/{messageID}/bookmark", bookmarkH.HandleCreateBookmark)
				r.With(s.requireMessageFeature("message_bookmarks")).Delete("/{messageID}/bookmark", bookmarkH.HandleDeleteBookmark)
			})

			// Voice routes.
			r.Route("/voice", func(r chi.Router) {
				r.Post("/{channelID}/join", s.handleVoiceJoin)
				r.Post("/{channelID}/leave", s.handleVoiceLeave)
				r.Get("/{channelID}/states", s.handleGetVoiceStates)
				r.Post("/{channelID}/members/{userID}/mute", s.handleVoiceServerMute)
				r.Post("/{channelID}/members/{userID}/deafen", s.handleVoiceServerDeafen)
				r.Post("/{channelID}/members/{userID}/move", s.handleVoiceMoveUser)

				// Voice preferences (PTT/VAD).
				r.Get("/preferences", s.handleGetVoicePreferences)
				r.Patch("/preferences", s.handleUpdateVoicePreferences)
				r.Post("/{channelID}/input-mode", s.handleSetInputMode)
				r.Post("/{channelID}/priority-speaker", s.handleSetPrioritySpeaker)
				r.Post("/{channelID}/members/{userID}/priority", s.handleSetPrioritySpeaker)

				// Soundboard.
				r.With(s.requireChannelFeature("voice_broadcasts")).Get("/{channelID}/soundboard", s.handleGetSoundboardSounds)
				r.With(s.requireChannelFeature("voice_broadcasts")).Post("/{channelID}/soundboard", s.handleCreateSoundboardSound)
				r.With(s.requireChannelFeature("voice_broadcasts")).Delete("/{channelID}/soundboard/{soundID}", s.handleDeleteSoundboardSound)
				r.With(s.requireChannelFeature("voice_broadcasts")).Post("/{channelID}/soundboard/{soundID}/play", s.handlePlaySoundboardSound)
				r.With(s.requireChannelFeature("voice_broadcasts")).Get("/{channelID}/soundboard/config", s.handleGetSoundboardConfig)
				r.With(s.requireChannelFeature("voice_broadcasts")).Patch("/{channelID}/soundboard/config", s.handleUpdateSoundboardConfig)

				// Voice broadcast.
				r.With(s.requireChannelFeature("voice_broadcasts")).Post("/{channelID}/broadcast", s.handleStartBroadcast)
				r.With(s.requireChannelFeature("voice_broadcasts")).Delete("/{channelID}/broadcast", s.handleStopBroadcast)
				r.With(s.requireChannelFeature("voice_broadcasts")).Get("/{channelID}/broadcast", s.handleGetBroadcast)

				// Screen sharing.
				r.With(s.requireChannelFeature("voice_broadcasts")).Post("/{channelID}/screen-share", s.handleStartScreenShare)
				r.With(s.requireChannelFeature("voice_broadcasts")).Delete("/{channelID}/screen-share", s.handleStopScreenShare)
				r.With(s.requireChannelFeature("voice_broadcasts")).Patch("/{channelID}/screen-share", s.handleUpdateScreenShare)
				r.With(s.requireChannelFeature("voice_broadcasts")).Get("/{channelID}/screen-shares", s.handleGetScreenShares)
			})

			// Issue reporting (any authenticated user).
			r.With(s.requireInstanceFeature("moderation_reports")).Post("/issues", modH.HandleCreateIssue)

			// Moderation panel routes (permission checks inside handlers).
			r.With(s.requireInstanceFeature("moderation_reports")).Route("/moderation", func(r chi.Router) {
				r.Get("/stats", modH.HandleGetModerationStats)
				r.Get("/user-reports", modH.HandleGetUserReports)
				r.Patch("/user-reports/{reportID}", modH.HandleResolveUserReport)
				r.Get("/message-reports", modH.HandleGetAllMessageReports)
				r.Patch("/message-reports/{reportID}", modH.HandleResolveMessageReport)
				r.Get("/issues", modH.HandleGetIssues)
				r.Patch("/issues/{issueID}", modH.HandleResolveIssue)
			})

			// Public ban lists.
			r.Get("/ban-lists/public", modH.HandleGetPublicBanLists)

			// Webhook templates, preview, and outgoing events.
			r.With(s.requireInstanceFeature("webhooks")).Route("/webhooks", func(r chi.Router) {
				r.Get("/templates", webhookH.HandleGetWebhookTemplates)
				r.Post("/preview", webhookH.HandlePreviewWebhookMessage)
				r.Get("/outgoing-events", webhookH.HandleGetOutgoingEvents)
			})

			// User sticker packs.
			r.Route("/stickers", func(r chi.Router) {
				r.Use(s.requireInstanceFeature("sticker_packs"))
				r.Get("/my-packs", stickerH.HandleGetUserPacks)
				r.Post("/my-packs", stickerH.HandleCreateUserPack)
				r.Get("/my-packs/{packID}/stickers", stickerH.HandleGetUserPackStickers)
				r.Post("/packs/{packID}/share", stickerH.HandleEnableSharing)
				r.Delete("/packs/{packID}/share", stickerH.HandleDisableSharing)
				r.Get("/shared/{shareCode}", stickerH.HandleGetSharedPack)
				r.Post("/shared/{shareCode}/clone", stickerH.HandleClonePack)
			})

			// Theme gallery routes.
			r.With(s.requireInstanceFeature("theme_editor")).Route("/themes", func(r chi.Router) {
				r.Get("/", themeH.HandleListSharedThemes)
				r.Post("/", themeH.HandleShareTheme)
				r.Get("/{shareCode}", themeH.HandleGetSharedTheme)
				r.Put("/{themeID}/like", themeH.HandleLikeTheme)
				r.Delete("/{themeID}/like", themeH.HandleUnlikeTheme)
				r.Delete("/{themeID}", themeH.HandleDeleteSharedTheme)
			})

			// Widget and plugin routes.
			r.Route("/widgets", func(r chi.Router) {
				r.With(s.requireGuildFeature("widgets")).Get("/guilds/{guildID}", widgetH.HandleGetGuildWidget)
				r.With(s.requireGuildFeature("widgets")).Patch("/guilds/{guildID}", widgetH.HandleUpdateGuildWidget)
			})
			r.Route("/channels/{channelID}/widgets", func(r chi.Router) {
				r.Use(s.requireChannelFeature("widgets"))
				r.Get("/", widgetH.HandleGetChannelWidgets)
				r.Post("/", widgetH.HandleCreateChannelWidget)
				r.Patch("/{widgetID}", widgetH.HandleUpdateChannelWidget)
				r.Delete("/{widgetID}", widgetH.HandleDeleteChannelWidget)
			})
			r.Route("/plugins", func(r chi.Router) {
				r.Use(s.requireInstanceFeature("widgets"))
				r.Get("/", widgetH.HandleListPlugins)
				r.Get("/{pluginID}", widgetH.HandleGetPlugin)
				r.Post("/{pluginID}/install", widgetH.HandleInstallPlugin)
				r.Get("/guilds/{guildID}", widgetH.HandleGetGuildPlugins)
				r.Patch("/guilds/{guildID}/{pluginID}", widgetH.HandleUpdateGuildPlugin)
				r.Delete("/guilds/{guildID}/{pluginID}", widgetH.HandleUninstallPlugin)
			})
			r.With(s.requireInstanceFeature("e2ee")).Route("/encryption/key-backup", func(r chi.Router) {
				r.Post("/", widgetH.HandleCreateKeyBackup)
				r.Get("/", widgetH.HandleGetKeyBackup)
				r.Get("/download", widgetH.HandleDownloadKeyBackup)
				r.Delete("/", widgetH.HandleDeleteKeyBackup)
				r.Post("/recovery-codes", widgetH.HandleGenerateRecoveryCodes)
			})

			// Experimental features.
			r.Route("/channels/{channelID}/experimental", func(r chi.Router) {
				// Location sharing.
				r.With(s.requireChannelFeature("location_sharing")).Post("/location", experimentalH.HandleShareLocation)
				r.With(s.requireChannelFeature("location_sharing")).Patch("/location/{shareID}", experimentalH.HandleUpdateLiveLocation)
				r.With(s.requireChannelFeature("location_sharing")).Delete("/location/{shareID}", experimentalH.HandleStopLiveLocation)
				r.With(s.requireChannelFeature("location_sharing")).Get("/locations", experimentalH.HandleGetLocationShares)
				// Message effects & super reactions.
				r.With(s.requireChannelFeature("message_effects")).Post("/messages/{messageID}/effects", experimentalH.HandleCreateMessageEffect)
				r.With(s.requireChannelFeature("super_reactions")).Post("/messages/{messageID}/super-reactions", experimentalH.HandleAddSuperReaction)
				r.With(s.requireChannelFeature("super_reactions")).Get("/messages/{messageID}/super-reactions", experimentalH.HandleGetSuperReactions)
				// Message summaries.
				r.With(s.requireChannelFeature("message_summaries")).Post("/summarize", experimentalH.HandleSummarizeMessages)
				r.With(s.requireChannelFeature("message_summaries")).Get("/summaries", experimentalH.HandleGetSummaries)
				// Voice transcription.
				r.With(s.requireChannelFeature("voice_transcription")).Get("/transcription/settings", experimentalH.HandleGetTranscriptionSettings)
				r.With(s.requireChannelFeature("voice_transcription")).Patch("/transcription/settings", experimentalH.HandleUpdateTranscriptionSettings)
				r.With(s.requireChannelFeature("voice_transcription")).Get("/transcriptions", experimentalH.HandleGetTranscriptions)
				// Whiteboards.
				r.With(s.requireChannelFeature("whiteboards")).Post("/whiteboards", experimentalH.HandleCreateWhiteboard)
				r.With(s.requireChannelFeature("whiteboards")).Get("/whiteboards", experimentalH.HandleGetWhiteboards)
				r.With(s.requireChannelFeature("whiteboards")).Patch("/whiteboards/{whiteboardID}", experimentalH.HandleUpdateWhiteboard)
				r.With(s.requireChannelFeature("whiteboards")).Get("/whiteboards/{whiteboardID}", experimentalH.HandleGetWhiteboardState)
				// Code snippets.
				r.With(s.requireChannelFeature("code_snippets")).Post("/code-snippets", experimentalH.HandleCreateCodeSnippet)
				r.With(s.requireChannelFeature("code_snippets")).Get("/code-snippets/{snippetID}", experimentalH.HandleGetCodeSnippet)
				r.With(s.requireChannelFeature("code_snippets")).Post("/code-snippets/{snippetID}/run", experimentalH.HandleRunCodeSnippet)
				// Video recordings.
				r.With(s.requireChannelFeature("video_recordings")).Post("/recordings", experimentalH.HandleCreateVideoRecording)
				r.With(s.requireChannelFeature("video_recordings")).Get("/recordings", experimentalH.HandleGetRecordings)
				// Kanban boards.
				r.With(s.requireChannelFeature("kanban_boards")).Post("/kanban", experimentalH.HandleCreateKanbanBoard)
				r.With(s.requireChannelFeature("kanban_boards")).Get("/kanban", experimentalH.HandleGetKanbanBoards)
				r.With(s.requireChannelFeature("kanban_boards")).Get("/kanban/{boardID}", experimentalH.HandleGetKanbanBoard)
				r.With(s.requireChannelFeature("kanban_boards")).Post("/kanban/{boardID}/columns", experimentalH.HandleCreateKanbanColumn)
				r.With(s.requireChannelFeature("kanban_boards")).Post("/kanban/{boardID}/columns/{columnID}/cards", experimentalH.HandleCreateKanbanCard)
				r.With(s.requireChannelFeature("kanban_boards")).Patch("/kanban/{boardID}/cards/{cardID}/move", experimentalH.HandleMoveKanbanCard)
				r.With(s.requireChannelFeature("kanban_boards")).Delete("/kanban/{boardID}/cards/{cardID}", experimentalH.HandleDeleteKanbanCard)
			})

			// Activities and games.
			r.With(s.requireInstanceFeature("activities")).Route("/activities", func(r chi.Router) {
				r.Get("/", activityH.HandleListActivities)
				r.Get("/{activityID}", activityH.HandleGetActivity)
				r.Post("/", activityH.HandleCreateActivity)
				r.Post("/{activityID}/rate", activityH.HandleRateActivity)
				r.Post("/{channelID}/sessions", activityH.HandleStartActivitySession)
				r.Get("/{channelID}/sessions/active", activityH.HandleGetActiveSession)
				r.Post("/sessions/{sessionID}/join", activityH.HandleJoinActivitySession)
				r.Post("/sessions/{sessionID}/leave", activityH.HandleLeaveActivitySession)
				r.Post("/sessions/{sessionID}/end", activityH.HandleEndActivitySession)
				r.Patch("/sessions/{sessionID}/state", activityH.HandleUpdateActivityState)
			})
			r.With(s.requireInstanceFeature("activities")).Route("/games", func(r chi.Router) {
				r.Post("/", activityH.HandleCreateGame)
				r.Post("/{gameSessionID}/join", activityH.HandleJoinGame)
				r.Post("/{gameSessionID}/move", activityH.HandleGameMove)
				r.Get("/{gameSessionID}", activityH.HandleGetGame)
				r.Get("/leaderboard/{activityID}", activityH.HandleGetLeaderboard)
			})
			r.With(s.requireInstanceFeature("activities")).Route("/watch-together", func(r chi.Router) {
				r.Post("/", activityH.HandleStartWatchTogether)
				r.Post("/{sessionID}/sync", activityH.HandleSyncWatchTogether)
			})
			r.With(s.requireInstanceFeature("activities")).Route("/music-party", func(r chi.Router) {
				r.Post("/", activityH.HandleStartMusicParty)
				r.Post("/{sessionID}/queue", activityH.HandleAddToMusicQueue)
			})

			// Social and growth features.
			r.Route("/guilds/{guildID}/insights", func(r chi.Router) {
				r.Get("/", socialH.HandleGetInsights)
			})
			r.Route("/guilds/{guildID}/boosts", func(r chi.Router) {
				r.Get("/", socialH.HandleGetBoosts)
				r.Post("/", socialH.HandleCreateBoost)
				r.Delete("/", socialH.HandleRemoveBoost)
			})
			r.Post("/guilds/{guildID}/vanity-claim", socialH.HandleClaimVanityURL)
			r.Delete("/guilds/{guildID}/vanity-claim", socialH.HandleReleaseVanityURL)
			r.Get("/guilds/{guildID}/vanity-check", socialH.HandleCheckVanityAvailability)
			r.Get("/achievements", socialH.HandleGetAchievements)
			r.Get("/users/{userID}/achievements", socialH.HandleGetUserAchievements)
			r.Post("/users/@me/achievements/check", socialH.HandleCheckAchievements)
			r.Route("/guilds/{guildID}/leveling", func(r chi.Router) {
				r.Get("/", socialH.HandleGetLevelingConfig)
				r.Patch("/", socialH.HandleUpdateLevelingConfig)
				r.Post("/roles", socialH.HandleAddLevelRole)
				r.Delete("/roles/{roleID}", socialH.HandleDeleteLevelRole)
			})
			r.Get("/guilds/{guildID}/leaderboard", socialH.HandleGetLeaderboard)
			r.Get("/guilds/{guildID}/members/{memberID}/xp", socialH.HandleGetMemberXP)
			r.Route("/guilds/{guildID}/starboard", func(r chi.Router) {
				r.Get("/", socialH.HandleGetStarboardConfig)
				r.Patch("/", socialH.HandleUpdateStarboardConfig)
				r.Get("/entries", socialH.HandleGetStarboardEntries)
			})
			r.Route("/guilds/{guildID}/welcome", func(r chi.Router) {
				r.Get("/", socialH.HandleGetWelcomeConfig)
				r.Patch("/", socialH.HandleUpdateWelcomeConfig)
			})
			r.Route("/guilds/{guildID}/auto-roles", func(r chi.Router) {
				r.Get("/", socialH.HandleGetAutoRoles)
				r.Post("/", socialH.HandleCreateAutoRole)
				r.Patch("/{ruleID}", socialH.HandleUpdateAutoRole)
				r.Delete("/{ruleID}", socialH.HandleDeleteAutoRole)
			})

			// Integration routes.
			r.With(s.requireGuildFeature("federated_messaging")).Route("/guilds/{guildID}/integrations", func(r chi.Router) {
				r.Get("/", integrationH.HandleListIntegrations)
				r.Post("/", integrationH.HandleCreateIntegration)
				r.Get("/log", integrationH.HandleGetIntegrationLog)
				r.Get("/{integrationID}", integrationH.HandleGetIntegration)
				r.Patch("/{integrationID}", integrationH.HandleUpdateIntegration)
				r.Delete("/{integrationID}", integrationH.HandleDeleteIntegration)
				r.Get("/{integrationID}/activitypub/follows", integrationH.HandleListActivityPubFollows)
				r.Post("/{integrationID}/activitypub/follows", integrationH.HandleAddActivityPubFollow)
				r.Delete("/{integrationID}/activitypub/follows/{followID}", integrationH.HandleRemoveActivityPubFollow)
			})
			r.With(s.requireGuildFeature("federated_messaging")).Route("/guilds/{guildID}/bridge-connections", func(r chi.Router) {
				r.Get("/", integrationH.HandleListBridgeConnections)
				r.Post("/", integrationH.HandleCreateBridgeConnection)
				r.Patch("/{connectionID}", integrationH.HandleUpdateBridgeConnection)
				r.Delete("/{connectionID}", integrationH.HandleDeleteBridgeConnection)
			})

			// Invite routes.
			r.Route("/invites", func(r chi.Router) {
				r.Get("/{code}", inviteH.HandleGetInvite)
				r.Post("/{code}", inviteH.HandleAcceptInvite)
				r.Delete("/{code}", inviteH.HandleDeleteInvite)
			})

			// File upload and media management.
			if s.Media != nil {
				r.Post("/files/upload", s.Media.HandleUpload)
				r.Route("/files/{fileID}", func(r chi.Router) {
					r.Patch("/", s.Media.HandleUpdateAttachment)
					r.Delete("/", s.Media.HandleDeleteAttachment)
					r.Put("/tags/{tagID}", s.Media.HandleTagAttachment)
					r.Delete("/tags/{tagID}", s.Media.HandleUntagAttachment)
				})
			} else {
				r.Post("/files/upload", stubHandler("upload_file"))
			}

			// MLS encryption delivery service routes.
			if s.Encryption != nil {
				r.With(s.requireInstanceFeature("e2ee")).Route("/encryption", func(r chi.Router) {
					// Key package management.
					r.Post("/key-packages", s.Encryption.HandleUploadKeyPackage)
					r.Get("/key-packages/{userID}", s.Encryption.HandleGetKeyPackages)
					r.Post("/key-packages/{userID}/claim", s.Encryption.HandleClaimKeyPackage)
					r.Delete("/key-packages/{packageID}", s.Encryption.HandleDeleteKeyPackage)

					// Welcome messages.
					r.Post("/channels/{channelID}/welcome", s.Encryption.HandleSendWelcome)
					r.Get("/welcome", s.Encryption.HandleGetWelcomes)
					r.Delete("/welcome/{welcomeID}", s.Encryption.HandleAckWelcome)

					// Group state.
					r.Get("/channels/{channelID}/group-state", s.Encryption.HandleGetGroupState)
					r.Put("/channels/{channelID}/group-state", s.Encryption.HandleUpdateGroupState)

					// Commits.
					r.Post("/channels/{channelID}/commits", s.Encryption.HandlePublishCommit)
					r.Get("/channels/{channelID}/commits", s.Encryption.HandleGetCommits)
				})
			}

			// Notification routes (preferences always available; push requires VAPID).
			if s.Notifications != nil {
				r.Route("/notifications", func(r chi.Router) {
					// Persistent notification CRUD.
					r.Get("/", s.Notifications.HandleListNotifications)
					r.Patch("/{id}", s.Notifications.HandleUpdateNotification)
					r.Delete("/{id}", s.Notifications.HandleDeleteNotification)
					r.Post("/mark-all-read", s.Notifications.HandleMarkAllRead)
					r.Delete("/", s.Notifications.HandleClearAll)
					r.Get("/search", s.Notifications.HandleSearchNotifications)
					r.Get("/unread-count", s.Notifications.HandleGetUnreadCount)

					// Per-type delivery preferences.
					r.Get("/type-preferences", s.Notifications.HandleGetTypePreferences)
					r.Put("/type-preferences", s.Notifications.HandleUpdateTypePreferences)

					// Guild/global preference management (always available).
					r.Get("/preferences", s.Notifications.HandleGetPreferences)
					r.Patch("/preferences", s.Notifications.HandleUpdatePreferences)
					r.Get("/preferences/channels", s.Notifications.HandleGetChannelPreferences)
					r.Patch("/preferences/channels", s.Notifications.HandleUpdateChannelPreference)
					r.Delete("/preferences/channels/{channelID}", s.Notifications.HandleDeleteChannelPreference)

					// Push subscription routes. The VAPID probe is always registered so
					// clients can detect disabled push without falling through to
					// /notifications/{id} and receiving a misleading 405.
					r.With(s.requireInstanceFeature("pwa_push")).Get("/vapid-key", s.Notifications.HandleGetVAPIDKey)
					r.With(s.requireInstanceFeature("pwa_push")).Post("/subscriptions", s.Notifications.HandleSubscribe)
					r.With(s.requireInstanceFeature("pwa_push")).Get("/subscriptions", s.Notifications.HandleListSubscriptions)
					r.With(s.requireInstanceFeature("pwa_push")).Delete("/subscriptions/{subscriptionID}", s.Notifications.HandleUnsubscribe)
				})
			}

			// Search routes (with search-specific rate limit).
			r.With(s.RateLimitSearch).Route("/search", func(r chi.Router) {
				r.With(s.requireInstanceFeature("full_text_search")).Get("/messages", s.handleSearchMessages)
				r.Get("/users", s.handleSearchUsers)
				r.Get("/guilds", s.handleSearchGuilds)
			})

			// Giphy proxy routes (only if enabled).
			if s.Config.Giphy.Enabled && s.Config.Giphy.APIKey != "" {
				r.With(s.requireInstanceFeature("gif_search")).Route("/giphy", func(r chi.Router) {
					r.Get("/search", s.handleGiphySearch)
					r.Get("/trending", s.handleGiphyTrending)
					r.Get("/categories", s.handleGiphyCategories)
				})
			}

			// Instance announcements (visible to all logged-in users).
			r.Get("/announcements", adminH.HandleGetAnnouncements)

			// Admin routes — protected by RequireAdmin middleware.
			r.Route("/admin", func(r chi.Router) {
				// First-run setup must be reachable before an admin user exists.
				// HandleCompleteSetup still requires admin access when setup has already completed.
				r.Get("/setup/status", adminH.HandleGetSetupStatus)
				r.Post("/setup/complete", adminH.HandleCompleteSetup)

				r.Group(func(r chi.Router) {
					r.Use(RequireAdmin(s.DB.Pool))
					r.Get("/instance", adminH.HandleGetInstance)
					r.Patch("/instance", adminH.HandleUpdateInstance)
					r.With(s.requireInstanceFeature("federated_messaging")).Get("/federation/peers", adminH.HandleGetFederationPeers)
					r.With(s.requireInstanceFeature("federated_messaging")).Post("/federation/peers", adminH.HandleAddFederationPeer)
					r.With(s.requireInstanceFeature("federated_messaging")).Delete("/federation/peers/{peerID}", adminH.HandleRemoveFederationPeer)
					r.Get("/stats", adminH.HandleGetStats)
					r.Get("/users", adminH.HandleListUsers)
					r.Post("/users/{userID}/suspend", adminH.HandleSuspendUser)
					r.Post("/users/{userID}/unsuspend", adminH.HandleUnsuspendUser)
					r.Post("/users/{userID}/set-admin", adminH.HandleSetAdmin)
					r.Post("/users/{userID}/set-globalmod", adminH.HandleSetGlobalMod)
					r.Post("/users/{userID}/instance-ban", adminH.HandleInstanceBanUser)
					r.Post("/users/{userID}/instance-unban", adminH.HandleInstanceUnbanUser)
					r.Get("/instance-bans", adminH.HandleGetInstanceBans)
					r.Get("/guilds", adminH.HandleListGuilds)
					r.Get("/guilds/{guildID}", adminH.HandleGetGuildDetails)
					r.Delete("/guilds/{guildID}", adminH.HandleAdminDeleteGuild)
					r.Get("/users/{userID}/guilds", adminH.HandleGetUserGuilds)
					r.Get("/registration", adminH.HandleGetRegistrationConfig)
					r.Patch("/registration", adminH.HandleUpdateRegistrationConfig)
					r.Get("/features", adminH.HandleGetFeatureFlags)
					r.Patch("/features/{featureKey}", adminH.HandleUpdateFeatureFlag)
					r.Get("/transcription", adminH.HandleGetTranscriptionConfig)
					r.Patch("/transcription", adminH.HandleUpdateTranscriptionConfig)
					r.Post("/registration/tokens", adminH.HandleCreateRegistrationToken)
					r.Get("/registration/tokens", adminH.HandleListRegistrationTokens)
					r.Delete("/registration/tokens/{tokenID}", adminH.HandleDeleteRegistrationToken)
					r.Post("/announcements", adminH.HandleCreateAnnouncement)
					r.Get("/announcements", adminH.HandleListAllAnnouncements)
					r.Patch("/announcements/{announcementID}", adminH.HandleUpdateAnnouncement)
					r.Delete("/announcements/{announcementID}", adminH.HandleDeleteAnnouncement)
					r.With(s.requireInstanceFeature("moderation_reports")).Get("/reports", modH.HandleGetAdminReports)
					r.Get("/bots", botH.HandleAdminListAllBots)
					r.Get("/rate-limits/stats", adminH.HandleGetRateLimitStats)
					r.Get("/rate-limits/log", adminH.HandleGetRateLimitLog)
					r.Patch("/rate-limits", adminH.HandleUpdateRateLimitConfig)
					r.Route("/content-scan", func(r chi.Router) {
						r.Get("/rules", adminH.HandleGetContentScanRules)
						r.Post("/rules", adminH.HandleCreateContentScanRule)
						r.Patch("/rules/{ruleID}", adminH.HandleUpdateContentScanRule)
						r.Delete("/rules/{ruleID}", adminH.HandleDeleteContentScanRule)
						r.Get("/log", adminH.HandleGetContentScanLog)
					})
					r.Get("/captcha", adminH.HandleGetCaptchaConfig)
					r.Patch("/captcha", adminH.HandleUpdateCaptchaConfig)

					// Federation dashboard and management.
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/dashboard", adminH.HandleGetFederationDashboard)
					r.With(s.requireInstanceFeature("federated_messaging")).Put("/federation/peers/{peerID}/control", adminH.HandleUpdatePeerControl)
					r.With(s.requireInstanceFeature("federated_messaging")).Post("/federation/peers/{peerID}/approve", adminH.HandleApproveFederationPeer)
					r.With(s.requireInstanceFeature("federated_messaging")).Post("/federation/peers/{peerID}/reject", adminH.HandleRejectFederationPeer)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/peers/controls", adminH.HandleGetPeerControls)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/key-audit", adminH.HandleGetKeyAudit)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Post("/federation/key-audit/{auditID}/acknowledge", adminH.HandleAcknowledgeKeyChange)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/delivery-receipts", adminH.HandleGetDeliveryReceipts)
					r.With(s.requireInstanceFeature("federated_messaging")).Post("/federation/delivery-receipts/{receiptID}/retry", adminH.HandleRetryDelivery)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/search-config", adminH.HandleGetFederatedSearchConfig)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Patch("/federation/search-config", adminH.HandleUpdateFederatedSearchConfig)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/protocol", adminH.HandleGetProtocolInfo)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Patch("/federation/protocol", adminH.HandleUpdateProtocolConfig)

					// Instance blocklist/allowlist.
					r.With(s.requireInstanceFeature("federated_messaging")).Get("/federation/blocklist", adminH.HandleGetInstanceBlocklist)
					r.With(s.requireInstanceFeature("federated_messaging")).Get("/federation/allowlist", adminH.HandleGetInstanceAllowlist)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/profiles", adminH.HandleGetInstanceProfiles)
					r.With(s.requireInstanceFeature("federated_messaging")).Post("/federation/profiles", adminH.HandleAddInstanceProfile)
					r.With(s.requireInstanceFeature("federated_messaging")).Delete("/federation/profiles/{profileID}", adminH.HandleRemoveInstanceProfile)
					r.With(s.requireInstanceFeature("federation_admin_diagnostics")).Get("/federation/users/{instanceID}/{userID}", adminH.HandleGetFederatedUserProfile)

					// Self-hosting management.
					r.Get("/updates", adminH.HandleCheckUpdates)
					r.Post("/updates/latest", adminH.HandleSetLatestVersion)
					r.Post("/updates/dismiss", adminH.HandleDismissUpdate)
					r.Get("/updates/config", adminH.HandleGetUpdateConfig)
					r.Patch("/updates/config", adminH.HandleUpdateUpdateConfig)
					r.Get("/health/dashboard", adminH.HandleGetHealthDashboard)
					r.Get("/health/history", adminH.HandleGetHealthHistory)
					r.Get("/storage", adminH.HandleGetStorageDashboard)
					r.Route("/retention", func(r chi.Router) {
						r.Get("/", adminH.HandleGetRetentionPolicies)
						r.Post("/", adminH.HandleCreateRetentionPolicy)
						r.Patch("/{policyID}", adminH.HandleUpdateRetentionPolicy)
						r.Delete("/{policyID}", adminH.HandleDeleteRetentionPolicy)
						r.Post("/{policyID}/run", adminH.HandleRunRetentionPolicy)
					})
					r.Route("/domains", func(r chi.Router) {
						r.Get("/", adminH.HandleGetCustomDomains)
						r.Post("/", adminH.HandleCreateCustomDomain)
						r.Post("/{domainID}/verify", adminH.HandleVerifyCustomDomain)
						r.Delete("/{domainID}", adminH.HandleDeleteCustomDomain)
					})
					r.With(s.requireInstanceFeature("admin_backups")).Route("/backups", func(r chi.Router) {
						r.Get("/", adminH.HandleGetBackupSchedules)
						r.Post("/", adminH.HandleCreateBackupSchedule)
						r.Patch("/{scheduleID}", adminH.HandleUpdateBackupSchedule)
						r.Delete("/{scheduleID}", adminH.HandleDeleteBackupSchedule)
						r.Get("/{scheduleID}/history", adminH.HandleGetBackupHistory)
						r.Post("/{scheduleID}/trigger", adminH.HandleTriggerBackup)
					})

					// Bridge management.
					r.With(s.requireInstanceFeature("federated_messaging")).Route("/bridges", func(r chi.Router) {
						r.Get("/", adminH.HandleGetBridges)
						r.Post("/", adminH.HandleCreateBridge)
						r.Patch("/{bridgeID}", adminH.HandleUpdateBridge)
						r.Delete("/{bridgeID}", adminH.HandleDeleteBridge)
						r.Get("/{bridgeID}/mappings", adminH.HandleGetBridgeChannelMappings)
						r.Post("/{bridgeID}/mappings", adminH.HandleCreateBridgeChannelMapping)
						r.Delete("/{bridgeID}/mappings/{mappingID}", adminH.HandleDeleteBridgeChannelMapping)
						r.Get("/{bridgeID}/virtual-users", adminH.HandleGetBridgeVirtualUsers)
					})

					// Admin media management.
					r.Get("/media", adminH.HandleAdminGetMedia)
					r.Delete("/media/{fileID}", adminH.HandleAdminDeleteMedia)
				})
			})
		})

		// Public routes — rate limited by IP (no authenticated user context).
		r.Group(func(r chi.Router) {
			r.Use(s.RateLimitGlobal())

			r.Get("/guilds/{guildID}/widget.json", widgetH.HandleGetGuildWidgetEmbed)

			if s.Media != nil {
				r.Get("/files/{fileID}", s.Media.HandleGetFile)
			}

			// Federation media proxy — streams remote instance media to avoid CORS issues.
			r.With(s.requireInstanceFeature("federated_attachments")).Get("/federation/media/{instanceId}/{fileId}", s.handleFederationMediaProxy)

			r.With(s.RateLimitWebhooks, s.requireInstanceFeature("webhooks")).Post("/webhooks/{webhookID}/{token}", webhookH.HandleExecute)
		})
	})

	// Start outgoing webhook event subscriber (delivers events to outgoing webhook URLs).
	if s.EventBus != nil {
		webhookH.StartOutgoingWebhookSubscriber()
	}
}

type clientConfigResponse struct {
	FileUploadsEnabled bool                      `json:"file_uploads_enabled"`
	MaxUploadBytes     int64                     `json:"max_upload_bytes"`
	LocalInstanceID    string                    `json:"local_instance_id"`
	Version            string                    `json:"version"`
	BuildVersion       string                    `json:"build_version"`
	FeatureFlags       map[string]features.State `json:"feature_flags"`
	Experimental       map[string]bool           `json:"experimental_features"`
}

func (s *Server) handleClientConfig(w http.ResponseWriter, r *http.Request) {
	guildID := r.URL.Query().Get("guild_id")
	states, err := features.Resolve(r.Context(), s.DB.Pool, guildID)
	if err != nil {
		apiutil.InternalError(w, s.Logger, "Failed to load feature flags", err)
		return
	}
	enabled := features.EnabledMap(states)
	resp := clientConfigResponse{
		FileUploadsEnabled: s.Media != nil,
		LocalInstanceID:    s.InstanceID,
		Version:            s.Version,
		BuildVersion:       s.BuildVersion,
		FeatureFlags:       states,
		Experimental: map[string]bool{
			"translation":    enabled["translation"],
			"whiteboard":     enabled["whiteboards"],
			"kanban":         enabled["kanban_boards"],
			"location_share": enabled["location_sharing"],
			"code_snippets":  enabled["code_snippets"],
			"transcription":  enabled["voice_transcription"],
		},
	}
	if s.Media != nil {
		resp.MaxUploadBytes = s.Media.MaxUploadBytes()
	}
	apiutil.WriteJSON(w, http.StatusOK, resp)
}

// Start begins listening for HTTP requests on the configured address.
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.Config.HTTP.Listen,
		Handler:      s.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.Logger.Info("HTTP server starting", slog.String("listen", s.Config.HTTP.Listen))
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.Logger.Info("HTTP server shutting down")
	return s.server.Shutdown(ctx)
}

// --- Auth Handlers ---

// handleGetPublicRegistrationConfig returns public registration mode metadata.
// GET /api/v1/auth/registration
func (s *Server) handleGetPublicRegistrationConfig(w http.ResponseWriter, r *http.Request) {
	var mode, message string
	if err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'registration_mode'), 'open'
		)`).Scan(&mode); err != nil {
		mode = "open"
	}
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'registration_message'), ''
		)`).Scan(&message)

	WriteJSON(w, http.StatusOK, map[string]string{
		"mode":    mode,
		"message": message,
	})
}

// handleRegister handles POST /api/v1/auth/register.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Check registration mode.
	var regMode string
	s.DB.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'registration_mode'), 'open'
		)`).Scan(&regMode)
	if regMode == "closed" {
		WriteError(w, http.StatusForbidden, "registration_closed", "Registration is currently closed on this instance")
		return
	}

	var req auth.RegisterRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	// In invite_only mode, require a valid registration token.
	var token string
	if regMode == "invite_only" {
		token = r.URL.Query().Get("token")
		if token == "" {
			// Also check request body for token field.
			token = req.Token
		}
		if token == "" {
			WriteError(w, http.StatusForbidden, "token_required", "A registration token is required to create an account on this instance")
			return
		}
		// Validate the token. Return the same generic error for all failure cases
		// (nonexistent, exhausted, expired) to prevent token enumeration.
		var uses, maxUses int
		var expiresAt *time.Time
		err := s.DB.Pool.QueryRow(r.Context(),
			`SELECT uses, max_uses, expires_at FROM registration_tokens WHERE id = $1`, token).Scan(&uses, &maxUses, &expiresAt)
		tokenValid := err == nil
		if tokenValid && uses >= maxUses {
			tokenValid = false
		}
		if tokenValid && expiresAt != nil && expiresAt.Before(time.Now()) {
			tokenValid = false
		}
		if !tokenValid {
			WriteError(w, http.StatusForbidden, "invalid_token", "Invalid or expired registration token")
			return
		}
	}

	ip := clientIP(r)

	user, session, err := s.AuthService.Register(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		InternalError(w, s.Logger, "Registration failed", err)
		return
	}

	// Increment token usage only after successful registration.
	if token != "" {
		s.DB.Pool.Exec(r.Context(),
			`UPDATE registration_tokens SET uses = uses + 1 WHERE id = $1`, token)
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user":  user.ToSelf(),
		"token": session.ID,
	})
}

// handleLogin handles POST /api/v1/auth/login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	ip := clientIP(r)

	user, session, err := s.AuthService.Login(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		InternalError(w, s.Logger, "Login failed", err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user":  user.ToSelf(),
		"token": session.ID,
	})
}

// handleLogout handles POST /api/v1/auth/logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := auth.SessionIDFromContext(r.Context())
	if sessionID == "" {
		WriteError(w, http.StatusUnauthorized, "missing_session", "No session to logout")
		return
	}

	if err := s.AuthService.Logout(r.Context(), sessionID); err != nil {
		InternalError(w, s.Logger, "Logout failed", err)
		return
	}

	WriteNoContent(w)
}

// handleChangePassword handles POST /api/v1/auth/password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req auth.ChangePasswordRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		WriteError(w, http.StatusBadRequest, "missing_fields", "Both current_password and new_password are required")
		return
	}

	if err := s.AuthService.ChangePassword(r.Context(), userID, req); err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		InternalError(w, s.Logger, "Failed to change password", err)
		return
	}

	WriteNoContent(w)
}

// handleChangeEmail handles POST /api/v1/auth/email.
func (s *Server) handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req auth.ChangeEmailRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Password == "" || req.NewEmail == "" {
		WriteError(w, http.StatusBadRequest, "missing_fields", "Both password and new_email are required")
		return
	}

	if err := s.AuthService.ChangeEmail(r.Context(), userID, req); err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		InternalError(w, s.Logger, "Failed to change email", err)
		return
	}

	WriteNoContent(w)
}

// handleHealthCheck responds with the health status of the server and its dependencies.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{
		"status":        "ok",
		"version":       s.Version,
		"build_version": s.BuildVersion,
	}

	if err := s.DB.HealthCheck(r.Context()); err != nil {
		status["status"] = "degraded"
		status["database"] = "unhealthy"
	} else {
		status["database"] = "healthy"
	}

	if s.EventBus != nil {
		if err := s.EventBus.HealthCheck(); err != nil {
			status["status"] = "degraded"
			status["nats"] = "unhealthy"
		} else {
			status["nats"] = "healthy"
		}
	}

	if s.Cache != nil {
		if err := s.Cache.HealthCheck(r.Context()); err != nil {
			status["status"] = "degraded"
			status["cache"] = "unhealthy"
		} else {
			status["cache"] = "healthy"
		}
	}

	httpStatus := http.StatusOK
	if status["status"] != "ok" {
		httpStatus = http.StatusServiceUnavailable
	}

	WriteJSON(w, httpStatus, status)
}

// stubHandler returns a handler that responds with 501 Not Implemented for
// endpoints that will be implemented in later phases.
func stubHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotImplemented, "not_implemented",
			fmt.Sprintf("Endpoint %q is not yet implemented", name))
	}
}

// ErrorResponse is the standard error envelope returned by the API.
// Aliased from apiutil so callers that reference api.ErrorResponse continue to compile.
type ErrorResponse = apiutil.ErrorResponse

// ErrorBody contains the error code and human-readable message.
type ErrorBody = apiutil.ErrorBody

// SuccessResponse is the standard success envelope returned by the API.
type SuccessResponse = apiutil.SuccessResponse

// WriteJSON writes a JSON response with the given status code and data wrapped
// in the standard success envelope {"data": ...}.
// Delegates to apiutil.WriteJSON.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	apiutil.WriteJSON(w, status, data)
}

// WriteJSONRaw writes a JSON response with the given status code without wrapping
// in the success envelope. Useful for responses that define their own structure.
// Delegates to apiutil.WriteJSONRaw.
func WriteJSONRaw(w http.ResponseWriter, status int, data interface{}) {
	apiutil.WriteJSONRaw(w, status, data)
}

// WriteError writes a JSON error response with the given status code, error code,
// and message using the standard error envelope {"error": {"code": ..., "message": ...}}.
// Delegates to apiutil.WriteError.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	apiutil.WriteError(w, status, code, message)
}

// WriteNoContent writes a 204 No Content response with no body.
// Delegates to apiutil.WriteNoContent.
func WriteNoContent(w http.ResponseWriter) {
	apiutil.WriteNoContent(w)
}

// DecodeJSON reads JSON from the request body into dst. On failure it writes a
// 400 error response and returns false. Delegates to apiutil.DecodeJSON.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	return apiutil.DecodeJSON(w, r, dst)
}

// InternalError logs the error and writes a 500 response.
// Delegates to apiutil.InternalError.
func InternalError(w http.ResponseWriter, logger *slog.Logger, msg string, err error) {
	apiutil.InternalError(w, logger, msg, err)
}

// slogMiddleware returns a chi middleware that logs HTTP requests using slog.
func slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote", r.RemoteAddr),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			}
			if uid := auth.UserIDFromContext(r.Context()); uid != "" {
				attrs = append(attrs, slog.String("user_id", uid))
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request", attrs...)
		})
	}
}

// maxBodySize limits the request body to the given number of bytes.
// Skips multipart/form-data requests (file uploads set their own limit).
func maxBodySize(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ct := r.Header.Get("Content-Type")
			if r.Body != nil && !strings.HasPrefix(ct, "multipart/form-data") {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware returns a chi middleware that sets CORS headers for the given
// allowed origins.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := false
			for _, o := range origins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
				// Only set Allow-Credentials when using explicit origins, not wildcard.
				isWildcard := len(origins) == 1 && origins[0] == "*"
				if !isWildcard {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
