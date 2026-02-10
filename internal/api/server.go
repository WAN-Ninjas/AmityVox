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

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/presence"
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
	Logger      *slog.Logger
	server      *http.Server
}

// NewServer creates a new API server with all routes and middleware registered.
func NewServer(db *database.DB, cfg *config.Config, authSvc *auth.Service, bus *events.Bus, cache *presence.Cache, logger *slog.Logger) *Server {
	s := &Server{
		Router:      chi.NewRouter(),
		DB:          db,
		Config:      cfg,
		AuthService: authSvc,
		EventBus:    bus,
		Cache:       cache,
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
}

// registerRoutes mounts all API route groups on the router.
func (s *Server) registerRoutes() {
	// Health check — outside versioned API prefix.
	s.Router.Get("/health", s.handleHealthCheck)

	// API v1 routes.
	s.Router.Route("/api/v1", func(r chi.Router) {
		// Auth routes — public, no Bearer token required.
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.handleRegister)
			r.Post("/login", s.handleLogin)
			r.With(auth.RequireAuth(s.AuthService)).Post("/logout", s.handleLogout)
			r.Post("/totp/enable", stubHandler("totp_enable"))
			r.Post("/totp/verify", stubHandler("totp_verify"))
			r.Post("/webauthn/register", stubHandler("webauthn_register"))
			r.Post("/webauthn/verify", stubHandler("webauthn_verify"))
		})

		// Authenticated routes — require Bearer token.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.AuthService))

			// User routes.
			r.Route("/users", func(r chi.Router) {
				r.Get("/@me", s.handleGetSelf)
				r.Patch("/@me", stubHandler("update_self"))
				r.Get("/@me/guilds", stubHandler("get_self_guilds"))
				r.Get("/@me/dms", stubHandler("get_self_dms"))
				r.Get("/{userID}", stubHandler("get_user"))
				r.Post("/{userID}/dm", stubHandler("create_dm"))
				r.Put("/{userID}/friend", stubHandler("add_friend"))
				r.Delete("/{userID}/friend", stubHandler("remove_friend"))
				r.Put("/{userID}/block", stubHandler("block_user"))
				r.Delete("/{userID}/block", stubHandler("unblock_user"))
			})

			// Guild routes.
			r.Route("/guilds", func(r chi.Router) {
				r.Post("/", stubHandler("create_guild"))
				r.Get("/{guildID}", stubHandler("get_guild"))
				r.Patch("/{guildID}", stubHandler("update_guild"))
				r.Delete("/{guildID}", stubHandler("delete_guild"))
				r.Get("/{guildID}/channels", stubHandler("get_guild_channels"))
				r.Post("/{guildID}/channels", stubHandler("create_guild_channel"))
				r.Get("/{guildID}/members", stubHandler("get_guild_members"))
				r.Get("/{guildID}/members/{memberID}", stubHandler("get_guild_member"))
				r.Patch("/{guildID}/members/{memberID}", stubHandler("update_guild_member"))
				r.Delete("/{guildID}/members/{memberID}", stubHandler("remove_guild_member"))
				r.Get("/{guildID}/bans", stubHandler("get_guild_bans"))
				r.Put("/{guildID}/bans/{userID}", stubHandler("create_guild_ban"))
				r.Delete("/{guildID}/bans/{userID}", stubHandler("remove_guild_ban"))
				r.Get("/{guildID}/roles", stubHandler("get_guild_roles"))
				r.Post("/{guildID}/roles", stubHandler("create_guild_role"))
				r.Patch("/{guildID}/roles/{roleID}", stubHandler("update_guild_role"))
				r.Delete("/{guildID}/roles/{roleID}", stubHandler("delete_guild_role"))
				r.Get("/{guildID}/invites", stubHandler("get_guild_invites"))
				r.Get("/{guildID}/audit-log", stubHandler("get_guild_audit_log"))
				r.Get("/{guildID}/emoji", stubHandler("get_guild_emoji"))
				r.Post("/{guildID}/emoji", stubHandler("create_guild_emoji"))
			})

			// Channel routes.
			r.Route("/channels", func(r chi.Router) {
				r.Get("/{channelID}", stubHandler("get_channel"))
				r.Patch("/{channelID}", stubHandler("update_channel"))
				r.Delete("/{channelID}", stubHandler("delete_channel"))
				r.Get("/{channelID}/messages", stubHandler("get_messages"))
				r.Post("/{channelID}/messages", stubHandler("create_message"))
				r.Get("/{channelID}/messages/{messageID}", stubHandler("get_message"))
				r.Patch("/{channelID}/messages/{messageID}", stubHandler("update_message"))
				r.Delete("/{channelID}/messages/{messageID}", stubHandler("delete_message"))
				r.Put("/{channelID}/messages/{messageID}/reactions/{emoji}", stubHandler("add_reaction"))
				r.Delete("/{channelID}/messages/{messageID}/reactions/{emoji}", stubHandler("remove_reaction"))
				r.Get("/{channelID}/pins", stubHandler("get_pins"))
				r.Put("/{channelID}/pins/{messageID}", stubHandler("pin_message"))
				r.Delete("/{channelID}/pins/{messageID}", stubHandler("unpin_message"))
				r.Post("/{channelID}/typing", stubHandler("trigger_typing"))
				r.Post("/{channelID}/ack", stubHandler("ack_channel"))
				r.Put("/{channelID}/permissions/{overrideID}", stubHandler("set_channel_permission"))
				r.Delete("/{channelID}/permissions/{overrideID}", stubHandler("delete_channel_permission"))
			})

			// Invite routes.
			r.Route("/invites", func(r chi.Router) {
				r.Get("/{code}", stubHandler("get_invite"))
				r.Post("/{code}", stubHandler("accept_invite"))
				r.Delete("/{code}", stubHandler("delete_invite"))
			})

			// File upload.
			r.Post("/files/upload", stubHandler("upload_file"))

			// Admin routes.
			r.Route("/admin", func(r chi.Router) {
				r.Get("/instance", stubHandler("get_instance"))
				r.Patch("/instance", stubHandler("update_instance"))
				r.Get("/federation/peers", stubHandler("get_federation_peers"))
				r.Post("/federation/peers", stubHandler("add_federation_peer"))
				r.Delete("/federation/peers/{peerID}", stubHandler("remove_federation_peer"))
				r.Get("/stats", stubHandler("get_stats"))
			})
		})

		// Webhook execution — uses token auth, no Bearer token needed.
		r.Post("/webhooks/{webhookID}/{token}", stubHandler("execute_webhook"))
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

// handleGetSelf handles GET /api/v1/users/@me.
func (s *Server) handleGetSelf(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	user, err := s.AuthService.GetUser(r.Context(), userID)
	if err != nil {
		if authErr, ok := err.(*auth.AuthError); ok {
			WriteError(w, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		s.Logger.Error("get self failed", slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get user")
		return
	}

	WriteJSON(w, http.StatusOK, user)
}

// handleHealthCheck responds with the health status of the server and its dependencies.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{"status": "ok"}

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
