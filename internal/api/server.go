// Package api implements the AmityVox REST API server using the chi router.
// It registers all route groups under /api/v1/, provides middleware for logging,
// recovery, CORS, and request IDs, and exposes JSON response helpers for
// consistent API envelope formatting.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/amityvox/amityvox/internal/api/admin"
	"github.com/amityvox/amityvox/internal/api/channels"
	"github.com/amityvox/amityvox/internal/api/guilds"
	"github.com/amityvox/amityvox/internal/api/invites"
	"github.com/amityvox/amityvox/internal/api/users"
	"github.com/amityvox/amityvox/internal/api/webhooks"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/media"
	"github.com/amityvox/amityvox/internal/presence"
	"github.com/amityvox/amityvox/internal/search"
)

// Server is the HTTP API server for AmityVox. It holds the chi router, database
// reference, services, configuration, and logger.
type Server struct {
	Router      *chi.Mux
	DB          *database.DB
	Config      *config.Config
	AuthService *auth.Service
	EventBus    *events.Bus
	Cache       *presence.Cache
	Media       *media.Service
	Search      *search.Service
	InstanceID  string
	Version     string
	Logger      *slog.Logger
	server      *http.Server
}

// NewServer creates a new API server with all routes and middleware registered.
func NewServer(db *database.DB, cfg *config.Config, authSvc *auth.Service, bus *events.Bus, cache *presence.Cache, mediaSvc *media.Service, searchSvc *search.Service, instanceID string, logger *slog.Logger) *Server {
	s := &Server{
		Router:      chi.NewRouter(),
		DB:          db,
		Config:      cfg,
		AuthService: authSvc,
		EventBus:    bus,
		Cache:       cache,
		Media:       mediaSvc,
		Search:      searchSvc,
		InstanceID:  instanceID,
		Logger:      logger,
	}

	s.registerMiddleware()
	s.registerRoutes()

	return s
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
	s.Router.Use(s.rateLimitMiddleware())
}

// registerRoutes mounts all API route groups on the router.
func (s *Server) registerRoutes() {
	// Create domain handlers.
	userH := &users.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	guildH := &guilds.Handler{
		Pool:       s.DB.Pool,
		EventBus:   s.EventBus,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
	}
	channelH := &channels.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}
	inviteH := &invites.Handler{
		Pool:       s.DB.Pool,
		EventBus:   s.EventBus,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
	}
	adminH := &admin.Handler{
		Pool:       s.DB.Pool,
		InstanceID: s.InstanceID,
		Logger:     s.Logger,
	}
	webhookH := &webhooks.Handler{
		Pool:     s.DB.Pool,
		EventBus: s.EventBus,
		Logger:   s.Logger,
	}

	// Health check — outside versioned API prefix.
	s.Router.Get("/health", s.handleHealthCheck)

	// API v1 routes.
	s.Router.Route("/api/v1", func(r chi.Router) {
		// Auth routes — public, no Bearer token required.
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
			r.With(auth.RequireAuth(s.AuthService)).Post("/logout", s.handleLogout)
			r.With(auth.RequireAuth(s.AuthService)).Post("/password", s.handleChangePassword)
			r.With(auth.RequireAuth(s.AuthService)).Post("/totp/enable", s.handleTOTPEnable)
			r.With(auth.RequireAuth(s.AuthService)).Post("/totp/verify", s.handleTOTPVerify)
			r.With(auth.RequireAuth(s.AuthService)).Delete("/totp", s.handleTOTPDisable)
			r.With(auth.RequireAuth(s.AuthService)).Post("/webauthn/register/begin", s.handleWebAuthnRegisterBegin)
			r.With(auth.RequireAuth(s.AuthService)).Post("/webauthn/register/finish", s.handleWebAuthnRegisterFinish)
			r.With(auth.RequireAuth(s.AuthService)).Post("/webauthn/login/begin", s.handleWebAuthnLoginBegin)
			r.With(auth.RequireAuth(s.AuthService)).Post("/webauthn/login/finish", s.handleWebAuthnLoginFinish)
		})

		// Authenticated routes — require Bearer token.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.AuthService))

			// User routes.
			r.Route("/users", func(r chi.Router) {
				r.Get("/@me", userH.HandleGetSelf)
				r.Patch("/@me", userH.HandleUpdateSelf)
				r.Delete("/@me", userH.HandleDeleteSelf)
				r.Get("/@me/guilds", userH.HandleGetSelfGuilds)
				r.Get("/@me/dms", userH.HandleGetSelfDMs)
				r.Get("/@me/read-state", userH.HandleGetSelfReadState)
				r.Get("/@me/sessions", userH.HandleGetSelfSessions)
				r.Delete("/@me/sessions/{sessionID}", userH.HandleDeleteSelfSession)
				r.Get("/{userID}", userH.HandleGetUser)
				r.Post("/{userID}/dm", userH.HandleCreateDM)
				r.Put("/{userID}/friend", userH.HandleAddFriend)
				r.Delete("/{userID}/friend", userH.HandleRemoveFriend)
				r.Put("/{userID}/block", userH.HandleBlockUser)
				r.Delete("/{userID}/block", userH.HandleUnblockUser)
			})

			// Guild routes.
			r.Route("/guilds", func(r chi.Router) {
				r.Post("/", guildH.HandleCreateGuild)
				r.Get("/discover", guildH.HandleDiscoverGuilds)
				r.Get("/{guildID}", guildH.HandleGetGuild)
				r.Patch("/{guildID}", guildH.HandleUpdateGuild)
				r.Delete("/{guildID}", guildH.HandleDeleteGuild)
				r.Post("/{guildID}/leave", guildH.HandleLeaveGuild)
				r.Post("/{guildID}/transfer", guildH.HandleTransferGuildOwnership)
				r.Get("/{guildID}/channels", guildH.HandleGetGuildChannels)
				r.Patch("/{guildID}/channels", guildH.HandleReorderGuildChannels)
				r.Post("/{guildID}/channels", guildH.HandleCreateGuildChannel)
				r.Get("/{guildID}/members", guildH.HandleGetGuildMembers)
				r.Get("/{guildID}/members/search", guildH.HandleSearchGuildMembers)
				r.Get("/{guildID}/members/{memberID}", guildH.HandleGetGuildMember)
				r.Patch("/{guildID}/members/{memberID}", guildH.HandleUpdateGuildMember)
				r.Delete("/{guildID}/members/{memberID}", guildH.HandleRemoveGuildMember)
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
				r.Get("/{guildID}/audit-log", guildH.HandleGetGuildAuditLog)
				r.Get("/{guildID}/emoji", guildH.HandleGetGuildEmoji)
				r.Post("/{guildID}/emoji", guildH.HandleCreateGuildEmoji)
				r.Get("/{guildID}/webhooks", guildH.HandleGetGuildWebhooks)
				r.Post("/{guildID}/webhooks", guildH.HandleCreateGuildWebhook)
				r.Patch("/{guildID}/webhooks/{webhookID}", guildH.HandleUpdateGuildWebhook)
				r.Delete("/{guildID}/webhooks/{webhookID}", guildH.HandleDeleteGuildWebhook)
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
				r.Get("/{channelID}/messages/{messageID}/reactions", channelH.HandleGetReactions)
				r.Put("/{channelID}/messages/{messageID}/reactions/{emoji}", channelH.HandleAddReaction)
				r.Delete("/{channelID}/messages/{messageID}/reactions/{emoji}", channelH.HandleRemoveReaction)
				r.Get("/{channelID}/pins", channelH.HandleGetPins)
				r.Put("/{channelID}/pins/{messageID}", channelH.HandlePinMessage)
				r.Delete("/{channelID}/pins/{messageID}", channelH.HandleUnpinMessage)
				r.Post("/{channelID}/typing", channelH.HandleTriggerTyping)
				r.Post("/{channelID}/ack", channelH.HandleAckChannel)
				r.Put("/{channelID}/permissions/{overrideID}", channelH.HandleSetChannelPermission)
				r.Delete("/{channelID}/permissions/{overrideID}", channelH.HandleDeleteChannelPermission)
				r.Get("/{channelID}/webhooks", channelH.HandleGetChannelWebhooks)
			})

			// Invite routes.
			r.Route("/invites", func(r chi.Router) {
				r.Get("/{code}", inviteH.HandleGetInvite)
				r.Post("/{code}", inviteH.HandleAcceptInvite)
				r.Delete("/{code}", inviteH.HandleDeleteInvite)
			})

			// File upload.
			if s.Media != nil {
				r.Post("/files/upload", s.Media.HandleUpload)
			} else {
				r.Post("/files/upload", stubHandler("upload_file"))
			}

			// Search routes (with search-specific rate limit).
			r.With(s.RateLimitSearch).Route("/search", func(r chi.Router) {
				r.Get("/messages", s.handleSearchMessages)
				r.Get("/users", s.handleSearchUsers)
				r.Get("/guilds", s.handleSearchGuilds)
			})

			// Admin routes.
			r.Route("/admin", func(r chi.Router) {
				r.Get("/instance", adminH.HandleGetInstance)
				r.Patch("/instance", adminH.HandleUpdateInstance)
				r.Get("/federation/peers", adminH.HandleGetFederationPeers)
				r.Post("/federation/peers", adminH.HandleAddFederationPeer)
				r.Delete("/federation/peers/{peerID}", adminH.HandleRemoveFederationPeer)
				r.Get("/stats", adminH.HandleGetStats)
			})
		})

		// Webhook execution — uses token auth, no Bearer token needed.
		r.Post("/webhooks/{webhookID}/{token}", webhookH.HandleExecute)
	})
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

// handleRegister handles POST /api/v1/auth/register.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req auth.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}

	user, session, err := s.AuthService.Register(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		s.Logger.Error("registration failed", slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Registration failed")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user":  user,
		"token": session.ID,
	})
}

// handleLogin handles POST /api/v1/auth/login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}

	user, session, err := s.AuthService.Login(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		s.Logger.Error("login failed", slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Login failed")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user":  user,
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
		s.Logger.Error("logout failed", slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Logout failed")
		return
	}

	WriteNoContent(w)
}

// handleChangePassword handles POST /api/v1/auth/password.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req auth.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
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
		s.Logger.Error("password change failed", slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to change password")
		return
	}

	WriteNoContent(w)
}

// handleHealthCheck responds with the health status of the server and its dependencies.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{"status": "ok", "version": s.Version}

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
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error code and human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SuccessResponse is the standard success envelope returned by the API.
type SuccessResponse struct {
	Data interface{} `json:"data"`
}

// WriteJSON writes a JSON response with the given status code and data wrapped
// in the standard success envelope {"data": ...}.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(SuccessResponse{Data: data})
}

// WriteJSONRaw writes a JSON response with the given status code without wrapping
// in the success envelope. Useful for responses that define their own structure.
func WriteJSONRaw(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a JSON error response with the given status code, error code,
// and message using the standard error envelope {"error": {"code": ..., "message": ...}}.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
		},
	})
}

// WriteNoContent writes a 204 No Content response with no body.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// slogMiddleware returns a chi middleware that logs HTTP requests using slog.
func slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote", r.RemoteAddr),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}

// maxBodySize limits the request body to the given number of bytes.
func maxBodySize(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
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
				w.Header().Set("Access-Control-Allow-Credentials", "true")
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
