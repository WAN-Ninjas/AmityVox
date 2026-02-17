// Package admin implements REST API handlers for instance administration
// including viewing and updating instance settings, managing federation peers,
// and retrieving server statistics. Mounted under /api/v1/admin.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// MediaDeleter can remove S3 objects. Implemented by media.Service.
type MediaDeleter interface {
	DeleteObject(ctx context.Context, bucket, key string) error
}

// Handler implements admin-related REST API endpoints.
type Handler struct {
	Pool       *pgxpool.Pool
	InstanceID string
	Logger     *slog.Logger
	Media      MediaDeleter // optional — enables S3 cleanup on admin media delete
	EventBus   *events.Bus  // optional — enables real-time announcement events
}

type updateInstanceRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	FederationMode *string `json:"federation_mode"`
}

type addPeerRequest struct {
	Domain string `json:"domain"`
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}

// isAdmin checks whether the requesting user has the admin flag set in the
// users.flags bitfield.
func (h *Handler) isAdmin(r *http.Request) bool {
	userID := auth.UserIDFromContext(r.Context())
	var flags int
	err := h.Pool.QueryRow(r.Context(),
		`SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	if err != nil {
		return false
	}
	return flags&models.UserFlagAdmin != 0
}

// HandleGetInstance handles GET /api/v1/admin/instance.
func (h *Handler) HandleGetInstance(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var inst models.Instance
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, domain, public_key, name, description, software, software_version,
		        federation_mode, created_at, last_seen_at
		 FROM instances WHERE id = $1`, h.InstanceID).Scan(
		&inst.ID, &inst.Domain, &inst.PublicKey, &inst.Name, &inst.Description,
		&inst.Software, &inst.SoftwareVersion, &inst.FederationMode,
		&inst.CreatedAt, &inst.LastSeenAt,
	)
	if err != nil {
		h.Logger.Error("failed to get instance", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get instance")
		return
	}

	writeJSON(w, http.StatusOK, inst)
}

// HandleUpdateInstance handles PATCH /api/v1/admin/instance.
func (h *Handler) HandleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req updateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.FederationMode != nil {
		valid := map[string]bool{"open": true, "allowlist": true, "closed": true}
		if !valid[*req.FederationMode] {
			writeError(w, http.StatusBadRequest, "invalid_federation_mode",
				"Federation mode must be one of: open, allowlist, closed")
			return
		}
	}

	var inst models.Instance
	err := h.Pool.QueryRow(r.Context(),
		`UPDATE instances
		 SET name = COALESCE($1, name),
		     description = COALESCE($2, description),
		     federation_mode = COALESCE($3, federation_mode)
		 WHERE id = $4
		 RETURNING id, domain, public_key, name, description, software, software_version,
		           federation_mode, created_at, last_seen_at`,
		req.Name, req.Description, req.FederationMode, h.InstanceID,
	).Scan(
		&inst.ID, &inst.Domain, &inst.PublicKey, &inst.Name, &inst.Description,
		&inst.Software, &inst.SoftwareVersion, &inst.FederationMode,
		&inst.CreatedAt, &inst.LastSeenAt,
	)
	if err != nil {
		h.Logger.Error("failed to update instance", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update instance")
		return
	}

	writeJSON(w, http.StatusOK, inst)
}

// HandleGetFederationPeers handles GET /api/v1/admin/federation/peers.
func (h *Handler) HandleGetFederationPeers(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fp.instance_id, fp.peer_id, fp.status, fp.established_at, fp.last_synced_at,
		        i.domain, i.name, i.software
		 FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 WHERE fp.instance_id = $1
		 ORDER BY fp.established_at DESC`, h.InstanceID)
	if err != nil {
		h.Logger.Error("failed to query federation peers", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get federation peers")
		return
	}
	defer rows.Close()

	type peerWithInfo struct {
		models.FederationPeer
		PeerDomain   string  `json:"peer_domain"`
		PeerName     *string `json:"peer_name,omitempty"`
		PeerSoftware string  `json:"peer_software"`
	}

	peers := []peerWithInfo{}
	for rows.Next() {
		var p peerWithInfo
		if err := rows.Scan(
			&p.InstanceID, &p.PeerID, &p.Status, &p.EstablishedAt, &p.LastSyncedAt,
			&p.PeerDomain, &p.PeerName, &p.PeerSoftware,
		); err != nil {
			h.Logger.Error("failed to scan federation peer", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read federation peers")
			return
		}
		peers = append(peers, p)
	}

	writeJSON(w, http.StatusOK, peers)
}

// HandleAddFederationPeer handles POST /api/v1/admin/federation/peers.
func (h *Handler) HandleAddFederationPeer(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req addPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "missing_domain", "Peer domain is required")
		return
	}

	// Look up the peer instance by domain.
	var peerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id FROM instances WHERE domain = $1`, req.Domain).Scan(&peerID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "peer_not_found",
			"No known instance with that domain. The instance must be discovered first.")
		return
	}
	if err != nil {
		h.Logger.Error("failed to look up peer", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up peer")
		return
	}

	now := time.Now().UTC()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO federation_peers (instance_id, peer_id, status, established_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (instance_id, peer_id) DO UPDATE SET status = $3`,
		h.InstanceID, peerID, models.FederationPeerActive, now)
	if err != nil {
		h.Logger.Error("failed to add federation peer", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add federation peer")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"instance_id":    h.InstanceID,
		"peer_id":        peerID,
		"peer_domain":    req.Domain,
		"status":         models.FederationPeerActive,
		"established_at": now,
	})
}

// HandleRemoveFederationPeer handles DELETE /api/v1/admin/federation/peers/{peerID}.
func (h *Handler) HandleRemoveFederationPeer(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	peerID := chi.URLParam(r, "peerID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM federation_peers WHERE instance_id = $1 AND peer_id = $2`,
		h.InstanceID, peerID)
	if err != nil {
		h.Logger.Error("failed to remove federation peer", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove federation peer")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "peer_not_found", "Federation peer not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetStats handles GET /api/v1/admin/stats.
func (h *Handler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	type stats struct {
		Users        int64  `json:"users"`
		OnlineUsers  int64  `json:"online_users"`
		Guilds       int64  `json:"guilds"`
		Channels     int64  `json:"channels"`
		Messages     int64  `json:"messages"`
		MessagesToday int64 `json:"messages_today"`
		Files        int64  `json:"files"`
		Roles        int64  `json:"roles"`
		Emoji        int64  `json:"emoji"`
		Invites      int64  `json:"invites"`
		FedPeers     int64  `json:"federation_peers"`
		DBSize       string `json:"database_size"`
		GoVersion    string `json:"go_version"`
		NumGoroutine int    `json:"goroutines"`
		MemAllocMB   uint64 `json:"mem_alloc_mb"`
		MemSysMB     uint64 `json:"mem_sys_mb"`
		NumCPU       int    `json:"num_cpu"`
		Uptime       string `json:"uptime"`
	}

	var s stats

	queries := []struct {
		sql      string
		dest     *int64
		hasParam bool
	}{
		{`SELECT COUNT(*) FROM users WHERE instance_id = $1`, &s.Users, true},
		{`SELECT COUNT(*) FROM guilds`, &s.Guilds, false},
		{`SELECT COUNT(*) FROM channels`, &s.Channels, false},
		{`SELECT COUNT(*) FROM messages`, &s.Messages, false},
		{`SELECT COUNT(*) FROM messages WHERE created_at >= CURRENT_DATE`, &s.MessagesToday, false},
		{`SELECT COUNT(*) FROM files`, &s.Files, false},
		{`SELECT COUNT(*) FROM roles`, &s.Roles, false},
		{`SELECT COUNT(*) FROM guild_emoji`, &s.Emoji, false},
		{`SELECT COUNT(*) FROM invites WHERE expires_at IS NULL OR expires_at > now()`, &s.Invites, false},
		{`SELECT COUNT(*) FROM federation_peers WHERE instance_id = $1`, &s.FedPeers, true},
		{`SELECT COUNT(*) FROM user_sessions WHERE expires_at > now()`, &s.OnlineUsers, false},
	}

	for _, q := range queries {
		var err error
		if q.hasParam {
			err = h.Pool.QueryRow(r.Context(), q.sql, h.InstanceID).Scan(q.dest)
		} else {
			err = h.Pool.QueryRow(r.Context(), q.sql).Scan(q.dest)
		}
		if err != nil {
			h.Logger.Warn("stats query failed",
				slog.String("sql", q.sql),
				slog.String("error", err.Error()),
			)
			*q.dest = 0
		}
	}

	// Database size.
	var dbSize string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&dbSize)
	if err != nil {
		dbSize = "unknown"
	}
	s.DBSize = dbSize

	// Runtime stats.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	s.GoVersion = runtime.Version()
	s.NumGoroutine = runtime.NumGoroutine()
	s.MemAllocMB = memStats.Alloc / 1024 / 1024
	s.MemSysMB = memStats.Sys / 1024 / 1024
	s.NumCPU = runtime.NumCPU()

	// Server uptime from instance created_at.
	var createdAt time.Time
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT created_at FROM instances WHERE id = $1`, h.InstanceID).Scan(&createdAt); err == nil {
		s.Uptime = time.Since(createdAt).Truncate(time.Second).String()
	}

	writeJSON(w, http.StatusOK, s)
}

// HandleListUsers handles GET /api/v1/admin/users.
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	q := r.URL.Query().Get("q")
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := parseInt(o); err == nil && v >= 0 {
			offset = v
		}
	}

	var rows pgx.Rows
	var err error
	if q != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, instance_id, username, display_name, avatar_id, status_text,
			        status_emoji, status_presence, status_expires_at, bio,
			        banner_id, accent_color, pronouns,
			        bot_owner_id, email, flags, created_at
			 FROM users
			 WHERE username ILIKE '%' || $1 || '%' OR display_name ILIKE '%' || $1 || '%'
			 ORDER BY created_at DESC
			 LIMIT $2 OFFSET $3`, q, limit, offset)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, instance_id, username, display_name, avatar_id, status_text,
			        status_emoji, status_presence, status_expires_at, bio,
			        banner_id, accent_color, pronouns,
			        bot_owner_id, email, flags, created_at
			 FROM users
			 ORDER BY created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list users")
		return
	}
	defer rows.Close()

	// Use an admin-specific wrapper to re-expose the email field
	// (models.User has json:"-" on Email to protect it in public endpoints).
	type adminUser struct {
		*models.User
		Email *string `json:"email,omitempty"`
	}

	var users []adminUser
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns,
			&u.BotOwnerID, &u.Email, &u.Flags, &u.CreatedAt,
		); err != nil {
			continue
		}
		users = append(users, adminUser{User: &u, Email: u.Email})
	}
	if users == nil {
		users = []adminUser{}
	}

	writeJSON(w, http.StatusOK, users)
}

// HandleSuspendUser handles POST /api/v1/admin/users/{userID}/suspend.
func (h *Handler) HandleSuspendUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	userID := chi.URLParam(r, "userID")

	_, err := h.Pool.Exec(r.Context(),
		`UPDATE users SET flags = flags | $1 WHERE id = $2`, models.UserFlagSuspended, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to suspend user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

// HandleUnsuspendUser handles POST /api/v1/admin/users/{userID}/unsuspend.
func (h *Handler) HandleUnsuspendUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	userID := chi.URLParam(r, "userID")

	_, err := h.Pool.Exec(r.Context(),
		`UPDATE users SET flags = flags & ~$1 WHERE id = $2`, models.UserFlagSuspended, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unsuspend user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsuspended"})
}

// HandleSetAdmin handles POST /api/v1/admin/users/{userID}/set-admin.
func (h *Handler) HandleSetAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	userID := chi.URLParam(r, "userID")

	var req struct {
		Admin bool `json:"admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var err error
	if req.Admin {
		_, err = h.Pool.Exec(r.Context(),
			`UPDATE users SET flags = flags | $1 WHERE id = $2`, models.UserFlagAdmin, userID)
	} else {
		_, err = h.Pool.Exec(r.Context(),
			`UPDATE users SET flags = flags & ~$1 WHERE id = $2`, models.UserFlagAdmin, userID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update admin status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"admin": req.Admin})
}

// HandleSetGlobalMod handles POST /api/v1/admin/users/{userID}/set-globalmod.
func (h *Handler) HandleSetGlobalMod(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	userID := chi.URLParam(r, "userID")

	var req struct {
		GlobalMod bool `json:"global_mod"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var tag pgconn.CommandTag
	var err error
	if req.GlobalMod {
		tag, err = h.Pool.Exec(r.Context(),
			`UPDATE users SET flags = flags | $1 WHERE id = $2`, models.UserFlagGlobalMod, userID)
	} else {
		tag, err = h.Pool.Exec(r.Context(),
			`UPDATE users SET flags = flags & ~$1 WHERE id = $2`, models.UserFlagGlobalMod, userID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update global mod status")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"global_mod": req.GlobalMod})
}

// HandleInstanceBanUser bans a user at the instance level (suspends + records reason).
// POST /api/v1/admin/users/{userID}/instance-ban
func (h *Handler) HandleInstanceBanUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	targetID := chi.URLParam(r, "userID")

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Suspend the user.
	_, err := h.Pool.Exec(r.Context(),
		`UPDATE users SET flags = flags | $1 WHERE id = $2`, models.UserFlagSuspended, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to ban user")
		return
	}

	// Record the ban in instance_bans table.
	adminID := auth.UserIDFromContext(r.Context())
	h.Pool.Exec(r.Context(),
		`INSERT INTO instance_bans (user_id, admin_id, reason, created_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (user_id) DO UPDATE SET reason = $3, admin_id = $2, created_at = now()`,
		targetID, adminID, req.Reason)

	// Invalidate all sessions for the banned user.
	h.Pool.Exec(r.Context(), `DELETE FROM user_sessions WHERE user_id = $1`, targetID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "banned"})
}

// HandleInstanceUnbanUser unbans a user at the instance level.
// POST /api/v1/admin/users/{userID}/instance-unban
func (h *Handler) HandleInstanceUnbanUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}
	targetID := chi.URLParam(r, "userID")

	_, err := h.Pool.Exec(r.Context(),
		`UPDATE users SET flags = flags & ~$1 WHERE id = $2`, models.UserFlagSuspended, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unban user")
		return
	}

	h.Pool.Exec(r.Context(), `DELETE FROM instance_bans WHERE user_id = $1`, targetID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "unbanned"})
}

// HandleGetInstanceBans lists all instance-level banned users.
// GET /api/v1/admin/instance-bans
func (h *Handler) HandleGetInstanceBans(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ib.user_id, ib.admin_id, ib.reason, ib.created_at,
		        u.username, u.display_name, u.avatar_id
		 FROM instance_bans ib
		 JOIN users u ON u.id = ib.user_id
		 ORDER BY ib.created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bans")
		return
	}
	defer rows.Close()

	type instanceBan struct {
		UserID      string  `json:"user_id"`
		AdminID     string  `json:"admin_id"`
		Reason      *string `json:"reason"`
		CreatedAt   string  `json:"created_at"`
		Username    string  `json:"username"`
		DisplayName *string `json:"display_name"`
		AvatarID    *string `json:"avatar_id"`
	}

	bans := make([]instanceBan, 0)
	for rows.Next() {
		var b instanceBan
		var createdAt time.Time
		if err := rows.Scan(&b.UserID, &b.AdminID, &b.Reason, &createdAt,
			&b.Username, &b.DisplayName, &b.AvatarID); err != nil {
			continue
		}
		b.CreatedAt = createdAt.Format(time.RFC3339)
		bans = append(bans, b)
	}

	writeJSON(w, http.StatusOK, bans)
}

// HandleGetRegistrationConfig returns the instance registration settings.
// GET /api/v1/admin/registration
func (h *Handler) HandleGetRegistrationConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var mode, message string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'registration_mode'), 'open'
		)`).Scan(&mode)
	if err != nil {
		mode = "open"
	}
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(
			(SELECT value FROM instance_settings WHERE key = 'registration_message'), ''
		)`).Scan(&message)

	writeJSON(w, http.StatusOK, map[string]string{
		"mode":    mode,
		"message": message,
	})
}

// HandleUpdateRegistrationConfig updates registration settings.
// PATCH /api/v1/admin/registration
func (h *Handler) HandleUpdateRegistrationConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Mode    *string `json:"mode"`
		Message *string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Mode != nil {
		switch *req.Mode {
		case "open", "invite_only", "closed":
			h.Pool.Exec(r.Context(),
				`INSERT INTO instance_settings (key, value) VALUES ('registration_mode', $1)
				 ON CONFLICT (key) DO UPDATE SET value = $1`, *req.Mode)
		default:
			writeError(w, http.StatusBadRequest, "invalid_mode", "Mode must be 'open', 'invite_only', or 'closed'")
			return
		}
	}
	if req.Message != nil {
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value) VALUES ('registration_message', $1)
			 ON CONFLICT (key) DO UPDATE SET value = $1`, *req.Message)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// --- Registration Token Handlers ---

// HandleCreateRegistrationToken creates a new registration token for invite-only mode.
// POST /api/v1/admin/registration/tokens
func (h *Handler) HandleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		MaxUses   int     `json:"max_uses"`
		Note      *string `json:"note"`
		ExpiresIn *int    `json:"expires_in_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	adminID := auth.UserIDFromContext(r.Context())
	tokenID := models.NewULID().String()

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Hour)
		expiresAt = &t
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO registration_tokens (id, created_by, max_uses, note, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tokenID, adminID, req.MaxUses, req.Note, expiresAt)
	if err != nil {
		h.Logger.Error("failed to create registration token", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         tokenID,
		"max_uses":   req.MaxUses,
		"uses":       0,
		"note":       req.Note,
		"expires_at": expiresAt,
		"created_by": adminID,
	})
}

// HandleListRegistrationTokens lists all registration tokens.
// GET /api/v1/admin/registration/tokens
func (h *Handler) HandleListRegistrationTokens(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT rt.id, rt.created_by, rt.max_uses, rt.uses, rt.note, rt.expires_at, rt.created_at,
		        u.username
		 FROM registration_tokens rt
		 JOIN users u ON u.id = rt.created_by
		 ORDER BY rt.created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tokens")
		return
	}
	defer rows.Close()

	type tokenEntry struct {
		ID            string     `json:"id"`
		CreatedBy     string     `json:"created_by"`
		CreatorName   string     `json:"creator_name"`
		MaxUses       int        `json:"max_uses"`
		Uses          int        `json:"uses"`
		Note          *string    `json:"note"`
		ExpiresAt     *time.Time `json:"expires_at"`
		CreatedAt     time.Time  `json:"created_at"`
		Expired       bool       `json:"expired"`
		Exhausted     bool       `json:"exhausted"`
	}

	tokens := make([]tokenEntry, 0)
	for rows.Next() {
		var t tokenEntry
		if err := rows.Scan(&t.ID, &t.CreatedBy, &t.MaxUses, &t.Uses, &t.Note,
			&t.ExpiresAt, &t.CreatedAt, &t.CreatorName); err != nil {
			continue
		}
		t.Expired = t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())
		t.Exhausted = t.Uses >= t.MaxUses
		tokens = append(tokens, t)
	}

	writeJSON(w, http.StatusOK, tokens)
}

// HandleDeleteRegistrationToken deletes a registration token.
// DELETE /api/v1/admin/registration/tokens/{tokenID}
func (h *Handler) HandleDeleteRegistrationToken(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	tokenID := chi.URLParam(r, "tokenID")
	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM registration_tokens WHERE id = $1`, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete token")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Instance Announcement Handlers ---

// HandleCreateAnnouncement creates a new instance-wide announcement.
// POST /api/v1/admin/announcements
func (h *Handler) HandleCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Title     string `json:"title"`
		Content   string `json:"content"`
		Severity  string `json:"severity"`
		ExpiresIn *int   `json:"expires_in_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Title == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and content are required")
		return
	}

	switch req.Severity {
	case "info", "warning", "critical":
		// valid
	case "":
		req.Severity = "info"
	default:
		writeError(w, http.StatusBadRequest, "invalid_severity", "Severity must be info, warning, or critical")
		return
	}

	adminID := auth.UserIDFromContext(r.Context())
	announcementID := models.NewULID().String()

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Hour)
		expiresAt = &t
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO instance_announcements (id, admin_id, title, content, severity, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		announcementID, adminID, req.Title, req.Content, req.Severity, expiresAt)
	if err != nil {
		h.Logger.Error("failed to create announcement", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create announcement")
		return
	}

	announcement := map[string]interface{}{
		"id":         announcementID,
		"title":      req.Title,
		"content":    req.Content,
		"severity":   req.Severity,
		"active":     true,
		"expires_at": expiresAt,
	}

	// Publish real-time event so all connected clients see the announcement immediately.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), events.SubjectAnnouncementCreate, "ANNOUNCEMENT_CREATE", announcement)
	}

	writeJSON(w, http.StatusCreated, announcement)
}

// HandleGetAnnouncements returns active instance announcements.
// GET /api/v1/announcements (public for logged-in users)
func (h *Handler) HandleGetAnnouncements(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT ia.id, ia.admin_id, ia.title, ia.content, ia.severity, ia.active,
		        ia.created_at, ia.expires_at, u.username
		 FROM instance_announcements ia
		 JOIN users u ON u.id = ia.admin_id
		 WHERE ia.active = true AND (ia.expires_at IS NULL OR ia.expires_at > now())
		 ORDER BY ia.created_at DESC
		 LIMIT 10`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get announcements")
		return
	}
	defer rows.Close()

	type announcement struct {
		ID        string     `json:"id"`
		AdminID   string     `json:"admin_id"`
		AdminName string     `json:"admin_name"`
		Title     string     `json:"title"`
		Content   string     `json:"content"`
		Severity  string     `json:"severity"`
		Active    bool       `json:"active"`
		CreatedAt time.Time  `json:"created_at"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	announcements := make([]announcement, 0)
	for rows.Next() {
		var a announcement
		if err := rows.Scan(&a.ID, &a.AdminID, &a.Title, &a.Content, &a.Severity,
			&a.Active, &a.CreatedAt, &a.ExpiresAt, &a.AdminName); err != nil {
			continue
		}
		announcements = append(announcements, a)
	}

	writeJSON(w, http.StatusOK, announcements)
}

// HandleListAllAnnouncements returns all announcements (admin only, including inactive).
// GET /api/v1/admin/announcements
func (h *Handler) HandleListAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT ia.id, ia.admin_id, ia.title, ia.content, ia.severity, ia.active,
		        ia.created_at, ia.expires_at, u.username
		 FROM instance_announcements ia
		 JOIN users u ON u.id = ia.admin_id
		 ORDER BY ia.created_at DESC
		 LIMIT 50`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get announcements")
		return
	}
	defer rows.Close()

	type announcement struct {
		ID        string     `json:"id"`
		AdminID   string     `json:"admin_id"`
		AdminName string     `json:"admin_name"`
		Title     string     `json:"title"`
		Content   string     `json:"content"`
		Severity  string     `json:"severity"`
		Active    bool       `json:"active"`
		CreatedAt time.Time  `json:"created_at"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	announcements := make([]announcement, 0)
	for rows.Next() {
		var a announcement
		if err := rows.Scan(&a.ID, &a.AdminID, &a.Title, &a.Content, &a.Severity,
			&a.Active, &a.CreatedAt, &a.ExpiresAt, &a.AdminName); err != nil {
			continue
		}
		announcements = append(announcements, a)
	}

	writeJSON(w, http.StatusOK, announcements)
}

// HandleUpdateAnnouncement deactivates an announcement or updates it.
// PATCH /api/v1/admin/announcements/{announcementID}
func (h *Handler) HandleUpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	announcementID := chi.URLParam(r, "announcementID")

	var req struct {
		Active  *bool   `json:"active"`
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE instance_announcements
		 SET active = COALESCE($1, active),
		     title = COALESCE($2, title),
		     content = COALESCE($3, content)
		 WHERE id = $4`,
		req.Active, req.Title, req.Content, announcementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update announcement")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Announcement not found")
		return
	}

	// Publish real-time update event.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), events.SubjectAnnouncementUpdate, "ANNOUNCEMENT_UPDATE", map[string]interface{}{
			"id":      announcementID,
			"active":  req.Active,
			"title":   req.Title,
			"content": req.Content,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDeleteAnnouncement removes an announcement permanently.
// DELETE /api/v1/admin/announcements/{announcementID}
func (h *Handler) HandleDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	announcementID := chi.URLParam(r, "announcementID")
	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM instance_announcements WHERE id = $1`, announcementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete announcement")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Announcement not found")
		return
	}

	// Publish real-time delete event.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), events.SubjectAnnouncementDelete, "ANNOUNCEMENT_DELETE", map[string]string{
			"id": announcementID,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Guild Admin Handlers
// =============================================================================

// HandleListGuilds returns all guilds with owner info and stats.
// GET /api/v1/admin/guilds
func (h *Handler) HandleListGuilds(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	q := r.URL.Query().Get("q")
	limit := 50
	offset := 0
	sort := r.URL.Query().Get("sort") // name, members, created_at
	if sort == "" {
		sort = "created_at"
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := parseInt(o); err == nil && v >= 0 {
			offset = v
		}
	}

	// Validate sort column.
	orderBy := "g.created_at DESC"
	switch sort {
	case "name":
		orderBy = "g.name ASC"
	case "members":
		orderBy = "member_count DESC"
	case "created_at":
		orderBy = "g.created_at DESC"
	}

	type guildRow struct {
		models.Guild
		OwnerName    string `json:"owner_name"`
		ChannelCount int    `json:"channel_count"`
		RoleCount    int    `json:"role_count"`
	}

	var baseQuery string
	if q != "" {
		baseQuery = fmt.Sprintf(`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description,
		        g.icon_id, g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
		        g.preferred_locale, g.max_members, g.vanity_url, g.verification_level, g.tags,
		        g.created_at,
		        COALESCE(u.username, 'unknown') AS owner_name,
		        (SELECT COUNT(*) FROM guild_members gm WHERE gm.guild_id = g.id) AS member_count,
		        (SELECT COUNT(*) FROM channels c WHERE c.guild_id = g.id) AS channel_count,
		        (SELECT COUNT(*) FROM roles r WHERE r.guild_id = g.id) AS role_count
		 FROM guilds g
		 LEFT JOIN users u ON u.id = g.owner_id
		 WHERE g.name ILIKE '%%' || $1 || '%%' OR COALESCE(g.description, '') ILIKE '%%' || $1 || '%%'
		 ORDER BY %s
		 LIMIT $2 OFFSET $3`, orderBy)
	} else {
		baseQuery = fmt.Sprintf(`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description,
		        g.icon_id, g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
		        g.preferred_locale, g.max_members, g.vanity_url, g.verification_level, g.tags,
		        g.created_at,
		        COALESCE(u.username, 'unknown') AS owner_name,
		        (SELECT COUNT(*) FROM guild_members gm WHERE gm.guild_id = g.id) AS member_count,
		        (SELECT COUNT(*) FROM channels c WHERE c.guild_id = g.id) AS channel_count,
		        (SELECT COUNT(*) FROM roles r WHERE r.guild_id = g.id) AS role_count
		 FROM guilds g
		 LEFT JOIN users u ON u.id = g.owner_id
		 ORDER BY %s
		 LIMIT $1 OFFSET $2`, orderBy)
	}

	var rows pgx.Rows
	var err error
	if q != "" {
		rows, err = h.Pool.Query(r.Context(), baseQuery, q, limit, offset)
	} else {
		rows, err = h.Pool.Query(r.Context(), baseQuery, limit, offset)
	}
	if err != nil {
		h.Logger.Error("failed to list guilds", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list guilds")
		return
	}
	defer rows.Close()

	guilds := make([]guildRow, 0)
	for rows.Next() {
		var g guildRow
		if err := rows.Scan(
			&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description,
			&g.IconID, &g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
			&g.PreferredLocale, &g.MaxMembers, &g.VanityURL, &g.VerificationLevel, &g.Tags,
			&g.CreatedAt,
			&g.OwnerName, &g.MemberCount, &g.ChannelCount, &g.RoleCount,
		); err != nil {
			continue
		}
		guilds = append(guilds, g)
	}

	writeJSON(w, http.StatusOK, guilds)
}

// HandleGetGuildDetails returns detailed info for a single guild (admin view).
// GET /api/v1/admin/guilds/{guildID}
func (h *Handler) HandleGetGuildDetails(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	guildID := chi.URLParam(r, "guildID")

	type guildDetail struct {
		models.Guild
		OwnerName      string `json:"owner_name"`
		ChannelCount   int    `json:"channel_count"`
		RoleCount      int    `json:"role_count"`
		EmojiCount     int    `json:"emoji_count"`
		InviteCount    int    `json:"invite_count"`
		MessageCount   int64  `json:"message_count"`
		MessagesToday  int64  `json:"messages_today"`
		BanCount       int    `json:"ban_count"`
	}

	var g guildDetail
	err := h.Pool.QueryRow(r.Context(),
		`SELECT g.id, g.instance_id, g.owner_id, g.name, g.description,
		        g.icon_id, g.banner_id, g.default_permissions, g.flags, g.nsfw, g.discoverable,
		        g.system_channel_join, g.system_channel_leave, g.system_channel_kick, g.system_channel_ban,
		        g.preferred_locale, g.max_members, g.vanity_url, g.verification_level,
		        g.afk_channel_id, g.afk_timeout, g.tags, g.created_at,
		        COALESCE(u.username, 'unknown') AS owner_name,
		        (SELECT COUNT(*) FROM guild_members gm WHERE gm.guild_id = g.id) AS member_count,
		        (SELECT COUNT(*) FROM channels c WHERE c.guild_id = g.id) AS channel_count,
		        (SELECT COUNT(*) FROM roles r WHERE r.guild_id = g.id) AS role_count,
		        (SELECT COUNT(*) FROM guild_emoji ge WHERE ge.guild_id = g.id) AS emoji_count,
		        (SELECT COUNT(*) FROM invites i WHERE i.guild_id = g.id AND (i.expires_at IS NULL OR i.expires_at > now())) AS invite_count,
		        (SELECT COUNT(*) FROM messages m JOIN channels c2 ON c2.id = m.channel_id WHERE c2.guild_id = g.id) AS message_count,
		        (SELECT COUNT(*) FROM messages m2 JOIN channels c3 ON c3.id = m2.channel_id WHERE c3.guild_id = g.id AND m2.created_at >= CURRENT_DATE) AS messages_today,
		        (SELECT COUNT(*) FROM guild_bans gb WHERE gb.guild_id = g.id) AS ban_count
		 FROM guilds g
		 LEFT JOIN users u ON u.id = g.owner_id
		 WHERE g.id = $1`, guildID,
	).Scan(
		&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description,
		&g.IconID, &g.BannerID, &g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
		&g.SystemChannelJoin, &g.SystemChannelLeave, &g.SystemChannelKick, &g.SystemChannelBan,
		&g.PreferredLocale, &g.MaxMembers, &g.VanityURL, &g.VerificationLevel,
		&g.AFKChannelID, &g.AFKTimeout, &g.Tags, &g.CreatedAt,
		&g.OwnerName, &g.MemberCount, &g.ChannelCount, &g.RoleCount,
		&g.EmojiCount, &g.InviteCount, &g.MessageCount, &g.MessagesToday, &g.BanCount,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Guild not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get guild details", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get guild details")
		return
	}

	writeJSON(w, http.StatusOK, g)
}

// HandleAdminDeleteGuild forcefully deletes a guild (admin action).
// DELETE /api/v1/admin/guilds/{guildID}
func (h *Handler) HandleAdminDeleteGuild(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	guildID := chi.URLParam(r, "guildID")

	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM guilds WHERE id = $1`, guildID)
	if err != nil {
		h.Logger.Error("failed to delete guild", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete guild")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Guild not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleGetUserGuilds returns all guilds a specific user is a member of (admin view).
// GET /api/v1/admin/users/{userID}/guilds
func (h *Handler) HandleGetUserGuilds(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	userID := chi.URLParam(r, "userID")

	type userGuild struct {
		GuildID     string    `json:"guild_id"`
		GuildName   string    `json:"guild_name"`
		IconID      *string   `json:"icon_id,omitempty"`
		OwnerID     string    `json:"owner_id"`
		IsOwner     bool      `json:"is_owner"`
		MemberCount int       `json:"member_count"`
		JoinedAt    time.Time `json:"joined_at"`
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT g.id, g.name, g.icon_id, g.owner_id, g.owner_id = $1 AS is_owner,
		        (SELECT COUNT(*) FROM guild_members gm2 WHERE gm2.guild_id = g.id) AS member_count,
		        gm.joined_at
		 FROM guild_members gm
		 JOIN guilds g ON g.id = gm.guild_id
		 WHERE gm.user_id = $1
		 ORDER BY gm.joined_at DESC`, userID)
	if err != nil {
		h.Logger.Error("failed to get user guilds", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get user guilds")
		return
	}
	defer rows.Close()

	guilds := make([]userGuild, 0)
	for rows.Next() {
		var g userGuild
		if err := rows.Scan(&g.GuildID, &g.GuildName, &g.IconID, &g.OwnerID,
			&g.IsOwner, &g.MemberCount, &g.JoinedAt); err != nil {
			continue
		}
		guilds = append(guilds, g)
	}

	writeJSON(w, http.StatusOK, guilds)
}

func parseInt(s string) (int, error) {
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid character in number")
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

// =============================================================================
// Rate Limit Handlers
// =============================================================================

// HandleGetRateLimitStats returns aggregated rate limit statistics.
// GET /api/v1/admin/rate-limits/stats
func (h *Handler) HandleGetRateLimitStats(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	type ipStat struct {
		IPAddress     string `json:"ip_address"`
		TotalRequests int64  `json:"total_requests"`
		BlockCount    int64  `json:"block_count"`
		LastSeen      string `json:"last_seen"`
	}

	// Top IPs by request count in the last 24 hours.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT ip_address,
		        SUM(requests_count) AS total_requests,
		        COUNT(*) FILTER (WHERE blocked = true) AS block_count,
		        MAX(created_at) AS last_seen
		 FROM rate_limit_log
		 WHERE created_at >= now() - INTERVAL '24 hours'
		 GROUP BY ip_address
		 ORDER BY total_requests DESC
		 LIMIT 50`)
	if err != nil {
		h.Logger.Error("failed to query rate limit stats", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get rate limit stats")
		return
	}
	defer rows.Close()

	topIPs := make([]ipStat, 0)
	for rows.Next() {
		var s ipStat
		var lastSeen time.Time
		if err := rows.Scan(&s.IPAddress, &s.TotalRequests, &s.BlockCount, &lastSeen); err != nil {
			continue
		}
		s.LastSeen = lastSeen.Format(time.RFC3339)
		topIPs = append(topIPs, s)
	}

	// Summary counts.
	var totalEntries, blockedEntries, uniqueIPs int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE blocked = true), COUNT(DISTINCT ip_address)
		 FROM rate_limit_log
		 WHERE created_at >= now() - INTERVAL '24 hours'`).Scan(&totalEntries, &blockedEntries, &uniqueIPs)

	// Current rate limit config.
	var reqsPerWindow, windowSecs string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'rate_limit_requests_per_window'), '100')`).Scan(&reqsPerWindow)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'rate_limit_window_seconds'), '60')`).Scan(&windowSecs)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"top_ips":                    topIPs,
		"total_entries_24h":          totalEntries,
		"blocked_entries_24h":        blockedEntries,
		"unique_ips_24h":             uniqueIPs,
		"requests_per_window":        reqsPerWindow,
		"window_seconds":             windowSecs,
	})
}

// HandleGetRateLimitLog returns paginated rate limit log entries.
// GET /api/v1/admin/rate-limits/log
func (h *Handler) HandleGetRateLimitLog(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := parseInt(o); err == nil && v >= 0 {
			offset = v
		}
	}

	blockedOnly := r.URL.Query().Get("blocked") == "true"
	ipFilter := r.URL.Query().Get("ip")

	var rows pgx.Rows
	var err error

	if blockedOnly && ipFilter != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, ip_address, endpoint, requests_count, window_start, blocked, created_at
			 FROM rate_limit_log
			 WHERE blocked = true AND ip_address = $1
			 ORDER BY created_at DESC
			 LIMIT $2 OFFSET $3`, ipFilter, limit, offset)
	} else if blockedOnly {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, ip_address, endpoint, requests_count, window_start, blocked, created_at
			 FROM rate_limit_log
			 WHERE blocked = true
			 ORDER BY created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
	} else if ipFilter != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, ip_address, endpoint, requests_count, window_start, blocked, created_at
			 FROM rate_limit_log
			 WHERE ip_address = $1
			 ORDER BY created_at DESC
			 LIMIT $2 OFFSET $3`, ipFilter, limit, offset)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, ip_address, endpoint, requests_count, window_start, blocked, created_at
			 FROM rate_limit_log
			 ORDER BY created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		h.Logger.Error("failed to query rate limit log", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get rate limit log")
		return
	}
	defer rows.Close()

	type logEntry struct {
		ID            string `json:"id"`
		IPAddress     string `json:"ip_address"`
		Endpoint      string `json:"endpoint"`
		RequestsCount int    `json:"requests_count"`
		WindowStart   string `json:"window_start"`
		Blocked       bool   `json:"blocked"`
		CreatedAt     string `json:"created_at"`
	}

	entries := make([]logEntry, 0)
	for rows.Next() {
		var e logEntry
		var windowStart, createdAt time.Time
		if err := rows.Scan(&e.ID, &e.IPAddress, &e.Endpoint, &e.RequestsCount,
			&windowStart, &e.Blocked, &createdAt); err != nil {
			continue
		}
		e.WindowStart = windowStart.Format(time.RFC3339)
		e.CreatedAt = createdAt.Format(time.RFC3339)
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// HandleUpdateRateLimitConfig updates rate limiting configuration.
// PATCH /api/v1/admin/rate-limits/config
func (h *Handler) HandleUpdateRateLimitConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		RequestsPerWindow *string `json:"requests_per_window"`
		WindowSeconds     *string `json:"window_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.RequestsPerWindow != nil {
		if v, err := parseInt(*req.RequestsPerWindow); err != nil || v < 1 || v > 100000 {
			writeError(w, http.StatusBadRequest, "invalid_value", "requests_per_window must be between 1 and 100000")
			return
		}
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('rate_limit_requests_per_window', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.RequestsPerWindow)
	}

	if req.WindowSeconds != nil {
		if v, err := parseInt(*req.WindowSeconds); err != nil || v < 1 || v > 3600 {
			writeError(w, http.StatusBadRequest, "invalid_value", "window_seconds must be between 1 and 3600")
			return
		}
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('rate_limit_window_seconds', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.WindowSeconds)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// =============================================================================
// Content Scan Handlers
// =============================================================================

// HandleGetContentScanRules returns all content scan rules.
// GET /api/v1/admin/content-scan/rules
func (h *Handler) HandleGetContentScanRules(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, name, pattern, action, target, enabled, created_at
		 FROM content_scan_rules
		 ORDER BY created_at DESC`)
	if err != nil {
		h.Logger.Error("failed to query content scan rules", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get content scan rules")
		return
	}
	defer rows.Close()

	type rule struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Pattern   string    `json:"pattern"`
		Action    string    `json:"action"`
		Target    string    `json:"target"`
		Enabled   bool      `json:"enabled"`
		CreatedAt time.Time `json:"created_at"`
	}

	rules := make([]rule, 0)
	for rows.Next() {
		var r rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Pattern, &r.Action, &r.Target, &r.Enabled, &r.CreatedAt); err != nil {
			continue
		}
		rules = append(rules, r)
	}

	writeJSON(w, http.StatusOK, rules)
}

// HandleCreateContentScanRule creates a new content scan rule.
// POST /api/v1/admin/content-scan/rules
func (h *Handler) HandleCreateContentScanRule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Name    string `json:"name"`
		Pattern string `json:"pattern"`
		Action  string `json:"action"`
		Target  string `json:"target"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Rule name is required")
		return
	}
	if req.Pattern == "" {
		writeError(w, http.StatusBadRequest, "missing_pattern", "Regex pattern is required")
		return
	}

	// Validate the regex pattern.
	if _, err := regexp.Compile(req.Pattern); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pattern", "Invalid regex pattern: "+err.Error())
		return
	}

	switch req.Action {
	case "block", "flag", "log":
		// valid
	case "":
		req.Action = "log"
	default:
		writeError(w, http.StatusBadRequest, "invalid_action", "Action must be 'block', 'flag', or 'log'")
		return
	}

	switch req.Target {
	case "filename", "content_type", "text_content":
		// valid
	case "":
		req.Target = "filename"
	default:
		writeError(w, http.StatusBadRequest, "invalid_target", "Target must be 'filename', 'content_type', or 'text_content'")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	ruleID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO content_scan_rules (id, name, pattern, action, target, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ruleID, req.Name, req.Pattern, req.Action, req.Target, enabled)
	if err != nil {
		h.Logger.Error("failed to create content scan rule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create content scan rule")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         ruleID,
		"name":       req.Name,
		"pattern":    req.Pattern,
		"action":     req.Action,
		"target":     req.Target,
		"enabled":    enabled,
		"created_at": time.Now().UTC(),
	})
}

// HandleUpdateContentScanRule updates an existing content scan rule.
// PATCH /api/v1/admin/content-scan/rules/{ruleID}
func (h *Handler) HandleUpdateContentScanRule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	ruleID := chi.URLParam(r, "ruleID")

	var req struct {
		Name    *string `json:"name"`
		Pattern *string `json:"pattern"`
		Action  *string `json:"action"`
		Target  *string `json:"target"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate pattern if provided.
	if req.Pattern != nil {
		if _, err := regexp.Compile(*req.Pattern); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_pattern", "Invalid regex pattern: "+err.Error())
			return
		}
	}

	// Validate action if provided.
	if req.Action != nil {
		switch *req.Action {
		case "block", "flag", "log":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid_action", "Action must be 'block', 'flag', or 'log'")
			return
		}
	}

	// Validate target if provided.
	if req.Target != nil {
		switch *req.Target {
		case "filename", "content_type", "text_content":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid_target", "Target must be 'filename', 'content_type', or 'text_content'")
			return
		}
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE content_scan_rules
		 SET name = COALESCE($1, name),
		     pattern = COALESCE($2, pattern),
		     action = COALESCE($3, action),
		     target = COALESCE($4, target),
		     enabled = COALESCE($5, enabled)
		 WHERE id = $6`,
		req.Name, req.Pattern, req.Action, req.Target, req.Enabled, ruleID)
	if err != nil {
		h.Logger.Error("failed to update content scan rule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update content scan rule")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Content scan rule not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDeleteContentScanRule deletes a content scan rule.
// DELETE /api/v1/admin/content-scan/rules/{ruleID}
func (h *Handler) HandleDeleteContentScanRule(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	ruleID := chi.URLParam(r, "ruleID")
	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM content_scan_rules WHERE id = $1`, ruleID)
	if err != nil {
		h.Logger.Error("failed to delete content scan rule", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete content scan rule")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Content scan rule not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetContentScanLog returns paginated content scan log entries.
// GET /api/v1/admin/content-scan/log
func (h *Handler) HandleGetContentScanLog(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := parseInt(o); err == nil && v >= 0 {
			offset = v
		}
	}

	ruleFilter := r.URL.Query().Get("rule_id")

	var rows pgx.Rows
	var err error

	if ruleFilter != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT cl.id, cl.rule_id, cl.user_id, cl.channel_id, cl.content_matched,
			        cl.action_taken, cl.created_at, cr.name AS rule_name,
			        u.username
			 FROM content_scan_log cl
			 JOIN content_scan_rules cr ON cr.id = cl.rule_id
			 JOIN users u ON u.id = cl.user_id
			 WHERE cl.rule_id = $1
			 ORDER BY cl.created_at DESC
			 LIMIT $2 OFFSET $3`, ruleFilter, limit, offset)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT cl.id, cl.rule_id, cl.user_id, cl.channel_id, cl.content_matched,
			        cl.action_taken, cl.created_at, cr.name AS rule_name,
			        u.username
			 FROM content_scan_log cl
			 JOIN content_scan_rules cr ON cr.id = cl.rule_id
			 JOIN users u ON u.id = cl.user_id
			 ORDER BY cl.created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		h.Logger.Error("failed to query content scan log", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get content scan log")
		return
	}
	defer rows.Close()

	type logEntry struct {
		ID             string    `json:"id"`
		RuleID         string    `json:"rule_id"`
		RuleName       string    `json:"rule_name"`
		UserID         string    `json:"user_id"`
		Username       string    `json:"username"`
		ChannelID      string    `json:"channel_id"`
		ContentMatched string    `json:"content_matched"`
		ActionTaken    string    `json:"action_taken"`
		CreatedAt      time.Time `json:"created_at"`
	}

	entries := make([]logEntry, 0)
	for rows.Next() {
		var e logEntry
		if err := rows.Scan(&e.ID, &e.RuleID, &e.UserID, &e.ChannelID,
			&e.ContentMatched, &e.ActionTaken, &e.CreatedAt, &e.RuleName, &e.Username); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}

// =============================================================================
// CAPTCHA Handlers
// =============================================================================

// HandleGetCaptchaConfig returns current CAPTCHA configuration.
// GET /api/v1/admin/captcha
func (h *Handler) HandleGetCaptchaConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var provider, siteKey, secretKey string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'captcha_provider'), 'none')`).Scan(&provider)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'captcha_site_key'), '')`).Scan(&siteKey)
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE((SELECT value FROM instance_settings WHERE key = 'captcha_secret_key'), '')`).Scan(&secretKey)

	// Mask the secret key for display (show only last 4 chars).
	maskedSecret := ""
	if len(secretKey) > 4 {
		maskedSecret = "****" + secretKey[len(secretKey)-4:]
	} else if secretKey != "" {
		maskedSecret = "****"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"provider":   provider,
		"site_key":   siteKey,
		"secret_key": maskedSecret,
	})
}

// HandleUpdateCaptchaConfig updates CAPTCHA settings.
// PATCH /api/v1/admin/captcha
func (h *Handler) HandleUpdateCaptchaConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Provider  *string `json:"provider"`
		SiteKey   *string `json:"site_key"`
		SecretKey *string `json:"secret_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Provider != nil {
		switch *req.Provider {
		case "none", "hcaptcha", "recaptcha":
			h.Pool.Exec(r.Context(),
				`INSERT INTO instance_settings (key, value, updated_at) VALUES ('captcha_provider', $1, now())
				 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.Provider)
		default:
			writeError(w, http.StatusBadRequest, "invalid_provider", "Provider must be 'none', 'hcaptcha', or 'recaptcha'")
			return
		}
	}

	if req.SiteKey != nil {
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('captcha_site_key', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.SiteKey)
	}

	if req.SecretKey != nil {
		h.Pool.Exec(r.Context(),
			`INSERT INTO instance_settings (key, value, updated_at) VALUES ('captcha_secret_key', $1, now())
			 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = now()`, *req.SecretKey)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
