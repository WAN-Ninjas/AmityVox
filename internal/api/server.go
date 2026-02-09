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

	"github.com/amityvox/amityvox/internal/config"
	"github.com/amityvox/amityvox/internal/database"
)

// Server is the HTTP API server for AmityVox. It holds the chi router, database
// reference, configuration, and logger.
type Server struct {
	Router *chi.Mux
	DB     *database.DB
	Config *config.Config
	Logger *slog.Logger
	server *http.Server
}

// NewServer creates a new API server with all routes and middleware registered.
func NewServer(db *database.DB, cfg *config.Config, logger *slog.Logger) *Server {
	s := &Server{
		Router: chi.NewRouter(),
		DB:     db,
		Config: cfg,
		Logger: logger,
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
		// Auth routes (Phase 2).
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", stubHandler("register"))
			r.Post("/login", stubHandler("login"))
			r.Post("/logout", stubHandler("logout"))
			r.Post("/totp/enable", stubHandler("totp_enable"))
			r.Post("/totp/verify", stubHandler("totp_verify"))
			r.Post("/webauthn/register", stubHandler("webauthn_register"))
			r.Post("/webauthn/verify", stubHandler("webauthn_verify"))
		})

		// User routes (Phase 2).
		r.Route("/users", func(r chi.Router) {
			r.Get("/@me", stubHandler("get_self"))
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

		// Guild routes (Phase 2).
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

		// Channel routes (Phase 2).
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

		// Invite routes (Phase 2).
		r.Route("/invites", func(r chi.Router) {
			r.Get("/{code}", stubHandler("get_invite"))
			r.Post("/{code}", stubHandler("accept_invite"))
			r.Delete("/{code}", stubHandler("delete_invite"))
		})

		// Webhook routes (Phase 2).
		r.Route("/webhooks", func(r chi.Router) {
			r.Post("/{webhookID}/{token}", stubHandler("execute_webhook"))
		})

		// File upload (Phase 2).
		r.Post("/files/upload", stubHandler("upload_file"))

		// Admin routes (Phase 2).
		r.Route("/admin", func(r chi.Router) {
			r.Get("/instance", stubHandler("get_instance"))
			r.Patch("/instance", stubHandler("update_instance"))
			r.Get("/federation/peers", stubHandler("get_federation_peers"))
			r.Post("/federation/peers", stubHandler("add_federation_peer"))
			r.Delete("/federation/peers/{peerID}", stubHandler("remove_federation_peer"))
			r.Get("/stats", stubHandler("get_stats"))
		})
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

// handleHealthCheck responds with the health status of the server and its database.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.HealthCheck(r.Context()); err != nil {
		WriteError(w, http.StatusServiceUnavailable, "database_unhealthy", "Database health check failed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
