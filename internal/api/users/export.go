// Package users — data portability handlers for GDPR data export, channel
// message archive, and account migration (import/export). These endpoints let
// users download their data, export channel archives, and migrate profiles
// between AmityVox instances.
package users

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/permissions"
)

// --- Rate limiting for data exports ---

// exportCooldowns tracks when each user last requested a data export.
// Map key is user ID, value is the timestamp of the last export.
var exportCooldowns = make(map[string]time.Time)

const exportCooldownDuration = 24 * time.Hour

// --- GDPR Data Export ---

// userDataExport represents the full GDPR data export for a user.
type userDataExport struct {
	ExportedAt   time.Time              `json:"exported_at"`
	User         userExportProfile      `json:"user"`
	Settings     json.RawMessage        `json:"settings"`
	Guilds       []userExportGuild      `json:"guilds"`
	Messages     []userExportMessage    `json:"messages"`
	Bookmarks    []userExportBookmark   `json:"bookmarks"`
	Reactions    []userExportReaction   `json:"reactions"`
	ReadStates   []userExportReadState  `json:"read_states"`
	Relationships []userExportRelation  `json:"relationships"`
}

type userExportProfile struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	Email          *string `json:"email,omitempty"`
	Bio            *string `json:"bio,omitempty"`
	StatusText     *string `json:"status_text,omitempty"`
	StatusEmoji    *string `json:"status_emoji,omitempty"`
	StatusPresence string  `json:"status_presence"`
	Pronouns       *string `json:"pronouns,omitempty"`
	AccentColor    *string `json:"accent_color,omitempty"`
	Flags          int     `json:"flags"`
	CreatedAt      string  `json:"created_at"`
}

type userExportGuild struct {
	GuildID   string  `json:"guild_id"`
	GuildName string  `json:"guild_name"`
	Nickname  *string `json:"nickname,omitempty"`
	JoinedAt  string  `json:"joined_at"`
	Roles     []string `json:"roles,omitempty"`
}

type userExportMessage struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channel_id"`
	Content   *string `json:"content,omitempty"`
	Type      string  `json:"message_type"`
	CreatedAt string  `json:"created_at"`
	EditedAt  *string `json:"edited_at,omitempty"`
}

type userExportBookmark struct {
	MessageID string  `json:"message_id"`
	Note      *string `json:"note,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type userExportReaction struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
	CreatedAt string `json:"created_at"`
}

type userExportReadState struct {
	ChannelID    string  `json:"channel_id"`
	LastReadID   *string `json:"last_read_id,omitempty"`
	MentionCount int     `json:"mention_count"`
}

type userExportRelation struct {
	TargetID  string `json:"target_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// HandleExportUserData collects all data associated with the authenticated user
// and returns it as a structured JSON document. This fulfills GDPR data
// portability requirements. Rate limited to one export per 24 hours.
// GET /api/v1/users/@me/export
func (h *Handler) HandleExportUserData(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Check rate limit.
	if lastExport, ok := exportCooldowns[userID]; ok {
		if time.Since(lastExport) < exportCooldownDuration {
			remaining := exportCooldownDuration - time.Since(lastExport)
			writeError(w, http.StatusTooManyRequests, "rate_limited",
				fmt.Sprintf("Data export is limited to once per 24 hours. Try again in %s.", remaining.Round(time.Minute).String()))
			return
		}
	}

	ctx := r.Context()
	export := userDataExport{
		ExportedAt: time.Now().UTC(),
	}

	// 1. User profile
	profile, err := h.collectUserProfile(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect user profile", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export user data")
		return
	}
	export.User = profile

	// 2. User settings
	settings, err := h.collectUserSettings(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect user settings", slog.String("error", err.Error()))
		// Not fatal -- proceed with empty settings.
		export.Settings = json.RawMessage("{}")
	} else {
		export.Settings = settings
	}

	// 3. Guild memberships
	guilds, err := h.collectGuildMemberships(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect guild memberships", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export user data")
		return
	}
	export.Guilds = guilds

	// 4. Messages (last 10000)
	messages, err := h.collectMessages(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect messages", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export user data")
		return
	}
	export.Messages = messages

	// 5. Bookmarks
	bookmarks, err := h.collectBookmarks(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect bookmarks", slog.String("error", err.Error()))
		// Not fatal.
		export.Bookmarks = []userExportBookmark{}
	} else {
		export.Bookmarks = bookmarks
	}

	// 6. Reactions
	reactions, err := h.collectReactions(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect reactions", slog.String("error", err.Error()))
		export.Reactions = []userExportReaction{}
	} else {
		export.Reactions = reactions
	}

	// 7. Read states
	readStates, err := h.collectReadStates(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect read states", slog.String("error", err.Error()))
		export.ReadStates = []userExportReadState{}
	} else {
		export.ReadStates = readStates
	}

	// 8. Relationships
	relationships, err := h.collectRelationships(ctx, userID)
	if err != nil {
		h.Logger.Error("export: failed to collect relationships", slog.String("error", err.Error()))
		export.Relationships = []userExportRelation{}
	} else {
		export.Relationships = relationships
	}

	// Record cooldown.
	exportCooldowns[userID] = time.Now()

	writeJSON(w, http.StatusOK, export)
}

// --- Channel Message Archive Export ---

// channelMessageExport represents a full archive of messages in a channel.
type channelMessageExport struct {
	ChannelID  string                   `json:"channel_id"`
	ExportedAt time.Time                `json:"exported_at"`
	ExportedBy string                   `json:"exported_by"`
	Messages   []channelExportMessage   `json:"messages"`
}

type channelExportMessage struct {
	ID          string                    `json:"id"`
	AuthorID    string                    `json:"author_id"`
	AuthorName  string                    `json:"author_name"`
	Content     *string                   `json:"content,omitempty"`
	MessageType string                    `json:"message_type"`
	Attachments []channelExportAttachment `json:"attachments,omitempty"`
	Reactions   []channelExportReaction   `json:"reactions,omitempty"`
	EditedAt    *string                   `json:"edited_at,omitempty"`
	CreatedAt   string                    `json:"created_at"`
}

type channelExportAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type channelExportReaction struct {
	Emoji string   `json:"emoji"`
	Users []string `json:"user_ids"`
	Count int      `json:"count"`
}

// HandleExportChannelMessages exports all messages in a channel as JSON.
// Requires MANAGE_CHANNELS permission on the channel's guild.
// GET /api/v1/channels/{channelID}/export
func (h *Handler) HandleExportChannelMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "missing_channel_id", "Channel ID is required")
		return
	}

	ctx := r.Context()

	// Get channel to check guild membership and permissions.
	var guildID *string
	err := h.Pool.QueryRow(ctx, `SELECT guild_id FROM channels WHERE id = $1`, channelID).Scan(&guildID)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "channel_not_found", "Channel not found")
		return
	}
	if err != nil {
		h.Logger.Error("export: failed to get channel", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get channel")
		return
	}

	// Check permissions: guild channels need MANAGE_CHANNELS, DM channels need to be a participant.
	if guildID != nil {
		if !h.hasChannelPermission(ctx, *guildID, channelID, userID, permissions.ManageChannels) {
			writeError(w, http.StatusForbidden, "missing_permission", "You need MANAGE_CHANNELS permission to export messages")
			return
		}
	} else {
		// DM channel: check if user is a participant.
		var isParticipant bool
		h.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM channel_recipients WHERE channel_id = $1 AND user_id = $2)`,
			channelID, userID,
		).Scan(&isParticipant)
		if !isParticipant {
			writeError(w, http.StatusForbidden, "not_participant", "You are not a participant in this channel")
			return
		}
	}

	export := channelMessageExport{
		ChannelID:  channelID,
		ExportedAt: time.Now().UTC(),
		ExportedBy: userID,
	}

	// Fetch all messages with author usernames.
	rows, err := h.Pool.Query(ctx,
		`SELECT m.id, m.author_id, COALESCE(u.display_name, u.username), m.content,
		        m.message_type, m.edited_at, m.created_at
		 FROM messages m
		 LEFT JOIN users u ON u.id = m.author_id
		 WHERE m.channel_id = $1
		 ORDER BY m.created_at ASC`,
		channelID,
	)
	if err != nil {
		h.Logger.Error("export: failed to query messages", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export messages")
		return
	}
	defer rows.Close()

	var messageIDs []string
	messages := make([]channelExportMessage, 0)
	for rows.Next() {
		var msg channelExportMessage
		var editedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&msg.ID, &msg.AuthorID, &msg.AuthorName, &msg.Content,
			&msg.MessageType, &editedAt, &createdAt); err != nil {
			h.Logger.Error("export: failed to scan message", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read messages")
			return
		}
		msg.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if editedAt != nil {
			s := editedAt.UTC().Format(time.RFC3339)
			msg.EditedAt = &s
		}
		messageIDs = append(messageIDs, msg.ID)
		messages = append(messages, msg)
	}

	// Batch-load attachments for all messages.
	if len(messageIDs) > 0 {
		attachmentMap := make(map[string][]channelExportAttachment)
		aRows, err := h.Pool.Query(ctx,
			`SELECT message_id, id, filename, content_type, size_bytes
			 FROM attachments
			 WHERE message_id = ANY($1)`,
			messageIDs,
		)
		if err == nil {
			defer aRows.Close()
			for aRows.Next() {
				var msgID string
				var att channelExportAttachment
				if err := aRows.Scan(&msgID, &att.ID, &att.Filename, &att.ContentType, &att.SizeBytes); err == nil {
					attachmentMap[msgID] = append(attachmentMap[msgID], att)
				}
			}
		}

		// Batch-load reactions for all messages.
		reactionMap := make(map[string]map[string][]string) // messageID -> emoji -> []userID
		rRows, err := h.Pool.Query(ctx,
			`SELECT message_id, emoji, user_id
			 FROM reactions
			 WHERE message_id = ANY($1)
			 ORDER BY message_id, emoji`,
			messageIDs,
		)
		if err == nil {
			defer rRows.Close()
			for rRows.Next() {
				var msgID, emoji, uid string
				if err := rRows.Scan(&msgID, &emoji, &uid); err == nil {
					if reactionMap[msgID] == nil {
						reactionMap[msgID] = make(map[string][]string)
					}
					reactionMap[msgID][emoji] = append(reactionMap[msgID][emoji], uid)
				}
			}
		}

		// Attach loaded data to messages.
		for i := range messages {
			if atts, ok := attachmentMap[messages[i].ID]; ok {
				messages[i].Attachments = atts
			}
			if reactions, ok := reactionMap[messages[i].ID]; ok {
				for emoji, users := range reactions {
					messages[i].Reactions = append(messages[i].Reactions, channelExportReaction{
						Emoji: emoji,
						Users: users,
						Count: len(users),
					})
				}
			}
		}
	}

	export.Messages = messages
	writeJSON(w, http.StatusOK, export)
}

// --- Account Migration Export/Import ---

// accountExport represents a portable account snapshot for migration between instances.
type accountExport struct {
	Version    int                    `json:"version"`
	ExportedAt time.Time              `json:"exported_at"`
	Profile    accountExportProfile   `json:"profile"`
	Settings   json.RawMessage        `json:"settings"`
}

type accountExportProfile struct {
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	Bio            *string `json:"bio,omitempty"`
	StatusText     *string `json:"status_text,omitempty"`
	StatusEmoji    *string `json:"status_emoji,omitempty"`
	StatusPresence string  `json:"status_presence"`
	Pronouns       *string `json:"pronouns,omitempty"`
	AccentColor    *string `json:"accent_color,omitempty"`
}

// HandleExportAccount exports a portable account snapshot for migration.
// This includes the user's profile and settings but NOT messages (those belong
// to the instance). The exported file can be imported on another instance.
// GET /api/v1/users/@me/export-account
func (h *Handler) HandleExportAccount(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	ctx := r.Context()

	var username string
	var displayName, bio, statusText, statusEmoji, statusPresence, pronouns, accentColor *string
	var presenceStr string
	err := h.Pool.QueryRow(ctx,
		`SELECT username, display_name, bio, status_text, status_emoji, status_presence,
		        pronouns, accent_color
		 FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &bio, &statusText, &statusEmoji, &presenceStr,
		&pronouns, &accentColor)
	if err != nil {
		h.Logger.Error("export account: failed to get user", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export account")
		return
	}
	statusPresence = &presenceStr

	// Get settings.
	var settings json.RawMessage
	err = h.Pool.QueryRow(ctx,
		`SELECT settings FROM user_settings WHERE user_id = $1`, userID,
	).Scan(&settings)
	if err != nil {
		settings = json.RawMessage("{}")
	}

	export := accountExport{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Profile: accountExportProfile{
			Username:       username,
			DisplayName:    displayName,
			Bio:            bio,
			StatusText:     statusText,
			StatusEmoji:    statusEmoji,
			StatusPresence: *statusPresence,
			Pronouns:       pronouns,
			AccentColor:    accentColor,
		},
		Settings: settings,
	}

	writeJSON(w, http.StatusOK, export)
}

// importAccountRequest is the JSON body for POST /users/@me/import-account.
type importAccountRequest struct {
	Profile  *importAccountProfile `json:"profile"`
	Settings json.RawMessage       `json:"settings"`
}

type importAccountProfile struct {
	DisplayName    *string `json:"display_name"`
	Bio            *string `json:"bio"`
	StatusText     *string `json:"status_text"`
	StatusEmoji    *string `json:"status_emoji"`
	StatusPresence *string `json:"status_presence"`
	Pronouns       *string `json:"pronouns"`
	AccentColor    *string `json:"accent_color"`
}

// HandleImportAccount applies imported profile data from another instance.
// Updates the user's display name, bio, status, pronouns, accent color, and settings.
// Does NOT change username, email, or password.
// POST /api/v1/users/@me/import-account
func (h *Handler) HandleImportAccount(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req importAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	ctx := r.Context()

	// Apply profile changes if provided.
	if req.Profile != nil {
		p := req.Profile

		// Validate field lengths.
		if p.DisplayName != nil && len(*p.DisplayName) > 32 {
			writeError(w, http.StatusBadRequest, "invalid_display_name", "Display name must be at most 32 characters")
			return
		}
		if p.Bio != nil && len(*p.Bio) > 2000 {
			writeError(w, http.StatusBadRequest, "invalid_bio", "Bio must be at most 2000 characters")
			return
		}
		if p.StatusText != nil && len(*p.StatusText) > 128 {
			writeError(w, http.StatusBadRequest, "invalid_status", "Status text must be at most 128 characters")
			return
		}
		if p.StatusPresence != nil {
			valid := map[string]bool{"online": true, "idle": true, "focus": true, "busy": true, "dnd": true, "invisible": true, "offline": true}
			if !valid[*p.StatusPresence] {
				writeError(w, http.StatusBadRequest, "invalid_presence", "Invalid status presence value")
				return
			}
		}
		if p.Pronouns != nil && len(*p.Pronouns) > 40 {
			writeError(w, http.StatusBadRequest, "invalid_pronouns", "Pronouns must be at most 40 characters")
			return
		}
		if p.AccentColor != nil && len(*p.AccentColor) > 7 {
			writeError(w, http.StatusBadRequest, "invalid_accent_color", "Accent color must be a hex color (e.g. #FF5500)")
			return
		}

		_, err := h.Pool.Exec(ctx,
			`UPDATE users SET
				display_name = COALESCE($2, display_name),
				bio = COALESCE($3, bio),
				status_text = COALESCE($4, status_text),
				status_emoji = COALESCE($5, status_emoji),
				status_presence = COALESCE($6, status_presence),
				pronouns = COALESCE($7, pronouns),
				accent_color = COALESCE($8, accent_color)
			 WHERE id = $1`,
			userID, p.DisplayName, p.Bio, p.StatusText, p.StatusEmoji,
			p.StatusPresence, p.Pronouns, p.AccentColor,
		)
		if err != nil {
			h.Logger.Error("import account: failed to update profile", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to import profile")
			return
		}
	}

	// Apply settings if provided.
	if len(req.Settings) > 0 && string(req.Settings) != "null" && string(req.Settings) != "{}" {
		_, err := h.Pool.Exec(ctx,
			`INSERT INTO user_settings (user_id, settings, updated_at)
			 VALUES ($1, $2, now())
			 ON CONFLICT (user_id) DO UPDATE SET settings = $2, updated_at = now()`,
			userID, req.Settings,
		)
		if err != nil {
			h.Logger.Error("import account: failed to update settings", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to import settings")
			return
		}
	}

	// Return the updated user profile.
	user, err := h.getUser(ctx, userID)
	if err != nil {
		h.Logger.Error("import account: failed to get updated user", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get updated profile")
		return
	}

	writeJSON(w, http.StatusOK, user.ToSelf())
}

// --- Internal data collection helpers ---

func (h *Handler) collectUserProfile(ctx context.Context, userID string) (userExportProfile, error) {
	var p userExportProfile
	var createdAt time.Time
	err := h.Pool.QueryRow(ctx,
		`SELECT id, username, display_name, email, bio, status_text, status_emoji,
		        status_presence, pronouns, accent_color, flags, created_at
		 FROM users WHERE id = $1`, userID,
	).Scan(&p.ID, &p.Username, &p.DisplayName, &p.Email, &p.Bio, &p.StatusText,
		&p.StatusEmoji, &p.StatusPresence, &p.Pronouns, &p.AccentColor, &p.Flags, &createdAt)
	if err != nil {
		return p, fmt.Errorf("querying user profile: %w", err)
	}
	p.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return p, nil
}

func (h *Handler) collectUserSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	var settings json.RawMessage
	err := h.Pool.QueryRow(ctx,
		`SELECT settings FROM user_settings WHERE user_id = $1`, userID,
	).Scan(&settings)
	if err == pgx.ErrNoRows {
		return json.RawMessage("{}"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying user settings: %w", err)
	}
	return settings, nil
}

func (h *Handler) collectGuildMemberships(ctx context.Context, userID string) ([]userExportGuild, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT g.id, g.name, gm.nickname, gm.joined_at
		 FROM guild_members gm
		 JOIN guilds g ON g.id = gm.guild_id
		 WHERE gm.user_id = $1
		 ORDER BY gm.joined_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying guild memberships: %w", err)
	}
	defer rows.Close()

	guilds := make([]userExportGuild, 0)
	for rows.Next() {
		var g userExportGuild
		var joinedAt time.Time
		if err := rows.Scan(&g.GuildID, &g.GuildName, &g.Nickname, &joinedAt); err != nil {
			return nil, fmt.Errorf("scanning guild membership: %w", err)
		}
		g.JoinedAt = joinedAt.UTC().Format(time.RFC3339)

		// Batch-load roles for each guild membership.
		rRows, err := h.Pool.Query(ctx,
			`SELECT r.name FROM roles r
			 JOIN member_roles mr ON r.id = mr.role_id
			 WHERE mr.guild_id = $1 AND mr.user_id = $2
			 ORDER BY r.position DESC`,
			g.GuildID, userID,
		)
		if err == nil {
			defer rRows.Close()
			for rRows.Next() {
				var roleName string
				if err := rRows.Scan(&roleName); err == nil {
					g.Roles = append(g.Roles, roleName)
				}
			}
		}

		guilds = append(guilds, g)
	}
	return guilds, nil
}

func (h *Handler) collectMessages(ctx context.Context, userID string) ([]userExportMessage, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT id, channel_id, content, message_type, edited_at, created_at
		 FROM messages
		 WHERE author_id = $1
		 ORDER BY created_at DESC
		 LIMIT 10000`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	messages := make([]userExportMessage, 0)
	for rows.Next() {
		var m userExportMessage
		var editedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.Content, &m.Type, &editedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		m.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if editedAt != nil {
			s := editedAt.UTC().Format(time.RFC3339)
			m.EditedAt = &s
		}
		messages = append(messages, m)
	}
	return messages, nil
}

func (h *Handler) collectBookmarks(ctx context.Context, userID string) ([]userExportBookmark, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT message_id, note, created_at
		 FROM message_bookmarks
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying bookmarks: %w", err)
	}
	defer rows.Close()

	bookmarks := make([]userExportBookmark, 0)
	for rows.Next() {
		var b userExportBookmark
		var createdAt time.Time
		if err := rows.Scan(&b.MessageID, &b.Note, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning bookmark: %w", err)
		}
		b.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, nil
}

func (h *Handler) collectReactions(ctx context.Context, userID string) ([]userExportReaction, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT message_id, emoji, created_at
		 FROM reactions
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 10000`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying reactions: %w", err)
	}
	defer rows.Close()

	reactions := make([]userExportReaction, 0)
	for rows.Next() {
		var r userExportReaction
		var createdAt time.Time
		if err := rows.Scan(&r.MessageID, &r.Emoji, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning reaction: %w", err)
		}
		r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		reactions = append(reactions, r)
	}
	return reactions, nil
}

func (h *Handler) collectReadStates(ctx context.Context, userID string) ([]userExportReadState, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT channel_id, last_read_id, mention_count
		 FROM read_state
		 WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying read states: %w", err)
	}
	defer rows.Close()

	states := make([]userExportReadState, 0)
	for rows.Next() {
		var rs userExportReadState
		if err := rows.Scan(&rs.ChannelID, &rs.LastReadID, &rs.MentionCount); err != nil {
			return nil, fmt.Errorf("scanning read state: %w", err)
		}
		states = append(states, rs)
	}
	return states, nil
}

func (h *Handler) collectRelationships(ctx context.Context, userID string) ([]userExportRelation, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT target_id, status, created_at
		 FROM user_relationships
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying relationships: %w", err)
	}
	defer rows.Close()

	rels := make([]userExportRelation, 0)
	for rows.Next() {
		var r userExportRelation
		var createdAt time.Time
		if err := rows.Scan(&r.TargetID, &r.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning relationship: %w", err)
		}
		r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		rels = append(rels, r)
	}
	return rels, nil
}

// hasChannelPermission checks if a user has a given permission on a channel's guild.
// For guild channels, it delegates to the permission computation logic.
func (h *Handler) hasChannelPermission(ctx context.Context, guildID, channelID, userID string, perm uint64) bool {
	// Owner has all permissions.
	var ownerID string
	if err := h.Pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID); err != nil {
		return false
	}
	if userID == ownerID {
		return true
	}

	// Check admin flag on user.
	var userFlags int
	h.Pool.QueryRow(ctx, `SELECT flags FROM users WHERE id = $1`, userID).Scan(&userFlags)
	if userFlags&4 != 0 { // UserFlagAdmin = 1 << 2
		return true
	}

	// Get guild default permissions.
	var defaultPerms int64
	h.Pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computedPerms := uint64(defaultPerms)

	// Apply member's role permissions.
	rows, _ := h.Pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, userID,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var allow, deny int64
			rows.Scan(&allow, &deny)
			computedPerms |= uint64(allow)
			computedPerms &^= uint64(deny)
		}
	}

	if computedPerms&permissions.Administrator != 0 {
		return true
	}

	return computedPerms&perm != 0
}
