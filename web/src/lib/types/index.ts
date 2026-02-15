// TypeScript types matching the Go backend models (internal/models/models.go).
// All IDs are ULIDs (string). Nullable fields use `| null`.

export interface User {
	id: string;
	instance_id: string;
	username: string;
	display_name: string | null;
	avatar_id: string | null;
	status_text: string | null;
	status_emoji: string | null;
	status_presence: 'online' | 'idle' | 'dnd' | 'invisible' | 'offline';
	status_expires_at: string | null;
	bio: string | null;
	bot_owner_id: string | null;
	email: string | null;
	banner_id: string | null;
	accent_color: string | null;
	pronouns: string | null;
	flags: number;
	handle?: string;
	created_at: string;
}

export interface Guild {
	id: string;
	instance_id: string;
	owner_id: string;
	name: string;
	description: string | null;
	icon_id: string | null;
	banner_id: string | null;
	default_permissions: number;
	flags: number;
	nsfw: boolean;
	discoverable: boolean;
	preferred_locale: string;
	max_members: number;
	vanity_url: string | null;
	verification_level: number;
	tags: string[];
	member_count: number;
	created_at: string;
}

export interface Channel {
	id: string;
	guild_id: string | null;
	category_id: string | null;
	channel_type: ChannelType;
	name: string | null;
	topic: string | null;
	position: number;
	slowmode_seconds: number;
	nsfw: boolean;
	encrypted: boolean;
	last_message_id: string | null;
	owner_id: string | null;
	user_limit: number;
	bitrate: number;
	locked: boolean;
	locked_by: string | null;
	locked_at: string | null;
	archived: boolean;
	created_at: string;
	recipients?: User[];
}

export type ChannelType = 'text' | 'voice' | 'dm' | 'group' | 'announcement' | 'forum' | 'stage';

export interface Message {
	id: string;
	channel_id: string;
	author_id: string;
	content: string | null;
	nonce: string | null;
	message_type: MessageType;
	edited_at: string | null;
	flags: number;
	reply_to_ids: string[];
	mention_user_ids: string[];
	mention_role_ids: string[];
	mention_everyone: boolean;
	thread_id: string | null;
	masquerade_name: string | null;
	masquerade_avatar: string | null;
	masquerade_color: string | null;
	encrypted: boolean;
	voice_duration_ms?: number | null;
	voice_waveform?: number[] | null;
	attachments: Attachment[];
	embeds: Embed[];
	reactions: Reaction[];
	pinned: boolean;
	created_at: string;
	author?: User;
}

export type MessageType =
	| 'default'
	| 'system_join'
	| 'system_leave'
	| 'system_kick'
	| 'system_ban'
	| 'system_pin'
	| 'reply'
	| 'thread_created'
	| 'voice'
	| 'poll'
	| 'system_lockdown';

export interface ScheduledMessage {
	id: string;
	channel_id: string;
	author_id: string;
	content: string | null;
	attachment_ids: string[];
	scheduled_for: string;
	created_at: string;
}

export interface Reaction {
	emoji: string;
	count: number;
	me: boolean;
}

export interface Attachment {
	id: string;
	message_id: string | null;
	uploader_id: string | null;
	filename: string;
	content_type: string;
	size_bytes: number;
	width: number | null;
	height: number | null;
	duration_seconds: number | null;
	s3_bucket: string;
	s3_key: string;
	blurhash: string | null;
	alt_text?: string;
	created_at: string;
}

export interface Embed {
	type: string;
	url: string | null;
	title: string | null;
	description: string | null;
	color: string | null;
	thumbnail_url: string | null;
	thumbnail_width: number | null;
	thumbnail_height: number | null;
	image_url: string | null;
	image_width: number | null;
	image_height: number | null;
	video_url: string | null;
	author_name: string | null;
	author_url: string | null;
	provider_name: string | null;
	provider_url: string | null;
}

export interface Role {
	id: string;
	guild_id: string;
	name: string;
	color: string | null;
	hoist: boolean;
	mentionable: boolean;
	position: number;
	permissions_allow: number;
	permissions_deny: number;
	created_at: string;
}

export interface GuildMember {
	guild_id: string;
	user_id: string;
	nickname: string | null;
	avatar_id: string | null;
	joined_at: string;
	timeout_until: string | null;
	deaf: boolean;
	mute: boolean;
	user?: User;
	roles?: string[];
}

export interface Invite {
	code: string;
	guild_id: string;
	channel_id: string | null;
	creator_id: string | null;
	max_uses: number | null;
	uses: number;
	max_age_seconds: number | null;
	temporary: boolean;
	created_at: string;
	expires_at: string | null;
}

export interface Category {
	id: string;
	guild_id: string;
	name: string;
	position: number;
}

// --- WebSocket Gateway Types ---

export interface GatewayMessage {
	op: number;
	t?: string;
	d?: unknown;
	s?: number;
}

export const GatewayOp = {
	Dispatch: 0,
	Heartbeat: 1,
	Identify: 2,
	PresenceUpdate: 3,
	VoiceStateUpdate: 4,
	Resume: 5,
	Reconnect: 6,
	RequestMembers: 7,
	Typing: 8,
	Subscribe: 9,
	Hello: 10,
	HeartbeatAck: 11
} as const;

export interface ReadyEvent {
	user: User;
	guild_ids: string[];
	session_id: string;
	presences?: Record<string, string>;
}

export interface TypingEvent {
	channel_id: string;
	user_id: string;
	timestamp: string;
}

export interface PresenceUpdateEvent {
	user_id: string;
	status: string;
}

// --- Admin Types ---

export interface AdminStats {
	users: number;
	online_users: number;
	guilds: number;
	channels: number;
	messages: number;
	messages_today: number;
	files: number;
	roles: number;
	emoji: number;
	invites: number;
	federation_peers: number;
	database_size: string;
	go_version: string;
	goroutines: number;
	mem_alloc_mb: number;
	mem_sys_mb: number;
	num_cpu: number;
	uptime: string;
}

export interface InstanceInfo {
	id: string;
	domain: string;
	name: string | null;
	description: string | null;
	software: string;
	software_version: string;
	federation_mode: string;
	created_at: string;
}

// --- Ban ---

export interface Ban {
	guild_id: string;
	user_id: string;
	reason: string | null;
	created_at: string;
	user?: User;
}

// --- Audit Log ---

export interface AuditLogEntry {
	id: string;
	guild_id: string;
	actor_id: string;
	action_type: string;
	target_id: string | null;
	changes: Record<string, unknown> | null;
	reason: string | null;
	created_at: string;
	actor?: User;
}

// --- Emoji ---

export interface CustomEmoji {
	id: string;
	guild_id: string;
	name: string;
	creator_id: string | null;
	animated: boolean;
	created_at: string;
}

// --- Session ---

export interface Session {
	id: string;
	user_id: string;
	ip_address: string;
	user_agent: string;
	created_at: string;
	last_active_at: string;
	current: boolean;
}

// --- Read State ---

export interface ReadState {
	channel_id: string;
	last_message_id: string | null;
	mention_count: number;
}

// --- Relationship (Friend/Block) ---

export interface Relationship {
	id: string;
	user_id: string;
	target_id: string;
	type: 'friend' | 'blocked' | 'pending_incoming' | 'pending_outgoing';
	created_at: string;
	user?: User;
}

// --- Polls ---

export interface Poll {
	id: string;
	channel_id: string;
	message_id: string | null;
	author_id: string;
	question: string;
	multi_vote: boolean;
	anonymous: boolean;
	expires_at: string | null;
	closed: boolean;
	created_at: string;
	options: PollOption[];
	total_votes: number;
	user_votes: string[];
}

export interface PollOption {
	id: string;
	poll_id: string;
	text: string;
	position: number;
	vote_count: number;
}

// --- Bookmarks ---

export interface MessageBookmark {
	user_id: string;
	message_id: string;
	note: string | null;
	reminder_at: string | null;
	reminded: boolean;
	created_at: string;
	message?: Message;
}

// --- Guild Events ---

export interface GuildEvent {
	id: string;
	guild_id: string;
	creator_id: string;
	name: string;
	description: string | null;
	location: string | null;
	channel_id: string | null;
	image_id: string | null;
	scheduled_start: string;
	scheduled_end: string | null;
	status: 'scheduled' | 'active' | 'completed' | 'cancelled';
	interested_count: number;
	created_at: string;
	creator?: { id: string; username: string; display_name?: string | null; avatar_id?: string | null };
	user_rsvp: string | null;
}

export interface EventRSVP {
	event_id: string;
	user_id: string;
	status: 'interested' | 'going';
	created_at: string;
	user?: { id: string; username: string; display_name?: string | null; avatar_id?: string | null };
}

// --- Moderation ---

export interface MemberWarning {
	id: string;
	guild_id: string;
	user_id: string;
	moderator_id: string;
	reason: string;
	created_at: string;
	user?: User;
	moderator?: User;
}

export interface MessageReport {
	id: string;
	guild_id: string | null;
	channel_id: string;
	message_id: string;
	reporter_id: string;
	reason: string;
	status: 'open' | 'resolved' | 'dismissed' | 'admin_pending';
	resolved_by: string | null;
	resolved_at: string | null;
	created_at: string;
	reporter?: User;
	message?: Message;
}

export interface RaidConfig {
	guild_id: string;
	enabled: boolean;
	join_rate_limit: number;
	join_rate_window: number;
	min_account_age: number;
	lockdown_active: boolean;
	lockdown_started_at: string | null;
	updated_at: string;
}

// --- AutoMod ---

export interface AutoModRule {
	id: string;
	guild_id: string;
	name: string;
	enabled: boolean;
	rule_type: 'word_filter' | 'regex_filter' | 'invite_filter' | 'mention_spam' | 'caps_filter' | 'spam_filter' | 'link_filter';
	action: 'delete' | 'warn' | 'timeout' | 'log';
	config: Record<string, unknown>;
	exempt_roles: string[];
	exempt_channels: string[];
	timeout_duration: number;
	created_at: string;
	updated_at: string;
}

export interface AutoModAction {
	id: string;
	rule_id: string;
	guild_id: string;
	channel_id: string;
	user_id: string;
	message_id: string | null;
	action_taken: string;
	rule_name: string;
	matched_content: string | null;
	created_at: string;
}

// --- User Badges ---

export interface UserBadge {
	id: string;
	name: string;
	icon: string;
}

// --- Instance Bans ---

export interface InstanceBan {
	user_id: string;
	admin_id: string;
	reason: string | null;
	created_at: string;
	username: string;
	display_name: string | null;
	avatar_id: string | null;
}

// --- Registration ---

export interface RegistrationSettings {
	mode: 'open' | 'invite_only' | 'closed';
	message: string | null;
}

export interface RegistrationToken {
	id: string;
	token: string;
	max_uses: number;
	uses: number;
	note: string | null;
	expires_at: string | null;
	created_by: string;
	created_at: string;
}

// --- Announcements ---

export type AnnouncementSeverity = 'info' | 'warning' | 'critical';

export interface Announcement {
	id: string;
	title: string;
	content: string;
	severity: AnnouncementSeverity;
	active: boolean;
	expires_at: string | null;
	created_by: string;
	created_at: string;
	updated_at: string;
}

// --- Bots ---

export interface BotToken {
	id: string;
	bot_id: string;
	name: string;
	token?: string; // Only present when first created
	created_at: string;
	last_used_at: string | null;
}

export interface SlashCommand {
	id: string;
	bot_id: string;
	guild_id: string | null;
	name: string;
	description: string;
	options: unknown[];
	created_at: string;
	updated_at: string;
}

// --- API Response Types ---

export interface ApiResponse<T> {
	data: T;
}

export interface ApiError {
	error: {
		code: string;
		message: string;
	};
}

export interface LoginResponse {
	token: string;
	user: User;
}

export interface RegisterResponse {
	token: string;
	user: User;
}

// --- Notification Preferences ---

export interface NotificationPreference {
	user_id: string;
	guild_id: string | null;
	level: 'all' | 'mentions' | 'none';
	suppress_everyone: boolean;
	suppress_roles: boolean;
	muted_until: string | null;
}

// --- Webhook ---

export interface Webhook {
	id: string;
	guild_id: string;
	channel_id: string;
	creator_id: string | null;
	name: string;
	avatar_id: string | null;
	token: string;
	webhook_type: 'incoming' | 'outgoing';
	outgoing_url: string | null;
	created_at: string;
}

// --- User Settings (Privacy/Notification client prefs) ---

export interface UserSettings {
	desktop_notifications: boolean;
	notification_sounds: boolean;
	notification_sound_preset?: string; // 'default' | 'chime' | 'bell' | 'pop' | 'none'
	notification_volume?: number; // 0-100, default 80
	dm_privacy: 'everyone' | 'friends' | 'nobody';
	friend_request_privacy: 'everyone' | 'mutual_guilds' | 'nobody';
	nsfw_content_filter?: 'blur_all' | 'blur_suspicious' | 'show_all';
	dnd_schedule?: {
		enabled: boolean;
		startHour: number;
		startMinute: number;
		endHour: number;
		endMinute: number;
	};
	custom_themes?: Array<{
		name: string;
		colors: Record<string, string>;
		createdAt: string;
	}>;
	active_custom_theme?: string | null;
	custom_css?: string;
	[key: string]: unknown;
}

// --- Federation Peer ---

export interface FederationPeer {
	id: string;
	domain: string;
	name: string | null;
	software: string | null;
	software_version: string | null;
	status: string;
	last_seen_at: string | null;
	created_at: string;
}

// --- Onboarding ---

export interface OnboardingConfig {
	enabled: boolean;
	welcome_message: string;
	rules: string[];
	default_channel_ids: string[];
	prompts: OnboardingPrompt[];
}

export interface OnboardingPrompt {
	id: string;
	title: string;
	required: boolean;
	single_select: boolean;
	position: number;
	options: OnboardingOption[];
}

export interface OnboardingOption {
	id: string;
	label: string;
	description?: string;
	emoji?: string;
	role_ids: string[];
	channel_ids: string[];
}

// --- Ban Lists ---

export interface BanList {
	id: string;
	guild_id: string;
	name: string;
	description: string | null;
	public: boolean;
	entry_count: number;
	created_at: string;
}

export interface BanListEntry {
	id: string;
	list_id: string;
	user_id: string;
	reason: string | null;
	added_by: string;
	created_at: string;
	username?: string;
}

export interface BanListSubscription {
	id: string;
	guild_id: string;
	list_id: string;
	list_name: string;
	auto_ban: boolean;
	created_at: string;
}

// --- Sticker Packs ---

export interface StickerPack {
	id: string;
	name: string;
	description: string | null;
	cover_sticker_id: string | null;
	owner_type: 'guild' | 'user' | 'system';
	owner_id: string;
	public: boolean;
	sticker_count: number;
	created_at: string;
}

export interface Sticker {
	id: string;
	pack_id: string;
	name: string;
	description: string | null;
	tags: string | null;
	file_id: string;
	format: 'png' | 'apng' | 'gif' | 'lottie';
	created_at: string;
}

// --- Channel Followers ---

export interface ChannelFollower {
	id: string;
	channel_id: string;
	guild_id: string;
	webhook_id: string;
	guild_name?: string;
	channel_name?: string;
	created_at: string;
}

// --- Global Moderation ---

export interface UserReport {
	id: string;
	reporter_id: string;
	reported_user_id: string;
	reason: string;
	context_guild_id: string | null;
	context_channel_id: string | null;
	status: 'open' | 'resolved' | 'dismissed';
	resolved_by: string | null;
	resolved_at: string | null;
	notes: string | null;
	created_at: string;
	reporter_name?: string;
	reported_user_name?: string;
}

export interface ReportedIssue {
	id: string;
	reporter_id: string;
	title: string;
	description: string;
	category: 'general' | 'bug' | 'abuse' | 'suggestion';
	status: 'open' | 'in_progress' | 'resolved' | 'dismissed';
	resolved_by: string | null;
	resolved_at: string | null;
	notes: string | null;
	created_at: string;
	reporter_name?: string;
}

export interface ModerationStats {
	open_message_reports: number;
	open_user_reports: number;
	open_issues: number;
}

export interface ModerationMessageReport {
	id: string;
	guild_id: string | null;
	channel_id: string;
	message_id: string;
	reporter_id: string;
	reason: string;
	status: 'open' | 'resolved' | 'dismissed' | 'admin_pending';
	resolved_by: string | null;
	resolved_at: string | null;
	created_at: string;
	reporter_name?: string;
}
