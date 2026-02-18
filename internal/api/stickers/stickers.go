// Package stickers implements REST API handlers for sticker pack management.
// Sticker packs can be owned by guilds (custom stickers), users (personal stickers),
// or the system (built-in sticker packs). Mounted under /api/v1/guilds and /api/v1/stickers.
package stickers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements sticker-related REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Request types ---

type createPackRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createStickerRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
	FileID      string `json:"file_id"`
	Format      string `json:"format"`
}

// isMember checks whether the user is a member of the guild.
func (h *Handler) isMember(ctx context.Context, guildID, userID string) bool {
	var exists bool
	h.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID,
	).Scan(&exists)
	return exists
}

// hasManageEmojis checks if the user can manage emojis/stickers in this guild.
func (h *Handler) hasManageEmojis(ctx context.Context, guildID, userID string) bool {
	// Guild owner always has permission.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err == nil && ownerID == userID {
		return true
	}
	// Instance admin always has permission.
	var flags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
	if flags&models.UserFlagAdmin != 0 {
		return true
	}
	// Check ManageEmojisAndStickers permission (bit 30).
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computed := uint64(defaultPerms)
	rows, _ := h.Pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2 ORDER BY r.position DESC`,
		guildID, userID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var allow, deny int64
			rows.Scan(&allow, &deny)
			computed |= uint64(allow)
			computed &^= uint64(deny)
		}
	}
	return computed&(1<<30) != 0 // ManageEmojisAndStickers
}

// HandleCreateGuildPack creates a new sticker pack for a guild.
// POST /api/v1/guilds/{guildID}/sticker-packs
func (h *Handler) HandleCreateGuildPack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	var req createPackRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "name", req.Name) {
		return
	}

	id := models.NewULID().String()
	now := time.Now()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO sticker_packs (id, name, description, owner_type, owner_id, created_at)
		 VALUES ($1, $2, $3, 'guild', $4, $5)`,
		id, req.Name, req.Description, guildID, now)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create sticker pack", err)
		return
	}

	_ = userID // logged for audit
	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
		"owner_type":  "guild",
		"owner_id":    guildID,
		"created_at":  now,
	})
}

// HandleGetGuildPacks returns all sticker packs for a guild.
// GET /api/v1/guilds/{guildID}/sticker-packs
func (h *Handler) HandleGetGuildPacks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")

	if !h.isMember(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this guild")
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT sp.id, sp.name, sp.description, sp.cover_sticker_id, sp.public, sp.created_at,
		        (SELECT COUNT(*) FROM stickers WHERE pack_id = sp.id) AS sticker_count
		 FROM sticker_packs sp
		 WHERE sp.owner_type = 'guild' AND sp.owner_id = $1
		 ORDER BY sp.created_at DESC`, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch sticker packs")
		return
	}
	defer rows.Close()

	var packs []map[string]interface{}
	for rows.Next() {
		var id, name string
		var desc, coverID *string
		var public bool
		var createdAt time.Time
		var count int
		if err := rows.Scan(&id, &name, &desc, &coverID, &public, &createdAt, &count); err != nil {
			continue
		}
		packs = append(packs, map[string]interface{}{
			"id":               id,
			"name":             name,
			"description":      desc,
			"cover_sticker_id": coverID,
			"public":           public,
			"sticker_count":    count,
			"created_at":       createdAt,
		})
	}
	if packs == nil {
		packs = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, packs)
}

// HandleDeletePack deletes a sticker pack and all its stickers.
// DELETE /api/v1/guilds/{guildID}/sticker-packs/{packID}
func (h *Handler) HandleDeletePack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	packID := chi.URLParam(r, "packID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM sticker_packs WHERE id = $1 AND owner_type = 'guild' AND owner_id = $2`,
		packID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete sticker pack")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker pack not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleAddSticker adds a sticker to a pack.
// POST /api/v1/guilds/{guildID}/sticker-packs/{packID}/stickers
func (h *Handler) HandleAddSticker(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	packID := chi.URLParam(r, "packID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	// Verify pack belongs to guild.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM sticker_packs WHERE id = $1 AND owner_type = 'guild' AND owner_id = $2)`,
		packID, guildID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker pack not found")
		return
	}

	var req createStickerRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.FileID == "" {
		apiutil.WriteError(w, http.StatusBadRequest, "fields_required", "name and file_id are required")
		return
	}
	if req.Format == "" {
		req.Format = "png"
	}

	id := models.NewULID().String()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO stickers (id, pack_id, name, description, tags, file_id, format)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, packID, req.Name, req.Description, req.Tags, req.FileID, req.Format)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to add sticker", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"pack_id":     packID,
		"name":        req.Name,
		"description": req.Description,
		"tags":        req.Tags,
		"file_id":     req.FileID,
		"format":      req.Format,
	})
}

// HandleGetPackStickers returns all stickers in a pack.
// GET /api/v1/guilds/{guildID}/sticker-packs/{packID}/stickers
func (h *Handler) HandleGetPackStickers(w http.ResponseWriter, r *http.Request) {
	packID := chi.URLParam(r, "packID")

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, pack_id, name, description, tags, file_id, format, created_at
		 FROM stickers WHERE pack_id = $1 ORDER BY created_at`, packID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch stickers")
		return
	}
	defer rows.Close()

	var stickers []map[string]interface{}
	for rows.Next() {
		var id, pid, name, fileID, format string
		var desc, tags *string
		var createdAt time.Time
		if err := rows.Scan(&id, &pid, &name, &desc, &tags, &fileID, &format, &createdAt); err != nil {
			continue
		}
		stickers = append(stickers, map[string]interface{}{
			"id":          id,
			"pack_id":     pid,
			"name":        name,
			"description": desc,
			"tags":        tags,
			"file_id":     fileID,
			"format":      format,
			"created_at":  createdAt,
		})
	}
	if stickers == nil {
		stickers = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, stickers)
}

// HandleDeleteSticker removes a sticker from a pack.
// DELETE /api/v1/guilds/{guildID}/sticker-packs/{packID}/stickers/{stickerID}
func (h *Handler) HandleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	packID := chi.URLParam(r, "packID")
	stickerID := chi.URLParam(r, "stickerID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	// Verify pack belongs to guild.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM sticker_packs WHERE id = $1 AND owner_type = 'guild' AND owner_id = $2)`,
		packID, guildID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker pack not found")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM stickers WHERE id = $1 AND pack_id = $2`, stickerID, packID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete sticker")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCreateUserPack creates a personal sticker pack for the current user.
// POST /api/v1/stickers/my-packs
func (h *Handler) HandleCreateUserPack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req createPackRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}
	if !apiutil.RequireNonEmpty(w, "name", req.Name) {
		return
	}

	// Limit user packs to 5.
	var packCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM sticker_packs WHERE owner_type = 'user' AND owner_id = $1`, userID).Scan(&packCount)
	if packCount >= 5 {
		apiutil.WriteError(w, http.StatusBadRequest, "limit_reached", "You can have at most 5 personal sticker packs")
		return
	}

	id := models.NewULID().String()
	now := time.Now()
	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO sticker_packs (id, name, description, owner_type, owner_id, created_at)
		 VALUES ($1, $2, $3, 'user', $4, $5)`,
		id, req.Name, req.Description, userID, now)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create sticker pack")
		return
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
		"owner_type":  "user",
		"owner_id":    userID,
		"created_at":  now,
	})
}

// HandleGetUserPacks returns the current user's personal sticker packs.
// GET /api/v1/stickers/my-packs
func (h *Handler) HandleGetUserPacks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT sp.id, sp.name, sp.description, sp.created_at,
		        (SELECT COUNT(*) FROM stickers WHERE pack_id = sp.id) AS sticker_count
		 FROM sticker_packs sp
		 WHERE sp.owner_type = 'user' AND sp.owner_id = $1
		 ORDER BY sp.created_at`, userID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch packs")
		return
	}
	defer rows.Close()

	var packs []map[string]interface{}
	for rows.Next() {
		var id, name string
		var desc *string
		var createdAt time.Time
		var count int
		if err := rows.Scan(&id, &name, &desc, &createdAt, &count); err != nil {
			continue
		}
		packs = append(packs, map[string]interface{}{
			"id":            id,
			"name":          name,
			"description":   desc,
			"sticker_count": count,
			"created_at":    createdAt,
		})
	}
	if packs == nil {
		packs = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, packs)
}

// --- Sticker Pack Sharing ---

// HandleEnableSharing generates a share code for a sticker pack and enables sharing.
// POST /api/v1/guilds/{guildID}/sticker-packs/{packID}/share
func (h *Handler) HandleEnableSharing(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	packID := chi.URLParam(r, "packID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	// Verify pack belongs to guild.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM sticker_packs WHERE id = $1 AND owner_type = 'guild' AND owner_id = $2)`,
		packID, guildID).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker pack not found")
		return
	}

	// Check if the pack already has a share code.
	var existingCode *string
	h.Pool.QueryRow(r.Context(),
		`SELECT share_code FROM sticker_packs WHERE id = $1`, packID).Scan(&existingCode)
	if existingCode != nil && *existingCode != "" {
		// Already shared — return existing code.
		apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"pack_id":    packID,
			"share_code": *existingCode,
			"shared":     true,
		})
		return
	}

	// Generate a unique share code using a ULID.
	shareCode := models.NewULID().String()
	_, err := h.Pool.Exec(r.Context(),
		`UPDATE sticker_packs SET share_code = $1, shared = true WHERE id = $2`,
		shareCode, packID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to enable sharing", err)
		return
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"pack_id":    packID,
		"share_code": shareCode,
		"shared":     true,
	})
}

// HandleDisableSharing removes the share code from a sticker pack.
// DELETE /api/v1/guilds/{guildID}/sticker-packs/{packID}/share
func (h *Handler) HandleDisableSharing(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	guildID := chi.URLParam(r, "guildID")
	packID := chi.URLParam(r, "packID")

	if !h.hasManageEmojis(r.Context(), guildID, userID) {
		apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage stickers")
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`UPDATE sticker_packs SET share_code = NULL, shared = false
		 WHERE id = $1 AND owner_type = 'guild' AND owner_id = $2`,
		packID, guildID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to disable sharing")
		return
	}
	if tag.RowsAffected() == 0 {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Sticker pack not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetSharedPack returns a sticker pack by its share code, including all stickers.
// GET /api/v1/stickers/shared/{shareCode}
func (h *Handler) HandleGetSharedPack(w http.ResponseWriter, r *http.Request) {
	shareCode := chi.URLParam(r, "shareCode")

	// Fetch the pack by share code.
	var id, name string
	var desc *string
	var ownerType, ownerID string
	var createdAt time.Time
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name, description, owner_type, owner_id, created_at
		 FROM sticker_packs WHERE share_code = $1 AND shared = true`,
		shareCode,
	).Scan(&id, &name, &desc, &ownerType, &ownerID, &createdAt)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Shared sticker pack not found or sharing disabled")
		return
	}

	// Fetch stickers in this pack.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, name, description, tags, file_id, format, created_at
		 FROM stickers WHERE pack_id = $1 ORDER BY created_at`, id)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch stickers")
		return
	}
	defer rows.Close()

	var stickers []map[string]interface{}
	for rows.Next() {
		var sid, sname, fileID, format string
		var sdesc, tags *string
		var sCreatedAt time.Time
		if err := rows.Scan(&sid, &sname, &sdesc, &tags, &fileID, &format, &sCreatedAt); err != nil {
			continue
		}
		stickers = append(stickers, map[string]interface{}{
			"id":          sid,
			"name":        sname,
			"description": sdesc,
			"tags":        tags,
			"file_id":     fileID,
			"format":      format,
			"created_at":  sCreatedAt,
		})
	}
	if stickers == nil {
		stickers = []map[string]interface{}{}
	}

	apiutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": desc,
		"owner_type":  ownerType,
		"owner_id":    ownerID,
		"share_code":  shareCode,
		"stickers":    stickers,
		"created_at":  createdAt,
	})
}

// HandleClonePack clones a shared sticker pack into the current user's personal packs.
// POST /api/v1/stickers/shared/{shareCode}/clone
func (h *Handler) HandleClonePack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	shareCode := chi.URLParam(r, "shareCode")

	// Fetch source pack.
	var srcPackID, srcName string
	var srcDesc *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name, description FROM sticker_packs WHERE share_code = $1 AND shared = true`,
		shareCode,
	).Scan(&srcPackID, &srcName, &srcDesc)
	if err != nil {
		apiutil.WriteError(w, http.StatusNotFound, "not_found", "Shared sticker pack not found or sharing disabled")
		return
	}

	// Limit user packs to 10 (allowing more for cloned packs than self-created).
	var packCount int
	h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM sticker_packs WHERE owner_type = 'user' AND owner_id = $1`, userID).Scan(&packCount)
	if packCount >= 10 {
		apiutil.WriteError(w, http.StatusBadRequest, "limit_reached", "You can have at most 10 personal sticker packs")
		return
	}

	// Create the new pack.
	newPackID := models.NewULID().String()
	now := time.Now()
	_, err = h.Pool.Exec(r.Context(),
		`INSERT INTO sticker_packs (id, name, description, owner_type, owner_id, created_at)
		 VALUES ($1, $2, $3, 'user', $4, $5)`,
		newPackID, srcName, srcDesc, userID, now)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to clone sticker pack", err)
		return
	}

	// Copy stickers from source pack to new pack.
	rows, err := h.Pool.Query(r.Context(),
		`SELECT name, description, tags, file_id, format FROM stickers WHERE pack_id = $1`, srcPackID)
	if err != nil {
		apiutil.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read source stickers")
		return
	}
	defer rows.Close()

	var clonedCount int
	for rows.Next() {
		var sname, fileID, format string
		var sdesc, tags *string
		if err := rows.Scan(&sname, &sdesc, &tags, &fileID, &format); err != nil {
			continue
		}
		stickerID := models.NewULID().String()
		_, err := h.Pool.Exec(r.Context(),
			`INSERT INTO stickers (id, pack_id, name, description, tags, file_id, format)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			stickerID, newPackID, sname, sdesc, tags, fileID, format)
		if err != nil {
			h.Logger.Warn("failed to clone sticker", "error", err.Error(), "name", sname)
			continue
		}
		clonedCount++
	}

	apiutil.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":             newPackID,
		"name":           srcName,
		"description":    srcDesc,
		"owner_type":     "user",
		"owner_id":       userID,
		"cloned_from":    srcPackID,
		"sticker_count":  clonedCount,
		"created_at":     now,
	})
}
