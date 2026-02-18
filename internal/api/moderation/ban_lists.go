package moderation

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// --- Ban List Request Types ---

type createBanListRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Public      bool   `json:"public"`
}

type addBanListEntryRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

type subscribeBanListRequest struct {
	BanListID string `json:"ban_list_id"`
	AutoBan   bool   `json:"auto_ban"`
}

// HandleCreateBanList creates a new shared ban list for a guild.
// POST /api/v1/guilds/{guildID}/ban-lists
func (h *Handler) HandleCreateBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	var req createBanListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Name == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "name_required", "Ban list name is required")
		return
	}

	id := models.NewULID().String()
	now := time.Now()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO ban_lists (id, guild_id, name, description, public, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		id, guildID, req.Name, req.Description, req.Public, userID, now)
	if err != nil {
		h.Logger.Error("failed to create ban list", "error", err.Error())
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create ban list")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"guild_id":    guildID,
		"name":        req.Name,
		"description": req.Description,
		"public":      req.Public,
		"created_by":  userID,
		"created_at":  now,
	})
}

// HandleGetBanLists returns all ban lists for a guild.
// GET /api/v1/guilds/{guildID}/ban-lists
func (h *Handler) HandleGetBanLists(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT bl.id, bl.guild_id, bl.name, bl.description, bl.public, bl.created_by, bl.created_at,
		        (SELECT COUNT(*) FROM ban_list_entries WHERE ban_list_id = bl.id) AS entry_count,
		        (SELECT COUNT(*) FROM ban_list_subscriptions WHERE ban_list_id = bl.id) AS subscriber_count
		 FROM ban_lists bl
		 WHERE bl.guild_id = $1
		 ORDER BY bl.created_at DESC`, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch ban lists")
		return
	}
	defer rows.Close()

	var lists []map[string]interface{}
	for rows.Next() {
		var id, guild, name, createdBy string
		var desc *string
		var public bool
		var createdAt time.Time
		var entryCount, subCount int
		if err := rows.Scan(&id, &guild, &name, &desc, &public, &createdBy, &createdAt, &entryCount, &subCount); err != nil {
			continue
		}
		lists = append(lists, map[string]interface{}{
			"id":               id,
			"guild_id":         guild,
			"name":             name,
			"description":      desc,
			"public":           public,
			"created_by":       createdBy,
			"created_at":       createdAt,
			"entry_count":      entryCount,
			"subscriber_count": subCount,
		})
	}
	if lists == nil {
		lists = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, lists)
}

// HandleDeleteBanList deletes a ban list.
// DELETE /api/v1/guilds/{guildID}/ban-lists/{listID}
func (h *Handler) HandleDeleteBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM ban_lists WHERE id = $1 AND guild_id = $2`, listID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete ban list")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Ban list not found")
		return
	}

	apiutil.WriteNoContent(w)
}

// HandleAddBanListEntry adds a user to a ban list.
// POST /api/v1/guilds/{guildID}/ban-lists/{listID}/entries
func (h *Handler) HandleAddBanListEntry(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	var req addBanListEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.UserID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "user_id_required", "user_id is required")
		return
	}

	// Verify ban list belongs to this guild.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM ban_lists WHERE id = $1 AND guild_id = $2)`, listID, guildID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Ban list not found")
		return
	}

	id := models.NewULID().String()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO ban_list_entries (id, ban_list_id, user_id, username, reason, added_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (ban_list_id, user_id) DO UPDATE SET reason = EXCLUDED.reason, username = EXCLUDED.username`,
		id, listID, req.UserID, req.Username, req.Reason, userID)
	if err != nil {
		h.Logger.Error("failed to add ban list entry", "error", err.Error())
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add entry")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       id,
		"user_id":  req.UserID,
		"username": req.Username,
		"reason":   req.Reason,
		"added_by": userID,
	})
}

// HandleGetBanListEntries returns all entries in a ban list.
// GET /api/v1/guilds/{guildID}/ban-lists/{listID}/entries
func (h *Handler) HandleGetBanListEntries(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT e.id, e.user_id, e.username, e.reason, e.added_by, e.added_at
		 FROM ban_list_entries e
		 JOIN ban_lists bl ON bl.id = e.ban_list_id
		 WHERE e.ban_list_id = $1 AND bl.guild_id = $2
		 ORDER BY e.added_at DESC`, listID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch entries")
		return
	}
	defer rows.Close()

	var entries []map[string]interface{}
	for rows.Next() {
		var id, entryUserID, addedBy string
		var username, reason *string
		var addedAt time.Time
		if err := rows.Scan(&id, &entryUserID, &username, &reason, &addedBy, &addedAt); err != nil {
			continue
		}
		entries = append(entries, map[string]interface{}{
			"id":       id,
			"user_id":  entryUserID,
			"username": username,
			"reason":   reason,
			"added_by": addedBy,
			"added_at": addedAt,
		})
	}
	if entries == nil {
		entries = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, entries)
}

// HandleRemoveBanListEntry removes a user from a ban list.
// DELETE /api/v1/guilds/{guildID}/ban-lists/{listID}/entries/{entryID}
func (h *Handler) HandleRemoveBanListEntry(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")
	entryID := chi.URLParam(r, "entryID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM ban_list_entries e
		 USING ban_lists bl
		 WHERE e.id = $1 AND e.ban_list_id = $2 AND bl.id = e.ban_list_id AND bl.guild_id = $3`,
		entryID, listID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove entry")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Entry not found")
		return
	}

	apiutil.WriteNoContent(w)
}

// HandleExportBanList exports a ban list as JSON.
// GET /api/v1/guilds/{guildID}/ban-lists/{listID}/export
func (h *Handler) HandleExportBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	// Get ban list metadata.
	var name, desc string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT name, COALESCE(description, '') FROM ban_lists WHERE id = $1 AND guild_id = $2`,
		listID, guildID).Scan(&name, &desc)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Ban list not found")
		return
	}

	// Get entries.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT user_id, username, reason, added_at FROM ban_list_entries WHERE ban_list_id = $1 ORDER BY added_at`,
		listID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to export")
		return
	}
	defer rows.Close()

	type exportEntry struct {
		UserID   string  `json:"user_id"`
		Username *string `json:"username,omitempty"`
		Reason   *string `json:"reason,omitempty"`
		AddedAt  string  `json:"added_at"`
	}
	var entries []exportEntry
	for rows.Next() {
		var e exportEntry
		var username, reason *string
		var addedAt time.Time
		if err := rows.Scan(&e.UserID, &username, &reason, &addedAt); err != nil {
			continue
		}
		e.Username = username
		e.Reason = reason
		e.AddedAt = addedAt.Format(time.RFC3339)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []exportEntry{}
	}

	export := map[string]interface{}{
		"format":      "amityvox_ban_list_v1",
		"name":        name,
		"description": desc,
		"entries":     entries,
		"exported_at": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=ban_list_"+listID+".json")
	json.NewEncoder(w).Encode(export)
}

// HandleImportBanList imports entries from a ban list JSON export.
// POST /api/v1/guilds/{guildID}/ban-lists/{listID}/import
func (h *Handler) HandleImportBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	listID := chi.URLParam(r, "listID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	// Verify ban list belongs to this guild.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM ban_lists WHERE id = $1 AND guild_id = $2)`, listID, guildID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Ban list not found")
		return
	}

	var importData struct {
		Entries []struct {
			UserID   string  `json:"user_id"`
			Username *string `json:"username"`
			Reason   *string `json:"reason"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&importData); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid import data")
		return
	}

	imported := 0
	for _, e := range importData.Entries {
		if e.UserID == "" {
			continue
		}
		id := models.NewULID().String()
		var username, reason string
		if e.Username != nil {
			username = *e.Username
		}
		if e.Reason != nil {
			reason = *e.Reason
		}
		_, err := h.Pool.Exec(r.Context(),
			`INSERT INTO ban_list_entries (id, ban_list_id, user_id, username, reason, added_by)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (ban_list_id, user_id) DO NOTHING`,
			id, listID, e.UserID, username, reason, userID)
		if err == nil {
			imported++
		}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"imported": imported,
		"total":    len(importData.Entries),
	})
}

// HandleSubscribeBanList subscribes a guild to another guild's public ban list.
// POST /api/v1/guilds/{guildID}/ban-list-subscriptions
func (h *Handler) HandleSubscribeBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	var req subscribeBanListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Verify the ban list exists and is public (or from same guild).
	var listGuildID string
	var isPublic bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT guild_id, public FROM ban_lists WHERE id = $1`, req.BanListID).Scan(&listGuildID, &isPublic)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Ban list not found")
		return
	}
	if !isPublic && listGuildID != guildID {
		apiutil.WriteError(w, http.StatusForbidden, "not_public", "This ban list is not public")
		return
	}

	id := models.NewULID().String()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO ban_list_subscriptions (id, guild_id, ban_list_id, auto_ban, subscribed_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (guild_id, ban_list_id) DO UPDATE SET auto_ban = EXCLUDED.auto_ban`,
		id, guildID, req.BanListID, req.AutoBan, userID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to subscribe")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"guild_id":    guildID,
		"ban_list_id": req.BanListID,
		"auto_ban":    req.AutoBan,
	})
}

// HandleGetBanListSubscriptions returns all ban list subscriptions for a guild.
// GET /api/v1/guilds/{guildID}/ban-list-subscriptions
func (h *Handler) HandleGetBanListSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT s.id, s.ban_list_id, s.auto_ban, s.subscribed_at,
		        bl.name, bl.guild_id,
		        (SELECT COUNT(*) FROM ban_list_entries WHERE ban_list_id = bl.id) AS entry_count
		 FROM ban_list_subscriptions s
		 JOIN ban_lists bl ON bl.id = s.ban_list_id
		 WHERE s.guild_id = $1
		 ORDER BY s.subscribed_at DESC`, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch subscriptions")
		return
	}
	defer rows.Close()

	var subs []map[string]interface{}
	for rows.Next() {
		var id, banListID, listName, listGuildID string
		var autoBan bool
		var subscribedAt time.Time
		var entryCount int
		if err := rows.Scan(&id, &banListID, &autoBan, &subscribedAt, &listName, &listGuildID, &entryCount); err != nil {
			continue
		}
		subs = append(subs, map[string]interface{}{
			"id":            id,
			"ban_list_id":   banListID,
			"auto_ban":      autoBan,
			"subscribed_at": subscribedAt,
			"list_name":     listName,
			"list_guild_id": listGuildID,
			"entry_count":   entryCount,
		})
	}
	if subs == nil {
		subs = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, subs)
}

// HandleUnsubscribeBanList removes a ban list subscription.
// DELETE /api/v1/guilds/{guildID}/ban-list-subscriptions/{subID}
func (h *Handler) HandleUnsubscribeBanList(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	subID := chi.URLParam(r, "subID")

	if !h.hasGuildPermission(r.Context(), guildID, userID, "ban_members") {
		apiutil.WriteError(w, http.StatusForbidden, "missing_permission", "You need BAN_MEMBERS permission")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM ban_list_subscriptions WHERE id = $1 AND guild_id = $2`, subID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unsubscribe")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Subscription not found")
		return
	}

	apiutil.WriteNoContent(w)
}

// HandleGetPublicBanLists returns public ban lists that can be subscribed to.
// GET /api/v1/ban-lists/public
func (h *Handler) HandleGetPublicBanLists(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT bl.id, bl.guild_id, bl.name, bl.description, bl.created_at,
		        g.name AS guild_name,
		        (SELECT COUNT(*) FROM ban_list_entries WHERE ban_list_id = bl.id) AS entry_count,
		        (SELECT COUNT(*) FROM ban_list_subscriptions WHERE ban_list_id = bl.id) AS subscriber_count
		 FROM ban_lists bl
		 JOIN guilds g ON g.id = bl.guild_id
		 WHERE bl.public = true
		 ORDER BY subscriber_count DESC, bl.created_at DESC
		 LIMIT 50`)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch public ban lists")
		return
	}
	defer rows.Close()

	var lists []map[string]interface{}
	for rows.Next() {
		var id, guild, name, guildName string
		var desc *string
		var createdAt time.Time
		var entryCount, subCount int
		if err := rows.Scan(&id, &guild, &name, &desc, &createdAt, &guildName, &entryCount, &subCount); err != nil {
			continue
		}
		lists = append(lists, map[string]interface{}{
			"id":               id,
			"guild_id":         guild,
			"name":             name,
			"description":      desc,
			"guild_name":       guildName,
			"entry_count":      entryCount,
			"subscriber_count": subCount,
			"created_at":       createdAt,
		})
	}
	if lists == nil {
		lists = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, lists)
}
