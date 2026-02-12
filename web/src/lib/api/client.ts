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
	Session,
	ReadState,
	Relationship,
	LoginResponse,
	RegisterResponse,
	ApiResponse,
	ApiError,
	AdminStats,
	InstanceInfo
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
	private del<T>(path: string) {
		return this.request<T>('DELETE', path);
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

	updateMe(data: Partial<Pick<User, 'username' | 'display_name' | 'bio' | 'status_text'>>): Promise<User> {
		return this.patch('/users/@me', data);
	}

	getUser(userId: string): Promise<User> {
		return this.get(`/users/${userId}`);
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

	// --- Guilds ---

	createGuild(name: string, description?: string): Promise<Guild> {
		return this.post('/guilds', { name, description });
	}

	getGuild(guildId: string): Promise<Guild> {
		return this.get(`/guilds/${guildId}`);
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

	// --- Messages ---

	getMessages(channelId: string, params?: { before?: string; after?: string; limit?: number }): Promise<Message[]> {
		const query = new URLSearchParams();
		if (params?.before) query.set('before', params.before);
		if (params?.after) query.set('after', params.after);
		if (params?.limit) query.set('limit', String(params.limit));
		const qs = query.toString();
		return this.get(`/channels/${channelId}/messages${qs ? '?' + qs : ''}`);
	}

	sendMessage(channelId: string, content: string, opts?: { reply_to_ids?: string[]; nonce?: string; attachment_ids?: string[] }): Promise<Message> {
		return this.post(`/channels/${channelId}/messages`, { content, ...opts });
	}

	editMessage(channelId: string, messageId: string, content: string): Promise<Message> {
		return this.patch(`/channels/${channelId}/messages/${messageId}`, { content });
	}

	deleteMessage(channelId: string, messageId: string): Promise<void> {
		return this.del(`/channels/${channelId}/messages/${messageId}`);
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

	blockUser(userId: string): Promise<void> {
		return this.put(`/users/${userId}/block`);
	}

	unblockUser(userId: string): Promise<void> {
		return this.del(`/users/${userId}/block`);
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

	kickMember(guildId: string, memberId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/members/${memberId}`);
	}

	banUser(guildId: string, userId: string, reason?: string): Promise<void> {
		return this.put(`/guilds/${guildId}/bans/${userId}`, { reason });
	}

	unbanUser(guildId: string, userId: string): Promise<void> {
		return this.del(`/guilds/${guildId}/bans/${userId}`);
	}

	// --- Roles ---

	getRoles(guildId: string): Promise<Role[]> {
		return this.get(`/guilds/${guildId}/roles`);
	}

	createRole(guildId: string, name: string): Promise<Role> {
		return this.post(`/guilds/${guildId}/roles`, { name });
	}

	// --- Invites ---

	createInvite(guildId: string, opts?: { max_uses?: number; max_age_seconds?: number }): Promise<Invite> {
		return this.post(`/guilds/${guildId}/invites`, opts);
	}

	getInvite(code: string): Promise<Invite> {
		return this.get(`/invites/${code}`);
	}

	acceptInvite(code: string): Promise<Guild> {
		return this.post(`/invites/${code}`);
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

	// --- File Upload ---

	async uploadFile(file: File): Promise<{ id: string; url: string }> {
		const formData = new FormData();
		formData.append('file', file);

		const headers: Record<string, string> = {};
		const token = this.getToken();
		if (token) headers['Authorization'] = `Bearer ${token}`;

		const res = await fetch(`${API_BASE}/files/upload`, {
			method: 'POST',
			headers,
			body: formData
		});

		const json = await res.json();
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

	// --- Admin ---

	getAdminStats(): Promise<AdminStats> {
		return this.get('/admin/stats');
	}

	getAdminInstance(): Promise<InstanceInfo> {
		return this.patch('/admin/instance');
	}

	updateAdminInstance(data: { name?: string; description?: string; federation_mode?: string }): Promise<InstanceInfo> {
		return this.patch('/admin/instance', data);
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
