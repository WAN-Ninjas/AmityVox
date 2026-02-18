// REST API client for AmityVox backend.
// All methods return unwrapped data (the envelope is handled internally).

import type {
	User,
	Guild,
	Channel,
	Message,
	GuildMember,
	Role,
	Invite,
	Ban,
	AuditLogEntry,
	CustomEmoji,
	Session,
	ReadState,
	Relationship,
	LoginResponse,
	RegisterResponse,
	ApiResponse,
	ApiError,
	AdminStats,
	InstanceInfo,
	NotificationPreference,
	ChannelNotificationPreference,
	Webhook,
	UserSettings,
	Category,
	FederationPeer,
	Poll,
	MessageBookmark,
	GuildEvent,
	EventRSVP,
	MemberWarning,
	MessageReport,
	RaidConfig,
	AutoModRule,
	AutoModAction,
	UserBadge,
	ScheduledMessage,
	InstanceBan,
	RegistrationSettings,
	RegistrationToken,
	Announcement,
	OnboardingConfig,
	OnboardingPrompt,
	BanList,
	BanListEntry,
	BanListSubscription,
	ChannelFollower,
	BotToken,
	SlashCommand,
	StickerPack,
	Sticker,
	UserReport,
	ReportedIssue,
	ModerationStats,
	ModerationMessageReport,
	VoicePreferences,
	MutualGuild,
	UserLink,
	Attachment,
	MediaTag,
	RetentionPolicy,
	ForumTag,
	ForumPost,
	GalleryTag,
	GalleryPost
} from '$lib/types';

const API_BASE = '/api/v1';

class ApiClient {
	private token: string | null = null;

	setToken(token: string | null) {
		this.token = token;
		if (token) {
			localStorage.setItem('token', token);
		} else {
			localStorage.removeItem('token');
		}
	}

	getToken(): string | null {
		if (!this.token) {
			this.token = localStorage.getItem('token');
		}
		return this.token;
	}

	private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
		const headers: Record<string, string> = {
			'Content-Type': 'application/json'
		};

		const token = this.getToken();
		if (token) {
			headers['Authorization'] = `Bearer ${token}`;
		}

		const res = await fetch(`${API_BASE}${path}`, {
			method,
			headers,
			body: body ? JSON.stringify(body) : undefined
		});

		if (res.status === 204) {
			return undefined as T;
		}

		const json = await res.json();

		if (!res.ok) {
			const err = json as ApiError;
			throw new ApiRequestError(
				err.error?.message || res.statusText,
				err.error?.code || 'unknown',
				res.status
			);
		}

		return (json as ApiResponse<T>).data;
	}

	private get<T>(path: string) {
		return this.request<T>('GET', path);
	}
	private post<T>(path: string, body?: unknown) {
		return this.request<T>('POST', path, body);
	}
	private patch<T>(path: string, body?: unknown) {
		return this.request<T>('PATCH', path, body);
	}
	private put<T>(path: string, body?: unknown) {
		return this.request<T>('PUT', path, body);
	}
	private del<T>(path: string, body?: unknown) {
		return this.request<T>('DELETE', path, body);
	}

	// --- Auth ---

	async register(username: string, email: string, password: string): Promise<LoginResponse> {
		const data = await this.post<RegisterResponse>('/auth/register', {
			username,
			email,
			password
		});
		this.setToken(data.token);
		return data;
	}

	async login(username: string, password: string): Promise<LoginResponse> {
		const data = await this.post<LoginResponse>('/auth/login', { username, password });
		this.setToken(data.token);
		return data;
	}

	async logout(): Promise<void> {
		await this.post('/auth/logout');
		this.setToken(null);
	}

	// --- Users ---

	getMe(): Promise<User> {
		return this.get('/users/@me');
	}

	updateMe(data: Partial<Pick<User, 'username' | 'display_name' | 'bio' | 'status_text' | 'status_emoji' | 'status_presence' | 'pronouns' | 'accent_color' | 'banner_id'>> & { status_expires_at?: string | null; avatar_id?: string | null }): Promise<User> {
		return this.patch('/users/@me', data);
	}

	getUser(userId: string): Promise<User> {
		return this.get(`/users/${userId}`);
	}

	getUserBadges(userId: string): Promise<UserBadge[]> {
		return this.get(`/users/${userId}/badges`);
	}

	getUserLinks(userId: string): Promise<UserLink[]> {
		return this.get(`/users/${userId}/links`);
	}

	getMyLinks(): Promise<UserLink[]> {
		return this.get('/users/@me/links');
	}

	createLink(platform: string, label: string, url: string): Promise<UserLink> {
		return this.post('/users/@me/links', { platform, label, url });
	}

	updateLink(linkId: string, data: Partial<{ platform: string; label: string; url: string; position: number }>): Promise<UserLink> {
		return this.patch(`/users/@me/links/${linkId}`, data);
	}

	deleteLink(linkId: string): Promise<void> {
		return this.del(`/users/@me/links/${linkId}`);
	}

	getMyGuilds(): Promise<Guild[]> {
		return this.get('/users/@me/guilds');
	}

	getMyDMs(): Promise<Channel[]> {
		return this.get('/users/@me/dms');
	}

	createDM(userId: string): Promise<Channel> {
		return this.post(`/users/${userId}/dm`);
	}

	createGroupDM(userIds: string[], name?: string): Promise<Channel> {
		return this.post('/users/@me/group-dms', { user_ids: userIds, name });
	}

	addGroupDMRecipient(channelId: string, userId: string): Promise<Channel> {
		return this.put(`/channels/${channelId}/recipients/${userId}`);
	}

	removeGroupDMRecipient(channelId: string, userId: string): Promise<void> {
		return this.del(`/channels/${channelId}/recipients/${userId}`);
	}

	// --- Guilds ---

	createGuild(name: string, description?: string): Promise<Guild> {
		return this.post('/guilds', { name, description });
	}

	getGuild(guildId: string): Promise<Guild> {
		return this.get(`/guilds/${guildId}`);
	}

	getMyPermissions(guildId: string): Promise<{ permissions: string }> {
		return this.get(`/guilds/${guildId}/members/@me/permissions`);
	}

	updateGuild(guildId: string, data: Partial<Guild>): Promise<Guild> {
		return this.patch(`/guilds/${guildId}`, data);
	}

	deleteGuild(guildId: string): Promise<void> {
		return this.del(`/guilds/${guildId}`);
	}

	leaveGuild(guildId: string): Promise<void> {
		return this.post(`/guilds/${guildId}/leave`);
	}

	// --- Channels ---

	getGuildChannels(guildId: string): Promise<Channel[]> {
		return this.get(`/guilds/${guildId}/channels`);
	}

	createChannel(guildId: string, name: string, type: string = 'text'): Promise<Channel> {
		return this.post(`/guilds/${guildId}/channels`, { name, channel_type: type });
	}

	getChannel(channelId: string): Promise<Channel> {
		return this.get(`/channels/${channelId}`);
	}

	updateChannel(channelId: string, data: Partial<Channel>): Promise<Channel> {
		return this.patch(`/channels/${channelId}`, data);
	}

	deleteChannel(channelId: string): Promise<void> {
		return this.del(`/channels/${channelId}`);
	}

	cloneChannel(guildId: string, channelId: string, name?: string): Promise<Channel> {
		return this.post(`/guilds/${guildId}/channels/${channelId}/clone`, name ? { name } : {});
	}

	// --- Messages ---

	getMessages(channelId: string, params?: { before?: string; after?: string; limit?: number }): Promise<Message[]> {
		const query = new URLSearchParams();
		if (params?.before) query.set('before', params.before);
		if (params?.after) query.set('after', params.after);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/channels/${channelId}/messages${qs ? '?' + qs : ''}`);
	}

	sendMessage(channelId: string, content: string, opts?: { reply_to_ids?: string[]; nonce?: string; attachment_ids?: string[]; silent?: boolean; voice_duration_ms?: number; voice_waveform?: number[]; encrypted?: boolean; encryption_session_id?: string }): Promise<Message> {
		return this.post(`/channels/${channelId}/messages`, { content, ...opts });
	}

	/** Batch-decrypt messages: send decrypted plaintext for encrypted messages. */
	batchDecryptMessages(channelId: string, messages: { id: string; content: string }[]): Promise<void> {
		return this.post(`/channels/${channelId}/decrypt-messages`, { messages });
	}

	// --- Scheduled Messages ---

	scheduleMessage(channelId: string, content: string, scheduledFor: string, opts?: { attachment_ids?: string[] }): Promise<ScheduledMessage> {
		return this.post(`/channels/${channelId}/scheduled-messages`, { content, scheduled_for: scheduledFor, ...opts });
	}

	getScheduledMessages(channelId: string): Promise<ScheduledMessage[]> {
		return this.get(`/channels/${channelId}/scheduled-messages`);
	}

	deleteScheduledMessage(channelId: string, messageId: string): Promise<void> {
		return this.del(`/channels/${channelId}/scheduled-messages/${messageId}`);
	}

	editMessage(channelId: string, messageId: string, content: string): Promise<Message> {
		return this.patch(`/channels/${channelId}/messages/${messageId}`, { content });
	}

	deleteMessage(channelId: string, messageId: string): Promise<void> {
		return this.del(`/channels/${channelId}/messages/${messageId}`);
	}

	bulkDeleteMessages(channelId: string, messageIds: string[]): Promise<void> {
		return this.post(`/channels/${channelId}/messages/bulk-delete`, { message_ids: messageIds });
	}

	// --- Pins ---

	getPins(channelId: string): Promise<Message[]> {
		return this.get(`/channels/${channelId}/pins`);
	}

	pinMessage(channelId: string, messageId: string): Promise<void> {
		return this.put(`/channels/${channelId}/pins/${messageId}`);
	}

	unpinMessage(channelId: string, messageId: string): Promise<void> {
		return this.del(`/channels/${channelId}/pins/${messageId}`);
	}

	// --- Read State ---

	getReadState(): Promise<ReadState[]> {
		return this.get('/users/@me/read-state');
	}

	ackChannel(channelId: string): Promise<void> {
		return this.post(`/channels/${channelId}/ack`);
	}

	// --- Friends ---

	getFriends(): Promise<Relationship[]> {
		return this.get('/users/@me/relationships');
	}

	addFriend(userId: string): Promise<Relationship> {
		return this.put(`/users/${userId}/friend`);
	}

	removeFriend(userId: string): Promise<void> {
		return this.del(`/users/${userId}/friend`);
	}

	blockUser(userId: string, level: 'ignore' | 'block' = 'block'): Promise<void> {
		return this.put(`/users/${userId}/block`, { level });
	}

	updateBlockLevel(userId: string, level: 'ignore' | 'block'): Promise<void> {
		return this.patch(`/users/${userId}/block`, { level });
	}

	unblockUser(userId: string): Promise<void> {
		return this.del(`/users/${userId}/block`);
	}

	getBlockedUsers(): Promise<{ id: string; user_id: string; target_id: string; reason: string | null; level: string; created_at: string; user?: User }[]> {
		return this.get('/users/@me/blocked');
	}

	resolveHandle(handle: string): Promise<User> {
		return this.get(`/users/resolve?handle=${encodeURIComponent(handle)}`);
	}

	// --- Reactions ---

	addReaction(channelId: string, messageId: string, emoji: string): Promise<void> {
		return this.put(`/channels/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`);
	}

	removeReaction(channelId: string, messageId: string, emoji: string): Promise<void> {
		return this.del(`/channels/${channelId}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`);
	}

	// --- Members ---

	getMembers(guildId: string): Promise<GuildMember[]> {
		return this.get(`/guilds/${guildId}/members`);
	}

	getMember(guildId: string, memberId: string): Promise<GuildMember> {
		return this.get(`/guilds/${guildId}/members/${memberId}`);
	}

	kickMember(guildId: string, memberId: string, reason?: string): Promise<void> {
		return this.del(`/guilds/${guildId}/members/${memberId}`, reason ? { reason } : undefined);
	}

	updateMember(guildId: string, memberId: string, data: { nickname?: string | null; timeout_until?: string | null; deaf?: boolean; mute?: boolean; reason?: string }): Promise<GuildMember> {
		return this.patch(`/guilds/${guildId}/members/${memberId}`, data);
	}

	getMemberRoles(guildId: string, memberId: string): Promise<Role[]> {
		return this.get(`/guilds/${guildId}/members/${memberId}/roles`);
	}

	banUser(guildId: string, userId: string, options?: { reason?: string; duration_seconds?: number; delete_message_seconds?: number }): Promise<void> {
		return this.put(`/guilds/${guildId}/bans/${userId}`, options ?? {});
	}

	unbanUser(guildId: string, userId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/bans/${userId}`);
	}

	// --- Roles ---

	getRoles(guildId: string): Promise<Role[]> {
		return this.get(`/guilds/${guildId}/roles`);
	}

	createRole(guildId: string, name: string, opts?: { color?: string; hoist?: boolean; mentionable?: boolean; permissions_allow?: string; permissions_deny?: string }): Promise<Role> {
		return this.post(`/guilds/${guildId}/roles`, { name, ...opts });
	}

	// --- Invites ---

	getGuildInvites(guildId: string): Promise<Invite[]> {
		return this.get(`/guilds/${guildId}/invites`);
	}

	createInvite(guildId: string, opts?: { max_uses?: number; max_age_seconds?: number }): Promise<Invite> {
		return this.post(`/guilds/${guildId}/invites`, opts);
	}

	deleteInvite(code: string): Promise<void> {
		return this.del(`/invites/${code}`);
	}

	getInvite(code: string): Promise<Invite> {
		return this.get(`/invites/${code}`);
	}

	acceptInvite(code: string): Promise<Guild> {
		return this.post(`/invites/${code}`);
	}

	// --- Bans ---

	getGuildBans(guildId: string): Promise<Ban[]> {
		return this.get(`/guilds/${guildId}/bans`);
	}

	// --- Audit Log ---

	getAuditLog(guildId: string, params?: { limit?: number; before?: string; action_type?: string }): Promise<AuditLogEntry[]> {
		const query = new URLSearchParams();
		if (params?.limit) query.set('limit', String(params.limit));
		if (params?.before) query.set('before', params.before);
		if (params?.action_type) query.set('action_type', params.action_type);
		const qs = query.toString();
		return this.get(`/guilds/${guildId}/audit-log${qs ? '?' + qs : ''}`);
	}

	// --- Emoji ---

	getGuildEmoji(guildId: string): Promise<CustomEmoji[]> {
		return this.get(`/guilds/${guildId}/emoji`);
	}

	deleteGuildEmoji(guildId: string, emojiId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/emoji/${emojiId}`);
	}

	// --- Auth / Security ---

	changePassword(currentPassword: string, newPassword: string): Promise<void> {
		return this.post('/auth/password', { current_password: currentPassword, new_password: newPassword });
	}

	enableTOTP(): Promise<{ secret: string; qr_url: string }> {
		return this.post('/auth/totp/enable');
	}

	verifyTOTP(code: string): Promise<{ backup_codes: string[] }> {
		return this.post('/auth/totp/verify', { code });
	}

	disableTOTP(code: string): Promise<void> {
		return this.del('/auth/totp');
	}

	generateBackupCodes(): Promise<{ codes: string[] }> {
		return this.post('/auth/backup-codes');
	}

	// --- Sessions ---

	getSessions(): Promise<Session[]> {
		return this.get('/users/@me/sessions');
	}

	deleteSession(sessionId: string): Promise<void> {
		return this.del(`/users/@me/sessions/${sessionId}`);
	}

	// --- Typing ---

	sendTyping(channelId: string): Promise<void> {
		return this.post(`/channels/${channelId}/typing`);
	}

	// --- Voice ---

	joinVoice(channelId: string): Promise<{ token: string; url: string }> {
		return this.post(`/voice/${channelId}/join`);
	}

	leaveVoice(channelId: string): Promise<void> {
		return this.post(`/voice/${channelId}/leave`);
	}

	getVoicePreferences(): Promise<VoicePreferences> {
		return this.get('/voice/preferences');
	}

	updateVoicePreferences(prefs: Partial<VoicePreferences>): Promise<VoicePreferences> {
		return this.patch('/voice/preferences', prefs);
	}

	// --- Federation Voice ---

	joinFederatedVoice(instanceDomain: string, channelId: string, screenShare = false): Promise<{ token: string; url: string; channel_id: string }> {
		return this.post('/federation/voice/join', { instance_domain: instanceDomain, channel_id: channelId, screen_share: screenShare });
	}

	joinFederatedVoiceByGuild(guildId: string, channelId: string, screenShare = false): Promise<{ token: string; url: string; channel_id: string }> {
		return this.post('/federation/voice/guild-join', { guild_id: guildId, channel_id: channelId, screen_share: screenShare });
	}

	// --- Federation Guilds ---

	joinFederatedGuild(instanceDomain: string, guildId?: string, inviteCode?: string): Promise<unknown> {
		return this.post('/federation/guilds/join', { instance_domain: instanceDomain, guild_id: guildId, invite_code: inviteCode });
	}

	leaveFederatedGuild(guildId: string): Promise<void> {
		return this.post(`/federation/guilds/${guildId}/leave`);
	}

	getFederatedGuildMessages(guildId: string, channelId: string, params?: { before?: string; after?: string; limit?: number }): Promise<Message[]> {
		const query = new URLSearchParams();
		if (params?.before) query.set('before', params.before);
		if (params?.after) query.set('after', params.after);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/federation/guilds/${guildId}/channels/${channelId}/messages${qs ? `?${qs}` : ''}`);
	}

	sendFederatedGuildMessage(guildId: string, channelId: string, content: string, nonce?: string): Promise<Message> {
		return this.post(`/federation/guilds/${guildId}/channels/${channelId}/messages`, { content, nonce });
	}

	// --- File Upload ---

	async uploadFile(file: File, altText?: string): Promise<{ id: string; url: string }> {
		const formData = new FormData();
		formData.append('file', file);
		if (altText) {
			formData.append('alt_text', altText);
		}

		const headers: Record<string, string> = {};
		const token = this.getToken();
		if (token) headers['Authorization'] = `Bearer ${token}`;

		let res: Response;
		try {
			res = await fetch(`${API_BASE}/files/upload`, {
				method: 'POST',
				headers,
				body: formData
			});
		} catch {
			throw new ApiRequestError('Network error during upload — file may be too large', 'network_error', 0);
		}

		let json: unknown;
		try {
			json = await res.json();
		} catch {
			throw new ApiRequestError(
				res.ok ? 'Invalid server response' : `Upload failed (${res.status})`,
				'upload_failed',
				res.status
			);
		}

		if (!res.ok) {
			const err = json as ApiError;
			throw new ApiRequestError(err.error?.message || 'Upload failed', 'upload_failed', res.status);
		}
		return (json as ApiResponse<{ id: string; url: string }>).data;
	}

	// --- Search ---

	searchMessages(query: string, guildId?: string, channelId?: string): Promise<Message[]> {
		const params = new URLSearchParams({ q: query });
		if (guildId) params.set('guild_id', guildId);
		if (channelId) params.set('channel_id', channelId);
		return this.get(`/search/messages?${params}`);
	}

	// --- Threads ---

	createThread(channelId: string, messageId: string, name: string): Promise<Channel> {
		return this.post(`/channels/${channelId}/messages/${messageId}/threads`, { name });
	}

	getThreads(channelId: string): Promise<Channel[]> {
		return this.get(`/channels/${channelId}/threads`);
	}

	hideThread(channelId: string, threadId: string): Promise<void> {
		return this.post(`/channels/${channelId}/threads/${threadId}/hide`);
	}

	unhideThread(channelId: string, threadId: string): Promise<void> {
		return this.del(`/channels/${channelId}/threads/${threadId}/hide`);
	}

	getHiddenThreads(): Promise<string[]> {
		return this.get('/users/@me/hidden-threads');
	}

	// --- Message Edit History ---

	getMessageEdits(channelId: string, messageId: string): Promise<{ content: string; edited_at: string }[]> {
		return this.get(`/channels/${channelId}/messages/${messageId}/edits`);
	}

	translateMessage(channelId: string, messageId: string, targetLang: string, force = false): Promise<{ message_id: string; source_lang: string; target_lang: string; translated_text: string; cached: boolean }> {
		return this.post(`/channels/${channelId}/messages/${messageId}/translate`, { target_lang: targetLang, force });
	}

	// --- User Notes ---

	getUserNote(userId: string): Promise<{ target_id: string; note: string }> {
		return this.get(`/users/${userId}/note`);
	}

	setUserNote(userId: string, note: string): Promise<{ target_id: string; note: string }> {
		return this.put(`/users/${userId}/note`, { note });
	}

	getMutualFriends(userId: string): Promise<User[]> {
		return this.get(`/users/${userId}/mutual-friends`);
	}

	getMutualGuilds(userId: string): Promise<MutualGuild[]> {
		return this.get(`/users/${userId}/mutual-guilds`);
	}

	// --- Giphy ---

	searchGiphy(query: string, limit = 25, offset = 0): Promise<any> {
		return this.get(`/giphy/search?q=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}`);
	}

	getTrendingGiphy(limit = 25, offset = 0): Promise<any> {
		return this.get(`/giphy/trending?limit=${limit}&offset=${offset}`);
	}

	getGiphyCategories(limit = 15): Promise<any> {
		return this.get(`/giphy/categories?limit=${limit}`);
	}

	// --- Gallery ---

	getChannelGallery(channelId: string, options?: { before?: string; type?: string }): Promise<Attachment[]> {
		const params = new URLSearchParams();
		if (options?.before) params.set('before', options.before);
		if (options?.type) params.set('type', options.type);
		const qs = params.toString();
		return this.get(`/channels/${channelId}/gallery${qs ? '?' + qs : ''}`);
	}

	getGuildGallery(guildId: string, options?: { before?: string; type?: string }): Promise<Attachment[]> {
		const params = new URLSearchParams();
		if (options?.before) params.set('before', options.before);
		if (options?.type) params.set('type', options.type);
		const qs = params.toString();
		return this.get(`/guilds/${guildId}/gallery${qs ? '?' + qs : ''}`);
	}

	updateAttachment(fileId: string, data: { nsfw?: boolean; alt_text?: string; description?: string }): Promise<Attachment> {
		return this.patch(`/files/${fileId}`, data);
	}

	deleteAttachment(fileId: string): Promise<void> {
		return this.del(`/files/${fileId}`);
	}

	getMediaTags(guildId: string): Promise<MediaTag[]> {
		return this.get(`/guilds/${guildId}/media-tags`);
	}

	createMediaTag(guildId: string, name: string): Promise<MediaTag> {
		return this.post(`/guilds/${guildId}/media-tags`, { name });
	}

	deleteMediaTag(guildId: string, tagId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/media-tags/${tagId}`);
	}

	tagAttachment(fileId: string, tagId: string): Promise<void> {
		return this.put(`/files/${fileId}/tags/${tagId}`);
	}

	untagAttachment(fileId: string, tagId: string): Promise<void> {
		return this.del(`/files/${fileId}/tags/${tagId}`);
	}

	getAdminMedia(before?: string): Promise<Attachment[]> {
		return this.get(before ? `/admin/media?before=${before}` : '/admin/media');
	}

	deleteAdminMedia(fileId: string): Promise<void> {
		return this.del(`/admin/media/${fileId}`);
	}

	// --- Admin ---

	getAdminStats(): Promise<AdminStats> {
		return this.get('/admin/stats');
	}

	getAdminInstance(): Promise<InstanceInfo> {
		return this.get('/admin/instance');
	}

	updateAdminInstance(data: { name?: string; description?: string; federation_mode?: string }): Promise<InstanceInfo> {
		return this.patch('/admin/instance', data);
	}

	// --- Admin Guilds ---

	getAdminGuilds(params?: { limit?: number; offset?: number; query?: string; sort?: string }): Promise<any[]> {
		const q = new URLSearchParams();
		if (params?.limit) q.set('limit', String(params.limit));
		if (params?.offset) q.set('offset', String(params.offset));
		if (params?.query) q.set('q', params.query);
		if (params?.sort) q.set('sort', params.sort);
		const qs = q.toString();
		return this.get(`/admin/guilds${qs ? '?' + qs : ''}`);
	}

	getAdminGuildDetails(guildId: string): Promise<any> {
		return this.get(`/admin/guilds/${guildId}`);
	}

	adminDeleteGuild(guildId: string): Promise<void> {
		return this.del(`/admin/guilds/${guildId}`);
	}

	getAdminUserGuilds(userId: string): Promise<any[]> {
		return this.get(`/admin/users/${userId}/guilds`);
	}

	// --- Admin Users ---

	getAdminUsers(params?: { limit?: number; offset?: number; query?: string }): Promise<User[]> {
		const q = new URLSearchParams();
		if (params?.limit) q.set('limit', String(params.limit));
		if (params?.offset) q.set('offset', String(params.offset));
		if (params?.query) q.set('q', params.query);
		const qs = q.toString();
		return this.get(`/admin/users${qs ? '?' + qs : ''}`);
	}

	suspendUser(userId: string): Promise<void> {
		return this.post(`/admin/users/${userId}/suspend`);
	}

	unsuspendUser(userId: string): Promise<void> {
		return this.post(`/admin/users/${userId}/unsuspend`);
	}

	setAdmin(userId: string, isAdmin: boolean): Promise<void> {
		return this.post(`/admin/users/${userId}/set-admin`, { admin: isAdmin });
	}

	// --- Admin Federation ---

	getFederationPeers(): Promise<FederationPeer[]> {
		return this.get('/admin/federation/peers');
	}

	addFederationPeer(domain: string): Promise<FederationPeer> {
		return this.post('/admin/federation/peers', { domain });
	}

	removeFederationPeer(peerId: string): Promise<void> {
		return this.del(`/admin/federation/peers/${peerId}`);
	}

	// --- Admin Instance Bans ---

	instanceBanUser(userId: string, reason: string): Promise<void> {
		return this.post(`/admin/users/${userId}/instance-ban`, { reason });
	}

	instanceUnbanUser(userId: string): Promise<void> {
		return this.post(`/admin/users/${userId}/instance-unban`);
	}

	getInstanceBans(): Promise<InstanceBan[]> {
		return this.get('/admin/instance-bans');
	}

	// --- Admin Registration ---

	getRegistrationSettings(): Promise<RegistrationSettings> {
		return this.get('/admin/registration');
	}

	updateRegistrationSettings(data: { mode?: string; message?: string | null }): Promise<RegistrationSettings> {
		return this.patch('/admin/registration', data);
	}

	createRegistrationToken(data: { max_uses?: number; note?: string; expires_in_hours?: number }): Promise<RegistrationToken> {
		return this.post('/admin/registration/tokens', data);
	}

	getRegistrationTokens(): Promise<RegistrationToken[]> {
		return this.get('/admin/registration/tokens');
	}

	deleteRegistrationToken(tokenId: string): Promise<void> {
		return this.del(`/admin/registration/tokens/${tokenId}`);
	}

	// --- Admin Announcements ---

	createAnnouncement(data: { title: string; content: string; severity: string; expires_in_hours?: number }): Promise<Announcement> {
		return this.post('/admin/announcements', data);
	}

	getAdminAnnouncements(): Promise<Announcement[]> {
		return this.get('/admin/announcements');
	}

	updateAnnouncement(id: string, data: { active?: boolean; title?: string; content?: string }): Promise<Announcement> {
		return this.patch(`/admin/announcements/${id}`, data);
	}

	deleteAnnouncement(id: string): Promise<void> {
		return this.del(`/admin/announcements/${id}`);
	}

	// --- Active Announcements (all users) ---

	getActiveAnnouncements(): Promise<Announcement[]> {
		return this.get('/announcements');
	}

	// --- Notification Preferences ---

	getNotificationPreferences(guildId?: string): Promise<NotificationPreference> {
		const qs = guildId ? `?guild_id=${guildId}` : '';
		return this.get(`/notifications/preferences${qs}`);
	}

	updateNotificationPreferences(data: Partial<NotificationPreference>): Promise<NotificationPreference> {
		return this.patch('/notifications/preferences', data);
	}

	// --- Channel Notification Preferences ---

	getChannelNotificationPreferences(): Promise<ChannelNotificationPreference[]> {
		return this.get('/notifications/preferences/channels');
	}

	updateChannelNotificationPreference(data: { channel_id: string; level: string; muted_until?: string | null }): Promise<ChannelNotificationPreference> {
		return this.patch('/notifications/preferences/channels', data);
	}

	deleteChannelNotificationPreference(channelId: string): Promise<void> {
		return this.del(`/notifications/preferences/channels/${channelId}`);
	}

	// --- User Settings (privacy/prefs) ---

	getUserSettings(): Promise<UserSettings> {
		return this.get('/users/@me/settings');
	}

	updateUserSettings(data: Partial<UserSettings>): Promise<UserSettings> {
		return this.patch('/users/@me/settings', data);
	}

	// --- Webhooks ---

	getGuildWebhooks(guildId: string): Promise<Webhook[]> {
		return this.get(`/guilds/${guildId}/webhooks`);
	}

	getChannelWebhooks(channelId: string): Promise<Webhook[]> {
		return this.get(`/channels/${channelId}/webhooks`);
	}

	createWebhook(guildId: string, data: { name: string; channel_id: string }): Promise<Webhook> {
		return this.post(`/guilds/${guildId}/webhooks`, data);
	}

	updateWebhook(guildId: string, webhookId: string, data: { name?: string; avatar_id?: string; channel_id?: string }): Promise<Webhook> {
		return this.patch(`/guilds/${guildId}/webhooks/${webhookId}`, data);
	}

	deleteWebhook(guildId: string, webhookId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/webhooks/${webhookId}`);
	}

	// --- Categories ---

	getCategories(guildId: string): Promise<Category[]> {
		return this.get(`/guilds/${guildId}/categories`);
	}

	createCategory(guildId: string, name: string): Promise<Category> {
		return this.post(`/guilds/${guildId}/categories`, { name });
	}

	updateCategory(guildId: string, categoryId: string, data: { name?: string; position?: number }): Promise<Category> {
		return this.patch(`/guilds/${guildId}/categories/${categoryId}`, data);
	}

	deleteCategory(guildId: string, categoryId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/categories/${categoryId}`);
	}

	// --- Message Forwarding ---

	forwardMessage(channelId: string, messageId: string, targetChannelId: string): Promise<Message> {
		return this.post(`/channels/${channelId}/messages/${messageId}/crosspost`, { target_channel_id: targetChannelId });
	}

	// --- Emoji Upload ---

	async uploadEmoji(guildId: string, name: string, file: File): Promise<CustomEmoji> {
		const formData = new FormData();
		formData.append('file', file);
		formData.append('name', name);

		const headers: Record<string, string> = {};
		const token = this.getToken();
		if (token) headers['Authorization'] = `Bearer ${token}`;

		const res = await fetch(`${API_BASE}/guilds/${guildId}/emoji`, {
			method: 'POST',
			headers,
			body: formData
		});

		const json = await res.json();
		if (!res.ok) {
			const err = json as ApiError;
			throw new ApiRequestError(err.error?.message || 'Upload failed', 'upload_failed', res.status);
		}
		return (json as ApiResponse<CustomEmoji>).data;
	}

	// --- Encryption/MLS ---

	uploadKeyPackage(keyPackage: string): Promise<{ id: string }> {
		return this.post('/encryption/key-packages', { key_package: keyPackage });
	}

	getKeyPackages(userId: string): Promise<{ id: string; key_package: string }[]> {
		return this.get(`/encryption/key-packages/${userId}`);
	}

	claimKeyPackage(userId: string): Promise<{ id: string; key_package: string }> {
		return this.post(`/encryption/key-packages/${userId}/claim`);
	}

	getWelcomeMessages(): Promise<{ id: string; channel_id: string; sender_id: string; data: string; created_at: string }[]> {
		return this.get('/encryption/welcome');
	}

	sendWelcome(channelId: string, userId: string, data: string): Promise<void> {
		return this.post(`/encryption/channels/${channelId}/welcome`, { user_id: userId, data });
	}

	getGroupState(channelId: string): Promise<{ epoch: number; state: string }> {
		return this.get(`/encryption/channels/${channelId}/group-state`);
	}

	updateGroupState(channelId: string, epoch: number, state: string): Promise<void> {
		return this.put(`/encryption/channels/${channelId}/group-state`, { epoch, state });
	}

	ackWelcome(welcomeId: string): Promise<void> {
		return this.del(`/encryption/welcome/${welcomeId}`);
	}

	deleteKeyPackage(keyPackageId: string): Promise<void> {
		return this.del(`/encryption/key-packages/${keyPackageId}`);
	}

	// --- Push Notifications ---

	getVapidKey(): Promise<{ public_key: string }> {
		return this.get('/notifications/vapid-key');
	}

	subscribePush(subscription: { endpoint: string; keys: { p256dh: string; auth: string } }): Promise<{ id: string }> {
		return this.post('/notifications/subscriptions', subscription);
	}

	getPushSubscriptions(): Promise<{ id: string; endpoint: string; created_at: string }[]> {
		return this.get('/notifications/subscriptions');
	}

	deletePushSubscription(subscriptionId: string): Promise<void> {
		return this.del(`/notifications/subscriptions/${subscriptionId}`);
	}

	// --- Polls ---

	createPoll(channelId: string, question: string, options: string[], opts?: { multi_vote?: boolean; anonymous?: boolean; duration?: number }): Promise<Poll> {
		return this.post(`/channels/${channelId}/polls`, { question, options, ...opts });
	}

	getPoll(channelId: string, pollId: string): Promise<Poll> {
		return this.get(`/channels/${channelId}/polls/${pollId}`);
	}

	votePoll(channelId: string, pollId: string, optionIds: string[]): Promise<{ poll_id: string; option_ids: string[] }> {
		return this.post(`/channels/${channelId}/polls/${pollId}/votes`, { option_ids: optionIds });
	}

	closePoll(channelId: string, pollId: string): Promise<{ poll_id: string; closed: boolean }> {
		return this.post(`/channels/${channelId}/polls/${pollId}/close`);
	}

	deletePoll(channelId: string, pollId: string): Promise<void> {
		return this.del(`/channels/${channelId}/polls/${pollId}`);
	}

	// --- Bookmarks ---

	createBookmark(messageId: string, note?: string, reminderAt?: string): Promise<MessageBookmark> {
		const body: Record<string, string> = {};
		if (note) body.note = note;
		if (reminderAt) body.reminder_at = reminderAt;
		return this.put(`/messages/${messageId}/bookmark`, Object.keys(body).length > 0 ? body : undefined);
	}

	deleteBookmark(messageId: string): Promise<void> {
		return this.del(`/messages/${messageId}/bookmark`);
	}

	getBookmarks(params?: { limit?: number; before?: string }): Promise<MessageBookmark[]> {
		const query = new URLSearchParams();
		if (params?.limit) query.set('limit', String(params.limit));
		if (params?.before) query.set('before', params.before);
		const qs = query.toString();
		return this.get(`/users/@me/bookmarks${qs ? '?' + qs : ''}`);
	}

	// --- Guild Events ---

	createGuildEvent(guildId: string, data: { name: string; description?: string; location?: string; channel_id?: string; image_id?: string; scheduled_start: string; scheduled_end?: string }): Promise<GuildEvent> {
		return this.post(`/guilds/${guildId}/events`, data);
	}

	getGuildEvents(guildId: string, params?: { status?: string; limit?: number }): Promise<GuildEvent[]> {
		const query = new URLSearchParams();
		if (params?.status) query.set('status', params.status);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/guilds/${guildId}/events${qs ? '?' + qs : ''}`);
	}

	getGuildEvent(guildId: string, eventId: string): Promise<GuildEvent> {
		return this.get(`/guilds/${guildId}/events/${eventId}`);
	}

	updateGuildEvent(guildId: string, eventId: string, data: Partial<{ name: string; description: string; location: string; channel_id: string; scheduled_start: string; scheduled_end: string; status: string }>): Promise<GuildEvent> {
		return this.patch(`/guilds/${guildId}/events/${eventId}`, data);
	}

	deleteGuildEvent(guildId: string, eventId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/events/${eventId}`);
	}

	rsvpEvent(guildId: string, eventId: string, status: 'interested' | 'going'): Promise<EventRSVP> {
		return this.post(`/guilds/${guildId}/events/${eventId}/rsvp`, { status });
	}

	deleteRsvp(guildId: string, eventId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/events/${eventId}/rsvp`);
	}

	getEventRsvps(guildId: string, eventId: string): Promise<EventRSVP[]> {
		return this.get(`/guilds/${guildId}/events/${eventId}/rsvps`);
	}

	// --- Server Discovery ---

	discoverGuilds(params?: Record<string, string>): Promise<Guild[]> {
		const query = new URLSearchParams(params);
		const qs = query.toString();
		return this.get(`/guilds/discover${qs ? '?' + qs : ''}`);
	}

	getGuildPreview(guildId: string): Promise<Guild & { member_count: number }> {
		return this.get(`/guilds/${guildId}/preview`);
	}

	joinGuild(guildId: string): Promise<{ guild_id: string; name: string; joined: boolean }> {
		return this.post(`/guilds/${guildId}/join`, {});
	}

	// --- Moderation: Warnings ---

	warnMember(guildId: string, memberId: string, reason: string): Promise<MemberWarning> {
		return this.post(`/guilds/${guildId}/members/${memberId}/warn`, { reason });
	}

	getMemberWarnings(guildId: string, memberId: string): Promise<MemberWarning[]> {
		return this.get(`/guilds/${guildId}/members/${memberId}/warnings`);
	}

	deleteWarning(guildId: string, warningId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/warnings/${warningId}`);
	}

	// --- Moderation: Reports ---

	reportMessage(channelId: string, messageId: string, reason: string): Promise<MessageReport> {
		return this.post(`/channels/${channelId}/messages/${messageId}/report`, { reason });
	}

	getReports(guildId: string, params?: { status?: string }): Promise<MessageReport[]> {
		const query = new URLSearchParams();
		if (params?.status) query.set('status', params.status);
		const qs = query.toString();
		return this.get(`/guilds/${guildId}/reports${qs ? '?' + qs : ''}`);
	}

	resolveReport(guildId: string, reportId: string, status: 'resolved' | 'dismissed'): Promise<MessageReport> {
		return this.patch(`/guilds/${guildId}/reports/${reportId}`, { status });
	}

	// --- Moderation: Channel Lock ---

	lockChannel(channelId: string): Promise<{ locked: boolean }> {
		return this.post(`/channels/${channelId}/lock`);
	}

	unlockChannel(channelId: string): Promise<{ locked: boolean }> {
		return this.post(`/channels/${channelId}/unlock`);
	}

	// --- Moderation: Raid Config ---

	getRaidConfig(guildId: string): Promise<RaidConfig> {
		return this.get(`/guilds/${guildId}/raid-config`);
	}

	updateRaidConfig(guildId: string, data: Partial<RaidConfig>): Promise<RaidConfig> {
		return this.patch(`/guilds/${guildId}/raid-config`, data);
	}

	// --- AutoMod ---

	getAutoModRules(guildId: string): Promise<AutoModRule[]> {
		return this.get(`/guilds/${guildId}/automod/rules`);
	}

	createAutoModRule(guildId: string, data: Partial<AutoModRule>): Promise<AutoModRule> {
		return this.post(`/guilds/${guildId}/automod/rules`, data);
	}

	getAutoModRule(guildId: string, ruleId: string): Promise<AutoModRule> {
		return this.get(`/guilds/${guildId}/automod/rules/${ruleId}`);
	}

	updateAutoModRule(guildId: string, ruleId: string, data: Partial<AutoModRule>): Promise<AutoModRule> {
		return this.patch(`/guilds/${guildId}/automod/rules/${ruleId}`, data);
	}

	deleteAutoModRule(guildId: string, ruleId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/automod/rules/${ruleId}`);
	}

	getAutoModActions(guildId: string): Promise<AutoModAction[]> {
		return this.get(`/guilds/${guildId}/automod/actions`);
	}

	testAutoModRule(guildId: string, data: { rule_type: string; config: Record<string, unknown>; sample_text: string }): Promise<{ matched: boolean; matched_content: string | null }> {
		return this.post(`/guilds/${guildId}/automod/rules/test`, data);
	}

	// --- Role Updates ---

	updateRole(guildId: string, roleId: string, data: Partial<Role>): Promise<Role> {
		return this.patch(`/guilds/${guildId}/roles/${roleId}`, data);
	}

	deleteRole(guildId: string, roleId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/roles/${roleId}`);
	}

	assignRole(guildId: string, memberId: string, roleId: string): Promise<void> {
		return this.put(`/guilds/${guildId}/members/${memberId}/roles/${roleId}`);
	}

	removeRole(guildId: string, memberId: string, roleId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/members/${memberId}/roles/${roleId}`);
	}

	reorderRoles(guildId: string, positions: { id: string; position: number }[]): Promise<Role[]> {
		return this.patch(`/guilds/${guildId}/roles`, positions);
	}

	// --- Onboarding ---

	getOnboarding(guildId: string): Promise<OnboardingConfig> {
		return this.get(`/guilds/${guildId}/onboarding`);
	}

	updateOnboarding(guildId: string, data: Partial<OnboardingConfig>): Promise<OnboardingConfig> {
		return this.put(`/guilds/${guildId}/onboarding`, data);
	}

	createOnboardingPrompt(guildId: string, data: { title: string; required?: boolean; single_select?: boolean; options: { label: string; description?: string; emoji?: string; role_ids?: string[]; channel_ids?: string[] }[] }): Promise<OnboardingPrompt> {
		return this.post(`/guilds/${guildId}/onboarding/prompts`, data);
	}

	updateOnboardingPrompt(guildId: string, promptId: string, data: Partial<OnboardingPrompt>): Promise<void> {
		return this.put(`/guilds/${guildId}/onboarding/prompts/${promptId}`, data);
	}

	deleteOnboardingPrompt(guildId: string, promptId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/onboarding/prompts/${promptId}`);
	}

	completeOnboarding(guildId: string, responses: Record<string, string[]>): Promise<void> {
		return this.post(`/guilds/${guildId}/onboarding/complete`, { prompt_responses: responses });
	}

	getOnboardingStatus(guildId: string): Promise<{ completed: boolean }> {
		return this.get(`/guilds/${guildId}/onboarding/status`);
	}

	// --- Ban Lists ---

	getBanLists(guildId: string): Promise<BanList[]> {
		return this.get(`/guilds/${guildId}/ban-lists`);
	}

	createBanList(guildId: string, data: { name: string; description?: string; public?: boolean }): Promise<BanList> {
		return this.post(`/guilds/${guildId}/ban-lists`, data);
	}

	deleteBanList(guildId: string, listId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/ban-lists/${listId}`);
	}

	getBanListEntries(guildId: string, listId: string): Promise<BanListEntry[]> {
		return this.get(`/guilds/${guildId}/ban-lists/${listId}/entries`);
	}

	addBanListEntry(guildId: string, listId: string, data: { user_id: string; reason?: string }): Promise<BanListEntry> {
		return this.post(`/guilds/${guildId}/ban-lists/${listId}/entries`, data);
	}

	removeBanListEntry(guildId: string, listId: string, entryId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/ban-lists/${listId}/entries/${entryId}`);
	}

	exportBanList(guildId: string, listId: string): Promise<any> {
		return this.get(`/guilds/${guildId}/ban-lists/${listId}/export`);
	}

	importBanList(guildId: string, listId: string, data: any): Promise<void> {
		return this.post(`/guilds/${guildId}/ban-lists/${listId}/import`, data);
	}

	getBanListSubscriptions(guildId: string): Promise<BanListSubscription[]> {
		return this.get(`/guilds/${guildId}/ban-list-subscriptions`);
	}

	subscribeBanList(guildId: string, data: { list_id: string; auto_ban?: boolean }): Promise<BanListSubscription> {
		return this.post(`/guilds/${guildId}/ban-list-subscriptions`, data);
	}

	unsubscribeBanList(guildId: string, subId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/ban-list-subscriptions/${subId}`);
	}

	getPublicBanLists(): Promise<BanList[]> {
		return this.get('/ban-lists/public');
	}

	// --- Channel Followers ---

	followChannel(channelId: string, webhookData: { target_channel_id: string }): Promise<ChannelFollower> {
		return this.post(`/channels/${channelId}/followers`, webhookData);
	}

	getChannelFollowers(channelId: string): Promise<ChannelFollower[]> {
		return this.get(`/channels/${channelId}/followers`);
	}

	unfollowChannel(channelId: string, followerId: string): Promise<void> {
		return this.del(`/channels/${channelId}/followers/${followerId}`);
	}

	// --- Bots ---

	getMyBots(): Promise<User[]> {
		return this.get('/users/@me/bots');
	}

	createBot(name: string, description?: string): Promise<User> {
		return this.post('/users/@me/bots', { name, description: description ?? '' });
	}

	getBot(botId: string): Promise<User> {
		return this.get(`/bots/${botId}`);
	}

	updateBot(botId: string, data: { name?: string; description?: string }): Promise<User> {
		return this.patch(`/bots/${botId}`, data);
	}

	deleteBot(botId: string): Promise<void> {
		return this.del(`/bots/${botId}`);
	}

	getBotTokens(botId: string): Promise<BotToken[]> {
		return this.get(`/bots/${botId}/tokens`);
	}

	createBotToken(botId: string, name?: string): Promise<BotToken> {
		return this.post(`/bots/${botId}/tokens`, { name: name ?? 'default' });
	}

	deleteBotToken(botId: string, tokenId: string): Promise<void> {
		return this.del(`/bots/${botId}/tokens/${tokenId}`);
	}

	getBotCommands(botId: string): Promise<SlashCommand[]> {
		return this.get(`/bots/${botId}/commands`);
	}

	registerBotCommand(botId: string, data: { name: string; description: string; guild_id?: string; options?: unknown[] }): Promise<SlashCommand> {
		return this.post(`/bots/${botId}/commands`, data);
	}

	updateBotCommand(botId: string, commandId: string, data: { name?: string; description?: string; options?: unknown[] }): Promise<SlashCommand> {
		return this.patch(`/bots/${botId}/commands/${commandId}`, data);
	}

	deleteBotCommand(botId: string, commandId: string): Promise<void> {
		return this.del(`/bots/${botId}/commands/${commandId}`);
	}

	// --- Sticker Packs ---

	getGuildStickerPacks(guildId: string): Promise<StickerPack[]> {
		return this.get(`/guilds/${guildId}/sticker-packs`);
	}

	createGuildStickerPack(guildId: string, name: string, description?: string): Promise<StickerPack> {
		return this.post(`/guilds/${guildId}/sticker-packs`, { name, description });
	}

	deleteGuildStickerPack(guildId: string, packId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/sticker-packs/${packId}`);
	}

	getPackStickers(guildId: string, packId: string): Promise<Sticker[]> {
		return this.get(`/guilds/${guildId}/sticker-packs/${packId}/stickers`);
	}

	addStickerToGuildPack(guildId: string, packId: string, data: { name: string; file_id: string; format: string; description?: string; tags?: string }): Promise<Sticker> {
		return this.post(`/guilds/${guildId}/sticker-packs/${packId}/stickers`, data);
	}

	deleteStickerFromGuildPack(guildId: string, packId: string, stickerId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/sticker-packs/${packId}/stickers/${stickerId}`);
	}

	getUserStickerPacks(): Promise<StickerPack[]> {
		return this.get('/stickers/my-packs');
	}

	createUserStickerPack(name: string, description?: string): Promise<StickerPack> {
		return this.post('/stickers/my-packs', { name, description });
	}

	// --- Activities ---

	getActiveSession<T>(activityId: string): Promise<T> {
		return this.get(`/activities/${activityId}/sessions/active`);
	}

	listActivities<T>(category?: string): Promise<T> {
		const url = category ? `/activities?category=${category}` : '/activities';
		return this.get(url);
	}

	createActivitySession<T>(activityId: string, body: unknown): Promise<T> {
		return this.post(`/activities/${activityId}/sessions`, body);
	}

	joinActivitySession(sessionId: string): Promise<void> {
		return this.post(`/activities/sessions/${sessionId}/join`);
	}

	leaveActivitySession(sessionId: string): Promise<void> {
		return this.post(`/activities/sessions/${sessionId}/leave`);
	}

	endActivitySession(sessionId: string): Promise<void> {
		return this.post(`/activities/sessions/${sessionId}/end`);
	}

	// --- Kanban ---

	createKanbanBoard<T>(channelId: string, body: unknown): Promise<T> {
		return this.post(`/channels/${channelId}/experimental/kanban`, body);
	}

	getKanbanBoard<T>(channelId: string, boardId: string): Promise<T> {
		return this.get(`/channels/${channelId}/experimental/kanban/${boardId}`);
	}

	createKanbanColumn(channelId: string, boardId: string, body: unknown): Promise<void> {
		return this.post(`/channels/${channelId}/experimental/kanban/${boardId}/columns`, body);
	}

	createKanbanCard(channelId: string, boardId: string, columnId: string, body: unknown): Promise<void> {
		return this.post(`/channels/${channelId}/experimental/kanban/${boardId}/columns/${columnId}/cards`, body);
	}

	moveKanbanCard(channelId: string, boardId: string, cardId: string, body: unknown): Promise<void> {
		return this.patch(`/channels/${channelId}/experimental/kanban/${boardId}/cards/${cardId}/move`, body);
	}

	deleteKanbanCard(channelId: string, boardId: string, cardId: string): Promise<void> {
		return this.del(`/channels/${channelId}/experimental/kanban/${boardId}/cards/${cardId}`);
	}

	// --- Global Moderation ---

	reportUser(userId: string, reason: string, contextGuildId?: string, contextChannelId?: string): Promise<{ id: string; status: string }> {
		return this.post(`/users/${userId}/report`, { reason, context_guild_id: contextGuildId, context_channel_id: contextChannelId });
	}

	reportMessageToAdmins(channelId: string, messageId: string, reason: string): Promise<MessageReport> {
		return this.post(`/channels/${channelId}/messages/${messageId}/report-admin`, { reason });
	}

	createIssue(title: string, description: string, category: string): Promise<{ id: string; status: string }> {
		return this.post('/issues', { title, description, category });
	}

	getMyIssues(): Promise<ReportedIssue[]> {
		return this.get('/users/@me/issues');
	}

	getModerationStats(): Promise<ModerationStats> {
		return this.get('/moderation/stats');
	}

	getModerationUserReports(status?: string): Promise<UserReport[]> {
		const url = status ? `/moderation/user-reports?status=${status}` : '/moderation/user-reports';
		return this.get(url);
	}

	resolveModerationUserReport(reportId: string, status: string, notes?: string): Promise<void> {
		return this.patch(`/moderation/user-reports/${reportId}`, { status, notes });
	}

	getModerationMessageReports(status?: string): Promise<ModerationMessageReport[]> {
		const url = status ? `/moderation/message-reports?status=${status}` : '/moderation/message-reports';
		return this.get(url);
	}

	resolveModerationMessageReport(reportId: string, status: string, notes?: string): Promise<void> {
		return this.patch(`/moderation/message-reports/${reportId}`, { status, notes });
	}

	getModerationIssues(status?: string): Promise<ReportedIssue[]> {
		const url = status ? `/moderation/issues?status=${status}` : '/moderation/issues';
		return this.get(url);
	}

	resolveModerationIssue(issueId: string, status: string, notes?: string): Promise<void> {
		return this.patch(`/moderation/issues/${issueId}`, { status, notes });
	}

	setGlobalMod(userId: string, globalMod: boolean): Promise<void> {
		return this.post(`/admin/users/${userId}/set-globalmod`, { global_mod: globalMod });
	}

	// --- Guild Retention Policies ---

	getGuildRetentionPolicies(guildId: string): Promise<RetentionPolicy[]> {
		return this.get(`/guilds/${guildId}/retention`);
	}

	createGuildRetentionPolicy(guildId: string, policy: {
		channel_id?: string;
		max_age_days: number;
		delete_attachments?: boolean;
		delete_pins?: boolean;
	}): Promise<RetentionPolicy> {
		return this.post(`/guilds/${guildId}/retention`, policy);
	}

	updateGuildRetentionPolicy(guildId: string, policyId: string, update: {
		max_age_days?: number;
		delete_attachments?: boolean;
		delete_pins?: boolean;
		enabled?: boolean;
	}): Promise<RetentionPolicy> {
		return this.patch(`/guilds/${guildId}/retention/${policyId}`, update);
	}

	deleteGuildRetentionPolicy(guildId: string, policyId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/retention/${policyId}`);
	}

	// --- Forum Tags ---

	getForumTags(channelId: string): Promise<ForumTag[]> {
		return this.get(`/channels/${channelId}/tags`);
	}

	createForumTag(channelId: string, tag: { name: string; emoji?: string; color?: string }): Promise<ForumTag> {
		return this.post(`/channels/${channelId}/tags`, tag);
	}

	updateForumTag(channelId: string, tagId: string, update: { name?: string; emoji?: string; color?: string }): Promise<ForumTag> {
		return this.patch(`/channels/${channelId}/tags/${tagId}`, update);
	}

	deleteForumTag(channelId: string, tagId: string): Promise<void> {
		return this.del(`/channels/${channelId}/tags/${tagId}`);
	}

	// --- Forum Posts ---

	getForumPosts(channelId: string, params?: {
		sort?: 'latest_activity' | 'creation_date';
		tag?: string;
		before?: string;
		limit?: number;
	}): Promise<ForumPost[]> {
		const query = new URLSearchParams();
		if (params?.sort) query.set('sort', params.sort);
		if (params?.tag) query.set('tag', params.tag);
		if (params?.before) query.set('before', params.before);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/channels/${channelId}/posts${qs ? '?' + qs : ''}`);
	}

	createForumPost(channelId: string, post: {
		title: string;
		content: string;
		tag_ids?: string[];
		attachment_ids?: string[];
	}): Promise<ForumPost> {
		return this.post(`/channels/${channelId}/posts`, post);
	}

	pinForumPost(channelId: string, postId: string): Promise<void> {
		return this.post(`/channels/${channelId}/posts/${postId}/pin`);
	}

	closeForumPost(channelId: string, postId: string): Promise<void> {
		return this.post(`/channels/${channelId}/posts/${postId}/close`);
	}

	// --- Gallery Tags ---

	getGalleryTags(channelId: string): Promise<GalleryTag[]> {
		return this.get(`/channels/${channelId}/gallery-tags`);
	}

	createGalleryTag(channelId: string, tag: { name: string; emoji?: string; color?: string }): Promise<GalleryTag> {
		return this.post(`/channels/${channelId}/gallery-tags`, tag);
	}

	updateGalleryTag(channelId: string, tagId: string, update: { name?: string; emoji?: string; color?: string }): Promise<GalleryTag> {
		return this.patch(`/channels/${channelId}/gallery-tags/${tagId}`, update);
	}

	deleteGalleryTag(channelId: string, tagId: string): Promise<void> {
		return this.del(`/channels/${channelId}/gallery-tags/${tagId}`);
	}

	// --- Gallery Posts ---

	getGalleryPosts(channelId: string, params?: {
		sort?: 'newest' | 'oldest' | 'most_comments';
		tag?: string;
		before?: string;
		limit?: number;
	}): Promise<GalleryPost[]> {
		const query = new URLSearchParams();
		if (params?.sort) query.set('sort', params.sort);
		if (params?.tag) query.set('tag', params.tag);
		if (params?.before) query.set('before', params.before);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/channels/${channelId}/gallery-posts${qs ? '?' + qs : ''}`);
	}

	createGalleryPost(channelId: string, post: {
		title?: string;
		description?: string;
		tag_ids?: string[];
		attachment_ids: string[];
	}): Promise<GalleryPost> {
		return this.post(`/channels/${channelId}/gallery-posts`, post);
	}

	pinGalleryPost(channelId: string, postId: string): Promise<void> {
		return this.post(`/channels/${channelId}/gallery-posts/${postId}/pin`);
	}

	closeGalleryPost(channelId: string, postId: string): Promise<void> {
		return this.post(`/channels/${channelId}/gallery-posts/${postId}/close`);
	}

	// --- Channel Groups ---

	getChannelGroups(): Promise<any[]> {
		return this.get('/users/@me/channel-groups');
	}

	createChannelGroup(data: { name: string }): Promise<any> {
		return this.post('/users/@me/channel-groups', data);
	}

	updateChannelGroup(groupId: string, data: { name?: string }): Promise<any> {
		return this.patch(`/users/@me/channel-groups/${groupId}`, data);
	}

	deleteChannelGroup(groupId: string): Promise<void> {
		return this.del(`/users/@me/channel-groups/${groupId}`);
	}

	removeChannelFromGroup(groupId: string, channelId: string): Promise<void> {
		return this.del(`/users/@me/channel-groups/${groupId}/channels/${channelId}`);
	}

	setChannelGroupChannels(groupId: string, channelIds: string[]): Promise<void> {
		return this.put(`/users/@me/channel-groups/${groupId}/channels`, { channel_ids: channelIds });
	}

	// --- Voice Extensions ---

	setVoiceInputMode(channelId: string, mode: string): Promise<void> {
		return this.post(`/voice/${channelId}/input-mode`, { mode });
	}

	setVoicePrioritySpeaker(channelId: string, userId: string, enabled: boolean): Promise<void> {
		return this.post(`/voice/${channelId}/members/${userId}/priority`, { enabled });
	}

	// --- Voice Broadcast ---

	getVoiceBroadcast(channelId: string): Promise<any> {
		return this.get(`/voice/${channelId}/broadcast`);
	}

	startVoiceBroadcast(channelId: string, data?: { title?: string }): Promise<any> {
		return this.post(`/voice/${channelId}/broadcast/start`, data);
	}

	stopVoiceBroadcast(channelId: string): Promise<void> {
		return this.post(`/voice/${channelId}/broadcast/stop`);
	}

	// --- Soundboard ---

	getSoundboardConfig(guildId: string): Promise<any> {
		return this.get(`/guilds/${guildId}/soundboard/config`);
	}

	updateSoundboardConfig(guildId: string, config: any): Promise<any> {
		return this.patch(`/guilds/${guildId}/soundboard/config`, config);
	}

	getSoundboardSounds(guildId: string): Promise<any[]> {
		return this.get(`/guilds/${guildId}/soundboard/sounds`);
	}

	createSoundboardSound(guildId: string, sound: any): Promise<any> {
		return this.post(`/guilds/${guildId}/soundboard/sounds`, sound);
	}

	deleteSoundboardSound(guildId: string, soundId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/soundboard/sounds/${soundId}`);
	}

	playSoundboardSound(guildId: string, soundId: string): Promise<void> {
		return this.post(`/guilds/${guildId}/soundboard/sounds/${soundId}/play`);
	}

	// --- Guild Widget ---

	getGuildWidget(guildId: string): Promise<any> {
		return this.get(`/guilds/${guildId}/widget`);
	}

	updateGuildWidget(guildId: string, data: any): Promise<any> {
		return this.patch(`/guilds/${guildId}/widget`, data);
	}

	// --- Guild Plugins ---

	getGuildPlugins(guildId: string): Promise<any[]> {
		return this.get(`/guilds/${guildId}/plugins`);
	}

	updateGuildPlugin(guildId: string, pluginId: string, data: any): Promise<any> {
		return this.patch(`/guilds/${guildId}/plugins/${pluginId}`, data);
	}

	deleteGuildPlugin(guildId: string, pluginId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/plugins/${pluginId}`);
	}

	// --- Guild Templates ---

	getGuildTemplates(guildId: string): Promise<any[]> {
		return this.get(`/guilds/${guildId}/templates`);
	}

	createGuildTemplate(guildId: string, data: { name: string; description?: string }): Promise<any> {
		return this.post(`/guilds/${guildId}/templates`, data);
	}

	deleteGuildTemplate(guildId: string, templateId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/templates/${templateId}`);
	}

	applyGuildTemplate(guildId: string, templateId: string, data?: Record<string, unknown>): Promise<void> {
		return this.post(`/guilds/${guildId}/templates/${templateId}/apply`, data);
	}

	// --- Channel Widgets ---

	getChannelWidgets(channelId: string): Promise<any[]> {
		return this.get(`/channels/${channelId}/widgets`);
	}

	createChannelWidget(channelId: string, widget: any): Promise<any> {
		return this.post(`/channels/${channelId}/widgets`, widget);
	}

	updateChannelWidget(channelId: string, widgetId: string, data: any): Promise<any> {
		return this.patch(`/channels/${channelId}/widgets/${widgetId}`, data);
	}

	deleteChannelWidget(channelId: string, widgetId: string): Promise<void> {
		return this.del(`/channels/${channelId}/widgets/${widgetId}`);
	}

	// --- Instance Profiles ---

	getInstanceProfiles(): Promise<any[]> {
		return this.get('/users/@me/instance-profiles');
	}

	createInstanceProfile(data: { instance_url: string; token: string; display_name?: string }): Promise<any> {
		return this.post('/users/@me/instance-profiles', data);
	}

	deleteInstanceProfile(profileId: string): Promise<void> {
		return this.del(`/users/@me/instance-profiles/${profileId}`);
	}

	// --- Webhook Extras ---

	getWebhookTemplates(): Promise<any[]> {
		return this.get('/webhooks/templates');
	}

	getOutgoingWebhookEvents(): Promise<any[]> {
		return this.get('/webhooks/outgoing-events');
	}

	getWebhookLogs(guildId: string, webhookId: string): Promise<any[]> {
		return this.get(`/guilds/${guildId}/webhooks/${webhookId}/logs`);
	}

	previewWebhook(data: any): Promise<any> {
		return this.post('/webhooks/preview', data);
	}

	// --- Channel Export ---

	exportChannelMessages(channelId: string, format: string = 'json'): Promise<any> {
		return this.get(`/channels/${channelId}/export?format=${format}`);
	}

	// --- Admin Health ---

	getHealthDashboard(): Promise<any> {
		return this.get('/admin/health/dashboard');
	}

	// --- Admin Updates ---

	getAdminUpdates(): Promise<any> {
		return this.get('/admin/updates');
	}

	getAdminUpdatesConfig(): Promise<any> {
		return this.get('/admin/updates/config');
	}

	updateAdminUpdatesConfig(config: any): Promise<void> {
		return this.patch('/admin/updates/config', config);
	}

	dismissAdminUpdate(): Promise<void> {
		return this.post('/admin/updates/dismiss');
	}

	// --- Admin Backups ---

	getBackupSchedules(): Promise<any[]> {
		return this.get('/admin/backups');
	}

	createBackupSchedule(schedule: any): Promise<any> {
		return this.post('/admin/backups', schedule);
	}

	updateBackupSchedule(scheduleId: string, update: any): Promise<void> {
		return this.patch(`/admin/backups/${scheduleId}`, update);
	}

	deleteBackupSchedule(scheduleId: string): Promise<void> {
		return this.del(`/admin/backups/${scheduleId}`);
	}

	runBackup(scheduleId: string): Promise<void> {
		return this.post(`/admin/backups/${scheduleId}/run`);
	}

	getBackupHistory(scheduleId: string): Promise<any[]> {
		return this.get(`/admin/backups/${scheduleId}/history`);
	}

	// --- Admin Retention ---

	getAdminRetentionPolicies(): Promise<any[]> {
		return this.get('/admin/retention');
	}

	createAdminRetentionPolicy(policy: any): Promise<any> {
		return this.post('/admin/retention', policy);
	}

	updateAdminRetentionPolicy(policyId: string, update: any): Promise<void> {
		return this.patch(`/admin/retention/${policyId}`, update);
	}

	deleteAdminRetentionPolicy(policyId: string): Promise<void> {
		return this.del(`/admin/retention/${policyId}`);
	}

	runAdminRetentionPolicy(policyId: string): Promise<any> {
		return this.post(`/admin/retention/${policyId}/run`);
	}

	// --- Admin Domains ---

	getAdminDomains(): Promise<any[]> {
		return this.get('/admin/domains');
	}

	addAdminDomain(data: { domain: string; guild_id?: string; type?: string }): Promise<any> {
		return this.post('/admin/domains', data);
	}

	verifyAdminDomain(domainId: string): Promise<void> {
		return this.post(`/admin/domains/${domainId}/verify`);
	}

	deleteAdminDomain(domainId: string): Promise<void> {
		return this.del(`/admin/domains/${domainId}`);
	}

	// --- Admin Storage ---

	getAdminStorage(): Promise<any> {
		return this.get('/admin/storage');
	}
}

export class ApiRequestError extends Error {
	constructor(
		message: string,
		public code: string,
		public status: number
	) {
		super(message);
		this.name = 'ApiRequestError';
	}
}

// Singleton API client.
export const api = new ApiClient();
