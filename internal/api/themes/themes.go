// Package themes implements REST API handlers for the community theme gallery.
// Users can share custom themes, browse shared themes, like/unlike them, and
// apply them to their own client. Mounted under /api/v1/themes.
package themes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// Handler implements theme gallery REST API endpoints.
type Handler struct {
	Pool     *pgxpool.Pool
	EventBus *events.Bus
	Logger   *slog.Logger
}

// --- Response types ---

type sharedTheme struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	AuthorName    string          `json:"author_name"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Variables     json.RawMessage `json:"variables"`
	CustomCSS     string          `json:"custom_css"`
	PreviewColors json.RawMessage `json:"preview_colors"`
	ShareCode     string          `json:"share_code"`
	Downloads     int             `json:"downloads"`
	LikeCount     int             `json:"like_count"`
	Liked         bool            `json:"liked"`
	CreatedAt     time.Time       `json:"created_at"`
}

// --- Request types ---

type shareThemeRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Variables     json.RawMessage `json:"variables"`
	CustomCSS     string          `json:"custom_css"`
	PreviewColors json.RawMessage `json:"preview_colors"`
}

// HandleListSharedThemes returns a paginated list of shared themes.
// GET /api/v1/themes?sort=downloads|likes|newest&limit=N&offset=N
func (h *Handler) HandleListSharedThemes(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "newest"
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	search := r.URL.Query().Get("q")

	var orderBy string
	switch sort {
	case "downloads":
		orderBy = "st.downloads DESC, st.created_at DESC"
	case "likes":
		orderBy = "like_count DESC, st.created_at DESC"
	default:
		orderBy = "st.created_at DESC"
	}

	query := `
		SELECT st.id, st.user_id, COALESCE(u.display_name, u.username) AS author_name,
		       st.name, st.description, st.variables, st.custom_css, st.preview_colors,
		       st.share_code, st.downloads, st.created_at,
		       COUNT(tl.user_id) AS like_count,
		       EXISTS(SELECT 1 FROM theme_likes WHERE user_id = $1 AND theme_id = st.id) AS liked
		FROM shared_themes st
		JOIN users u ON u.id = st.user_id
		LEFT JOIN theme_likes tl ON tl.theme_id = st.id`

	args := []interface{}{userID}
	argIdx := 2

	if search != "" {
		query += ` WHERE (st.name ILIKE '%' || $` + strconv.Itoa(argIdx) + ` || '%' OR st.description ILIKE '%' || $` + strconv.Itoa(argIdx) + ` || '%')`
		args = append(args, search)
		argIdx++
	}

	query += ` GROUP BY st.id, u.display_name, u.username
		ORDER BY ` + orderBy + `
		LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)

	args = append(args, limit, offset)

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to list themes", err)
		return
	}
	defer rows.Close()

	themes := make([]sharedTheme, 0)
	for rows.Next() {
		var t sharedTheme
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.AuthorName, &t.Name, &t.Description,
			&t.Variables, &t.CustomCSS, &t.PreviewColors,
			&t.ShareCode, &t.Downloads, &t.CreatedAt,
			&t.LikeCount, &t.Liked,
		); err != nil {
			apiutil.InternalError(w, h.Logger, "Failed to list themes", err)
			return
		}
		themes = append(themes, t)
	}

	apiutil.WriteJSON(w, http.StatusOK, themes)
}

// HandleShareTheme creates a new shared theme in the gallery.
// POST /api/v1/themes
func (h *Handler) HandleShareTheme(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req shareThemeRequest
	if !apiutil.DecodeJSON(w, r, &req) {
		return
	}

	if !apiutil.RequireNonEmpty(w, "name", req.Name) {
		return
	}
	if len(req.Name) > 64 {
		apiutil.WriteError(w, http.StatusBadRequest, "name_too_long", "Theme name must be at most 64 characters")
		return
	}
	if len(req.Description) > 500 {
		apiutil.WriteError(w, http.StatusBadRequest, "description_too_long", "Description must be at most 500 characters")
		return
	}
	if len(req.CustomCSS) > 10000 {
		apiutil.WriteError(w, http.StatusBadRequest, "css_too_long", "Custom CSS must be at most 10000 characters")
		return
	}
	if req.Variables == nil {
		req.Variables = json.RawMessage(`{}`)
	}
	if req.PreviewColors == nil {
		req.PreviewColors = json.RawMessage(`[]`)
	}

	id := models.NewULID().String()
	shareCode, err := generateShareCode()
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create theme", err)
		return
	}

	var t sharedTheme
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO shared_themes (id, user_id, name, description, variables, custom_css, preview_colors, share_code, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 RETURNING id, user_id, name, description, variables, custom_css, preview_colors, share_code, downloads, created_at`,
		id, userID, req.Name, req.Description, req.Variables, req.CustomCSS, req.PreviewColors, shareCode,
	).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Description,
		&t.Variables, &t.CustomCSS, &t.PreviewColors,
		&t.ShareCode, &t.Downloads, &t.CreatedAt,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to create theme", err)
		return
	}

	// Fetch author name.
	var authorName string
	h.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(display_name, username) FROM users WHERE id = $1`, userID,
	).Scan(&authorName)
	t.AuthorName = authorName

	h.Logger.Info("theme shared",
		slog.String("theme_id", t.ID),
		slog.String("user_id", userID),
		slog.String("name", t.Name),
	)

	apiutil.WriteJSON(w, http.StatusCreated, t)
}

// HandleGetSharedTheme returns a single shared theme by share code.
// GET /api/v1/themes/{shareCode}
func (h *Handler) HandleGetSharedTheme(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	shareCode := chi.URLParam(r, "shareCode")

	if !apiutil.RequireNonEmpty(w, "share_code", shareCode) {
		return
	}

	var t sharedTheme
	err := h.Pool.QueryRow(r.Context(),
		`SELECT st.id, st.user_id, COALESCE(u.display_name, u.username) AS author_name,
		        st.name, st.description, st.variables, st.custom_css, st.preview_colors,
		        st.share_code, st.downloads, st.created_at,
		        (SELECT COUNT(*) FROM theme_likes WHERE theme_id = st.id) AS like_count,
		        EXISTS(SELECT 1 FROM theme_likes WHERE user_id = $1 AND theme_id = st.id) AS liked
		 FROM shared_themes st
		 JOIN users u ON u.id = st.user_id
		 WHERE st.share_code = $2`,
		userID, shareCode,
	).Scan(
		&t.ID, &t.UserID, &t.AuthorName, &t.Name, &t.Description,
		&t.Variables, &t.CustomCSS, &t.PreviewColors,
		&t.ShareCode, &t.Downloads, &t.CreatedAt,
		&t.LikeCount, &t.Liked,
	)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "theme_not_found", "Theme not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to get theme", err)
		return
	}

	// Increment downloads on retrieval.
	h.Pool.Exec(r.Context(),
		`UPDATE shared_themes SET downloads = downloads + 1 WHERE share_code = $1`, shareCode)
	t.Downloads++

	apiutil.WriteJSON(w, http.StatusOK, t)
}

// HandleLikeTheme toggles a like on a shared theme.
// PUT /api/v1/themes/{themeID}/like
func (h *Handler) HandleLikeTheme(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	themeID := chi.URLParam(r, "themeID")

	if !apiutil.RequireNonEmpty(w, "theme_id", themeID) {
		return
	}

	// Verify theme exists.
	var exists bool
	h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM shared_themes WHERE id = $1)`, themeID,
	).Scan(&exists)
	if !exists {
		apiutil.WriteError(w, http.StatusNotFound, "theme_not_found", "Theme not found")
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`INSERT INTO theme_likes (user_id, theme_id, created_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (user_id, theme_id) DO NOTHING`,
		userID, themeID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to like theme", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUnlikeTheme removes a like from a shared theme.
// DELETE /api/v1/themes/{themeID}/like
func (h *Handler) HandleUnlikeTheme(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	themeID := chi.URLParam(r, "themeID")

	if !apiutil.RequireNonEmpty(w, "theme_id", themeID) {
		return
	}

	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM theme_likes WHERE user_id = $1 AND theme_id = $2`,
		userID, themeID,
	)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to unlike theme", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleDeleteSharedTheme removes a shared theme. Only the author can delete it.
// DELETE /api/v1/themes/{themeID}
func (h *Handler) HandleDeleteSharedTheme(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	themeID := chi.URLParam(r, "themeID")

	if !apiutil.RequireNonEmpty(w, "theme_id", themeID) {
		return
	}

	// Verify ownership.
	var ownerID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM shared_themes WHERE id = $1`, themeID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		apiutil.WriteError(w, http.StatusNotFound, "theme_not_found", "Theme not found")
		return
	}
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete theme", err)
		return
	}

	// Allow owner or admins to delete.
	if ownerID != userID {
		var flags int
		h.Pool.QueryRow(r.Context(), `SELECT flags FROM users WHERE id = $1`, userID).Scan(&flags)
		const flagAdmin = 1 << 2
		if flags&flagAdmin == 0 {
			apiutil.WriteError(w, http.StatusForbidden, "forbidden", "You can only delete your own themes")
			return
		}
	}

	_, err = h.Pool.Exec(r.Context(),
		`DELETE FROM shared_themes WHERE id = $1`, themeID)
	if err != nil {
		apiutil.InternalError(w, h.Logger, "Failed to delete theme", err)
		return
	}

	h.Logger.Info("theme deleted",
		slog.String("theme_id", themeID),
		slog.String("deleted_by", userID),
	)

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func generateShareCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}


