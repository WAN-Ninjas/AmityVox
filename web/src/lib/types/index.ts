// TypeScript types matching the Go backend models (internal/models/models.go).
// All IDs are ULIDs (string). Nullable fields use `| null`.

export interface User {
	id: string;
	instance_id: string;
	username: string;
	display_name: string | null;
	avatar_id: string | null;
	status_text: string | null;
	status_presence: 'online' | 'idle' | 'dnd' | 'invisible' | 'offline';
	bio: string | null;
	bot_owner_id: string | null;
	email: string | null;
	flags: number;
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
	created_at: string;
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
	| 'thread_created';

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
