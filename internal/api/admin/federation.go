// Federation administration handlers for the admin panel.
// All handlers add methods to the existing admin.Handler struct and use the
// admin package's writeJSON/writeError helpers.
package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// =============================================================================
// Federation Dashboard
// =============================================================================

// HandleGetFederationDashboard returns federation peer health, sync status,
// event lag, and aggregate statistics for the admin dashboard.
// GET /api/v1/admin/federation/dashboard
func (h *Handler) HandleGetFederationDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	type peerHealth struct {
		PeerID         string     `json:"peer_id"`
		PeerDomain     string     `json:"peer_domain"`
		PeerName       *string    `json:"peer_name,omitempty"`
		PeerSoftware   string     `json:"peer_software"`
		PeerStatus     string     `json:"peer_status"`
		FederationStatus string   `json:"federation_status"` // active, blocked, pending
		HealthStatus   string     `json:"health_status"`     // healthy, degraded, unreachable, unknown
		LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
		LastEventAt    *time.Time `json:"last_event_at,omitempty"`
		EventLagMs     int        `json:"event_lag_ms"`
		EventsSent     int64      `json:"events_sent"`
		EventsReceived int64      `json:"events_received"`
		Errors24h      int        `json:"errors_24h"`
		Version        *string    `json:"version,omitempty"`
		Capabilities   json.RawMessage `json:"capabilities"`
		EstablishedAt  time.Time  `json:"established_at"`
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fp.peer_id, i.domain, i.name, i.software, fp.status, fp.established_at,
		        COALESCE(fps.status, 'unknown') AS health_status,
		        fps.last_sync_at, fps.last_event_at,
		        COALESCE(fps.event_lag_ms, 0), COALESCE(fps.events_sent, 0),
		        COALESCE(fps.events_received, 0), COALESCE(fps.errors_24h, 0),
		        fps.version, COALESCE(fps.capabilities, '[]'::jsonb)
		 FROM federation_peers fp
		 JOIN instances i ON i.id = fp.peer_id
		 LEFT JOIN federation_peer_status fps ON fps.peer_id = fp.peer_id
		 WHERE fp.instance_id = $1
		 ORDER BY fp.established_at DESC`, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get federation dashboard", err)
		return
	}
	defer rows.Close()

	peers := make([]peerHealth, 0)
	for rows.Next() {
		var p peerHealth
		if err := rows.Scan(
			&p.PeerID, &p.PeerDomain, &p.PeerName, &p.PeerSoftware,
			&p.FederationStatus, &p.EstablishedAt,
			&p.HealthStatus, &p.LastSyncAt, &p.LastEventAt,
			&p.EventLagMs, &p.EventsSent, &p.EventsReceived,
			&p.Errors24h, &p.Version, &p.Capabilities,
		); err != nil {
			h.Logger.Error("failed to scan federation peer", slog.String("error", err.Error()))
			continue
		}
		p.PeerStatus = p.HealthStatus
		peers = append(peers, p)
	}

	// Aggregate stats.
	var totalPeers, activePeers, blockedPeers, degradedPeers int64
	for _, p := range peers {
		totalPeers++
		switch p.FederationStatus {
		case "active":
			activePeers++
		case "blocked":
			blockedPeers++
		}
		if p.HealthStatus == "degraded" || p.HealthStatus == "unreachable" {
			degradedPeers++
		}
	}

	// Get total delivery stats.
	var pendingDeliveries, failedDeliveries, totalDeliveries int64
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM federation_delivery_receipts
		 WHERE source_instance = $1 AND status = 'pending'`, h.InstanceID).Scan(&pendingDeliveries)
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM federation_delivery_receipts
		 WHERE source_instance = $1 AND status = 'failed'`, h.InstanceID).Scan(&failedDeliveries)
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM federation_delivery_receipts
		 WHERE source_instance = $1`, h.InstanceID).Scan(&totalDeliveries)

	// Get federation mode.
	var fedMode string
	h.Pool.QueryRow(r.Context(),
		`SELECT federation_mode FROM instances WHERE id = $1`, h.InstanceID).Scan(&fedMode)

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"peers":              peers,
		"federation_mode":    fedMode,
		"total_peers":        totalPeers,
		"active_peers":       activePeers,
		"blocked_peers":      blockedPeers,
		"degraded_peers":     degradedPeers,
		"pending_deliveries": pendingDeliveries,
		"failed_deliveries":  failedDeliveries,
		"total_deliveries":   totalDeliveries,
	})
}

// =============================================================================
// Per-Peer Federation Controls
// =============================================================================

// HandleUpdatePeerControl sets the allow/block/mute action for a specific peer.
// PUT /api/v1/admin/federation/peers/{peerID}/control
func (h *Handler) HandleUpdatePeerControl(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	peerID := chi.URLParam(r, "peerID")

	var req struct {
		Action string  `json:"action"` // allow, block, mute
		Reason *string `json:"reason,omitempty"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	switch req.Action {
	case "allow", "block", "mute":
		// valid
	default:
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_action", "Action must be 'allow', 'block', or 'mute'")
		return
	}

	adminID := auth.UserIDFromContext(r.Context())
	controlID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO federation_peer_controls (id, instance_id, peer_id, action, reason, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (instance_id, peer_id) DO UPDATE SET
			action = $4, reason = $5, created_by = $6, updated_at = now()`,
		controlID, h.InstanceID, peerID, req.Action, req.Reason, adminID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update peer control", err)
		return
	}

	// If blocking, also update the federation_peers status.
	if req.Action == "block" {
		h.Pool.Exec(r.Context(),
			`UPDATE federation_peers SET status = 'blocked' WHERE instance_id = $1 AND peer_id = $2`,
			h.InstanceID, peerID)
	} else if req.Action == "allow" {
		h.Pool.Exec(r.Context(),
			`UPDATE federation_peers SET status = 'active' WHERE instance_id = $1 AND peer_id = $2`,
			h.InstanceID, peerID)
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"peer_id": peerID,
		"action":  req.Action,
		"reason":  req.Reason,
		"status":  "updated",
	})
}

// HandleGetPeerControls returns the current allow/block/mute list.
// GET /api/v1/admin/federation/peers/controls
func (h *Handler) HandleGetPeerControls(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fpc.id, fpc.peer_id, fpc.action, fpc.reason, fpc.created_by, fpc.created_at,
		        i.domain, i.name
		 FROM federation_peer_controls fpc
		 JOIN instances i ON i.id = fpc.peer_id
		 WHERE fpc.instance_id = $1
		 ORDER BY fpc.created_at DESC`, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get peer controls", err)
		return
	}
	defer rows.Close()

	type control struct {
		ID         string    `json:"id"`
		PeerID     string    `json:"peer_id"`
		PeerDomain string    `json:"peer_domain"`
		PeerName   *string   `json:"peer_name,omitempty"`
		Action     string    `json:"action"`
		Reason     *string   `json:"reason,omitempty"`
		CreatedBy  string    `json:"created_by"`
		CreatedAt  time.Time `json:"created_at"`
	}

	controls := make([]control, 0)
	for rows.Next() {
		var c control
		if err := rows.Scan(&c.ID, &c.PeerID, &c.Action, &c.Reason, &c.CreatedBy, &c.CreatedAt,
			&c.PeerDomain, &c.PeerName); err != nil {
			continue
		}
		controls = append(controls, c)
	}

	apiutil.WriteJSON(w, http.StatusOK, controls)
}

// =============================================================================
// Federated Delivery Receipts
// =============================================================================

// HandleGetDeliveryReceipts returns paginated delivery receipt logs.
// GET /api/v1/admin/federation/delivery-receipts
func (h *Handler) HandleGetDeliveryReceipts(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
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

	statusFilter := r.URL.Query().Get("status")

	var query string
	var args []interface{}
	if statusFilter != "" {
		query = `SELECT id, message_id, source_instance, target_instance, status,
		                attempts, last_attempt_at, delivered_at, error_message, created_at
		         FROM federation_delivery_receipts
		         WHERE source_instance = $1 AND status = $2
		         ORDER BY created_at DESC
		         LIMIT $3 OFFSET $4`
		args = []interface{}{h.InstanceID, statusFilter, limit, offset}
	} else {
		query = `SELECT id, message_id, source_instance, target_instance, status,
		                attempts, last_attempt_at, delivered_at, error_message, created_at
		         FROM federation_delivery_receipts
		         WHERE source_instance = $1
		         ORDER BY created_at DESC
		         LIMIT $2 OFFSET $3`
		args = []interface{}{h.InstanceID, limit, offset}
	}

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get delivery receipts", err)
		return
	}
	defer rows.Close()

	type receipt struct {
		ID             string     `json:"id"`
		MessageID      string     `json:"message_id"`
		SourceInstance string     `json:"source_instance"`
		TargetInstance string     `json:"target_instance"`
		Status         string     `json:"status"`
		Attempts       int        `json:"attempts"`
		LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
		DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
		ErrorMessage   *string    `json:"error_message,omitempty"`
		CreatedAt      time.Time  `json:"created_at"`
	}

	receipts := make([]receipt, 0)
	for rows.Next() {
		var rc receipt
		if err := rows.Scan(&rc.ID, &rc.MessageID, &rc.SourceInstance, &rc.TargetInstance,
			&rc.Status, &rc.Attempts, &rc.LastAttemptAt, &rc.DeliveredAt,
			&rc.ErrorMessage, &rc.CreatedAt); err != nil {
			continue
		}
		receipts = append(receipts, rc)
	}

	apiutil.WriteJSON(w, http.StatusOK, receipts)
}

// HandleRetryDelivery retries a failed delivery.
// POST /api/v1/admin/federation/delivery-receipts/{receiptID}/retry
func (h *Handler) HandleRetryDelivery(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	receiptID := chi.URLParam(r, "receiptID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE federation_delivery_receipts
		 SET status = 'retrying', attempts = attempts + 1, last_attempt_at = now()
		 WHERE id = $1 AND source_instance = $2 AND status IN ('failed', 'pending')`,
		receiptID, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to retry delivery", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Delivery receipt not found or already delivered")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

// =============================================================================
// Federated Search Config
// =============================================================================

// HandleGetFederatedSearchConfig returns the federated search settings.
// GET /api/v1/admin/federation/search-config
func (h *Handler) HandleGetFederatedSearchConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	type searchConfig struct {
		Enabled       bool            `json:"enabled"`
		IndexOutgoing bool            `json:"index_outgoing"`
		IndexIncoming bool            `json:"index_incoming"`
		AllowedPeers  json.RawMessage `json:"allowed_peers"`
	}

	var cfg searchConfig
	err := h.Pool.QueryRow(r.Context(),
		`SELECT enabled, index_outgoing, index_incoming, allowed_peers
		 FROM federation_search_config WHERE instance_id = $1`, h.InstanceID).Scan(
		&cfg.Enabled, &cfg.IndexOutgoing, &cfg.IndexIncoming, &cfg.AllowedPeers)
	if err != nil {
		// Return defaults if no config exists.
		cfg = searchConfig{
			Enabled:       false,
			IndexOutgoing: false,
			IndexIncoming: false,
			AllowedPeers:  json.RawMessage("[]"),
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, cfg)
}

// HandleUpdateFederatedSearchConfig updates federated search settings.
// PATCH /api/v1/admin/federation/search-config
func (h *Handler) HandleUpdateFederatedSearchConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		Enabled       *bool    `json:"enabled"`
		IndexOutgoing *bool    `json:"index_outgoing"`
		IndexIncoming *bool    `json:"index_incoming"`
		AllowedPeers  []string `json:"allowed_peers,omitempty"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	// Upsert the configuration.
	peersJSON, _ := json.Marshal(req.AllowedPeers)
	if req.AllowedPeers == nil {
		peersJSON = nil
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO federation_search_config (instance_id, enabled, index_outgoing, index_incoming, allowed_peers)
		 VALUES ($1, COALESCE($2, false), COALESCE($3, false), COALESCE($4, false), COALESCE($5, '[]'::jsonb))
		 ON CONFLICT (instance_id) DO UPDATE SET
			enabled = COALESCE($2, federation_search_config.enabled),
			index_outgoing = COALESCE($3, federation_search_config.index_outgoing),
			index_incoming = COALESCE($4, federation_search_config.index_incoming),
			allowed_peers = COALESCE($5, federation_search_config.allowed_peers),
			updated_at = now()`,
		h.InstanceID, req.Enabled, req.IndexOutgoing, req.IndexIncoming, peersJSON)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update search config", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// =============================================================================
// Bridge Configuration
// =============================================================================

// HandleGetBridges returns all configured bridges for the instance.
// GET /api/v1/admin/bridges
func (h *Handler) HandleGetBridges(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT bc.id, bc.bridge_type, bc.enabled, bc.display_name, bc.config,
		        bc.status, bc.last_sync_at, bc.error_message, bc.created_at, bc.updated_at,
		        (SELECT COUNT(*) FROM bridge_channel_mappings bcm WHERE bcm.bridge_id = bc.id) AS channel_count,
		        (SELECT COUNT(*) FROM bridge_virtual_users bvu WHERE bvu.bridge_id = bc.id) AS virtual_user_count
		 FROM bridge_configs bc
		 WHERE bc.instance_id = $1
		 ORDER BY bc.created_at DESC`, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get bridges", err)
		return
	}
	defer rows.Close()

	type bridge struct {
		ID               string          `json:"id"`
		BridgeType       string          `json:"bridge_type"`
		Enabled          bool            `json:"enabled"`
		DisplayName      string          `json:"display_name"`
		Config           json.RawMessage `json:"config"`
		Status           string          `json:"status"`
		LastSyncAt       *time.Time      `json:"last_sync_at,omitempty"`
		ErrorMessage     *string         `json:"error_message,omitempty"`
		ChannelCount     int             `json:"channel_count"`
		VirtualUserCount int             `json:"virtual_user_count"`
		CreatedAt        time.Time       `json:"created_at"`
		UpdatedAt        time.Time       `json:"updated_at"`
	}

	bridges := make([]bridge, 0)
	for rows.Next() {
		var b bridge
		if err := rows.Scan(&b.ID, &b.BridgeType, &b.Enabled, &b.DisplayName, &b.Config,
			&b.Status, &b.LastSyncAt, &b.ErrorMessage, &b.CreatedAt, &b.UpdatedAt,
			&b.ChannelCount, &b.VirtualUserCount); err != nil {
			h.Logger.Error("failed to scan bridge", slog.String("error", err.Error()))
			continue
		}
		bridges = append(bridges, b)
	}

	apiutil.WriteJSON(w, http.StatusOK, bridges)
}

// HandleCreateBridge creates a new bridge configuration.
// POST /api/v1/admin/bridges
func (h *Handler) HandleCreateBridge(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		BridgeType  string          `json:"bridge_type"`
		DisplayName string          `json:"display_name"`
		Config      json.RawMessage `json:"config"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	validTypes := map[string]bool{
		"matrix": true, "discord": true, "telegram": true,
		"slack": true, "irc": true, "xmpp": true,
	}
	if !validTypes[req.BridgeType] {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_bridge_type",
			"Bridge type must be one of: matrix, discord, telegram, slack, irc, xmpp")
		return
	}

	if req.DisplayName == "" {
		req.DisplayName = req.BridgeType
	}
	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	adminID := auth.UserIDFromContext(r.Context())
	bridgeID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO bridge_configs (id, instance_id, bridge_type, display_name, config, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		bridgeID, h.InstanceID, req.BridgeType, req.DisplayName, req.Config, adminID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create bridge", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           bridgeID,
		"bridge_type":  req.BridgeType,
		"display_name": req.DisplayName,
		"status":       "disconnected",
	})
}

// HandleUpdateBridge updates a bridge configuration.
// PATCH /api/v1/admin/bridges/{bridgeID}
func (h *Handler) HandleUpdateBridge(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	bridgeID := chi.URLParam(r, "bridgeID")

	var req struct {
		Enabled     *bool           `json:"enabled"`
		DisplayName *string         `json:"display_name"`
		Config      json.RawMessage `json:"config,omitempty"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE bridge_configs
		 SET enabled = COALESCE($1, enabled),
		     display_name = COALESCE($2, display_name),
		     config = COALESCE($3, config),
		     updated_at = now()
		 WHERE id = $4 AND instance_id = $5`,
		req.Enabled, req.DisplayName, req.Config, bridgeID, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to update bridge", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Bridge not found")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleDeleteBridge removes a bridge configuration and all its mappings.
// DELETE /api/v1/admin/bridges/{bridgeID}
func (h *Handler) HandleDeleteBridge(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	bridgeID := chi.URLParam(r, "bridgeID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM bridge_configs WHERE id = $1 AND instance_id = $2`, bridgeID, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete bridge", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Bridge not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetBridgeChannelMappings returns channel mappings for a bridge.
// GET /api/v1/admin/bridges/{bridgeID}/mappings
func (h *Handler) HandleGetBridgeChannelMappings(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	bridgeID := chi.URLParam(r, "bridgeID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT bcm.id, bcm.local_channel_id, bcm.remote_channel_id,
		        bcm.remote_channel_name, bcm.direction, bcm.active,
		        bcm.last_message_at, bcm.message_count, bcm.created_at,
		        c.name AS local_channel_name
		 FROM bridge_channel_mappings bcm
		 LEFT JOIN channels c ON c.id = bcm.local_channel_id
		 WHERE bcm.bridge_id = $1
		 ORDER BY bcm.created_at DESC`, bridgeID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get bridge mappings", err)
		return
	}
	defer rows.Close()

	type mapping struct {
		ID                string     `json:"id"`
		LocalChannelID    string     `json:"local_channel_id"`
		LocalChannelName  *string    `json:"local_channel_name,omitempty"`
		RemoteChannelID   string     `json:"remote_channel_id"`
		RemoteChannelName *string    `json:"remote_channel_name,omitempty"`
		Direction         string     `json:"direction"`
		Active            bool       `json:"active"`
		LastMessageAt     *time.Time `json:"last_message_at,omitempty"`
		MessageCount      int64      `json:"message_count"`
		CreatedAt         time.Time  `json:"created_at"`
	}

	mappings := make([]mapping, 0)
	for rows.Next() {
		var m mapping
		if err := rows.Scan(&m.ID, &m.LocalChannelID, &m.RemoteChannelID,
			&m.RemoteChannelName, &m.Direction, &m.Active,
			&m.LastMessageAt, &m.MessageCount, &m.CreatedAt, &m.LocalChannelName); err != nil {
			continue
		}
		mappings = append(mappings, m)
	}

	apiutil.WriteJSON(w, http.StatusOK, mappings)
}

// HandleCreateBridgeChannelMapping creates a new channel mapping for a bridge.
// POST /api/v1/admin/bridges/{bridgeID}/mappings
func (h *Handler) HandleCreateBridgeChannelMapping(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	bridgeID := chi.URLParam(r, "bridgeID")

	var req struct {
		LocalChannelID    string  `json:"local_channel_id"`
		RemoteChannelID   string  `json:"remote_channel_id"`
		RemoteChannelName *string `json:"remote_channel_name,omitempty"`
		Direction         string  `json:"direction"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.LocalChannelID == "" || req.RemoteChannelID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "missing_fields", "local_channel_id and remote_channel_id are required")
		return
	}

	switch req.Direction {
	case "bidirectional", "inbound", "outbound":
		// valid
	case "":
		req.Direction = "bidirectional"
	default:
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_direction", "Direction must be 'bidirectional', 'inbound', or 'outbound'")
		return
	}

	mappingID := models.NewULID().String()

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO bridge_channel_mappings (id, bridge_id, local_channel_id, remote_channel_id, remote_channel_name, direction)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		mappingID, bridgeID, req.LocalChannelID, req.RemoteChannelID, req.RemoteChannelName, req.Direction)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create channel mapping", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                mappingID,
		"local_channel_id":  req.LocalChannelID,
		"remote_channel_id": req.RemoteChannelID,
		"direction":         req.Direction,
	})
}

// HandleDeleteBridgeChannelMapping removes a channel mapping.
// DELETE /api/v1/admin/bridges/{bridgeID}/mappings/{mappingID}
func (h *Handler) HandleDeleteBridgeChannelMapping(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	mappingID := chi.URLParam(r, "mappingID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM bridge_channel_mappings WHERE id = $1`, mappingID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete channel mapping", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Channel mapping not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetBridgeVirtualUsers returns virtual users for a bridge.
// GET /api/v1/admin/bridges/{bridgeID}/virtual-users
func (h *Handler) HandleGetBridgeVirtualUsers(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	bridgeID := chi.URLParam(r, "bridgeID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, remote_user_id, remote_username, remote_avatar, platform, last_active_at, created_at
		 FROM bridge_virtual_users
		 WHERE bridge_id = $1
		 ORDER BY last_active_at DESC NULLS LAST
		 LIMIT 100`, bridgeID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get virtual users", err)
		return
	}
	defer rows.Close()

	type virtualUser struct {
		ID             string     `json:"id"`
		RemoteUserID   string     `json:"remote_user_id"`
		RemoteUsername string     `json:"remote_username"`
		RemoteAvatar   *string    `json:"remote_avatar,omitempty"`
		Platform       string     `json:"platform"`
		LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
		CreatedAt      time.Time  `json:"created_at"`
	}

	users := make([]virtualUser, 0)
	for rows.Next() {
		var u virtualUser
		if err := rows.Scan(&u.ID, &u.RemoteUserID, &u.RemoteUsername, &u.RemoteAvatar,
			&u.Platform, &u.LastActiveAt, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	apiutil.WriteJSON(w, http.StatusOK, users)
}

// =============================================================================
// Instance Connection Profiles (Multi-Instance)
// =============================================================================

// HandleGetInstanceProfiles returns saved instance connections for the user.
// GET /api/v1/users/@me/instance-profiles
func (h *Handler) HandleGetInstanceProfiles(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, instance_url, instance_name, instance_icon, is_primary, last_connected, created_at
		 FROM instance_connection_profiles
		 WHERE user_id = $1
		 ORDER BY is_primary DESC, last_connected DESC NULLS LAST`, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get instance profiles", err)
		return
	}
	defer rows.Close()

	type profile struct {
		ID            string     `json:"id"`
		InstanceURL   string     `json:"instance_url"`
		InstanceName  *string    `json:"instance_name,omitempty"`
		InstanceIcon  *string    `json:"instance_icon,omitempty"`
		IsPrimary     bool       `json:"is_primary"`
		LastConnected *time.Time `json:"last_connected,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
	}

	profiles := make([]profile, 0)
	for rows.Next() {
		var p profile
		if err := rows.Scan(&p.ID, &p.InstanceURL, &p.InstanceName, &p.InstanceIcon,
			&p.IsPrimary, &p.LastConnected, &p.CreatedAt); err != nil {
			continue
		}
		profiles = append(profiles, p)
	}

	apiutil.WriteJSON(w, http.StatusOK, profiles)
}

// HandleAddInstanceProfile adds a new instance connection profile.
// POST /api/v1/users/@me/instance-profiles
func (h *Handler) HandleAddInstanceProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		InstanceURL  string  `json:"instance_url"`
		InstanceName *string `json:"instance_name,omitempty"`
		InstanceIcon *string `json:"instance_icon,omitempty"`
		SessionToken *string `json:"session_token,omitempty"`
		IsPrimary    bool    `json:"is_primary"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "Instance URL", req.InstanceURL) {
		return
	}

	profileID := models.NewULID().String()

	// If setting as primary, unmark existing primary.
	if req.IsPrimary {
		h.Pool.Exec(r.Context(),
			`UPDATE instance_connection_profiles SET is_primary = false WHERE user_id = $1`, userID)
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO instance_connection_profiles (id, user_id, instance_url, instance_name, instance_icon, session_token, is_primary)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id, instance_url) DO UPDATE SET
			instance_name = COALESCE($4, instance_connection_profiles.instance_name),
			instance_icon = COALESCE($5, instance_connection_profiles.instance_icon),
			session_token = COALESCE($6, instance_connection_profiles.session_token),
			is_primary = $7`,
		profileID, userID, req.InstanceURL, req.InstanceName, req.InstanceIcon,
		req.SessionToken, req.IsPrimary)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to add instance profile", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            profileID,
		"instance_url":  req.InstanceURL,
		"instance_name": req.InstanceName,
		"is_primary":    req.IsPrimary,
	})
}

// HandleRemoveInstanceProfile removes an instance connection profile.
// DELETE /api/v1/users/@me/instance-profiles/{profileID}
func (h *Handler) HandleRemoveInstanceProfile(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	profileID := chi.URLParam(r, "profileID")

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM instance_connection_profiles WHERE id = $1 AND user_id = $2`,
		profileID, userID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to remove instance profile", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Instance profile not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Federated User Profiles
// =============================================================================

// HandleGetFederatedUserProfile retrieves a user profile from a remote instance.
// GET /api/v1/admin/federation/users/{instanceDomain}/{username}
func (h *Handler) HandleGetFederatedUserProfile(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	instanceDomain := chi.URLParam(r, "instanceDomain")
	username := chi.URLParam(r, "username")

	// Look up the user locally first (they may have been cached from federation).
	type fedUser struct {
		ID             string     `json:"id"`
		InstanceID     string     `json:"instance_id"`
		Username       string     `json:"username"`
		DisplayName    *string    `json:"display_name,omitempty"`
		AvatarID       *string    `json:"avatar_id,omitempty"`
		Bio            *string    `json:"bio,omitempty"`
		StatusPresence string     `json:"status_presence"`
		CreatedAt      time.Time  `json:"created_at"`
		InstanceDomain string     `json:"instance_domain"`
	}

	var u fedUser
	err := h.Pool.QueryRow(r.Context(),
		`SELECT u.id, u.instance_id, u.username, u.display_name, u.avatar_id, u.bio,
		        u.status_presence, u.created_at, i.domain
		 FROM users u
		 JOIN instances i ON i.id = u.instance_id
		 WHERE u.username = $1 AND i.domain = $2`, username, instanceDomain).Scan(
		&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
		&u.Bio, &u.StatusPresence, &u.CreatedAt, &u.InstanceDomain)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "user_not_found",
			"User not found. The remote instance may need to be discovered first.")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, u)
}

// =============================================================================
// Instance Blocklist/Allowlist Management
// =============================================================================

// HandleGetInstanceBlocklist returns the instance blocklist (blocked peers).
// GET /api/v1/admin/federation/blocklist
func (h *Handler) HandleGetInstanceBlocklist(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fpc.id, fpc.peer_id, fpc.reason, fpc.created_at, i.domain, i.name
		 FROM federation_peer_controls fpc
		 JOIN instances i ON i.id = fpc.peer_id
		 WHERE fpc.instance_id = $1 AND fpc.action = 'block'
		 ORDER BY fpc.created_at DESC`, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get blocklist", err)
		return
	}
	defer rows.Close()

	type entry struct {
		ID         string    `json:"id"`
		PeerID     string    `json:"peer_id"`
		PeerDomain string    `json:"peer_domain"`
		PeerName   *string   `json:"peer_name,omitempty"`
		Reason     *string   `json:"reason,omitempty"`
		CreatedAt  time.Time `json:"created_at"`
	}

	entries := make([]entry, 0)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.PeerID, &e.Reason, &e.CreatedAt, &e.PeerDomain, &e.PeerName); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// HandleGetInstanceAllowlist returns the instance allowlist (allowed peers).
// GET /api/v1/admin/federation/allowlist
func (h *Handler) HandleGetInstanceAllowlist(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fpc.id, fpc.peer_id, fpc.reason, fpc.created_at, i.domain, i.name
		 FROM federation_peer_controls fpc
		 JOIN instances i ON i.id = fpc.peer_id
		 WHERE fpc.instance_id = $1 AND fpc.action = 'allow'
		 ORDER BY fpc.created_at DESC`, h.InstanceID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get allowlist", err)
		return
	}
	defer rows.Close()

	type entry struct {
		ID         string    `json:"id"`
		PeerID     string    `json:"peer_id"`
		PeerDomain string    `json:"peer_domain"`
		PeerName   *string   `json:"peer_name,omitempty"`
		Reason     *string   `json:"reason,omitempty"`
		CreatedAt  time.Time `json:"created_at"`
	}

	entries := make([]entry, 0)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.PeerID, &e.Reason, &e.CreatedAt, &e.PeerDomain, &e.PeerName); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// =============================================================================
// Protocol Versioning
// =============================================================================

// HandleGetProtocolInfo returns protocol version and capabilities for this instance.
// GET /api/v1/admin/federation/protocol
func (h *Handler) HandleGetProtocolInfo(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var protocolVersion string
	var capabilities json.RawMessage

	err := h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(protocol_version, 'amityvox-federation/1.0'),
		        COALESCE(capabilities, '[]'::jsonb)
		 FROM instances WHERE id = $1`, h.InstanceID).Scan(&protocolVersion, &capabilities)
	if err != nil {
		protocolVersion = "amityvox-federation/1.0"
		capabilities = json.RawMessage(`[]`)
	}

	// Default capabilities.
	defaultCapabilities := []string{
		"messages", "presence", "profiles", "channels", "guilds",
		"reactions", "attachments", "embeds", "typing",
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"protocol_version":      protocolVersion,
		"capabilities":          capabilities,
		"supported_protocols":   []string{"amityvox-federation/1.0", "amityvox-federation/1.1"},
		"default_capabilities":  defaultCapabilities,
	})
}

// HandleUpdateProtocolConfig updates protocol version and capabilities.
// PATCH /api/v1/admin/federation/protocol
func (h *Handler) HandleUpdateProtocolConfig(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	var req struct {
		ProtocolVersion *string  `json:"protocol_version,omitempty"`
		Capabilities    []string `json:"capabilities,omitempty"`
	}
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if req.ProtocolVersion != nil {
		_, err := h.Pool.Exec(r.Context(),
			`UPDATE instances SET protocol_version = $1 WHERE id = $2`,
			*req.ProtocolVersion, h.InstanceID)
		if err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to update protocol version", err)
			return
		}
	}

	if req.Capabilities != nil {
		capsJSON, _ := json.Marshal(req.Capabilities)
		_, err := h.Pool.Exec(r.Context(),
			`UPDATE instances SET capabilities = $1 WHERE id = $2`, capsJSON, h.InstanceID)
		if err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to update capabilities", err)
			return
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// =============================================================================
// Federation Peer Approval / Key Audit
// =============================================================================

// HandleApproveFederationPeer approves a pending federation peer.
// POST /api/v1/admin/federation/peers/{peerID}/approve
func (h *Handler) HandleApproveFederationPeer(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	peerID := chi.URLParam(r, "peerID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE federation_peers
		 SET status = 'active', handshake_completed_at = now()
		 WHERE instance_id = $1 AND peer_id = $2 AND status = 'pending'`,
		h.InstanceID, peerID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to approve peer", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "No pending peer found with that ID")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// HandleRejectFederationPeer rejects a pending federation peer.
// POST /api/v1/admin/federation/peers/{peerID}/reject
func (h *Handler) HandleRejectFederationPeer(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	peerID := chi.URLParam(r, "peerID")

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE federation_peers
		 SET status = 'blocked'
		 WHERE instance_id = $1 AND peer_id = $2 AND status = 'pending'`,
		h.InstanceID, peerID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to reject peer", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "No pending peer found with that ID")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// HandleGetKeyAudit returns unacknowledged key change audit entries.
// GET /api/v1/admin/federation/key-audit
func (h *Handler) HandleGetKeyAudit(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT fka.id, fka.instance_id, fka.old_fingerprint, fka.new_fingerprint,
		        fka.detected_at, fka.acknowledged_by, fka.acknowledged_at,
		        i.domain, i.name
		 FROM federation_key_audit fka
		 JOIN instances i ON i.id = fka.instance_id
		 WHERE fka.acknowledged_at IS NULL
		 ORDER BY fka.detected_at DESC`)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get key audit entries", err)
		return
	}
	defer rows.Close()

	type auditEntry struct {
		ID             string     `json:"id"`
		InstanceID     string     `json:"instance_id"`
		InstanceDomain string     `json:"instance_domain"`
		InstanceName   *string    `json:"instance_name,omitempty"`
		OldFingerprint string     `json:"old_fingerprint"`
		NewFingerprint string     `json:"new_fingerprint"`
		DetectedAt     time.Time  `json:"detected_at"`
		AcknowledgedBy *string    `json:"acknowledged_by,omitempty"`
		AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	}

	entries := make([]auditEntry, 0)
	for rows.Next() {
		var e auditEntry
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.OldFingerprint, &e.NewFingerprint,
			&e.DetectedAt, &e.AcknowledgedBy, &e.AcknowledgedAt,
			&e.InstanceDomain, &e.InstanceName); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// HandleAcknowledgeKeyChange acknowledges a key change audit entry.
// POST /api/v1/admin/federation/key-audit/{auditID}/acknowledge
func (h *Handler) HandleAcknowledgeKeyChange(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "Admin access required")
		return
	}

	auditID := chi.URLParam(r, "auditID")
	adminID := auth.UserIDFromContext(r.Context())

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE federation_key_audit
		 SET acknowledged_by = $1, acknowledged_at = now()
		 WHERE id = $2 AND acknowledged_at IS NULL`,
		adminID, auditID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to acknowledge key change", err)
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Key audit entry not found or already acknowledged")
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}
