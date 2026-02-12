package amityvox

import (
	"encoding/json"
	"time"
)

// GatewayOp defines the WebSocket gateway opcodes.
const (
	OpDispatch       = 0
	OpHeartbeat      = 1
	OpIdentify       = 2
	OpPresenceUpdate = 3
	OpVoiceState     = 4
	OpResume         = 5
	OpReconnect      = 6
	OpRequestMembers = 7
	OpTyping         = 8
	OpSubscribe      = 9
	OpHello          = 10
	OpHeartbeatAck   = 11
)

// GatewayMessage is the envelope for all WebSocket messages.
type GatewayMessage struct {
	Op       int              `json:"op"`
	Type     string           `json:"t,omitempty"`
	Sequence int64            `json:"s,omitempty"`
	Data     json.RawMessage  `json:"d,omitempty"`
}

// Event type constants match the NATS subjects used by the backend.
const (
	EventReady             = "READY"
	EventMessageCreate     = "MESSAGE_CREATE"
	EventMessageUpdate     = "MESSAGE_UPDATE"
	EventMessageDelete     = "MESSAGE_DELETE"
	EventMessageReactionAdd = "MESSAGE_REACTION_ADD"
	EventMessageReactionDel = "MESSAGE_REACTION_REMOVE"
	EventChannelCreate     = "CHANNEL_CREATE"
	EventChannelUpdate     = "CHANNEL_UPDATE"
	EventChannelDelete     = "CHANNEL_DELETE"
	EventGuildCreate       = "GUILD_CREATE"
	EventGuildUpdate       = "GUILD_UPDATE"
	EventGuildDelete       = "GUILD_DELETE"
	EventGuildMemberAdd    = "GUILD_MEMBER_ADD"
	EventGuildMemberUpdate = "GUILD_MEMBER_UPDATE"
	EventGuildMemberRemove = "GUILD_MEMBER_REMOVE"
	EventGuildRoleCreate   = "GUILD_ROLE_CREATE"
	EventGuildRoleUpdate   = "GUILD_ROLE_UPDATE"
	EventGuildRoleDelete   = "GUILD_ROLE_DELETE"
	EventGuildBanAdd       = "GUILD_BAN_ADD"
	EventGuildBanRemove    = "GUILD_BAN_REMOVE"
	EventPresenceUpdate    = "PRESENCE_UPDATE"
	EventTypingStart       = "TYPING_START"
	EventVoiceStateUpdate  = "VOICE_STATE_UPDATE"
)

// ReadyEvent is dispatched when the gateway connection is established.
type ReadyEvent struct {
	User      User              `json:"user"`
	GuildIDs  []string          `json:"guild_ids"`
	SessionID string            `json:"session_id"`
	Presences map[string]string `json:"presences,omitempty"`
}

// --- Data model types for the SDK ---

// User represents an AmityVox user account.
type User struct {
	ID             string  `json:"id"`
	InstanceID     string  `json:"instance_id"`
	Username       string  `json:"username"`
	DisplayName    *string `json:"display_name,omitempty"`
	AvatarID       *string `json:"avatar_id,omitempty"`
	StatusText     *string `json:"status_text,omitempty"`
	StatusEmoji    *string `json:"status_emoji,omitempty"`
	StatusPresence string  `json:"status_presence"`
	Bio            *string `json:"bio,omitempty"`
	BotOwnerID     *string `json:"bot_owner_id,omitempty"`
	Flags          int     `json:"flags"`
	CreatedAt      string  `json:"created_at"`
}

// IsBot reports whether the user is a bot account.
func (u User) IsBot() bool { return u.Flags&(1<<3) != 0 }

// IsAdmin reports whether the user is an instance admin.
func (u User) IsAdmin() bool { return u.Flags&(1<<2) != 0 }

// Guild represents an AmityVox guild (server).
type Guild struct {
	ID                 string  `json:"id"`
	InstanceID         string  `json:"instance_id"`
	OwnerID            string  `json:"owner_id"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	IconID             *string `json:"icon_id,omitempty"`
	DefaultPermissions int64   `json:"default_permissions"`
	MemberCount        int     `json:"member_count,omitempty"`
	CreatedAt          string  `json:"created_at"`
}

// Channel represents a guild or DM channel.
type Channel struct {
	ID          string  `json:"id"`
	GuildID     *string `json:"guild_id,omitempty"`
	ChannelType string  `json:"channel_type"`
	Name        *string `json:"name,omitempty"`
	Topic       *string `json:"topic,omitempty"`
	Position    int     `json:"position"`
	NSFW        bool    `json:"nsfw"`
	Encrypted   bool    `json:"encrypted"`
	CreatedAt   string  `json:"created_at"`
}

// Message represents a chat message.
type Message struct {
	ID              string       `json:"id"`
	ChannelID       string       `json:"channel_id"`
	AuthorID        string       `json:"author_id"`
	Content         *string      `json:"content,omitempty"`
	MessageType     string       `json:"message_type"`
	EditedAt        *string      `json:"edited_at,omitempty"`
	Flags           int          `json:"flags"`
	ReplyToIDs      []string     `json:"reply_to_ids,omitempty"`
	MentionUserIDs  []string     `json:"mention_user_ids,omitempty"`
	MentionRoleIDs  []string     `json:"mention_role_ids,omitempty"`
	MentionEveryone bool         `json:"mention_everyone"`
	ThreadID        *string      `json:"thread_id,omitempty"`
	Attachments     []Attachment `json:"attachments,omitempty"`
	CreatedAt       string       `json:"created_at"`
	Author          *User        `json:"author,omitempty"`
}

// ContentString returns the message content as a string (empty if nil).
func (m Message) ContentString() string {
	if m.Content == nil {
		return ""
	}
	return *m.Content
}

// Attachment represents a file attached to a message.
type Attachment struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	ContentType string  `json:"content_type"`
	SizeBytes   int64   `json:"size_bytes"`
	Width       *int    `json:"width,omitempty"`
	Height      *int    `json:"height,omitempty"`
	S3Key       string  `json:"s3_key"`
	CreatedAt   string  `json:"created_at"`
}

// MessageEmbed is a rich embed that can be attached to a message.
type MessageEmbed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	URL         string  `json:"url,omitempty"`
	Color       string  `json:"color,omitempty"`
	ImageURL    string  `json:"image_url,omitempty"`
}

// GuildMember represents a user's membership in a guild.
type GuildMember struct {
	GuildID  string  `json:"guild_id"`
	UserID   string  `json:"user_id"`
	Nickname *string `json:"nickname,omitempty"`
	JoinedAt string  `json:"joined_at"`
	User     *User   `json:"user,omitempty"`
}

// Role represents a guild permission role.
type Role struct {
	ID               string  `json:"id"`
	GuildID          string  `json:"guild_id"`
	Name             string  `json:"name"`
	Color            *string `json:"color,omitempty"`
	Hoist            bool    `json:"hoist"`
	Position         int     `json:"position"`
	PermissionsAllow int64   `json:"permissions_allow"`
	PermissionsDeny  int64   `json:"permissions_deny"`
}

// Invite represents a guild invite link.
type Invite struct {
	Code      string  `json:"code"`
	GuildID   string  `json:"guild_id"`
	ChannelID *string `json:"channel_id,omitempty"`
	CreatorID *string `json:"creator_id,omitempty"`
	MaxUses   *int    `json:"max_uses,omitempty"`
	Uses      int     `json:"uses"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

// TypingEvent is dispatched when a user starts typing in a channel.
type TypingEvent struct {
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Timestamp string `json:"timestamp"`
}

// PresenceUpdateEvent is dispatched when a user's presence changes.
type PresenceUpdateEvent struct {
	UserID string `json:"user_id"`
	Status string `json:"status"`
}

// MessageDeleteEvent is dispatched when a message is deleted.
type MessageDeleteEvent struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// ReactionEvent is dispatched when a reaction is added or removed.
type ReactionEvent struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Emoji     string `json:"emoji"`
}

// GuildMemberEvent is dispatched when a member joins or is updated/removed.
type GuildMemberEvent struct {
	GuildID  string  `json:"guild_id"`
	UserID   string  `json:"user_id"`
	User     *User   `json:"user,omitempty"`
	Nickname *string `json:"nickname,omitempty"`
}

// GuildBanEvent is dispatched when a user is banned or unbanned.
type GuildBanEvent struct {
	GuildID string  `json:"guild_id"`
	UserID  string  `json:"user_id"`
	Reason  *string `json:"reason,omitempty"`
}

// Timestamp parses a time string from the API into a time.Time.
func Timestamp(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
