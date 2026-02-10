package models

import (
	"time"
)

// Instance represents a single AmityVox deployment. Each instance has a unique
// domain and Ed25519 keypair for federation. Corresponds to the instances table.
type Instance struct {
	ID              string     `json:"id"`
	Domain          string     `json:"domain"`
	PublicKey       string     `json:"public_key"`
	Name            *string    `json:"name,omitempty"`
	Description     *string    `json:"description,omitempty"`
	Software        string     `json:"software"`
	SoftwareVersion *string    `json:"software_version,omitempty"`
	FederationMode  string     `json:"federation_mode"`
	CreatedAt       time.Time  `json:"created_at"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
}

// User represents a user account on an AmityVox instance. Users are identified
// globally as @username@instance.domain. Corresponds to the users table.
type User struct {
	ID             string    `json:"id"`
	InstanceID     string    `json:"instance_id"`
	Username       string    `json:"username"`
	DisplayName    *string   `json:"display_name,omitempty"`
	AvatarID       *string   `json:"avatar_id,omitempty"`
	StatusText     *string   `json:"status_text,omitempty"`
	StatusPresence string    `json:"status_presence"`
	Bio            *string   `json:"bio,omitempty"`
	BotOwnerID     *string   `json:"bot_owner_id,omitempty"`
	PasswordHash   *string   `json:"-"`
	TOTPSecret     *string   `json:"-"`
	Email          *string   `json:"email,omitempty"`
	Flags          int       `json:"flags"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserFlags defines bitfield flags for user account status.
const (
	UserFlagSuspended  = 1 << 0
	UserFlagDeleted    = 1 << 1
	UserFlagAdmin      = 1 << 2
	UserFlagBot        = 1 << 3
	UserFlagVerified   = 1 << 4
)

// IsSuspended reports whether the user is suspended.
func (u User) IsSuspended() bool { return u.Flags&UserFlagSuspended != 0 }

// IsDeleted reports whether the user is deleted.
func (u User) IsDeleted() bool { return u.Flags&UserFlagDeleted != 0 }

// IsAdmin reports whether the user is an instance admin.
func (u User) IsAdmin() bool { return u.Flags&UserFlagAdmin != 0 }

// IsBot reports whether the user is a bot account.
func (u User) IsBot() bool { return u.Flags&UserFlagBot != 0 }

// UserSession represents an active login session. Session tokens are stored as
// the primary key and used as Bearer tokens for API authentication.
// Corresponds to the user_sessions table.
type UserSession struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	DeviceName   *string   `json:"device_name,omitempty"`
	IPAddress    *string   `json:"ip_address,omitempty"`
	UserAgent    *string   `json:"user_agent,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// UserRelationship represents a friend, block, or pending friend request between
// two users. Corresponds to the user_relationships table.
type UserRelationship struct {
	UserID    string    `json:"user_id"`
	TargetID  string    `json:"target_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// RelationshipStatus constants for user_relationships.status.
const (
	RelationshipFriend          = "friend"
	RelationshipBlocked         = "blocked"
	RelationshipPendingOutgoing = "pending_outgoing"
	RelationshipPendingIncoming = "pending_incoming"
)

// WebAuthnCredential represents a WebAuthn/FIDO2 credential registered by a user
// for passwordless authentication. Corresponds to the webauthn_credentials table.
type WebAuthnCredential struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	CredentialID []byte    `json:"-"`
	PublicKey    []byte    `json:"-"`
	SignCount    int64     `json:"sign_count"`
	Name         *string   `json:"name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Guild represents a community server (the Discord "server" equivalent). Guilds
// belong to a specific instance and contain channels, roles, and members.
// Corresponds to the guilds table.
type Guild struct {
	ID                   string    `json:"id"`
	InstanceID           string    `json:"instance_id"`
	OwnerID              string    `json:"owner_id"`
	Name                 string    `json:"name"`
	Description          *string   `json:"description,omitempty"`
	IconID               *string   `json:"icon_id,omitempty"`
	BannerID             *string   `json:"banner_id,omitempty"`
	DefaultPermissions   int64     `json:"default_permissions"`
	Flags                int       `json:"flags"`
	NSFW                 bool      `json:"nsfw"`
	Discoverable         bool      `json:"discoverable"`
	SystemChannelJoin    *string   `json:"system_channel_join,omitempty"`
	SystemChannelLeave   *string   `json:"system_channel_leave,omitempty"`
	SystemChannelKick    *string   `json:"system_channel_kick,omitempty"`
	SystemChannelBan     *string   `json:"system_channel_ban,omitempty"`
	PreferredLocale      string    `json:"preferred_locale"`
	MaxMembers           int       `json:"max_members"`
	MemberCount          int       `json:"member_count,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// GuildCategory represents a channel category within a guild, used to organize
// channels visually. Corresponds to the guild_categories table.
type GuildCategory struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// Channel represents a text, voice, DM, group, or other channel type. Guild
// channels belong to a guild; DM/group channels are standalone.
// Corresponds to the channels table.
type Channel struct {
	ID                 string    `json:"id"`
	GuildID            *string   `json:"guild_id,omitempty"`
	CategoryID         *string   `json:"category_id,omitempty"`
	ChannelType        string    `json:"channel_type"`
	Name               *string   `json:"name,omitempty"`
	Topic              *string   `json:"topic,omitempty"`
	Position           int       `json:"position"`
	SlowmodeSeconds    int       `json:"slowmode_seconds"`
	NSFW               bool      `json:"nsfw"`
	Encrypted          bool      `json:"encrypted"`
	LastMessageID      *string   `json:"last_message_id,omitempty"`
	OwnerID            *string   `json:"owner_id,omitempty"`
	DefaultPermissions *int64    `json:"default_permissions,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// ChannelType constants for channels.channel_type.
const (
	ChannelTypeText         = "text"
	ChannelTypeVoice        = "voice"
	ChannelTypeDM           = "dm"
	ChannelTypeGroup        = "group"
	ChannelTypeAnnouncement = "announcement"
	ChannelTypeForum        = "forum"
	ChannelTypeStage        = "stage"
)

// ChannelRecipient represents a participant in a DM or group channel.
// Corresponds to the channel_recipients table.
type ChannelRecipient struct {
	ChannelID string    `json:"channel_id"`
	UserID    string    `json:"user_id"`
	JoinedAt  time.Time `json:"joined_at"`
}

// Role represents a permission bundle within a guild. Roles have allow/deny
// bitfield pairs and are rank-ordered by position. Corresponds to the roles table.
type Role struct {
	ID               string    `json:"id"`
	GuildID          string    `json:"guild_id"`
	Name             string    `json:"name"`
	Color            *string   `json:"color,omitempty"`
	Hoist            bool      `json:"hoist"`
	Mentionable      bool      `json:"mentionable"`
	Position         int       `json:"position"`
	PermissionsAllow int64     `json:"permissions_allow"`
	PermissionsDeny  int64     `json:"permissions_deny"`
	CreatedAt        time.Time `json:"created_at"`
}

// GuildMember represents a user's membership in a guild, including per-guild
// nickname, avatar override, and timeout status. Corresponds to the guild_members table.
type GuildMember struct {
	GuildID      string     `json:"guild_id"`
	UserID       string     `json:"user_id"`
	Nickname     *string    `json:"nickname,omitempty"`
	AvatarID     *string    `json:"avatar_id,omitempty"`
	JoinedAt     time.Time  `json:"joined_at"`
	TimeoutUntil *time.Time `json:"timeout_until,omitempty"`
	Deaf         bool       `json:"deaf"`
	Mute         bool       `json:"mute"`
}

// IsTimedOut reports whether the member is currently timed out.
func (m GuildMember) IsTimedOut() bool {
	return m.TimeoutUntil != nil && m.TimeoutUntil.After(time.Now())
}

// MemberRole associates a guild member with a role. Corresponds to the
// member_roles table.
type MemberRole struct {
	GuildID string `json:"guild_id"`
	UserID  string `json:"user_id"`
	RoleID  string `json:"role_id"`
}

// ChannelPermissionOverride represents a per-channel permission override for a
// specific role or user. Corresponds to the channel_permission_overrides table.
type ChannelPermissionOverride struct {
	ChannelID        string `json:"channel_id"`
	TargetType       string `json:"target_type"`
	TargetID         string `json:"target_id"`
	PermissionsAllow int64  `json:"permissions_allow"`
	PermissionsDeny  int64  `json:"permissions_deny"`
}

// OverrideTargetType constants for channel_permission_overrides.target_type.
const (
	OverrideTargetRole = "role"
	OverrideTargetUser = "user"
)

// Message represents a chat message in a channel. Messages use ULIDs as IDs so
// they sort by creation time. Corresponds to the messages table.
type Message struct {
	ID                  string     `json:"id"`
	ChannelID           string     `json:"channel_id"`
	AuthorID            string     `json:"author_id"`
	Content             *string    `json:"content,omitempty"`
	Nonce               *string    `json:"nonce,omitempty"`
	MessageType         string     `json:"message_type"`
	EditedAt            *time.Time `json:"edited_at,omitempty"`
	Flags               int        `json:"flags"`
	ReplyToIDs          []string   `json:"reply_to_ids,omitempty"`
	MentionUserIDs      []string   `json:"mention_user_ids,omitempty"`
	MentionRoleIDs      []string   `json:"mention_role_ids,omitempty"`
	MentionEveryone     bool       `json:"mention_everyone"`
	ThreadID            *string    `json:"thread_id,omitempty"`
	MasqueradeName      *string    `json:"masquerade_name,omitempty"`
	MasqueradeAvatar    *string    `json:"masquerade_avatar,omitempty"`
	MasqueradeColor     *string    `json:"masquerade_color,omitempty"`
	Encrypted           bool         `json:"encrypted"`
	EncryptionSessionID *string      `json:"encryption_session_id,omitempty"`
	Attachments         []Attachment `json:"attachments,omitempty"`
	Embeds              []Embed      `json:"embeds,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
}

// MessageType constants for messages.message_type.
const (
	MessageTypeDefault       = "default"
	MessageTypeSystemJoin    = "system_join"
	MessageTypeSystemLeave   = "system_leave"
	MessageTypeSystemKick    = "system_kick"
	MessageTypeSystemBan     = "system_ban"
	MessageTypeSystemPin     = "system_pin"
	MessageTypeReply         = "reply"
	MessageTypeThreadCreated = "thread_created"
)

// Attachment represents a file attached to a message, stored in S3-compatible
// object storage. Corresponds to the attachments table.
type Attachment struct {
	ID              string    `json:"id"`
	MessageID       *string   `json:"message_id,omitempty"`
	UploaderID      *string   `json:"uploader_id,omitempty"`
	Filename        string    `json:"filename"`
	ContentType     string    `json:"content_type"`
	SizeBytes       int64     `json:"size_bytes"`
	Width           *int      `json:"width,omitempty"`
	Height          *int      `json:"height,omitempty"`
	DurationSeconds *float32  `json:"duration_seconds,omitempty"`
	S3Bucket        string    `json:"s3_bucket"`
	S3Key           string    `json:"s3_key"`
	Blurhash        *string   `json:"blurhash,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Embed represents rich content extracted from URLs in messages, such as website
// previews, images, and video embeds. Corresponds to the embeds table.
type Embed struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id"`
	EmbedType   string    `json:"embed_type"`
	URL         *string   `json:"url,omitempty"`
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	SiteName    *string   `json:"site_name,omitempty"`
	IconURL     *string   `json:"icon_url,omitempty"`
	Color       *string   `json:"color,omitempty"`
	ImageURL    *string   `json:"image_url,omitempty"`
	ImageWidth  *int      `json:"image_width,omitempty"`
	ImageHeight *int      `json:"image_height,omitempty"`
	VideoURL    *string   `json:"video_url,omitempty"`
	SpecialType *string   `json:"special_type,omitempty"`
	SpecialID   *string   `json:"special_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// EmbedType constants for embeds.embed_type.
const (
	EmbedTypeWebsite = "website"
	EmbedTypeImage   = "image"
	EmbedTypeVideo   = "video"
	EmbedTypeRich    = "rich"
	EmbedTypeSpecial = "special"
)

// Reaction represents a user's emoji reaction to a message. Corresponds to the
// reactions table.
type Reaction struct {
	MessageID string    `json:"message_id"`
	UserID    string    `json:"user_id"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

// Pin represents a pinned message in a channel. Corresponds to the pins table.
type Pin struct {
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	PinnedBy  string    `json:"pinned_by"`
	PinnedAt  time.Time `json:"pinned_at"`
}

// Invite represents a guild invite link with optional usage limits and expiry.
// Corresponds to the invites table.
type Invite struct {
	Code          string     `json:"code"`
	GuildID       string     `json:"guild_id"`
	ChannelID     *string    `json:"channel_id,omitempty"`
	CreatorID     *string    `json:"creator_id,omitempty"`
	MaxUses       *int       `json:"max_uses,omitempty"`
	Uses          int        `json:"uses"`
	MaxAgeSeconds *int       `json:"max_age_seconds,omitempty"`
	Temporary     bool       `json:"temporary"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

// IsExpired reports whether the invite has expired.
func (i Invite) IsExpired() bool {
	return i.ExpiresAt != nil && i.ExpiresAt.Before(time.Now())
}

// IsMaxUsesReached reports whether the invite has reached its maximum usage.
func (i Invite) IsMaxUsesReached() bool {
	return i.MaxUses != nil && i.Uses >= *i.MaxUses
}

// GuildBan represents a user ban from a guild. Corresponds to the guild_bans table.
type GuildBan struct {
	GuildID   string    `json:"guild_id"`
	UserID    string    `json:"user_id"`
	Reason    *string   `json:"reason,omitempty"`
	BannedBy  *string   `json:"banned_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CustomEmoji represents a custom emoji uploaded to a guild. Corresponds to the
// custom_emoji table.
type CustomEmoji struct {
	ID        string    `json:"id"`
	GuildID   string    `json:"guild_id"`
	Name      string    `json:"name"`
	CreatorID *string   `json:"creator_id,omitempty"`
	Animated  bool      `json:"animated"`
	S3Key     string    `json:"s3_key"`
	CreatedAt time.Time `json:"created_at"`
}

// Webhook represents an incoming or outgoing webhook for a guild channel.
// Corresponds to the webhooks table.
type Webhook struct {
	ID          string    `json:"id"`
	GuildID     string    `json:"guild_id"`
	ChannelID   string    `json:"channel_id"`
	CreatorID   *string   `json:"creator_id,omitempty"`
	Name        string    `json:"name"`
	AvatarID    *string   `json:"avatar_id,omitempty"`
	Token       string    `json:"-"`
	WebhookType string    `json:"webhook_type"`
	OutgoingURL *string   `json:"outgoing_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// WebhookType constants for webhooks.webhook_type.
const (
	WebhookTypeIncoming = "incoming"
	WebhookTypeOutgoing = "outgoing"
)

// AuditLogEntry represents an administrative action recorded for auditing purposes.
// Corresponds to the audit_log table.
type AuditLogEntry struct {
	ID         string                 `json:"id"`
	GuildID    string                 `json:"guild_id"`
	ActorID    string                 `json:"actor_id"`
	Action     string                 `json:"action"`
	TargetType *string                `json:"target_type,omitempty"`
	TargetID   *string                `json:"target_id,omitempty"`
	Reason     *string                `json:"reason,omitempty"`
	Changes    map[string]interface{} `json:"changes,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// FederationPeer represents a federation relationship between two instances.
// Corresponds to the federation_peers table.
type FederationPeer struct {
	InstanceID    string     `json:"instance_id"`
	PeerID        string     `json:"peer_id"`
	Status        string     `json:"status"`
	EstablishedAt time.Time  `json:"established_at"`
	LastSyncedAt  *time.Time `json:"last_synced_at,omitempty"`
}

// FederationPeerStatus constants for federation_peers.status.
const (
	FederationPeerActive  = "active"
	FederationPeerBlocked = "blocked"
	FederationPeerPending = "pending"
)

// ReadState tracks a user's read position in a channel for unread indicators.
// Corresponds to the read_state table.
type ReadState struct {
	UserID       string  `json:"user_id"`
	ChannelID    string  `json:"channel_id"`
	LastReadID   *string `json:"last_read_id,omitempty"`
	MentionCount int     `json:"mention_count"`
}
