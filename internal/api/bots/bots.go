// Package bots implements REST API handlers for bot account management,
// bot token authentication, and slash command registration.
// Mounted under /api/v1/users/@me/bots and /api/v1/bots/{botID}.
package bots

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

var commandNameRegex = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Handler implements bot-related REST API endpoints.
type Handler struct {
	Pool        *pgxpool.Pool
	AuthService *auth.Service
	EventBus    *events.Bus
	Logger      *slog.Logger
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

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// generateToken creates a cryptographically random bot token and its SHA-256 hash.
// The raw token is prefixed with "avbot_" for identification. Only the hash is stored.
func generateToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}
	raw = "avbot_" + hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

// verifyBotOwnership checks that the specified bot exists, is a bot user, and is
// owned by the given user. Writes an error response and returns false if
// verification fails; returns true if the caller should continue.
func (h *Handler) verifyBotOwnership(w http.ResponseWriter, r *http.Request, botID, userID string) bool {
	var ownerID *string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT bot_owner_id FROM users WHERE id = $1 AND flags & $2 != 0`,
		botID, models.UserFlagBot,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "bot_not_found", "Bot not found")
		return false
	}
	if err != nil {
		h.Logger.Error("failed to verify bot ownership", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return false
	}
	if ownerID == nil || *ownerID != userID {
		writeError(w, http.StatusForbidden, "not_owner", "You do not own this bot")
		return false
	}
	return true
}

// itoa converts a small positive integer to a string for building SQL
// parameter placeholders without importing strconv.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// userScanColumns is the SQL column list for scanning a models.User.
const userScanColumns = `id, instance_id, username, display_name, avatar_id, status_text, status_emoji,
        status_presence, status_expires_at, bio, bot_owner_id, email, banner_id,
        accent_color, pronouns, flags, created_at`

// scanUser scans a models.User from a row.
func scanUser(row pgx.Row) (models.User, error) {
	var u models.User
	err := row.Scan(
		&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
		&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
		&u.Bio, &u.BotOwnerID, &u.Email, &u.BannerID, &u.AccentColor,
		&u.Pronouns, &u.Flags, &u.CreatedAt,
	)
	return u, err
}

// --- Bot CRUD ---

// HandleCreateBot creates a new bot account owned by the current user.
// POST /api/v1/users/@me/bots
func (h *Handler) HandleCreateBot(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Description = strings.TrimSpace(body.Description)

	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "Bot name is required")
		return
	}
	if len(body.Name) > 32 {
		writeError(w, http.StatusBadRequest, "name_too_long", "Bot name must be 32 characters or fewer")
		return
	}

	// Check that the user does not exceed bot limit (max 25 bots per user).
	var botCount int
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM users WHERE bot_owner_id = $1`, userID,
	).Scan(&botCount); err != nil {
		h.Logger.Error("failed to count bots", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create bot")
		return
	}
	if botCount >= 25 {
		writeError(w, http.StatusBadRequest, "bot_limit", "You have reached the maximum of 25 bots")
		return
	}

	botID := models.NewULID().String()

	// Get instance_id from the creating user.
	var instanceID string
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT instance_id FROM users WHERE id = $1`, userID,
	).Scan(&instanceID); err != nil {
		h.Logger.Error("failed to get user instance_id", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create bot")
		return
	}

	// Use description as display_name if provided.
	var displayName *string
	if body.Description != "" {
		displayName = &body.Description
	}

	bot, err := scanUser(h.Pool.QueryRow(r.Context(),
		`INSERT INTO users (id, instance_id, username, display_name, bot_owner_id, flags, status_presence, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'offline', now())
		 RETURNING `+userScanColumns,
		botID, instanceID, body.Name, displayName, userID, models.UserFlagBot,
	))
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "username_taken", "A user with that name already exists")
			return
		}
		h.Logger.Error("failed to create bot", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create bot")
		return
	}

	h.Logger.Info("bot created",
		slog.String("bot_id", bot.ID),
		slog.String("owner_id", userID),
		slog.String("name", body.Name),
	)

	writeJSON(w, http.StatusCreated, bot)
}

// HandleListMyBots lists all bots owned by the current user.
// GET /api/v1/users/@me/bots
func (h *Handler) HandleListMyBots(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+userScanColumns+`
		 FROM users
		 WHERE bot_owner_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		h.Logger.Error("failed to list bots", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list bots")
		return
	}
	defer rows.Close()

	bots := []models.User{}
	for rows.Next() {
		var bot models.User
		if err := rows.Scan(
			&bot.ID, &bot.InstanceID, &bot.Username, &bot.DisplayName, &bot.AvatarID,
			&bot.StatusText, &bot.StatusEmoji, &bot.StatusPresence, &bot.StatusExpiresAt,
			&bot.Bio, &bot.BotOwnerID, &bot.Email, &bot.BannerID, &bot.AccentColor,
			&bot.Pronouns, &bot.Flags, &bot.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan bot", slog.String("error", err.Error()))
			continue
		}
		bots = append(bots, bot)
	}

	writeJSON(w, http.StatusOK, bots)
}

// HandleGetBot retrieves a bot by ID.
// GET /api/v1/bots/{botID}
func (h *Handler) HandleGetBot(w http.ResponseWriter, r *http.Request) {
	botID := chi.URLParam(r, "botID")

	bot, err := scanUser(h.Pool.QueryRow(r.Context(),
		`SELECT `+userScanColumns+`
		 FROM users
		 WHERE id = $1 AND flags & $2 != 0`,
		botID, models.UserFlagBot,
	))
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "bot_not_found", "Bot not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get bot", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bot")
		return
	}

	writeJSON(w, http.StatusOK, bot)
}

// HandleUpdateBot updates a bot's name or description. Only the bot owner can update.
// PATCH /api/v1/bots/{botID}
func (h *Handler) HandleUpdateBot(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Name != nil {
		trimmed := strings.TrimSpace(*body.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name_required", "Bot name cannot be empty")
			return
		}
		if len(trimmed) > 32 {
			writeError(w, http.StatusBadRequest, "name_too_long", "Bot name must be 32 characters or fewer")
			return
		}
		body.Name = &trimmed
	}

	// Build dynamic UPDATE.
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if body.Name != nil {
		setClauses = append(setClauses, "username = $"+itoa(argIdx))
		args = append(args, *body.Name)
		argIdx++
	}
	if body.Description != nil {
		trimmed := strings.TrimSpace(*body.Description)
		setClauses = append(setClauses, "display_name = $"+itoa(argIdx))
		args = append(args, trimmed)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no_changes", "No fields to update")
		return
	}

	args = append(args, botID)
	query := "UPDATE users SET " + strings.Join(setClauses, ", ") + " WHERE id = $" + itoa(argIdx) +
		" RETURNING " + userScanColumns

	bot, err := scanUser(h.Pool.QueryRow(r.Context(), query, args...))
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "username_taken", "A user with that name already exists")
			return
		}
		h.Logger.Error("failed to update bot", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update bot")
		return
	}

	writeJSON(w, http.StatusOK, bot)
}

// HandleDeleteBot deletes a bot account. Only the bot owner can delete.
// DELETE /api/v1/bots/{botID}
func (h *Handler) HandleDeleteBot(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	// Delete the bot user (CASCADE will remove bot_tokens and slash_commands).
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, botID); err != nil {
		h.Logger.Error("failed to delete bot", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete bot")
		return
	}

	h.Logger.Info("bot deleted",
		slog.String("bot_id", botID),
		slog.String("owner_id", userID),
	)

	writeNoContent(w)
}

// --- Token Management ---

// HandleCreateToken generates a new API token for a bot.
// The raw token is returned only once; it is stored as a SHA-256 hash.
// POST /api/v1/bots/{botID}/tokens
func (h *Handler) HandleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}
	if body.Name == "" {
		body.Name = "default"
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name_too_long", "Token name must be 64 characters or fewer")
		return
	}

	// Limit tokens per bot (max 10).
	var tokenCount int
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM bot_tokens WHERE bot_id = $1`, botID,
	).Scan(&tokenCount); err != nil {
		h.Logger.Error("failed to count tokens", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}
	if tokenCount >= 10 {
		writeError(w, http.StatusBadRequest, "token_limit", "This bot has reached the maximum of 10 tokens")
		return
	}

	raw, hash, err := generateToken()
	if err != nil {
		h.Logger.Error("failed to generate token", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}

	tokenID := models.NewULID().String()
	var token models.BotToken
	err = h.Pool.QueryRow(r.Context(),
		`INSERT INTO bot_tokens (id, bot_id, token_hash, name, created_at)
		 VALUES ($1, $2, $3, $4, now())
		 RETURNING id, bot_id, name, created_at, last_used_at`,
		tokenID, botID, hash, body.Name,
	).Scan(&token.ID, &token.BotID, &token.Name, &token.CreatedAt, &token.LastUsedAt)
	if err != nil {
		h.Logger.Error("failed to insert token", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}

	// Include the raw token in the response (only time it's visible).
	token.Token = raw

	h.Logger.Info("bot token created",
		slog.String("bot_id", botID),
		slog.String("token_id", tokenID),
	)

	writeJSON(w, http.StatusCreated, token)
}

// HandleListTokens lists all tokens for a bot (without hashes).
// GET /api/v1/bots/{botID}/tokens
func (h *Handler) HandleListTokens(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, bot_id, name, created_at, last_used_at
		 FROM bot_tokens
		 WHERE bot_id = $1
		 ORDER BY created_at DESC`, botID)
	if err != nil {
		h.Logger.Error("failed to list tokens", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tokens")
		return
	}
	defer rows.Close()

	tokens := []models.BotToken{}
	for rows.Next() {
		var t models.BotToken
		if err := rows.Scan(&t.ID, &t.BotID, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			h.Logger.Error("failed to scan token", slog.String("error", err.Error()))
			continue
		}
		tokens = append(tokens, t)
	}

	writeJSON(w, http.StatusOK, tokens)
}

// HandleDeleteToken revokes a bot token.
// DELETE /api/v1/bots/{botID}/tokens/{tokenID}
func (h *Handler) HandleDeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")
	tokenID := chi.URLParam(r, "tokenID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM bot_tokens WHERE id = $1 AND bot_id = $2`, tokenID, botID)
	if err != nil {
		h.Logger.Error("failed to delete token", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete token")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "token_not_found", "Token not found")
		return
	}

	h.Logger.Info("bot token deleted",
		slog.String("bot_id", botID),
		slog.String("token_id", tokenID),
	)

	writeNoContent(w)
}

// --- Slash Command Management ---

// HandleRegisterCommand registers a new slash command for a bot.
// POST /api/v1/bots/{botID}/commands
func (h *Handler) HandleRegisterCommand(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		GuildID     *string         `json:"guild_id"`
		Options     json.RawMessage `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Description = strings.TrimSpace(body.Description)

	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "Command name is required")
		return
	}
	if !commandNameRegex.MatchString(body.Name) {
		writeError(w, http.StatusBadRequest, "invalid_name", "Command name must be 1-32 lowercase alphanumeric characters, hyphens, or underscores")
		return
	}
	if body.Description == "" {
		writeError(w, http.StatusBadRequest, "description_required", "Command description is required")
		return
	}
	if len(body.Description) > 100 {
		writeError(w, http.StatusBadRequest, "description_too_long", "Command description must be 100 characters or fewer")
		return
	}

	if body.Options == nil {
		body.Options = json.RawMessage("[]")
	}

	commandID := models.NewULID().String()
	now := time.Now()

	var cmd models.SlashCommand
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO slash_commands (id, bot_id, guild_id, name, description, options, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		 RETURNING id, bot_id, guild_id, name, description, options, created_at, updated_at`,
		commandID, botID, body.GuildID, body.Name, body.Description, body.Options, now,
	).Scan(
		&cmd.ID, &cmd.BotID, &cmd.GuildID, &cmd.Name, &cmd.Description,
		&cmd.Options, &cmd.CreatedAt, &cmd.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "command_exists", "A command with that name already exists for this bot in the specified scope")
			return
		}
		h.Logger.Error("failed to register command", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to register command")
		return
	}

	h.Logger.Info("slash command registered",
		slog.String("bot_id", botID),
		slog.String("command_id", commandID),
		slog.String("name", body.Name),
	)

	writeJSON(w, http.StatusCreated, cmd)
}

// HandleListCommands lists all slash commands for a bot.
// GET /api/v1/bots/{botID}/commands
func (h *Handler) HandleListCommands(w http.ResponseWriter, r *http.Request) {
	botID := chi.URLParam(r, "botID")

	// Verify the bot exists and is a bot user.
	var exists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND flags & $2 != 0)`,
		botID, models.UserFlagBot,
	).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "bot_not_found", "Bot not found")
		return
	}

	// Optional guild_id filter.
	guildID := r.URL.Query().Get("guild_id")

	var rows pgx.Rows
	var err error
	if guildID != "" {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, bot_id, guild_id, name, description, options, created_at, updated_at
			 FROM slash_commands
			 WHERE bot_id = $1 AND (guild_id = $2 OR guild_id IS NULL)
			 ORDER BY name`, botID, guildID)
	} else {
		rows, err = h.Pool.Query(r.Context(),
			`SELECT id, bot_id, guild_id, name, description, options, created_at, updated_at
			 FROM slash_commands
			 WHERE bot_id = $1
			 ORDER BY name`, botID)
	}
	if err != nil {
		h.Logger.Error("failed to list commands", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list commands")
		return
	}
	defer rows.Close()

	commands := []models.SlashCommand{}
	for rows.Next() {
		var cmd models.SlashCommand
		if err := rows.Scan(
			&cmd.ID, &cmd.BotID, &cmd.GuildID, &cmd.Name, &cmd.Description,
			&cmd.Options, &cmd.CreatedAt, &cmd.UpdatedAt,
		); err != nil {
			h.Logger.Error("failed to scan command", slog.String("error", err.Error()))
			continue
		}
		commands = append(commands, cmd)
	}

	writeJSON(w, http.StatusOK, commands)
}

// HandleUpdateCommand updates a slash command.
// PATCH /api/v1/bots/{botID}/commands/{commandID}
func (h *Handler) HandleUpdateCommand(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")
	commandID := chi.URLParam(r, "commandID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Options     json.RawMessage `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Build dynamic UPDATE.
	setClauses := []string{"updated_at = now()"}
	args := []interface{}{}
	argIdx := 1

	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if !commandNameRegex.MatchString(name) {
			writeError(w, http.StatusBadRequest, "invalid_name", "Command name must be 1-32 lowercase alphanumeric characters, hyphens, or underscores")
			return
		}
		setClauses = append(setClauses, "name = $"+itoa(argIdx))
		args = append(args, name)
		argIdx++
	}
	if body.Description != nil {
		desc := strings.TrimSpace(*body.Description)
		if len(desc) > 100 {
			writeError(w, http.StatusBadRequest, "description_too_long", "Command description must be 100 characters or fewer")
			return
		}
		setClauses = append(setClauses, "description = $"+itoa(argIdx))
		args = append(args, desc)
		argIdx++
	}
	if body.Options != nil {
		setClauses = append(setClauses, "options = $"+itoa(argIdx))
		args = append(args, body.Options)
		argIdx++
	}

	if len(setClauses) == 1 {
		// Only updated_at, no actual changes.
		writeError(w, http.StatusBadRequest, "no_changes", "No fields to update")
		return
	}

	args = append(args, commandID, botID)
	query := "UPDATE slash_commands SET " + strings.Join(setClauses, ", ") +
		" WHERE id = $" + itoa(argIdx) + " AND bot_id = $" + itoa(argIdx+1) +
		" RETURNING id, bot_id, guild_id, name, description, options, created_at, updated_at"

	var cmd models.SlashCommand
	err := h.Pool.QueryRow(r.Context(), query, args...).Scan(
		&cmd.ID, &cmd.BotID, &cmd.GuildID, &cmd.Name, &cmd.Description,
		&cmd.Options, &cmd.CreatedAt, &cmd.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "command_not_found", "Command not found")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "command_exists", "A command with that name already exists")
			return
		}
		h.Logger.Error("failed to update command", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update command")
		return
	}

	writeJSON(w, http.StatusOK, cmd)
}

// HandleDeleteCommand deletes a slash command.
// DELETE /api/v1/bots/{botID}/commands/{commandID}
func (h *Handler) HandleDeleteCommand(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")
	commandID := chi.URLParam(r, "commandID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM slash_commands WHERE id = $1 AND bot_id = $2`, commandID, botID)
	if err != nil {
		h.Logger.Error("failed to delete command", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete command")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "command_not_found", "Command not found")
		return
	}

	h.Logger.Info("slash command deleted",
		slog.String("bot_id", botID),
		slog.String("command_id", commandID),
	)

	writeNoContent(w)
}

// --- Bot Guild Permissions ---

// HandleGetBotGuildPermissions returns the permission scopes a bot has within a guild.
// GET /api/v1/bots/{botID}/guilds/{guildID}/permissions
func (h *Handler) HandleGetBotGuildPermissions(w http.ResponseWriter, r *http.Request) {
	botID := chi.URLParam(r, "botID")
	guildID := chi.URLParam(r, "guildID")

	var perm models.BotGuildPermission
	err := h.Pool.QueryRow(r.Context(),
		`SELECT bot_id, guild_id, scopes, max_role_position, created_at, updated_at
		 FROM bot_guild_permissions
		 WHERE bot_id = $1 AND guild_id = $2`, botID, guildID,
	).Scan(&perm.BotID, &perm.GuildID, &perm.Scopes, &perm.MaxRolePosition, &perm.CreatedAt, &perm.UpdatedAt)

	if err == pgx.ErrNoRows {
		// Return an empty permission set rather than 404 (bot has no specific perms in this guild).
		writeJSON(w, http.StatusOK, models.BotGuildPermission{
			BotID:   botID,
			GuildID: guildID,
			Scopes:  []string{},
		})
		return
	}
	if err != nil {
		h.Logger.Error("failed to get bot guild permissions", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bot permissions")
		return
	}

	writeJSON(w, http.StatusOK, perm)
}

// HandleUpdateBotGuildPermissions creates or updates a bot's permission scopes for a guild.
// Only the bot owner or a guild admin can update.
// PUT /api/v1/bots/{botID}/guilds/{guildID}/permissions
func (h *Handler) HandleUpdateBotGuildPermissions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")
	guildID := chi.URLParam(r, "guildID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Scopes          []string `json:"scopes"`
		MaxRolePosition *int     `json:"max_role_position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate scopes.
	for _, s := range body.Scopes {
		if !models.ValidBotScopes[s] {
			writeError(w, http.StatusBadRequest, "invalid_scope", "Invalid scope: "+s)
			return
		}
	}

	maxRolePos := 0
	if body.MaxRolePosition != nil {
		if *body.MaxRolePosition < 0 {
			writeError(w, http.StatusBadRequest, "invalid_max_role_position", "max_role_position must be non-negative")
			return
		}
		maxRolePos = *body.MaxRolePosition
	}

	// Verify the guild exists.
	var guildExists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guilds WHERE id = $1)`, guildID,
	).Scan(&guildExists); err != nil || !guildExists {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}

	var perm models.BotGuildPermission
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO bot_guild_permissions (bot_id, guild_id, scopes, max_role_position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (bot_id, guild_id)
		 DO UPDATE SET scopes = EXCLUDED.scopes, max_role_position = EXCLUDED.max_role_position, updated_at = now()
		 RETURNING bot_id, guild_id, scopes, max_role_position, created_at, updated_at`,
		botID, guildID, body.Scopes, maxRolePos,
	).Scan(&perm.BotID, &perm.GuildID, &perm.Scopes, &perm.MaxRolePosition, &perm.CreatedAt, &perm.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to update bot guild permissions", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update bot permissions")
		return
	}

	h.Logger.Info("bot guild permissions updated",
		slog.String("bot_id", botID),
		slog.String("guild_id", guildID),
	)

	writeJSON(w, http.StatusOK, perm)
}

// --- Message Component Interaction ---

// HandleComponentInteraction processes a user interaction with a message component
// (button click, select menu choice). Publishes a COMPONENT_INTERACTION event so
// the bot that created the component can respond.
// POST /api/v1/channels/{channelID}/messages/{messageID}/components/{componentID}/interact
func (h *Handler) HandleComponentInteraction(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	channelID := chi.URLParam(r, "channelID")
	messageID := chi.URLParam(r, "messageID")
	componentID := chi.URLParam(r, "componentID")

	var body struct {
		Values []string `json:"values"` // For select menus, the chosen option IDs.
	}
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	// Verify the component exists and belongs to the specified message in the channel.
	var comp models.MessageComponent
	var msgAuthorID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT mc.id, mc.message_id, mc.component_type, mc.style, mc.label,
		        mc.custom_id, mc.url, mc.disabled, mc.options, mc.min_values,
		        mc.max_values, mc.placeholder, mc.position, m.author_id
		 FROM message_components mc
		 JOIN messages m ON m.id = mc.message_id
		 WHERE mc.id = $1 AND mc.message_id = $2 AND m.channel_id = $3`,
		componentID, messageID, channelID,
	).Scan(
		&comp.ID, &comp.MessageID, &comp.ComponentType, &comp.Style, &comp.Label,
		&comp.CustomID, &comp.URL, &comp.Disabled, &comp.Options, &comp.MinValues,
		&comp.MaxValues, &comp.Placeholder, &comp.Position, &msgAuthorID,
	)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "component_not_found", "Component not found")
		return
	}
	if err != nil {
		h.Logger.Error("failed to get component", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to process interaction")
		return
	}

	if comp.Disabled {
		writeError(w, http.StatusBadRequest, "component_disabled", "This component is disabled")
		return
	}

	// Publish the interaction event via NATS so the bot can handle it.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), "component_interaction", "COMPONENT_INTERACTION", map[string]interface{}{
			"component_id":   componentID,
			"message_id":     messageID,
			"channel_id":     channelID,
			"user_id":        userID,
			"custom_id":      comp.CustomID,
			"component_type": comp.ComponentType,
			"values":         body.Values,
			"bot_id":         msgAuthorID,
		})
	}

	h.Logger.Info("component interaction",
		slog.String("component_id", componentID),
		slog.String("user_id", userID),
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"acknowledged": true,
	})
}

// --- Bot Presence ---

// HandleGetBotPresence returns the current presence/status of a bot.
// GET /api/v1/bots/{botID}/presence
func (h *Handler) HandleGetBotPresence(w http.ResponseWriter, r *http.Request) {
	botID := chi.URLParam(r, "botID")

	var pres models.BotPresence
	err := h.Pool.QueryRow(r.Context(),
		`SELECT bot_id, status, activity_type, activity_name, updated_at
		 FROM bot_presence
		 WHERE bot_id = $1`, botID,
	).Scan(&pres.BotID, &pres.Status, &pres.ActivityType, &pres.ActivityName, &pres.UpdatedAt)

	if err == pgx.ErrNoRows {
		// Return a default offline presence rather than 404.
		writeJSON(w, http.StatusOK, models.BotPresence{
			BotID:  botID,
			Status: "offline",
		})
		return
	}
	if err != nil {
		h.Logger.Error("failed to get bot presence", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get bot presence")
		return
	}

	writeJSON(w, http.StatusOK, pres)
}

// HandleUpdateBotPresence sets the bot's presence/status. Only the bot owner can update.
// PUT /api/v1/bots/{botID}/presence
func (h *Handler) HandleUpdateBotPresence(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		Status       string  `json:"status"`
		ActivityType *string `json:"activity_type"`
		ActivityName *string `json:"activity_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Validate status.
	validStatuses := map[string]bool{
		"online": true, "idle": true, "dnd": true, "offline": true,
	}
	if body.Status == "" {
		body.Status = "online"
	}
	if !validStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid_status", "Status must be one of: online, idle, dnd, offline")
		return
	}

	// Validate activity_type if provided.
	if body.ActivityType != nil {
		validTypes := map[string]bool{
			"playing": true, "listening": true, "watching": true, "competing": true, "custom": true,
		}
		if !validTypes[*body.ActivityType] {
			writeError(w, http.StatusBadRequest, "invalid_activity_type", "Activity type must be one of: playing, listening, watching, competing, custom")
			return
		}
	}

	var pres models.BotPresence
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO bot_presence (bot_id, status, activity_type, activity_name, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (bot_id)
		 DO UPDATE SET status = EXCLUDED.status, activity_type = EXCLUDED.activity_type,
		               activity_name = EXCLUDED.activity_name, updated_at = now()
		 RETURNING bot_id, status, activity_type, activity_name, updated_at`,
		botID, body.Status, body.ActivityType, body.ActivityName,
	).Scan(&pres.BotID, &pres.Status, &pres.ActivityType, &pres.ActivityName, &pres.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to update bot presence", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update bot presence")
		return
	}

	// Publish presence update via NATS.
	if h.EventBus != nil {
		h.EventBus.PublishJSON(r.Context(), "bot_presence_update", "BOT_PRESENCE_UPDATE", map[string]interface{}{
			"bot_id":        botID,
			"status":        pres.Status,
			"activity_type": pres.ActivityType,
			"activity_name": pres.ActivityName,
		})
	}

	h.Logger.Info("bot presence updated",
		slog.String("bot_id", botID),
		slog.String("status", body.Status),
	)

	writeJSON(w, http.StatusOK, pres)
}

// --- Bot Event Subscriptions ---

// HandleCreateEventSubscription creates a new per-guild event subscription for a bot.
// POST /api/v1/bots/{botID}/event-subscriptions
func (h *Handler) HandleCreateEventSubscription(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		GuildID    string   `json:"guild_id"`
		EventTypes []string `json:"event_types"`
		WebhookURL string   `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.GuildID == "" {
		writeError(w, http.StatusBadRequest, "guild_id_required", "guild_id is required")
		return
	}
	if len(body.EventTypes) == 0 {
		writeError(w, http.StatusBadRequest, "event_types_required", "At least one event type is required")
		return
	}
	if body.WebhookURL == "" {
		writeError(w, http.StatusBadRequest, "webhook_url_required", "webhook_url is required")
		return
	}
	if !strings.HasPrefix(body.WebhookURL, "https://") {
		writeError(w, http.StatusBadRequest, "webhook_url_https", "webhook_url must use HTTPS")
		return
	}
	if len(body.WebhookURL) > 2048 {
		writeError(w, http.StatusBadRequest, "webhook_url_too_long", "webhook_url must be 2048 characters or fewer")
		return
	}

	// Validate event types.
	validEventTypes := map[string]bool{
		"message_create":  true,
		"message_update":  true,
		"message_delete":  true,
		"member_join":     true,
		"member_leave":    true,
		"member_update":   true,
		"channel_create":  true,
		"channel_update":  true,
		"channel_delete":  true,
		"role_create":     true,
		"role_update":     true,
		"role_delete":     true,
		"guild_update":    true,
		"reaction_add":    true,
		"reaction_remove": true,
	}
	for _, t := range body.EventTypes {
		if !validEventTypes[t] {
			writeError(w, http.StatusBadRequest, "invalid_event_type", "Invalid event type: "+t)
			return
		}
	}

	// Verify guild exists.
	var guildExists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM guilds WHERE id = $1)`, body.GuildID,
	).Scan(&guildExists); err != nil || !guildExists {
		writeError(w, http.StatusNotFound, "guild_not_found", "Guild not found")
		return
	}

	// Limit subscriptions per bot (max 50).
	var subCount int
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM bot_event_subscriptions WHERE bot_id = $1`, botID,
	).Scan(&subCount); err != nil {
		h.Logger.Error("failed to count event subscriptions", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create subscription")
		return
	}
	if subCount >= 50 {
		writeError(w, http.StatusBadRequest, "subscription_limit", "This bot has reached the maximum of 50 event subscriptions")
		return
	}

	subID := models.NewULID().String()
	var sub models.BotEventSubscription
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO bot_event_subscriptions (id, bot_id, guild_id, event_types, webhook_url, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 RETURNING id, bot_id, guild_id, event_types, webhook_url, created_at`,
		subID, botID, body.GuildID, body.EventTypes, body.WebhookURL,
	).Scan(&sub.ID, &sub.BotID, &sub.GuildID, &sub.EventTypes, &sub.WebhookURL, &sub.CreatedAt)
	if err != nil {
		h.Logger.Error("failed to create event subscription", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create subscription")
		return
	}

	h.Logger.Info("bot event subscription created",
		slog.String("bot_id", botID),
		slog.String("subscription_id", subID),
		slog.String("guild_id", body.GuildID),
	)

	writeJSON(w, http.StatusCreated, sub)
}

// HandleListEventSubscriptions lists all event subscriptions for a bot.
// GET /api/v1/bots/{botID}/event-subscriptions
func (h *Handler) HandleListEventSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, bot_id, guild_id, event_types, webhook_url, created_at
		 FROM bot_event_subscriptions
		 WHERE bot_id = $1
		 ORDER BY created_at DESC`, botID)
	if err != nil {
		h.Logger.Error("failed to list event subscriptions", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list subscriptions")
		return
	}
	defer rows.Close()

	subs := []models.BotEventSubscription{}
	for rows.Next() {
		var sub models.BotEventSubscription
		if err := rows.Scan(&sub.ID, &sub.BotID, &sub.GuildID, &sub.EventTypes, &sub.WebhookURL, &sub.CreatedAt); err != nil {
			h.Logger.Error("failed to scan event subscription", slog.String("error", err.Error()))
			continue
		}
		subs = append(subs, sub)
	}

	writeJSON(w, http.StatusOK, subs)
}

// HandleDeleteEventSubscription deletes an event subscription.
// DELETE /api/v1/bots/{botID}/event-subscriptions/{subscriptionID}
func (h *Handler) HandleDeleteEventSubscription(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")
	subscriptionID := chi.URLParam(r, "subscriptionID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	tag, err := h.Pool.Exec(r.Context(),
		`DELETE FROM bot_event_subscriptions WHERE id = $1 AND bot_id = $2`,
		subscriptionID, botID)
	if err != nil {
		h.Logger.Error("failed to delete event subscription", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete subscription")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "subscription_not_found", "Subscription not found")
		return
	}

	h.Logger.Info("bot event subscription deleted",
		slog.String("bot_id", botID),
		slog.String("subscription_id", subscriptionID),
	)

	writeNoContent(w)
}

// --- Bot Rate Limits ---

// HandleGetBotRateLimit returns the configured rate limit for a bot.
// GET /api/v1/bots/{botID}/rate-limit
func (h *Handler) HandleGetBotRateLimit(w http.ResponseWriter, r *http.Request) {
	botID := chi.URLParam(r, "botID")

	var rl models.BotRateLimit
	err := h.Pool.QueryRow(r.Context(),
		`SELECT bot_id, requests_per_second, burst, updated_at
		 FROM bot_rate_limits
		 WHERE bot_id = $1`, botID,
	).Scan(&rl.BotID, &rl.RequestsPerSecond, &rl.Burst, &rl.UpdatedAt)

	if err == pgx.ErrNoRows {
		// Return the default rate limit.
		writeJSON(w, http.StatusOK, models.BotRateLimit{
			BotID:             botID,
			RequestsPerSecond: 50,
			Burst:             100,
		})
		return
	}
	if err != nil {
		h.Logger.Error("failed to get bot rate limit", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get rate limit")
		return
	}

	writeJSON(w, http.StatusOK, rl)
}

// HandleUpdateBotRateLimit updates the rate limit configuration for a bot.
// Only the bot owner can update.
// PUT /api/v1/bots/{botID}/rate-limit
func (h *Handler) HandleUpdateBotRateLimit(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	botID := chi.URLParam(r, "botID")

	if !h.verifyBotOwnership(w, r, botID, userID) {
		return
	}

	var body struct {
		RequestsPerSecond *int `json:"requests_per_second"`
		Burst             *int `json:"burst"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	rps := 50
	burst := 100
	if body.RequestsPerSecond != nil {
		if *body.RequestsPerSecond < 1 || *body.RequestsPerSecond > 1000 {
			writeError(w, http.StatusBadRequest, "invalid_rps", "requests_per_second must be between 1 and 1000")
			return
		}
		rps = *body.RequestsPerSecond
	}
	if body.Burst != nil {
		if *body.Burst < 1 || *body.Burst > 5000 {
			writeError(w, http.StatusBadRequest, "invalid_burst", "burst must be between 1 and 5000")
			return
		}
		burst = *body.Burst
	}

	var rl models.BotRateLimit
	err := h.Pool.QueryRow(r.Context(),
		`INSERT INTO bot_rate_limits (bot_id, requests_per_second, burst, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (bot_id)
		 DO UPDATE SET requests_per_second = EXCLUDED.requests_per_second,
		               burst = EXCLUDED.burst, updated_at = now()
		 RETURNING bot_id, requests_per_second, burst, updated_at`,
		botID, rps, burst,
	).Scan(&rl.BotID, &rl.RequestsPerSecond, &rl.Burst, &rl.UpdatedAt)
	if err != nil {
		h.Logger.Error("failed to update bot rate limit", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update rate limit")
		return
	}

	h.Logger.Info("bot rate limit updated",
		slog.String("bot_id", botID),
	)

	writeJSON(w, http.StatusOK, rl)
}

// --- Admin Bot Management ---

// HandleAdminListAllBots lists all bot accounts on the instance. Admin only.
// GET /api/v1/admin/bots
func (h *Handler) HandleAdminListAllBots(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+userScanColumns+`
		 FROM users
		 WHERE flags & $1 != 0
		 ORDER BY created_at DESC`, models.UserFlagBot)
	if err != nil {
		h.Logger.Error("failed to list all bots", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list bots")
		return
	}
	defer rows.Close()

	type BotWithDetails struct {
		models.User
		GuildPermissions   []models.BotGuildPermission    `json:"guild_permissions"`
		EventSubscriptions []models.BotEventSubscription  `json:"event_subscriptions"`
		RateLimit          *models.BotRateLimit           `json:"rate_limit,omitempty"`
		Presence           *models.BotPresence            `json:"presence,omitempty"`
	}

	bots := []BotWithDetails{}
	botIDs := []string{}
	for rows.Next() {
		var bot models.User
		if err := rows.Scan(
			&bot.ID, &bot.InstanceID, &bot.Username, &bot.DisplayName, &bot.AvatarID,
			&bot.StatusText, &bot.StatusEmoji, &bot.StatusPresence, &bot.StatusExpiresAt,
			&bot.Bio, &bot.BotOwnerID, &bot.Email, &bot.BannerID, &bot.AccentColor,
			&bot.Pronouns, &bot.Flags, &bot.CreatedAt,
		); err != nil {
			h.Logger.Error("failed to scan bot", slog.String("error", err.Error()))
			continue
		}
		bots = append(bots, BotWithDetails{
			User:               bot,
			GuildPermissions:   []models.BotGuildPermission{},
			EventSubscriptions: []models.BotEventSubscription{},
		})
		botIDs = append(botIDs, bot.ID)
	}

	if len(botIDs) == 0 {
		writeJSON(w, http.StatusOK, bots)
		return
	}

	// Batch-load guild permissions for all bots.
	permRows, err := h.Pool.Query(r.Context(),
		`SELECT bot_id, guild_id, scopes, max_role_position, created_at, updated_at
		 FROM bot_guild_permissions
		 WHERE bot_id = ANY($1)`, botIDs)
	if err == nil {
		defer permRows.Close()
		permMap := map[string][]models.BotGuildPermission{}
		for permRows.Next() {
			var p models.BotGuildPermission
			if err := permRows.Scan(&p.BotID, &p.GuildID, &p.Scopes, &p.MaxRolePosition, &p.CreatedAt, &p.UpdatedAt); err != nil {
				continue
			}
			permMap[p.BotID] = append(permMap[p.BotID], p)
		}
		for i := range bots {
			if perms, ok := permMap[bots[i].ID]; ok {
				bots[i].GuildPermissions = perms
			}
		}
	}

	// Batch-load event subscriptions for all bots.
	subRows, err := h.Pool.Query(r.Context(),
		`SELECT id, bot_id, guild_id, event_types, webhook_url, created_at
		 FROM bot_event_subscriptions
		 WHERE bot_id = ANY($1)`, botIDs)
	if err == nil {
		defer subRows.Close()
		subMap := map[string][]models.BotEventSubscription{}
		for subRows.Next() {
			var s models.BotEventSubscription
			if err := subRows.Scan(&s.ID, &s.BotID, &s.GuildID, &s.EventTypes, &s.WebhookURL, &s.CreatedAt); err != nil {
				continue
			}
			subMap[s.BotID] = append(subMap[s.BotID], s)
		}
		for i := range bots {
			if subs, ok := subMap[bots[i].ID]; ok {
				bots[i].EventSubscriptions = subs
			}
		}
	}

	// Batch-load rate limits for all bots.
	rlRows, err := h.Pool.Query(r.Context(),
		`SELECT bot_id, requests_per_second, burst, updated_at
		 FROM bot_rate_limits
		 WHERE bot_id = ANY($1)`, botIDs)
	if err == nil {
		defer rlRows.Close()
		rlMap := map[string]*models.BotRateLimit{}
		for rlRows.Next() {
			var rl models.BotRateLimit
			if err := rlRows.Scan(&rl.BotID, &rl.RequestsPerSecond, &rl.Burst, &rl.UpdatedAt); err != nil {
				continue
			}
			rlMap[rl.BotID] = &rl
		}
		for i := range bots {
			if rl, ok := rlMap[bots[i].ID]; ok {
				bots[i].RateLimit = rl
			}
		}
	}

	// Batch-load presence for all bots.
	presRows, err := h.Pool.Query(r.Context(),
		`SELECT bot_id, status, activity_type, activity_name, updated_at
		 FROM bot_presence
		 WHERE bot_id = ANY($1)`, botIDs)
	if err == nil {
		defer presRows.Close()
		presMap := map[string]*models.BotPresence{}
		for presRows.Next() {
			var p models.BotPresence
			if err := presRows.Scan(&p.BotID, &p.Status, &p.ActivityType, &p.ActivityName, &p.UpdatedAt); err != nil {
				continue
			}
			presMap[p.BotID] = &p
		}
		for i := range bots {
			if pres, ok := presMap[bots[i].ID]; ok {
				bots[i].Presence = pres
			}
		}
	}

	writeJSON(w, http.StatusOK, bots)
}
