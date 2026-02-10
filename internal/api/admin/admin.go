// Package admin implements REST API handlers for instance administration
// including viewing and updating instance settings, managing federation peers,
// and retrieving server statistics. Mounted under /api/v1/admin.
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
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

// isAdmin checks whether the requesting user has the is_admin flag.
func (h *Handler) isAdmin(r *http.Request) bool {
	userID := auth.UserIDFromContext(r.Context())
	var isAdmin bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	if err != nil {
		return false
	}
	return isAdmin
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
		Users       int64  `json:"users"`
		Guilds      int64  `json:"guilds"`
		Channels    int64  `json:"channels"`
		Messages    int64  `json:"messages"`
		Files       int64  `json:"files"`
		FedPeers    int64  `json:"federation_peers"`
		DBSizeMB    string `json:"database_size_mb"`
		GoVersion   string `json:"go_version"`
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
		{`SELECT COUNT(*) FROM files`, &s.Files},
		{`SELECT COUNT(*) FROM federation_peers WHERE instance_id = $1`, &s.FedPeers},
	}

	for _, q := range queries {
		if err := h.Pool.QueryRow(r.Context(), q.sql, h.InstanceID).Scan(q.dest); err != nil {
			// Table might not exist yet; default to 0.
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
	s.DBSizeMB = dbSize
	s.GoVersion = "go1.23"

	writeJSON(w, http.StatusOK, s)
}
