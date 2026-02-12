// Package admin implements REST API handlers for instance administration
// including viewing and updating instance settings, managing federation peers,
// and retrieving server statistics. Mounted under /api/v1/admin.
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements admin-related REST API endpoints.
type Handler struct {
	Pool       *pgxpool.Pool
	InstanceID string
	Logger     *slog.Logger
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
		sql  string
		dest *int64
	}{
		{`SELECT COUNT(*) FROM users WHERE instance_id = $1`, &s.Users},
		{`SELECT COUNT(*) FROM guilds`, &s.Guilds},
		{`SELECT COUNT(*) FROM channels`, &s.Channels},
		{`SELECT COUNT(*) FROM messages`, &s.Messages},
		{`SELECT COUNT(*) FROM messages WHERE created_at >= CURRENT_DATE`, &s.MessagesToday},
		{`SELECT COUNT(*) FROM files`, &s.Files},
		{`SELECT COUNT(*) FROM roles`, &s.Roles},
		{`SELECT COUNT(*) FROM guild_emoji`, &s.Emoji},
		{`SELECT COUNT(*) FROM invites WHERE expires_at IS NULL OR expires_at > now()`, &s.Invites},
		{`SELECT COUNT(*) FROM federation_peers WHERE instance_id = $1`, &s.FedPeers},
		{`SELECT COUNT(*) FROM user_sessions WHERE expires_at > now()`, &s.OnlineUsers},
	}

	for _, q := range queries {
		if err := h.Pool.QueryRow(r.Context(), q.sql, h.InstanceID).Scan(q.dest); err != nil {
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
			        status_presence, bio, bot_owner_id, email, flags, created_at
			 FROM users
			 WHERE username ILIKE '%' || $1 || '%' OR display_name ILIKE '%' || $1 || '%'
			 ORDER BY created_at DESC
			 LIMIT $2 OFFSET $3`, q, limit, offset)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, instance_id, username, display_name, avatar_id, status_text,
			        status_presence, bio, bot_owner_id, email, flags, created_at
			 FROM users
			 ORDER BY created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list users")
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusPresence, &u.Bio, &u.BotOwnerID,
			&u.Email, &u.Flags, &u.CreatedAt,
		); err != nil {
			continue
		}
		users = append(users, u)
	}
	if users == nil {
		users = []models.User{}
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

func parseInt(s string) (int, error) {
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, json.Unmarshal(nil, nil)
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}
