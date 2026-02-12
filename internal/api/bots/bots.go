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
