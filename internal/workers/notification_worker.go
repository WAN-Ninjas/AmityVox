package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/amityvox/amityvox/internal/events"
	"github.com/amityvox/amityvox/internal/models"
)

// startNotificationWorker subscribes to multiple NATS event subjects and creates
// persistent notifications for the appropriate recipients.
func (m *Manager) startNotificationWorker(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		subs := []struct {
			subject string
			handler func(context.Context, events.Event)
		}{
			{events.SubjectMessageCreate, m.handleMessageNotification},
			{events.SubjectMessageReactionAdd, m.handleReactionNotification},
			{events.SubjectChannelPinsUpdate, m.handlePinNotification},
			{events.SubjectRelationshipAdd, m.handleRelationshipAddNotification},
			{events.SubjectRelationshipUpdate, m.handleRelationshipUpdateNotification},
			{events.SubjectGuildMemberAdd, m.handleMemberJoinNotification},
			{events.SubjectGuildBanAdd, m.handleBanNotification},
			{events.SubjectGuildMemberRemove, m.handleMemberRemoveNotification},
			{events.SubjectAutomodAction, m.handleAutomodNotification},
		}

		for _, s := range subs {
			handler := s.handler
			_, err := m.bus.Subscribe(s.subject, func(event events.Event) {
				handler(ctx, event)
			})
			if err != nil {
				m.logger.Error("failed to subscribe for notifications",
					slog.String("subject", s.subject),
					slog.String("error", err.Error()))
			}
		}

		m.logger.Info("notification worker started (all event types)")
		<-ctx.Done()
	}()
}

// lookupUser fetches display name and avatar_id for a user.
func (m *Manager) lookupUser(ctx context.Context, userID string) (name string, avatarID *string) {
	err := m.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, username), avatar_id FROM users WHERE id = $1`, userID,
	).Scan(&name, &avatarID)
	if err != nil {
		name = "Someone"
	}
	return
}

// lookupGuild fetches name and icon_id for a guild.
func (m *Manager) lookupGuild(ctx context.Context, guildID string) (name string, iconID *string) {
	m.pool.QueryRow(ctx,
		`SELECT name, icon_id FROM guilds WHERE id = $1`, guildID,
	).Scan(&name, &iconID)
	return
}

// lookupChannel fetches the channel name.
func (m *Manager) lookupChannel(ctx context.Context, channelID string) (name string) {
	m.pool.QueryRow(ctx,
		`SELECT COALESCE(name, '') FROM channels WHERE id = $1`, channelID,
	).Scan(&name)
	return
}

// createNotifForRecipients creates a notification for each recipient using
// the new persistent notification system.
func (m *Manager) createNotifForRecipients(ctx context.Context, recipients map[string]bool, base models.Notification) {
	for uid := range recipients {
		n := base
		n.UserID = uid
		n.CreatedAt = time.Now().UTC()
		if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
			m.logger.Debug("failed to create notification",
				slog.String("user_id", uid),
				slog.String("type", n.Type),
				slog.String("error", err.Error()),
			)
		}
	}
}

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// handleMessageNotification handles MESSAGE_CREATE — produces mention, reply, dm notifications.
func (m *Manager) handleMessageNotification(ctx context.Context, event events.Event) {
	var msg struct {
		ID             string   `json:"id"`
		ChannelID      string   `json:"channel_id"`
		GuildID        string   `json:"guild_id"`
		AuthorID       string   `json:"author_id"`
		Content        string   `json:"content"`
		Flags          int      `json:"flags"`
		MessageType    string   `json:"message_type"`
		ReplyToIDs     []string `json:"reply_to_ids"`
		MentionUserIDs []string `json:"mention_user_ids"`
		MentionRoleIDs []string `json:"mention_role_ids"`
		MentionHere    bool     `json:"mention_here"`
		ThreadID       *string  `json:"thread_id"`
	}
	if err := json.Unmarshal(event.Data, &msg); err != nil {
		return
	}

	// Skip empty or silent messages.
	if msg.Content == "" || msg.Flags&models.MessageFlagSilent != 0 {
		return
	}

	authorName, authorAvatar := m.lookupUser(ctx, msg.AuthorID)
	isDM := msg.GuildID == ""

	// Collect recipients per notification type.
	mentionRecipients := map[string]bool{}
	replyRecipients := map[string]bool{}
	dmRecipients := map[string]bool{}

	// DM recipients.
	if isDM {
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM dm_participants WHERE channel_id = $1 AND user_id <> $2`,
			msg.ChannelID, msg.AuthorID)
		if err == nil {
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil {
					dmRecipients[uid] = true
				}
			}
			rows.Close()
		}
	}

	// Reply recipients — notify the original message authors.
	if len(msg.ReplyToIDs) > 0 || msg.MessageType == "reply" {
		for _, replyID := range msg.ReplyToIDs {
			var origAuthor string
			if m.pool.QueryRow(ctx,
				`SELECT author_id FROM messages WHERE id = $1`, replyID,
			).Scan(&origAuthor) == nil && origAuthor != msg.AuthorID {
				replyRecipients[origAuthor] = true
			}
		}
	}

	// Direct user mentions.
	for _, uid := range msg.MentionUserIDs {
		if uid != msg.AuthorID && !replyRecipients[uid] && !dmRecipients[uid] {
			mentionRecipients[uid] = true
		}
	}

	// Role mentions.
	for _, roleID := range msg.MentionRoleIDs {
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM member_roles WHERE role_id = $1 AND guild_id = $2`,
			roleID, msg.GuildID)
		if err == nil {
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil && uid != msg.AuthorID &&
					!replyRecipients[uid] && !dmRecipients[uid] {
					mentionRecipients[uid] = true
				}
			}
			rows.Close()
		}
	}

	// @here mentions.
	if msg.MentionHere && msg.GuildID != "" {
		rows, err := m.pool.Query(ctx,
			`SELECT user_id FROM guild_members WHERE guild_id = $1 AND user_id <> $2`,
			msg.GuildID, msg.AuthorID)
		if err == nil {
			for rows.Next() {
				var uid string
				if rows.Scan(&uid) == nil && !replyRecipients[uid] && !dmRecipients[uid] {
					mentionRecipients[uid] = true
				}
			}
			rows.Close()
		}
	}

	// Build context.
	content := msg.Content
	if len(content) > 200 {
		content = content[:200]
	}

	var guildName string
	var guildIconID *string
	var guildIDPtr *string
	if msg.GuildID != "" {
		guildName, guildIconID = m.lookupGuild(ctx, msg.GuildID)
		guildIDPtr = strPtr(msg.GuildID)
	}
	channelName := m.lookupChannel(ctx, msg.ChannelID)

	base := models.Notification{
		GuildID:       guildIDPtr,
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ChannelID:     strPtr(msg.ChannelID),
		ChannelName:   nilIfEmpty(channelName),
		MessageID:     strPtr(msg.ID),
		ActorID:       msg.AuthorID,
		ActorName:     authorName,
		ActorAvatarID: authorAvatar,
		Content:       strPtr(content),
	}

	// Check notification preferences for each recipient before creating.
	for uid := range mentionRecipients {
		if !m.notifications.ShouldNotify(ctx, uid, msg.GuildID, msg.ChannelID, true, false, msg.MentionHere) {
			delete(mentionRecipients, uid)
		}
	}
	for uid := range replyRecipients {
		if !m.notifications.ShouldNotify(ctx, uid, msg.GuildID, msg.ChannelID, true, false, false) {
			delete(replyRecipients, uid)
		}
	}
	for uid := range dmRecipients {
		if !m.notifications.ShouldNotify(ctx, uid, "", msg.ChannelID, false, true, false) {
			delete(dmRecipients, uid)
		}
	}

	// Create mention notifications.
	if len(mentionRecipients) > 0 {
		n := base
		n.Type = models.NotifTypeMention
		m.createNotifForRecipients(ctx, mentionRecipients, n)
	}

	// Create reply notifications.
	if len(replyRecipients) > 0 {
		n := base
		n.Type = models.NotifTypeReply
		m.createNotifForRecipients(ctx, replyRecipients, n)
	}

	// Create DM notifications.
	if len(dmRecipients) > 0 {
		n := base
		n.Type = models.NotifTypeDM
		m.createNotifForRecipients(ctx, dmRecipients, n)
	}
}

// handleReactionNotification handles MESSAGE_REACTION_ADD — notifies message author.
func (m *Manager) handleReactionNotification(ctx context.Context, event events.Event) {
	var data struct {
		MessageID string `json:"message_id"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		UserID    string `json:"user_id"`
		Emoji     string `json:"emoji"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	// Look up message author.
	var authorID string
	if err := m.pool.QueryRow(ctx,
		`SELECT author_id FROM messages WHERE id = $1`, data.MessageID,
	).Scan(&authorID); err != nil || authorID == data.UserID {
		return // skip if reactor is the author
	}

	actorName, actorAvatar := m.lookupUser(ctx, data.UserID)
	var guildIDPtr *string
	var guildName string
	var guildIconID *string
	if data.GuildID != "" {
		guildName, guildIconID = m.lookupGuild(ctx, data.GuildID)
		guildIDPtr = strPtr(data.GuildID)
	}
	channelName := m.lookupChannel(ctx, data.ChannelID)

	metadata, _ := json.Marshal(map[string]string{"emoji": data.Emoji})

	n := models.Notification{
		UserID:        authorID,
		Type:          models.NotifTypeReactionAdded,
		GuildID:       guildIDPtr,
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ChannelID:     strPtr(data.ChannelID),
		ChannelName:   nilIfEmpty(channelName),
		MessageID:     strPtr(data.MessageID),
		ActorID:       data.UserID,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr(fmt.Sprintf("reacted %s", data.Emoji)),
		Metadata:      metadata,
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create reaction notification", slog.String("error", err.Error()))
	}
}

// handlePinNotification handles CHANNEL_PINS_UPDATE — notifies channel members.
func (m *Manager) handlePinNotification(ctx context.Context, event events.Event) {
	var data struct {
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		PinnedBy  string `json:"pinned_by"`
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil || data.PinnedBy == "" {
		return
	}

	actorName, actorAvatar := m.lookupUser(ctx, data.PinnedBy)
	var guildIDPtr *string
	var guildName string
	var guildIconID *string
	if data.GuildID != "" {
		guildName, guildIconID = m.lookupGuild(ctx, data.GuildID)
		guildIDPtr = strPtr(data.GuildID)
	}
	channelName := m.lookupChannel(ctx, data.ChannelID)

	// Look up the message author to notify them.
	var msgAuthorID string
	if data.MessageID != "" {
		m.pool.QueryRow(ctx,
			`SELECT author_id FROM messages WHERE id = $1`, data.MessageID,
		).Scan(&msgAuthorID)
	}

	if msgAuthorID == "" || msgAuthorID == data.PinnedBy {
		return
	}

	n := models.Notification{
		UserID:        msgAuthorID,
		Type:          models.NotifTypeMessagePinned,
		GuildID:       guildIDPtr,
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ChannelID:     strPtr(data.ChannelID),
		ChannelName:   nilIfEmpty(channelName),
		MessageID:     strPtr(data.MessageID),
		ActorID:       data.PinnedBy,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr("pinned your message"),
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create pin notification", slog.String("error", err.Error()))
	}
}

// handleRelationshipAddNotification handles RELATIONSHIP_ADD — friend request notifications.
func (m *Manager) handleRelationshipAddNotification(ctx context.Context, event events.Event) {
	var data struct {
		UserID   string `json:"user_id"`
		TargetID string `json:"target_id"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	if data.Type != "pending_incoming" {
		return
	}

	actorName, actorAvatar := m.lookupUser(ctx, data.UserID)

	n := models.Notification{
		UserID:        data.TargetID,
		Type:          models.NotifTypeFriendRequest,
		ActorID:       data.UserID,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr("sent you a friend request"),
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create friend request notification", slog.String("error", err.Error()))
	}
}

// handleRelationshipUpdateNotification handles RELATIONSHIP_UPDATE — friend accepted.
func (m *Manager) handleRelationshipUpdateNotification(ctx context.Context, event events.Event) {
	var data struct {
		UserID   string `json:"user_id"`
		TargetID string `json:"target_id"`
		Type     string `json:"type"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	relType := data.Type
	if relType == "" {
		relType = data.Status
	}
	if relType != "friend" {
		return
	}

	// Notify the user who originally sent the friend request (the user_id in the event
	// is the one whose relationship list was updated).
	actorName, actorAvatar := m.lookupUser(ctx, data.TargetID)

	n := models.Notification{
		UserID:        data.UserID,
		Type:          models.NotifTypeFriendAccepted,
		ActorID:       data.TargetID,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr("accepted your friend request"),
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create friend accepted notification", slog.String("error", err.Error()))
	}
}

// handleMemberJoinNotification handles GUILD_MEMBER_ADD — notifies guild owner.
func (m *Manager) handleMemberJoinNotification(ctx context.Context, event events.Event) {
	var data struct {
		GuildID string `json:"guild_id"`
		UserID  string `json:"user_id"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	// Notify the guild owner.
	var ownerID string
	if err := m.pool.QueryRow(ctx,
		`SELECT owner_id FROM guilds WHERE id = $1`, data.GuildID,
	).Scan(&ownerID); err != nil || ownerID == data.UserID {
		return
	}

	actorName, actorAvatar := m.lookupUser(ctx, data.UserID)
	guildName, guildIconID := m.lookupGuild(ctx, data.GuildID)

	n := models.Notification{
		UserID:        ownerID,
		Type:          models.NotifTypeMemberJoined,
		GuildID:       strPtr(data.GuildID),
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ActorID:       data.UserID,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr(fmt.Sprintf("joined %s", guildName)),
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create member join notification", slog.String("error", err.Error()))
	}
}

// handleBanNotification handles GUILD_BAN_ADD — notifies the banned user.
func (m *Manager) handleBanNotification(ctx context.Context, event events.Event) {
	var data struct {
		GuildID  string  `json:"guild_id"`
		UserID   string  `json:"user_id"`
		BannedBy *string `json:"banned_by"`
		Reason   *string `json:"reason"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	actorID := "system"
	if data.BannedBy != nil {
		actorID = *data.BannedBy
	}
	actorName, actorAvatar := m.lookupUser(ctx, actorID)
	guildName, guildIconID := m.lookupGuild(ctx, data.GuildID)

	metadata, _ := json.Marshal(map[string]*string{"reason": data.Reason})

	n := models.Notification{
		UserID:        data.UserID,
		Type:          models.NotifTypeBanned,
		GuildID:       strPtr(data.GuildID),
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ActorID:       actorID,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr(fmt.Sprintf("You were banned from %s", guildName)),
		Metadata:      metadata,
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create ban notification", slog.String("error", err.Error()))
	}
}

// handleMemberRemoveNotification handles GUILD_MEMBER_REMOVE — kick notification.
func (m *Manager) handleMemberRemoveNotification(ctx context.Context, event events.Event) {
	var data struct {
		GuildID  string  `json:"guild_id"`
		UserID   string  `json:"user_id"`
		KickedBy *string `json:"kicked_by"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	// Only create kicked notification if kicked_by is present (voluntarily leaving has no kicked_by).
	if data.KickedBy == nil || *data.KickedBy == "" {
		return
	}

	actorName, actorAvatar := m.lookupUser(ctx, *data.KickedBy)
	guildName, guildIconID := m.lookupGuild(ctx, data.GuildID)

	n := models.Notification{
		UserID:        data.UserID,
		Type:          models.NotifTypeKicked,
		GuildID:       strPtr(data.GuildID),
		GuildName:     nilIfEmpty(guildName),
		GuildIconID:   guildIconID,
		ActorID:       *data.KickedBy,
		ActorName:     actorName,
		ActorAvatarID: actorAvatar,
		Content:       strPtr(fmt.Sprintf("You were kicked from %s", guildName)),
		CreatedAt:     time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create kick notification", slog.String("error", err.Error()))
	}
}

// handleAutomodNotification handles AUTOMOD_ACTION — warned/muted notifications.
func (m *Manager) handleAutomodNotification(ctx context.Context, event events.Event) {
	var data struct {
		GuildID    string  `json:"guild_id"`
		UserID     string  `json:"user_id"`
		ActionType string  `json:"action_type"`
		Reason     *string `json:"reason"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}

	var notifType string
	switch data.ActionType {
	case "warn":
		notifType = models.NotifTypeWarned
	case "mute", "timeout":
		notifType = models.NotifTypeMuted
	default:
		return
	}

	guildName, guildIconID := m.lookupGuild(ctx, data.GuildID)
	metadata, _ := json.Marshal(map[string]*string{"reason": data.Reason})

	content := "You were warned"
	if notifType == models.NotifTypeMuted {
		content = "You were muted"
	}

	n := models.Notification{
		UserID:      data.UserID,
		Type:        notifType,
		GuildID:     strPtr(data.GuildID),
		GuildName:   nilIfEmpty(guildName),
		GuildIconID: guildIconID,
		ActorID:     "system",
		ActorName:   "AutoMod",
		Content:     strPtr(content),
		Metadata:    metadata,
		CreatedAt:   time.Now().UTC(),
	}
	if err := m.notifications.CreateNotification(ctx, m.bus, &n); err != nil {
		m.logger.Debug("failed to create automod notification", slog.String("error", err.Error()))
	}
}

// nilIfEmpty returns a pointer to s if non-empty, nil otherwise.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
