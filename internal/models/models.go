package models

import (
	"encoding/json"
	"time"
)

// Instance represents a single AmityVox deployment. Each instance has a unique
// domain and Ed25519 keypair for federation. Corresponds to the instances table.
type Instance struct {
	ID              string          `json:"id"`
	Domain          string          `json:"domain"`
	PublicKey       string          `json:"public_key"`
	Name            *string         `json:"name,omitempty"`
	Description     *string         `json:"description,omitempty"`
	Software        string          `json:"software"`
	SoftwareVersion *string         `json:"software_version,omitempty"`
	FederationMode  string          `json:"federation_mode"`
	ProtocolVersion *string         `json:"protocol_version,omitempty"`
	Capabilities    json.RawMessage `json:"capabilities,omitempty"`
	LiveKitURL      *string         `json:"livekit_url,omitempty"`
	PrivateKeyPEM   *string         `json:"-"`
	ResolvedIPs     []string        `json:"resolved_ips,omitempty"`
	KeyFingerprint  *string         `json:"key_fingerprint,omitempty"`
	Shorthand       *string         `json:"shorthand,omitempty"`
	VoiceMode       string          `json:"voice_mode,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	LastSeenAt      *time.Time      `json:"last_seen_at,omitempty"`
}

// User represents a user account on an AmityVox instance. Users are identified
// globally as @username@instance.domain. Corresponds to the users table.
type User struct {
	ID             string    `json:"id"`
	InstanceID     string    `json:"instance_id"`
	Username       string    `json:"username"`
	DisplayName    *string   `json:"display_name,omitempty"`
	AvatarID       *string   `json:"avatar_id,omitempty"`
	StatusText      *string    `json:"status_text,omitempty"`
	StatusEmoji     *string    `json:"status_emoji,omitempty"`
	StatusPresence  string     `json:"status_presence"`
	StatusExpiresAt *time.Time `json:"status_expires_at,omitempty"`
	Bio             *string    `json:"bio,omitempty"`
	BannerID        *string    `json:"banner_id,omitempty"`
	AccentColor     *string    `json:"accent_color,omitempty"`
	Pronouns        *string    `json:"pronouns,omitempty"`
	BotOwnerID     *string   `json:"bot_owner_id,omitempty"`
	PasswordHash   *string   `json:"-"`
	TOTPSecret     *string   `json:"-"`
	Email          *string   `json:"-"`
	Flags          int       `json:"flags"`
	Handle         string     `json:"handle,omitempty"`
	LastOnline     *time.Time `json:"last_online,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	InstanceDomain *string    `json:"instance_domain,omitempty"` // Set for remote/federated users
}

// SelfUser is a response-only wrapper that includes the email field.
// Used for endpoints where the user is viewing their own profile (@me, login, register).
type SelfUser struct {
	*User
	Email *string `json:"email,omitempty"`
}

// ToSelf returns a SelfUser wrapper that includes the email field in JSON output.
func (u *User) ToSelf() SelfUser {
	return SelfUser{User: u, Email: u.Email}
}

// UserFlags defines bitfield flags for user account status.
const (
	UserFlagSuspended  = 1 << 0
	UserFlagDeleted    = 1 << 1
	UserFlagAdmin      = 1 << 2
	UserFlagBot        = 1 << 3
	UserFlagVerified   = 1 << 4
	UserFlagGlobalMod  = 1 << 5
)

// IsSuspended reports whether the user is suspended.
func (u User) IsSuspended() bool { return u.Flags&UserFlagSuspended != 0 }

// IsDeleted reports whether the user is deleted.
func (u User) IsDeleted() bool { return u.Flags&UserFlagDeleted != 0 }

// IsAdmin reports whether the user is an instance admin.
func (u User) IsAdmin() bool { return u.Flags&UserFlagAdmin != 0 }

// IsBot reports whether the user is a bot account.
func (u User) IsBot() bool { return u.Flags&UserFlagBot != 0 }

// IsGlobalMod reports whether the user is a global moderator.
func (u User) IsGlobalMod() bool { return u.Flags&UserFlagGlobalMod != 0 }

// UserLink represents a social or external link on a user's profile.
// Corresponds to the user_links table.
type UserLink struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Platform  string    `json:"platform"`
	Label     string    `json:"label"`
	URL       string    `json:"url"`
	Position  int       `json:"position"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

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

// UserBlock represents an entry in the user_blocks table, providing richer
// metadata than the user_relationships blocked status. Includes an optional
// reason and the blocked user's profile for list responses.
type UserBlock struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TargetID  string    `json:"target_id"`
	Reason    *string   `json:"reason,omitempty"`
	Level     string    `json:"level"` // "ignore" or "block"
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty"` // Populated on list (the blocked user's profile)
}

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
	InstanceDomain       string    `json:"instance_domain,omitempty"`
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
	VanityURL            *string   `json:"vanity_url,omitempty"`
	VerificationLevel    int       `json:"verification_level"`
	AFKChannelID         *string   `json:"afk_channel_id,omitempty"`
	AFKTimeout           int       `json:"afk_timeout"`
	Tags                 []string  `json:"tags,omitempty"`
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
	DefaultPermissions *int64     `json:"default_permissions,omitempty"`
	UserLimit          int        `json:"user_limit"`
	Bitrate            int        `json:"bitrate"`
	Locked                    bool       `json:"locked"`
	LockedBy                  *string    `json:"locked_by,omitempty"`
	LockedAt                  *time.Time `json:"locked_at,omitempty"`
	Archived                  bool       `json:"archived"`
	ReadOnly                  bool       `json:"read_only"`
	ReadOnlyRoleIDs           []string   `json:"read_only_role_ids,omitempty"`
	DefaultAutoArchiveDuration int        `json:"default_auto_archive_duration"`
	ParentChannelID           *string    `json:"parent_channel_id,omitempty"`
	LastActivityAt            *time.Time `json:"last_activity_at,omitempty"`
	ForumDefaultSort          string     `json:"forum_default_sort,omitempty"`
	ForumPostGuidelines       *string    `json:"forum_post_guidelines,omitempty"`
	ForumRequireTags          bool       `json:"forum_require_tags,omitempty"`
	GalleryDefaultSort        string     `json:"gallery_default_sort,omitempty"`
	GalleryPostGuidelines     *string    `json:"gallery_post_guidelines,omitempty"`
	GalleryRequireTags        bool       `json:"gallery_require_tags,omitempty"`
	Pinned                    bool       `json:"pinned,omitempty"`
	ReplyCount                int        `json:"reply_count,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	Recipients                []User     `json:"recipients,omitempty"`
}

// ChannelType constants for channels.channel_type.
const (
	ChannelTypeText         = "text"
	ChannelTypeVoice        = "voice"
	ChannelTypeDM           = "dm"
	ChannelTypeGroup        = "group"
	ChannelTypeAnnouncement = "announcement"
	ChannelTypeForum        = "forum"
	ChannelTypeGallery      = "gallery"
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
	User         *User      `json:"user,omitempty"`
	Roles        []string   `json:"roles,omitempty"`
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
	MentionHere         bool       `json:"mention_here"`
	ThreadID            *string    `json:"thread_id,omitempty"`
	MasqueradeName      *string    `json:"masquerade_name,omitempty"`
	MasqueradeAvatar    *string    `json:"masquerade_avatar,omitempty"`
	MasqueradeColor     *string    `json:"masquerade_color,omitempty"`
	Encrypted           bool            `json:"encrypted"`
	EncryptionSessionID *string         `json:"encryption_session_id,omitempty"`
	VoiceDurationMs     *int            `json:"voice_duration_ms,omitempty"`
	VoiceWaveform       json.RawMessage `json:"voice_waveform,omitempty"`
	Components          json.RawMessage `json:"components,omitempty"`
	Attachments         []Attachment    `json:"attachments,omitempty"`
	Embeds              []Embed         `json:"embeds,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	Author              *User           `json:"author,omitempty"`
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
	MessageTypeVoice         = "voice"
	MessageTypePoll          = "poll"
	MessageTypeForward       = "forward"
	MessageTypeScheduled     = "scheduled"
	MessageTypeSystemLockdown = "system_lockdown"
)

// MessageFlag constants for messages.flags bitfield.
const (
	MessageFlagCrosspost = 1 << 0
	MessageFlagPinned    = 1 << 1
	MessageFlagUrgent    = 1 << 2
	MessageFlagSilent    = 1 << 3
)

// IsSilent reports whether the message has the silent flag set (no notifications).
func (m Message) IsSilent() bool { return m.Flags&MessageFlagSilent != 0 }

// ScheduledMessage represents a message scheduled for future delivery.
// Corresponds to the scheduled_messages table.
type ScheduledMessage struct {
	ID            string    `json:"id"`
	ChannelID     string    `json:"channel_id"`
	AuthorID      string    `json:"author_id"`
	Content       *string   `json:"content,omitempty"`
	AttachmentIDs []string  `json:"attachment_ids,omitempty"`
	ScheduledFor  time.Time `json:"scheduled_for"`
	CreatedAt     time.Time `json:"created_at"`
}

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
	AltText         *string   `json:"alt_text,omitempty"`
	NSFW            bool      `json:"nsfw"`
	Description     *string   `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// MediaTag represents a guild-scoped tag for categorizing attachments.
type MediaTag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	GuildID   string    `json:"guild_id"`
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AttachmentTag represents a tag applied to an attachment.
type AttachmentTag struct {
	AttachmentID string `json:"attachment_id"`
	TagID        string `json:"tag_id"`
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
	GuildID   string     `json:"guild_id"`
	UserID    string     `json:"user_id"`
	Reason    *string    `json:"reason,omitempty"`
	BannedBy  *string    `json:"banned_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
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
// Audit log action constants for categorizing guild events.
const (
	AuditActionGuildUpdate         = "guild_update"
	AuditActionChannelCreate       = "channel_create"
	AuditActionChannelUpdate       = "channel_update"
	AuditActionChannelDelete       = "channel_delete"
	AuditActionRoleCreate          = "role_create"
	AuditActionRoleUpdate          = "role_update"
	AuditActionRoleDelete          = "role_delete"
	AuditActionMemberKick          = "member_kick"
	AuditActionMemberBan           = "member_ban"
	AuditActionMemberUnban         = "member_unban"
	AuditActionMemberUpdate        = "member_update"
	AuditActionInviteCreate        = "invite_create"
	AuditActionInviteDelete        = "invite_delete"
	AuditActionWebhookCreate       = "webhook_create"
	AuditActionWebhookUpdate       = "webhook_update"
	AuditActionWebhookDelete       = "webhook_delete"
	AuditActionEmojiCreate         = "emoji_create"
	AuditActionEmojiUpdate         = "emoji_update"
	AuditActionEmojiDelete         = "emoji_delete"
	AuditActionMessageDelete       = "message_delete"
	AuditActionMessageBulkDelete   = "message_bulk_delete"
	AuditActionOwnershipTransfer   = "ownership_transfer"
)

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

// Poll represents a poll attached to a message in a channel. Corresponds to the polls table.
type Poll struct {
	ID              string       `json:"id"`
	ChannelID       string       `json:"channel_id"`
	MessageID       *string      `json:"message_id,omitempty"`
	AuthorID        string       `json:"author_id"`
	Question        string       `json:"question"`
	MultiVote       bool         `json:"multi_vote"`
	Anonymous       bool         `json:"anonymous"`
	ExpiresAt       *time.Time   `json:"expires_at,omitempty"`
	Closed          bool         `json:"closed"`
	CreatedAt       time.Time    `json:"created_at"`
	Options         []PollOption `json:"options,omitempty"`
	TotalVotes      int          `json:"total_votes"`
	UserVotes       []string     `json:"user_votes,omitempty"` // option IDs the requesting user voted for
}

// PollOption represents a single option within a poll.
type PollOption struct {
	ID        string `json:"id"`
	PollID    string `json:"poll_id"`
	Text      string `json:"text"`
	Position  int    `json:"position"`
	VoteCount int    `json:"vote_count"`
}

// MessageBookmark represents a user's bookmark on a message. Corresponds to the message_bookmarks table.
type MessageBookmark struct {
	UserID     string     `json:"user_id"`
	MessageID  string     `json:"message_id"`
	Note       *string    `json:"note,omitempty"`
	ReminderAt *time.Time `json:"reminder_at"`
	Reminded   bool       `json:"reminded"`
	CreatedAt  time.Time  `json:"created_at"`
	Message    *Message   `json:"message,omitempty"` // Populated on list
}

// GuildEvent represents a scheduled event in a guild. Corresponds to the guild_events table.
type GuildEvent struct {
	ID               string     `json:"id"`
	GuildID          string     `json:"guild_id"`
	CreatorID        string     `json:"creator_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description,omitempty"`
	Location         *string    `json:"location,omitempty"`
	ChannelID        *string    `json:"channel_id,omitempty"`
	ImageID          *string    `json:"image_id,omitempty"`
	ScheduledStart   time.Time  `json:"scheduled_start"`
	ScheduledEnd     *time.Time `json:"scheduled_end,omitempty"`
	Status           string     `json:"status"`
	InterestedCount  int        `json:"interested_count"`
	CreatedAt        time.Time  `json:"created_at"`
	Creator          *User      `json:"creator,omitempty"`
	UserRSVP         *string    `json:"user_rsvp,omitempty"` // Requesting user's RSVP status
}

// GuildEventStatus constants.
const (
	EventStatusScheduled = "scheduled"
	EventStatusActive    = "active"
	EventStatusCompleted = "completed"
	EventStatusCancelled = "cancelled"
)

// EventRSVP represents a user's RSVP to a guild event.
type EventRSVP struct {
	EventID   string    `json:"event_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty"`
}

// MemberWarning represents a moderation warning issued to a guild member.
type MemberWarning struct {
	ID          string    `json:"id"`
	GuildID     string    `json:"guild_id"`
	UserID      string    `json:"user_id"`
	ModeratorID string    `json:"moderator_id"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
	User        *User     `json:"user,omitempty"`
	Moderator   *User     `json:"moderator,omitempty"`
}

// MessageReport represents a user report on a message.
type MessageReport struct {
	ID         string     `json:"id"`
	GuildID    *string    `json:"guild_id,omitempty"`
	ChannelID  string     `json:"channel_id"`
	MessageID  string     `json:"message_id"`
	ReporterID string     `json:"reporter_id"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	ResolvedBy *string    `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	Reporter   *User      `json:"reporter,omitempty"`
	Message    *Message   `json:"message,omitempty"`
}

// GuildRaidConfig stores raid protection settings for a guild.
type GuildRaidConfig struct {
	GuildID           string     `json:"guild_id"`
	Enabled           bool       `json:"enabled"`
	JoinRateLimit     int        `json:"join_rate_limit"`
	JoinRateWindow    int        `json:"join_rate_window"`
	MinAccountAge     int        `json:"min_account_age"`
	LockdownActive    bool       `json:"lockdown_active"`
	LockdownStartedAt *time.Time `json:"lockdown_started_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ChannelFollower represents a subscription that forwards messages from an
// announcement channel to a target channel via a webhook. Corresponds to the
// channel_followers table.
type ChannelFollower struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	WebhookID string    `json:"webhook_id"`
	GuildID   string    `json:"guild_id"`
	CreatedAt time.Time `json:"created_at"`
}

// BotToken represents an API authentication token for a bot user.
// Bot tokens are hashed with SHA-256 before storage; the raw token is only
// returned once at creation time. Corresponds to the bot_tokens table.
type BotToken struct {
	ID         string     `json:"id"`
	BotID      string     `json:"bot_id"`
	TokenHash  string     `json:"-"` // never expose hash
	Name       string     `json:"name"`
	Token      string     `json:"token,omitempty"` // only set on creation response
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// SlashCommand represents a bot-registered slash command. Commands can be
// global (guild_id is nil) or guild-scoped. Corresponds to the slash_commands table.
type SlashCommand struct {
	ID          string          `json:"id"`
	BotID       string          `json:"bot_id"`
	GuildID     *string         `json:"guild_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Options     json.RawMessage `json:"options"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ChannelTemplate represents a saved channel configuration that can be reused
// when creating new channels in a guild. Corresponds to the channel_templates table.
type ChannelTemplate struct {
	ID                   string          `json:"id"`
	GuildID              string          `json:"guild_id"`
	Name                 string          `json:"name"`
	ChannelType          string          `json:"channel_type"`
	Topic                *string         `json:"topic,omitempty"`
	SlowmodeSeconds      int             `json:"slowmode_seconds"`
	NSFW                 bool            `json:"nsfw"`
	PermissionOverwrites json.RawMessage `json:"permission_overwrites,omitempty"`
	CreatedBy            string          `json:"created_by"`
	CreatedAt            time.Time       `json:"created_at"`
}

// Verification level constants for guilds.
const (
	VerificationNone    = 0
	VerificationLow     = 1 // Verified email
	VerificationMedium  = 2 // Registered 5+ minutes
	VerificationHigh    = 3 // Member 10+ minutes
	VerificationHighest = 4 // Admin bypass only
)

// BotGuildPermission represents a bot's scoped permissions within a specific guild.
// Scopes limit what API endpoints the bot can access and max_role_position
// prevents the bot from managing roles above its assigned ceiling.
// Corresponds to the bot_guild_permissions table.
type BotGuildPermission struct {
	BotID           string    `json:"bot_id"`
	GuildID         string    `json:"guild_id"`
	Scopes          []string  `json:"scopes"`
	MaxRolePosition int       `json:"max_role_position"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Bot permission scope constants.
const (
	BotScopeMessagesRead   = "messages.read"
	BotScopeMessagesWrite  = "messages.write"
	BotScopeMembersRead    = "members.read"
	BotScopeChannelsManage = "channels.manage"
	BotScopeRolesManage    = "roles.manage"
	BotScopeWebhooksManage = "webhooks.manage"
	BotScopeEventsManage   = "events.manage"
)

// ValidBotScopes is the set of recognized bot permission scopes.
var ValidBotScopes = map[string]bool{
	BotScopeMessagesRead:   true,
	BotScopeMessagesWrite:  true,
	BotScopeMembersRead:    true,
	BotScopeChannelsManage: true,
	BotScopeRolesManage:    true,
	BotScopeWebhooksManage: true,
	BotScopeEventsManage:   true,
}

// MessageComponent represents an interactive UI element attached to a message
// (button, select menu, etc.). Corresponds to the message_components table.
type MessageComponent struct {
	ID            string          `json:"id"`
	MessageID     string          `json:"message_id"`
	ComponentType string          `json:"component_type"`
	Style         *string         `json:"style,omitempty"`
	Label         *string         `json:"label,omitempty"`
	CustomID      *string         `json:"custom_id,omitempty"`
	URL           *string         `json:"url,omitempty"`
	Disabled      bool            `json:"disabled"`
	Options       json.RawMessage `json:"options,omitempty"`
	MinValues     *int            `json:"min_values,omitempty"`
	MaxValues     *int            `json:"max_values,omitempty"`
	Placeholder   *string         `json:"placeholder,omitempty"`
	Position      int             `json:"position"`
}

// ComponentType constants for message_components.component_type.
const (
	ComponentTypeButton     = "button"
	ComponentTypeSelectMenu = "select_menu"
	ComponentTypeActionRow  = "action_row"
)

// BotPresence represents a bot's advertised status and activity.
// Corresponds to the bot_presence table.
type BotPresence struct {
	BotID        string    `json:"bot_id"`
	Status       string    `json:"status"`
	ActivityType *string   `json:"activity_type,omitempty"`
	ActivityName *string   `json:"activity_name,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BotEventSubscription represents a per-guild webhook subscription that
// delivers specified event types to a bot's endpoint.
// Corresponds to the bot_event_subscriptions table.
type BotEventSubscription struct {
	ID         string    `json:"id"`
	BotID      string    `json:"bot_id"`
	GuildID    string    `json:"guild_id"`
	EventTypes []string  `json:"event_types"`
	WebhookURL string    `json:"webhook_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// BotRateLimit represents per-bot configurable request throttling.
// Corresponds to the bot_rate_limits table.
type BotRateLimit struct {
	BotID             string    `json:"bot_id"`
	RequestsPerSecond int       `json:"requests_per_second"`
	Burst             int       `json:"burst"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// UserReport represents a report filed against a user by another user.
// Corresponds to the user_reports table.
type UserReport struct {
	ID               string     `json:"id"`
	ReporterID       string     `json:"reporter_id"`
	ReportedUserID   string     `json:"reported_user_id"`
	Reason           string     `json:"reason"`
	ContextGuildID   *string    `json:"context_guild_id,omitempty"`
	ContextChannelID *string    `json:"context_channel_id,omitempty"`
	Status           string     `json:"status"`
	ResolvedBy       *string    `json:"resolved_by,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Notes            *string    `json:"notes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ReporterName     *string    `json:"reporter_name,omitempty"`
	ReportedUserName *string    `json:"reported_user_name,omitempty"`
}

// ReportedIssue represents a general issue/bug/concern reported by a user.
// Corresponds to the reported_issues table.
type ReportedIssue struct {
	ID           string     `json:"id"`
	ReporterID   string     `json:"reporter_id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	ResolvedBy   *string    `json:"resolved_by,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ReporterName *string    `json:"reporter_name,omitempty"`
}

// ForumTag represents a tag that can be applied to posts in a forum channel.
// Corresponds to the forum_tags table.
type ForumTag struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Name      string    `json:"name"`
	Emoji     *string   `json:"emoji,omitempty"`
	Color     *string   `json:"color,omitempty"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// ForumPost represents a post (thread) in a forum channel with tags and metadata.
type ForumPost struct {
	ID             string     `json:"id"`
	Name           *string    `json:"name,omitempty"`
	OwnerID        *string    `json:"owner_id,omitempty"`
	Pinned         bool       `json:"pinned"`
	Locked         bool       `json:"locked"`
	ReplyCount     int        `json:"reply_count"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Tags           []ForumTag `json:"tags"`
	Author         *User      `json:"author,omitempty"`
	ContentPreview *string    `json:"content_preview,omitempty"`
}

// GalleryTag represents a tag that can be applied to posts in a gallery channel.
// Corresponds to the gallery_tags table.
type GalleryTag struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Name      string    `json:"name"`
	Emoji     *string   `json:"emoji,omitempty"`
	Color     *string   `json:"color,omitempty"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// GalleryPost represents a post (thread) in a gallery channel with tags, thumbnail, and metadata.
type GalleryPost struct {
	ID             string       `json:"id"`
	Name           *string      `json:"name,omitempty"`
	OwnerID        *string      `json:"owner_id,omitempty"`
	Pinned         bool         `json:"pinned"`
	Locked         bool         `json:"locked"`
	ReplyCount     int          `json:"reply_count"`
	LastActivityAt *time.Time   `json:"last_activity_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	Tags           []GalleryTag `json:"tags"`
	Author         *User        `json:"author,omitempty"`
	Thumbnail      *Attachment  `json:"thumbnail,omitempty"`
	Description    *string      `json:"description,omitempty"`
}

// ModerationStats holds counts of open moderation items.
type ModerationStats struct {
	OpenMessageReports int `json:"open_message_reports"`
	OpenUserReports    int `json:"open_user_reports"`
	OpenIssues         int `json:"open_issues"`
}

// Notification represents a persistent server-side notification delivered to a
// user. Notifications are generated by the notification worker in response to
// events such as mentions, DMs, friend requests, moderation actions, etc.
// Corresponds to the notifications table.
type Notification struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	Type          string          `json:"type"`
	Category      string          `json:"category"`
	GuildID       *string         `json:"guild_id"`
	GuildName     *string         `json:"guild_name"`
	GuildIconID   *string         `json:"guild_icon_id"`
	ChannelID     *string         `json:"channel_id"`
	ChannelName   *string         `json:"channel_name"`
	MessageID     *string         `json:"message_id"`
	ActorID       string          `json:"actor_id"`
	ActorName     string          `json:"actor_name"`
	ActorAvatarID *string         `json:"actor_avatar_id"`
	Content       *string         `json:"content"`
	Metadata      json.RawMessage `json:"metadata"`
	Read          bool            `json:"read"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Notification type constants.
const (
	NotifTypeMention        = "mention"
	NotifTypeReply          = "reply"
	NotifTypeDM             = "dm"
	NotifTypeThreadReply    = "thread_reply"
	NotifTypeMessagePinned  = "message_pinned"
	NotifTypeReactionAdded  = "reaction_added"
	NotifTypeFriendRequest  = "friend_request"
	NotifTypeFriendAccepted = "friend_accepted"
	NotifTypeGuildInvite    = "guild_invite"
	NotifTypeMemberJoined   = "member_joined"
	NotifTypeWarned         = "warned"
	NotifTypeMuted          = "muted"
	NotifTypeKicked         = "kicked"
	NotifTypeBanned         = "banned"
	NotifTypeReportResolved = "report_resolved"
	NotifTypeEventStarting  = "event_starting"
	NotifTypeAnnouncement   = "announcement"
)

// Notification category constants.
const (
	NotifCategoryMessages   = "messages"
	NotifCategorySocial     = "social"
	NotifCategoryModeration = "moderation"
	NotifCategoryContent    = "content"
)

// NotificationCategoryForType returns the category for a given notification type.
func NotificationCategoryForType(notifType string) string {
	switch notifType {
	case NotifTypeMention, NotifTypeReply, NotifTypeDM, NotifTypeThreadReply, NotifTypeMessagePinned, NotifTypeReactionAdded:
		return NotifCategoryMessages
	case NotifTypeFriendRequest, NotifTypeFriendAccepted, NotifTypeGuildInvite, NotifTypeMemberJoined:
		return NotifCategorySocial
	case NotifTypeWarned, NotifTypeMuted, NotifTypeKicked, NotifTypeBanned, NotifTypeReportResolved:
		return NotifCategoryModeration
	case NotifTypeEventStarting, NotifTypeAnnouncement:
		return NotifCategoryContent
	default:
		return NotifCategoryMessages
	}
}

// NotificationTypePreference holds per-type notification delivery settings for
// a user. Each type can independently toggle in-app, push, and sound delivery.
// Corresponds to the notification_type_preferences table.
type NotificationTypePreference struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	InApp  bool   `json:"in_app"`
	Push   bool   `json:"push"`
	Sound  bool   `json:"sound"`
}
