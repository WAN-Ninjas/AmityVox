package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/amityvox/amityvox/internal/api/apiutil"
	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/search"
)

// validIDPattern matches ULID/alphanumeric IDs to prevent filter injection.
var validIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{26}$`)

// handleSearchMessages handles GET /api/v1/search/messages.
// Query params: q (required), channel_id, guild_id, author_id, limit, offset.
func (s *Server) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if !apiutil.RequireNonEmpty(w, "q", query) {
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit, offset := parsePagination(r)

	// Build filter string for Meilisearch with input validation.
	var filters []string
	if channelID := r.URL.Query().Get("channel_id"); channelID != "" {
		if !validIDPattern.MatchString(channelID) {
			WriteError(w, http.StatusBadRequest, "invalid_channel_id", "Invalid channel_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("channel_id = %q", channelID))
	}
	if guildID := r.URL.Query().Get("guild_id"); guildID != "" {
		if !validIDPattern.MatchString(guildID) {
			WriteError(w, http.StatusBadRequest, "invalid_guild_id", "Invalid guild_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("guild_id = %q", guildID))
	}
	if authorID := r.URL.Query().Get("author_id"); authorID != "" {
		if !validIDPattern.MatchString(authorID) {
			WriteError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("author_id = %q", authorID))
	}

	filterStr := ""
	for i, f := range filters {
		if i > 0 {
			filterStr += " AND "
		}
		filterStr += f
	}

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:   query,
		Index:   search.IndexMessages,
		Filters: filterStr,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		s.Logger.Error("search messages failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	if len(result.IDs) == 0 {
		WriteJSON(w, http.StatusOK, []models.Message{})
		return
	}

	// Hydrate from database with full message data.
	rows, err := s.DB.Pool.Query(r.Context(),
		`SELECT id, channel_id, author_id, content, nonce, message_type, edited_at, flags,
		        reply_to_ids, mention_user_ids, mention_role_ids, mention_everyone,
		        thread_id, masquerade_name, masquerade_avatar, masquerade_color,
		        encrypted, encryption_session_id, created_at
		 FROM messages WHERE id = ANY($1)`, result.IDs)
	if err != nil {
		s.Logger.Error("search messages hydration failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Failed to load search results")
		return
	}
	defer rows.Close()

	msgMap := make(map[string]models.Message, len(result.IDs))
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce, &m.MessageType,
			&m.EditedAt, &m.Flags, &m.ReplyToIDs, &m.MentionUserIDs, &m.MentionRoleIDs,
			&m.MentionEveryone, &m.ThreadID, &m.MasqueradeName, &m.MasqueradeAvatar,
			&m.MasqueradeColor, &m.Encrypted, &m.EncryptionSessionID, &m.CreatedAt,
		); err != nil {
			s.Logger.Error("scan search message", "error", err.Error())
			continue
		}
		msgMap[m.ID] = m
	}

	// Preserve Meilisearch relevance ordering.
	messages := make([]models.Message, 0, len(result.IDs))
	for _, id := range result.IDs {
		if m, ok := msgMap[id]; ok {
			messages = append(messages, m)
		}
	}

	// --- Access control: filter out messages from channels the user cannot see ---
	messages = s.filterAuthorizedMessages(r.Context(), userID, messages)

	// Enrich with authors, attachments, and embeds.
	s.enrichSearchMessagesWithAuthors(r.Context(), messages)
	s.enrichSearchMessagesWithAttachments(r.Context(), messages)
	s.enrichSearchMessagesWithEmbeds(r.Context(), messages)

	WriteJSON(w, http.StatusOK, messages)
}

// handleSearchUsers handles GET /api/v1/search/users.
// Query params: q (required), limit, offset.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if !apiutil.RequireNonEmpty(w, "q", query) {
		return
	}

	limit, offset := parsePagination(r)

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:  query,
		Index:  search.IndexUsers,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.Logger.Error("search users failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	if len(result.IDs) == 0 {
		WriteJSON(w, http.StatusOK, []models.User{})
		return
	}

	rows, err := s.DB.Pool.Query(r.Context(),
		`SELECT id, instance_id, username, display_name, avatar_id,
		        status_text, status_emoji, status_presence, status_expires_at,
		        bio, banner_id, accent_color, pronouns, flags, created_at
		 FROM users WHERE id = ANY($1)`, result.IDs)
	if err != nil {
		s.Logger.Error("search users hydration failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Failed to load search results")
		return
	}
	defer rows.Close()

	userMap := make(map[string]models.User, len(result.IDs))
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns, &u.Flags, &u.CreatedAt,
		); err != nil {
			continue
		}
		userMap[u.ID] = u
	}

	// Preserve Meilisearch relevance ordering.
	users := make([]models.User, 0, len(result.IDs))
	for _, id := range result.IDs {
		if u, ok := userMap[id]; ok {
			users = append(users, u)
		}
	}

	WriteJSON(w, http.StatusOK, users)
}

// handleSearchGuilds handles GET /api/v1/search/guilds.
// Query params: q (required), limit, offset.
func (s *Server) handleSearchGuilds(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if !apiutil.RequireNonEmpty(w, "q", query) {
		return
	}

	limit, offset := parsePagination(r)

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:  query,
		Index:  search.IndexGuilds,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.Logger.Error("search guilds failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	if len(result.IDs) == 0 {
		WriteJSON(w, http.StatusOK, []models.Guild{})
		return
	}

	rows, err := s.DB.Pool.Query(r.Context(),
		`SELECT id, instance_id, owner_id, name, description, icon_id, banner_id,
		        default_permissions, flags, nsfw, discoverable,
		        system_channel_join, system_channel_leave, system_channel_kick, system_channel_ban,
		        preferred_locale, max_members, vanity_url, verification_level,
		        afk_channel_id, afk_timeout, tags, member_count, created_at
		 FROM guilds WHERE id = ANY($1)`, result.IDs)
	if err != nil {
		s.Logger.Error("search guilds hydration failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Failed to load search results")
		return
	}
	defer rows.Close()

	guildMap := make(map[string]models.Guild, len(result.IDs))
	for rows.Next() {
		var g models.Guild
		if err := rows.Scan(
			&g.ID, &g.InstanceID, &g.OwnerID, &g.Name, &g.Description, &g.IconID, &g.BannerID,
			&g.DefaultPermissions, &g.Flags, &g.NSFW, &g.Discoverable,
			&g.SystemChannelJoin, &g.SystemChannelLeave, &g.SystemChannelKick, &g.SystemChannelBan,
			&g.PreferredLocale, &g.MaxMembers, &g.VanityURL, &g.VerificationLevel,
			&g.AFKChannelID, &g.AFKTimeout, &g.Tags, &g.MemberCount, &g.CreatedAt,
		); err != nil {
			continue
		}
		guildMap[g.ID] = g
	}

	// Preserve Meilisearch relevance ordering.
	guilds := make([]models.Guild, 0, len(result.IDs))
	for _, id := range result.IDs {
		if g, ok := guildMap[id]; ok {
			guilds = append(guilds, g)
		}
	}

	WriteJSON(w, http.StatusOK, guilds)
}

// filterAuthorizedMessages removes messages from channels the requesting user
// does not have access to. For guild channels, the user must be a member of the
// guild. For DM channels (guild_id IS NULL), the user must be a channel recipient.
func (s *Server) filterAuthorizedMessages(ctx context.Context, userID string, messages []models.Message) []models.Message {
	if len(messages) == 0 {
		return messages
	}

	// Collect unique channel IDs.
	channelIDSet := make(map[string]struct{})
	for _, m := range messages {
		channelIDSet[m.ChannelID] = struct{}{}
	}
	channelIDs := make([]string, 0, len(channelIDSet))
	for id := range channelIDSet {
		channelIDs = append(channelIDs, id)
	}

	// Look up channel -> guild_id mapping.
	type channelInfo struct {
		guildID *string
	}
	channelMap := make(map[string]channelInfo, len(channelIDs))
	rows, err := s.DB.Pool.Query(ctx,
		`SELECT id, guild_id FROM channels WHERE id = ANY($1)`, channelIDs)
	if err != nil {
		s.Logger.Error("search access control: channel lookup failed", "error", err.Error())
		return nil // fail closed
	}
	defer rows.Close()
	for rows.Next() {
		var cID string
		var gID *string
		if err := rows.Scan(&cID, &gID); err != nil {
			continue
		}
		channelMap[cID] = channelInfo{guildID: gID}
	}
	rows.Close()

	// Collect unique guild IDs for membership check.
	guildIDSet := make(map[string]struct{})
	dmChannelIDs := make([]string, 0)
	for cID, info := range channelMap {
		if info.guildID != nil && *info.guildID != "" {
			guildIDSet[*info.guildID] = struct{}{}
		} else {
			dmChannelIDs = append(dmChannelIDs, cID)
		}
	}

	// Batch-check guild membership.
	allowedGuilds := make(map[string]bool)
	if len(guildIDSet) > 0 {
		guildIDs := make([]string, 0, len(guildIDSet))
		for id := range guildIDSet {
			guildIDs = append(guildIDs, id)
		}
		memberRows, err := s.DB.Pool.Query(ctx,
			`SELECT guild_id FROM guild_members WHERE user_id = $1 AND guild_id = ANY($2)`,
			userID, guildIDs)
		if err == nil {
			defer memberRows.Close()
			for memberRows.Next() {
				var gID string
				if err := memberRows.Scan(&gID); err == nil {
					allowedGuilds[gID] = true
				}
			}
			memberRows.Close()
		}
	}

	// Batch-check DM channel recipients.
	allowedDMChannels := make(map[string]bool)
	if len(dmChannelIDs) > 0 {
		dmRows, err := s.DB.Pool.Query(ctx,
			`SELECT channel_id FROM channel_recipients WHERE user_id = $1 AND channel_id = ANY($2)`,
			userID, dmChannelIDs)
		if err == nil {
			defer dmRows.Close()
			for dmRows.Next() {
				var cID string
				if err := dmRows.Scan(&cID); err == nil {
					allowedDMChannels[cID] = true
				}
			}
			dmRows.Close()
		}
	}

	// Filter messages.
	filtered := make([]models.Message, 0, len(messages))
	for _, m := range messages {
		info, ok := channelMap[m.ChannelID]
		if !ok {
			continue // channel not found — fail closed
		}
		if info.guildID != nil && *info.guildID != "" {
			if allowedGuilds[*info.guildID] {
				filtered = append(filtered, m)
			}
		} else {
			if allowedDMChannels[m.ChannelID] {
				filtered = append(filtered, m)
			}
		}
	}
	return filtered
}

// enrichSearchMessagesWithAuthors batch-loads author data for search results.
func (s *Server) enrichSearchMessagesWithAuthors(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	authorIDs := make(map[string]struct{})
	for _, m := range messages {
		authorIDs[m.AuthorID] = struct{}{}
	}

	ids := make([]string, 0, len(authorIDs))
	for id := range authorIDs {
		ids = append(ids, id)
	}

	rows, err := s.DB.Pool.Query(ctx,
		`SELECT id, instance_id, username, display_name, avatar_id,
		        status_text, status_emoji, status_presence, status_expires_at,
		        bio, banner_id, accent_color, pronouns, flags, created_at
		 FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return
	}
	defer rows.Close()

	userMap := make(map[string]*models.User)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.InstanceID, &u.Username, &u.DisplayName, &u.AvatarID,
			&u.StatusText, &u.StatusEmoji, &u.StatusPresence, &u.StatusExpiresAt,
			&u.Bio, &u.BannerID, &u.AccentColor, &u.Pronouns, &u.Flags, &u.CreatedAt,
		); err != nil {
			continue
		}
		userCopy := u
		userMap[u.ID] = &userCopy
	}

	for i := range messages {
		if u, ok := userMap[messages[i].AuthorID]; ok {
			messages[i].Author = u
		}
	}
}

// enrichSearchMessagesWithAttachments batch-loads attachments for search results.
func (s *Server) enrichSearchMessagesWithAttachments(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	msgIDs := make([]string, len(messages))
	for i, m := range messages {
		msgIDs[i] = m.ID
	}

	rows, err := s.DB.Pool.Query(ctx,
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash, alt_text, created_at
		 FROM attachments WHERE message_id = ANY($1)
		 ORDER BY created_at`, msgIDs)
	if err != nil {
		return
	}
	defer rows.Close()

	attachMap := make(map[string][]models.Attachment)
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(
			&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash, &a.AltText, &a.CreatedAt,
		); err != nil {
			continue
		}
		if a.MessageID != nil {
			attachMap[*a.MessageID] = append(attachMap[*a.MessageID], a)
		}
	}

	for i := range messages {
		if atts, ok := attachMap[messages[i].ID]; ok {
			messages[i].Attachments = atts
		}
	}
}

// enrichSearchMessagesWithEmbeds batch-loads embeds for search results.
func (s *Server) enrichSearchMessagesWithEmbeds(ctx context.Context, messages []models.Message) {
	if len(messages) == 0 {
		return
	}

	msgIDs := make([]string, len(messages))
	for i, m := range messages {
		msgIDs[i] = m.ID
	}

	rows, err := s.DB.Pool.Query(ctx,
		`SELECT id, message_id, embed_type, url, title, description, site_name,
		        icon_url, color, image_url, image_width, image_height,
		        video_url, special_type, special_id, created_at
		 FROM embeds WHERE message_id = ANY($1)
		 ORDER BY created_at`, msgIDs)
	if err != nil {
		return
	}
	defer rows.Close()

	embedMap := make(map[string][]models.Embed)
	for rows.Next() {
		var e models.Embed
		if err := rows.Scan(
			&e.ID, &e.MessageID, &e.EmbedType, &e.URL, &e.Title, &e.Description, &e.SiteName,
			&e.IconURL, &e.Color, &e.ImageURL, &e.ImageWidth, &e.ImageHeight,
			&e.VideoURL, &e.SpecialType, &e.SpecialID, &e.CreatedAt,
		); err != nil {
			continue
		}
		if e.MessageID != "" {
			embedMap[e.MessageID] = append(embedMap[e.MessageID], e)
		}
	}

	for i := range messages {
		if embs, ok := embedMap[messages[i].ID]; ok {
			messages[i].Embeds = embs
		}
	}
}

// parsePagination extracts limit and offset from query parameters with defaults.
func parsePagination(r *http.Request) (int64, int64) {
	limit := int64(20)
	offset := int64(0)

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}
