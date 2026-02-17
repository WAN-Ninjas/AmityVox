<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';
	import HealthMonitor from '$lib/components/admin/HealthMonitor.svelte';
	import StorageDashboard from '$lib/components/admin/StorageDashboard.svelte';
	import BackupScheduler from '$lib/components/admin/BackupScheduler.svelte';
	import DomainSettings from '$lib/components/admin/DomainSettings.svelte';
	import RetentionSettings from '$lib/components/admin/RetentionSettings.svelte';
	import UpdateNotifications from '$lib/components/admin/UpdateNotifications.svelte';
	import type {
		AdminStats, InstanceInfo, User, FederationPeer,
		InstanceBan, RegistrationSettings, RegistrationToken,
		Announcement, AnnouncementSeverity
	} from '$lib/types';

	type Tab = 'dashboard' | 'users' | 'guilds' | 'bots' | 'bans' | 'registration' | 'announcements' | 'instance' | 'federation' | 'rate_limits' | 'content_safety' | 'captcha' | 'health' | 'storage' | 'backups' | 'domains' | 'retention' | 'updates';
	let currentTab = $state<Tab>('dashboard');

	// --- Dashboard ---
	let stats = $state<AdminStats | null>(null);
	let loading = $state(true);
	let error = $state('');

	// --- Users ---
	let users = $state<User[]>([]);
	let usersLoaded = $state(false);
	let loadingUsers = $state(false);
	let userSearch = $state('');
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;

	// --- Ban Modal ---
	let banModalOpen = $state(false);
	let banTargetUser = $state<User | null>(null);
	let banReason = $state('');
	let banning = $state(false);

	// --- Instance Bans ---
	let instanceBans = $state<InstanceBan[]>([]);
	let bansLoaded = $state(false);
	let loadingBans = $state(false);

	// --- Instance ---
	let instance = $state<InstanceInfo | null>(null);
	let loadingInstance = $state(false);
	let instanceName = $state('');
	let instanceDesc = $state('');
	let instanceFedMode = $state('');
	let savingInstance = $state(false);

	// --- Federation ---
	let peers = $state<FederationPeer[]>([]);
	let peersLoaded = $state(false);
	let loadingPeers = $state(false);
	let newPeerDomain = $state('');
	let addingPeer = $state(false);

	// --- Registration ---
	let regSettings = $state<RegistrationSettings | null>(null);
	let regTokens = $state<RegistrationToken[]>([]);
	let loadingReg = $state(false);
	let savingReg = $state(false);
	let regMode = $state<'open' | 'invite_only' | 'closed'>('open');
	let regMessage = $state('');
	let createTokenModalOpen = $state(false);
	let newTokenMaxUses = $state(1);
	let newTokenNote = $state('');
	let newTokenExpiryHours = $state(0);
	let creatingToken = $state(false);

	// --- Bots ---
	interface BotGuildPermission {
		bot_id: string;
		guild_id: string;
		scopes: string[];
		max_role_position: number;
		created_at: string;
		updated_at: string;
	}
	interface BotEventSubscription {
		id: string;
		bot_id: string;
		guild_id: string;
		event_types: string[];
		webhook_url: string;
		created_at: string;
	}
	interface BotRateLimit {
		bot_id: string;
		requests_per_second: number;
		burst: number;
		updated_at: string;
	}
	interface BotPresenceInfo {
		bot_id: string;
		status: string;
		activity_type: string | null;
		activity_name: string | null;
		updated_at: string;
	}
	interface BotWithDetails extends User {
		guild_permissions: BotGuildPermission[];
		event_subscriptions: BotEventSubscription[];
		rate_limit: BotRateLimit | null;
		presence: BotPresenceInfo | null;
	}
	let allBots = $state<BotWithDetails[]>([]);
	let botsLoaded = $state(false);
	let loadingAllBots = $state(false);
	let expandedBotId = $state<string | null>(null);

	// --- Announcements ---
	let announcements = $state<Announcement[]>([]);
	let announcementsLoaded = $state(false);
	let loadingAnnouncements = $state(false);
	let createAnnouncementModalOpen = $state(false);
	let editAnnouncementModalOpen = $state(false);
	let editingAnnouncement = $state<Announcement | null>(null);
	let announcementTitle = $state('');
	let announcementContent = $state('');
	let announcementSeverity = $state<AnnouncementSeverity>('info');
	let announcementExpiryHours = $state(0);
	let savingAnnouncement = $state(false);

	// --- Rate Limiting ---
	interface RateLimitIPStat {
		ip_address: string;
		total_requests: number;
		block_count: number;
		last_seen: string;
	}
	interface RateLimitStats {
		top_ips: RateLimitIPStat[];
		total_entries_24h: number;
		blocked_entries_24h: number;
		unique_ips_24h: number;
		requests_per_window: string;
		window_seconds: string;
	}
	interface RateLimitLogEntry {
		id: string;
		ip_address: string;
		endpoint: string;
		requests_count: number;
		window_start: string;
		blocked: boolean;
		created_at: string;
	}
	let rateLimitStats = $state<RateLimitStats | null>(null);
	let rateLimitLog = $state<RateLimitLogEntry[]>([]);
	let loadingRateLimits = $state(false);
	let loadingRateLimitLog = $state(false);
	let rateLimitLogFilter = $state<'all' | 'blocked'>('all');
	let rateLimitIPFilter = $state('');
	let savingRateLimitConfig = $state(false);
	let editReqsPerWindow = $state('100');
	let editWindowSeconds = $state('60');

	// --- Content Safety ---
	interface ContentScanRule {
		id: string;
		name: string;
		pattern: string;
		action: string;
		target: string;
		enabled: boolean;
		created_at: string;
	}
	interface ContentScanLogEntry {
		id: string;
		rule_id: string;
		rule_name: string;
		user_id: string;
		username: string;
		channel_id: string;
		content_matched: string;
		action_taken: string;
		created_at: string;
	}
	let contentScanRules = $state<ContentScanRule[]>([]);
	let contentRulesLoaded = $state(false);
	let contentScanLog = $state<ContentScanLogEntry[]>([]);
	let loadingContentRules = $state(false);
	let loadingContentLog = $state(false);
	let createRuleModalOpen = $state(false);
	let editRuleModalOpen = $state(false);
	let editingRule = $state<ContentScanRule | null>(null);
	let ruleName = $state('');
	let rulePattern = $state('');
	let ruleAction = $state<'block' | 'flag' | 'log'>('log');
	let ruleTarget = $state<'filename' | 'content_type' | 'text_content'>('filename');
	let ruleEnabled = $state(true);
	let savingRule = $state(false);
	let contentLogSubTab = $state<'rules' | 'log'>('rules');

	// --- CAPTCHA ---
	interface CaptchaConfig {
		provider: string;
		site_key: string;
		secret_key: string;
	}
	let captchaConfig = $state<CaptchaConfig | null>(null);
	let loadingCaptcha = $state(false);
	let savingCaptcha = $state(false);
	let captchaProvider = $state<'none' | 'hcaptcha' | 'recaptcha'>('none');
	let captchaSiteKey = $state('');
	let captchaSecretKey = $state('');

	// --- Guilds ---
	interface AdminGuild {
		id: string;
		name: string;
		icon_url: string | null;
		owner_id: string;
		owner_name: string;
		member_count: number;
		channel_count: number;
		role_count: number;
		created_at: string;
	}
	interface AdminGuildDetail extends AdminGuild {
		description: string | null;
		emoji_count: number;
		invite_count: number;
		message_count: number;
		messages_today: number;
		ban_count: number;
	}
	interface AdminUserGuild {
		id: string;
		name: string;
		icon_url: string | null;
		is_owner: boolean;
		member_count: number;
		joined_at: string;
	}
	let adminGuilds = $state<AdminGuild[]>([]);
	let loadingGuilds = $state(false);
	let guildSearch = $state('');
	let guildSort = $state('newest');
	let guildSearchTimeout: ReturnType<typeof setTimeout> | null = null;
	let selectedGuildDetail = $state<AdminGuildDetail | null>(null);
	let guildDetailModalOpen = $state(false);
	let loadingGuildDetail = $state(false);
	let expandedUserGuilds = $state<string | null>(null);
	let userGuildsList = $state<AdminUserGuild[]>([]);
	let loadingUserGuilds = $state(false);

	onMount(async () => {
		try {
			stats = await api.getAdminStats();
		} catch (err: any) {
			error = err.message || 'Failed to load stats. You may not have admin access.';
		} finally {
			loading = false;
		}
	});

	$effect(() => {
		if (currentTab === 'users' && !usersLoaded) loadUsers();
		if (currentTab === 'guilds' && adminGuilds.length === 0) loadGuilds();
		if (currentTab === 'bots' && !botsLoaded) loadAllBots();
		if (currentTab === 'bans' && !bansLoaded) loadInstanceBans();
		if (currentTab === 'instance' && !instance) loadInstance();
		if (currentTab === 'federation' && !peersLoaded) loadPeers();
		if (currentTab === 'registration' && !regSettings) loadRegistration();
		if (currentTab === 'announcements' && !announcementsLoaded) loadAnnouncements();
		if (currentTab === 'rate_limits' && !rateLimitStats) loadRateLimitStats();
		if (currentTab === 'content_safety' && !contentRulesLoaded) loadContentScanRules();
		if (currentTab === 'captcha' && !captchaConfig) loadCaptchaConfig();
	});

	async function refresh() {
		loading = true;
		error = '';
		try {
			stats = await api.getAdminStats();
		} catch (err: any) {
			error = err.message || 'Failed to refresh stats';
		} finally {
			loading = false;
		}
	}

	// --- Users ---

	async function loadUsers(query?: string) {
		loadingUsers = true;
		try {
			users = await api.getAdminUsers({ limit: 50, query });
			usersLoaded = true;
		} catch { users = []; }
		finally { loadingUsers = false; }
	}

	async function loadAllBots() {
		loadingAllBots = true;
		try {
			const token = localStorage.getItem('token');
			const res = await fetch('/api/v1/admin/bots', {
				headers: { 'Authorization': `Bearer ${token}` }
			});
			if (!res.ok) throw new Error('Failed to load bots');
			const json = await res.json();
			allBots = json.data ?? [];
			botsLoaded = true;
		} catch {
			// Fallback: load basic bot list without details.
			try {
				const allUsers = await api.getAdminUsers({ limit: 200, query: '' });
				allBots = allUsers.filter((u: User) => (u.flags & 8) !== 0).map((u: User) => ({
					...u,
					guild_permissions: [],
					event_subscriptions: [],
					rate_limit: null,
					presence: null
				}));
				botsLoaded = true;
			} catch { allBots = []; }
		}
		finally { loadingAllBots = false; }
	}

	function toggleBotExpand(botId: string) {
		expandedBotId = expandedBotId === botId ? null : botId;
	}

	function formatScopes(scopes: string[]): string {
		if (!scopes || scopes.length === 0) return 'None';
		return scopes.map(s => s.replace('.', ' ')).join(', ');
	}

	function presenceStatusColor(status: string): string {
		switch (status) {
			case 'online': return 'bg-green-500/20 text-green-400';
			case 'idle': return 'bg-yellow-500/20 text-yellow-400';
			case 'dnd': return 'bg-red-500/20 text-red-400';
			default: return 'bg-gray-500/20 text-gray-400';
		}
	}

	function handleUserSearch() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			loadUsers(userSearch || undefined);
		}, 300);
	}

	async function handleSuspend(userId: string) {
		try {
			await api.suspendUser(userId);
			users = users.map(u => u.id === userId ? { ...u, flags: u.flags | 1 } : u);
			addToast('User suspended', 'success');
		} catch { addToast('Failed to suspend user', 'error'); }
	}

	async function handleUnsuspend(userId: string) {
		try {
			await api.unsuspendUser(userId);
			users = users.map(u => u.id === userId ? { ...u, flags: u.flags & ~1 } : u);
			addToast('User unsuspended', 'success');
		} catch { addToast('Failed to unsuspend user', 'error'); }
	}

	async function handleToggleAdmin(userId: string, currentFlags: number) {
		const isAdmin = (currentFlags & 4) !== 0;
		try {
			await api.setAdmin(userId, !isAdmin);
			users = users.map(u => u.id === userId ? { ...u, flags: isAdmin ? u.flags & ~4 : u.flags | 4 } : u);
			addToast(isAdmin ? 'Admin removed' : 'Admin granted', 'success');
		} catch { addToast('Failed to update admin status', 'error'); }
	}

	async function handleToggleGlobalMod(userId: string, currentFlags: number) {
		const isMod = (currentFlags & 32) !== 0;
		try {
			await api.setGlobalMod(userId, !isMod);
			users = users.map(u => u.id === userId ? { ...u, flags: isMod ? u.flags & ~32 : u.flags | 32 } : u);
			addToast(isMod ? 'Global Mod removed' : 'Global Mod granted', 'success');
		} catch { addToast('Failed to update global mod status', 'error'); }
	}

	// --- Instance Ban ---

	function openBanModal(user: User) {
		banTargetUser = user;
		banReason = '';
		banModalOpen = true;
	}

	async function handleInstanceBan() {
		if (!banTargetUser || !banReason.trim()) return;
		banning = true;
		try {
			await api.instanceBanUser(banTargetUser.id, banReason.trim());
			addToast(`${banTargetUser.display_name ?? banTargetUser.username} has been instance-banned`, 'success');
			banModalOpen = false;
			banTargetUser = null;
			banReason = '';
			// Refresh bans list if it was loaded
			if (instanceBans.length > 0) loadInstanceBans();
		} catch { addToast('Failed to ban user', 'error'); }
		finally { banning = false; }
	}

	async function loadInstanceBans() {
		loadingBans = true;
		try { instanceBans = await api.getInstanceBans(); bansLoaded = true; } catch { instanceBans = []; }
		finally { loadingBans = false; }
	}

	async function handleInstanceUnban(userId: string) {
		try {
			await api.instanceUnbanUser(userId);
			instanceBans = instanceBans.filter(b => b.user_id !== userId);
			addToast('User unbanned', 'success');
		} catch { addToast('Failed to unban user', 'error'); }
	}

	// --- Instance ---

	async function loadInstance() {
		loadingInstance = true;
		try {
			instance = await api.getAdminInstance();
			instanceName = instance?.name ?? '';
			instanceDesc = instance?.description ?? '';
			const rawMode = instance?.federation_mode ?? 'closed';
			instanceFedMode =
				rawMode === 'allow' ? 'open' :
				rawMode === 'deny' || rawMode === 'disabled' ? 'closed' :
				rawMode;
		} catch {}
		finally { loadingInstance = false; }
	}

	async function saveInstance() {
		savingInstance = true;
		try {
			instance = await api.updateAdminInstance({
				name: instanceName || undefined,
				description: instanceDesc || undefined,
				federation_mode: instanceFedMode || undefined
			});
			addToast('Instance settings saved', 'success');
		} catch { addToast('Failed to save instance settings', 'error'); }
		finally { savingInstance = false; }
	}

	// --- Federation ---

	async function loadPeers() {
		loadingPeers = true;
		try { peers = await api.getFederationPeers(); peersLoaded = true; } catch { peers = []; }
		finally { loadingPeers = false; }
	}

	async function handleAddPeer() {
		if (!newPeerDomain.trim()) return;
		addingPeer = true;
		try {
			const peer = await api.addFederationPeer(newPeerDomain.trim());
			peers = [...peers, peer];
			newPeerDomain = '';
			addToast('Peer added', 'success');
		} catch { addToast('Failed to add peer', 'error'); }
		finally { addingPeer = false; }
	}

	async function handleRemovePeer(peerId: string) {
		if (!confirm('Remove this federation peer?')) return;
		try {
			await api.removeFederationPeer(peerId);
			peers = peers.filter(p => p.id !== peerId);
			addToast('Peer removed', 'success');
		} catch { addToast('Failed to remove peer', 'error'); }
	}

	// --- Registration ---

	async function loadRegistration() {
		loadingReg = true;
		try {
			const [settings, tokens] = await Promise.all([
				api.getRegistrationSettings(),
				api.getRegistrationTokens()
			]);
			regSettings = settings;
			regMode = settings.mode;
			regMessage = settings.message ?? '';
			regTokens = tokens;
		} catch {}
		finally { loadingReg = false; }
	}

	async function saveRegistration() {
		savingReg = true;
		try {
			regSettings = await api.updateRegistrationSettings({
				mode: regMode,
				message: regMessage || null
			});
			addToast('Registration settings saved', 'success');
		} catch { addToast('Failed to save registration settings', 'error'); }
		finally { savingReg = false; }
	}

	function openCreateTokenModal() {
		newTokenMaxUses = 1;
		newTokenNote = '';
		newTokenExpiryHours = 0;
		createTokenModalOpen = true;
	}

	async function handleCreateToken() {
		creatingToken = true;
		try {
			const token = await api.createRegistrationToken({
				max_uses: newTokenMaxUses || undefined,
				note: newTokenNote || undefined,
				expires_in_hours: newTokenExpiryHours || undefined
			});
			regTokens = [...regTokens, token];
			createTokenModalOpen = false;
			addToast('Registration token created', 'success');
		} catch { addToast('Failed to create token', 'error'); }
		finally { creatingToken = false; }
	}

	async function handleDeleteToken(tokenId: string) {
		if (!confirm('Delete this registration token?')) return;
		try {
			await api.deleteRegistrationToken(tokenId);
			regTokens = regTokens.filter(t => t.id !== tokenId);
			addToast('Token deleted', 'success');
		} catch { addToast('Failed to delete token', 'error'); }
	}

	function copyToken(token: string) {
		navigator.clipboard.writeText(token).then(
			() => addToast('Token copied to clipboard', 'success'),
			() => addToast('Failed to copy token', 'error')
		);
	}

	function getTokenStatus(token: RegistrationToken): string {
		if (token.expires_at && new Date(token.expires_at) < new Date()) return 'expired';
		if (token.max_uses > 0 && token.uses >= token.max_uses) return 'exhausted';
		return 'active';
	}

	// --- Announcements ---

	async function loadAnnouncements() {
		loadingAnnouncements = true;
		try { announcements = await api.getAdminAnnouncements(); announcementsLoaded = true; } catch { announcements = []; }
		finally { loadingAnnouncements = false; }
	}

	function openCreateAnnouncement() {
		announcementTitle = '';
		announcementContent = '';
		announcementSeverity = 'info';
		announcementExpiryHours = 0;
		createAnnouncementModalOpen = true;
	}

	function openEditAnnouncement(a: Announcement) {
		editingAnnouncement = a;
		announcementTitle = a.title;
		announcementContent = a.content;
		editAnnouncementModalOpen = true;
	}

	async function handleCreateAnnouncement() {
		if (!announcementTitle.trim() || !announcementContent.trim()) return;
		savingAnnouncement = true;
		try {
			const a = await api.createAnnouncement({
				title: announcementTitle.trim(),
				content: announcementContent.trim(),
				severity: announcementSeverity,
				expires_in_hours: announcementExpiryHours || undefined
			});
			announcements = [a, ...announcements];
			createAnnouncementModalOpen = false;
			addToast('Announcement created', 'success');
		} catch { addToast('Failed to create announcement', 'error'); }
		finally { savingAnnouncement = false; }
	}

	async function handleUpdateAnnouncement() {
		if (!editingAnnouncement) return;
		savingAnnouncement = true;
		try {
			const updated = await api.updateAnnouncement(editingAnnouncement.id, {
				title: announcementTitle.trim(),
				content: announcementContent.trim()
			});
			announcements = announcements.map(a => a.id === updated.id ? updated : a);
			editAnnouncementModalOpen = false;
			editingAnnouncement = null;
			addToast('Announcement updated', 'success');
		} catch { addToast('Failed to update announcement', 'error'); }
		finally { savingAnnouncement = false; }
	}

	async function handleToggleAnnouncement(announcement: Announcement) {
		try {
			const updated = await api.updateAnnouncement(announcement.id, { active: !announcement.active });
			announcements = announcements.map(a => a.id === updated.id ? updated : a);
			addToast(updated.active ? 'Announcement activated' : 'Announcement deactivated', 'success');
		} catch { addToast('Failed to update announcement', 'error'); }
	}

	async function handleDeleteAnnouncement(id: string) {
		if (!confirm('Permanently delete this announcement?')) return;
		try {
			await api.deleteAnnouncement(id);
			announcements = announcements.filter(a => a.id !== id);
			addToast('Announcement deleted', 'success');
		} catch { addToast('Failed to delete announcement', 'error'); }
	}

	// --- Rate Limiting ---

	async function loadRateLimitStats() {
		loadingRateLimits = true;
		try {
			const res = await fetch('/api/v1/admin/rate-limits/stats', {
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to load');
			const json = await res.json();
			rateLimitStats = json.data;
			editReqsPerWindow = rateLimitStats?.requests_per_window ?? '100';
			editWindowSeconds = rateLimitStats?.window_seconds ?? '60';
		} catch { rateLimitStats = null; }
		finally { loadingRateLimits = false; }
	}

	async function loadRateLimitLog() {
		loadingRateLimitLog = true;
		try {
			const params = new URLSearchParams({ limit: '50' });
			if (rateLimitLogFilter === 'blocked') params.set('blocked', 'true');
			if (rateLimitIPFilter.trim()) params.set('ip', rateLimitIPFilter.trim());
			const res = await fetch(`/api/v1/admin/rate-limits/log?${params}`, {
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to load');
			const json = await res.json();
			rateLimitLog = json.data;
		} catch { rateLimitLog = []; }
		finally { loadingRateLimitLog = false; }
	}

	async function saveRateLimitConfig() {
		savingRateLimitConfig = true;
		try {
			const res = await fetch('/api/v1/admin/rate-limits/config', {
				method: 'PATCH',
				headers: {
					'Authorization': `Bearer ${localStorage.getItem('token')}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					requests_per_window: editReqsPerWindow,
					window_seconds: editWindowSeconds
				})
			});
			if (!res.ok) throw new Error('Failed to save');
			addToast('Rate limit configuration saved', 'success');
		} catch { addToast('Failed to save rate limit configuration', 'error'); }
		finally { savingRateLimitConfig = false; }
	}

	// --- Content Safety ---

	async function loadContentScanRules() {
		loadingContentRules = true;
		try {
			const res = await fetch('/api/v1/admin/content-scan/rules', {
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to load');
			const json = await res.json();
			contentScanRules = json.data;
			contentRulesLoaded = true;
		} catch { contentScanRules = []; }
		finally { loadingContentRules = false; }
	}

	async function loadContentScanLog() {
		loadingContentLog = true;
		try {
			const res = await fetch('/api/v1/admin/content-scan/log?limit=50', {
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to load');
			const json = await res.json();
			contentScanLog = json.data;
		} catch { contentScanLog = []; }
		finally { loadingContentLog = false; }
	}

	function openCreateRuleModal() {
		ruleName = '';
		rulePattern = '';
		ruleAction = 'log';
		ruleTarget = 'filename';
		ruleEnabled = true;
		createRuleModalOpen = true;
	}

	function openEditRuleModal(rule: ContentScanRule) {
		editingRule = rule;
		ruleName = rule.name;
		rulePattern = rule.pattern;
		ruleAction = rule.action as 'block' | 'flag' | 'log';
		ruleTarget = rule.target as 'filename' | 'content_type' | 'text_content';
		ruleEnabled = rule.enabled;
		editRuleModalOpen = true;
	}

	async function handleCreateRule() {
		if (!ruleName.trim() || !rulePattern.trim()) return;
		savingRule = true;
		try {
			const res = await fetch('/api/v1/admin/content-scan/rules', {
				method: 'POST',
				headers: {
					'Authorization': `Bearer ${localStorage.getItem('token')}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					name: ruleName.trim(),
					pattern: rulePattern.trim(),
					action: ruleAction,
					target: ruleTarget,
					enabled: ruleEnabled
				})
			});
			if (!res.ok) {
				const errData = await res.json().catch(() => null);
				throw new Error(errData?.error?.message ?? 'Failed to create rule');
			}
			const json = await res.json();
			contentScanRules = [json.data, ...contentScanRules];
			createRuleModalOpen = false;
			addToast('Content scan rule created', 'success');
		} catch (err: any) { addToast(err.message ?? 'Failed to create rule', 'error'); }
		finally { savingRule = false; }
	}

	async function handleUpdateRule() {
		if (!editingRule) return;
		savingRule = true;
		try {
			const res = await fetch(`/api/v1/admin/content-scan/rules/${editingRule.id}`, {
				method: 'PATCH',
				headers: {
					'Authorization': `Bearer ${localStorage.getItem('token')}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					name: ruleName.trim(),
					pattern: rulePattern.trim(),
					action: ruleAction,
					target: ruleTarget,
					enabled: ruleEnabled
				})
			});
			if (!res.ok) {
				const errData = await res.json().catch(() => null);
				throw new Error(errData?.error?.message ?? 'Failed to update rule');
			}
			contentScanRules = contentScanRules.map(r =>
				r.id === editingRule!.id ? { ...r, name: ruleName.trim(), pattern: rulePattern.trim(), action: ruleAction, target: ruleTarget, enabled: ruleEnabled } : r
			);
			editRuleModalOpen = false;
			editingRule = null;
			addToast('Content scan rule updated', 'success');
		} catch (err: any) { addToast(err.message ?? 'Failed to update rule', 'error'); }
		finally { savingRule = false; }
	}

	async function handleToggleRule(rule: ContentScanRule) {
		try {
			const res = await fetch(`/api/v1/admin/content-scan/rules/${rule.id}`, {
				method: 'PATCH',
				headers: {
					'Authorization': `Bearer ${localStorage.getItem('token')}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ enabled: !rule.enabled })
			});
			if (!res.ok) throw new Error('Failed to toggle');
			contentScanRules = contentScanRules.map(r =>
				r.id === rule.id ? { ...r, enabled: !r.enabled } : r
			);
			addToast(rule.enabled ? 'Rule disabled' : 'Rule enabled', 'success');
		} catch { addToast('Failed to toggle rule', 'error'); }
	}

	async function handleDeleteRule(id: string) {
		if (!confirm('Delete this content scan rule? Associated log entries will also be deleted.')) return;
		try {
			const res = await fetch(`/api/v1/admin/content-scan/rules/${id}`, {
				method: 'DELETE',
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to delete');
			contentScanRules = contentScanRules.filter(r => r.id !== id);
			addToast('Rule deleted', 'success');
		} catch { addToast('Failed to delete rule', 'error'); }
	}

	function actionClasses(action: string): string {
		switch (action) {
			case 'block': return 'bg-red-500/20 text-red-400';
			case 'flag': return 'bg-yellow-500/20 text-yellow-400';
			case 'log': return 'bg-blue-500/20 text-blue-400';
			default: return 'bg-gray-500/20 text-gray-400';
		}
	}

	function targetLabel(target: string): string {
		switch (target) {
			case 'filename': return 'Filename';
			case 'content_type': return 'Content Type';
			case 'text_content': return 'Text Content';
			default: return target;
		}
	}

	// --- CAPTCHA ---

	async function loadCaptchaConfig() {
		loadingCaptcha = true;
		try {
			const res = await fetch('/api/v1/admin/captcha', {
				headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
			});
			if (!res.ok) throw new Error('Failed to load');
			const json = await res.json();
			captchaConfig = json.data;
			captchaProvider = (captchaConfig?.provider ?? 'none') as 'none' | 'hcaptcha' | 'recaptcha';
			captchaSiteKey = captchaConfig?.site_key ?? '';
			captchaSecretKey = '';
		} catch { captchaConfig = null; }
		finally { loadingCaptcha = false; }
	}

	async function saveCaptchaConfig() {
		savingCaptcha = true;
		try {
			const body: Record<string, string> = { provider: captchaProvider };
			if (captchaSiteKey) body.site_key = captchaSiteKey;
			if (captchaSecretKey) body.secret_key = captchaSecretKey;
			const res = await fetch('/api/v1/admin/captcha', {
				method: 'PATCH',
				headers: {
					'Authorization': `Bearer ${localStorage.getItem('token')}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(body)
			});
			if (!res.ok) throw new Error('Failed to save');
			addToast('CAPTCHA settings saved', 'success');
			loadCaptchaConfig();
		} catch { addToast('Failed to save CAPTCHA settings', 'error'); }
		finally { savingCaptcha = false; }
	}

	function severityLabel(s: AnnouncementSeverity): string {
		return s.charAt(0).toUpperCase() + s.slice(1);
	}

	// --- Guild Functions ---
	async function loadGuilds() {
		loadingGuilds = true;
		try {
			adminGuilds = await api.getAdminGuilds({ query: guildSearch, sort: guildSort, limit: 100 });
		} catch (err: any) {
			addToast(err.message || 'Failed to load guilds', 'error');
		} finally {
			loadingGuilds = false;
		}
	}

	function handleGuildSearch() {
		if (guildSearchTimeout) clearTimeout(guildSearchTimeout);
		guildSearchTimeout = setTimeout(() => loadGuilds(), 300);
	}

	async function viewGuildDetail(guildId: string) {
		guildDetailModalOpen = true;
		loadingGuildDetail = true;
		try {
			selectedGuildDetail = await api.getAdminGuildDetails(guildId);
		} catch (err: any) {
			addToast(err.message || 'Failed to load guild details', 'error');
			guildDetailModalOpen = false;
		} finally {
			loadingGuildDetail = false;
		}
	}

	async function handleDeleteGuild(guildId: string, guildName: string) {
		if (!confirm(`Are you sure you want to delete "${guildName}"? This action is irreversible.`)) return;
		try {
			await api.adminDeleteGuild(guildId);
			adminGuilds = adminGuilds.filter(g => g.id !== guildId);
			guildDetailModalOpen = false;
			addToast(`Guild "${guildName}" deleted.`, 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete guild', 'error');
		}
	}

	async function loadUserGuilds(userId: string) {
		if (expandedUserGuilds === userId) {
			expandedUserGuilds = null;
			return;
		}
		expandedUserGuilds = userId;
		loadingUserGuilds = true;
		try {
			userGuildsList = await api.getAdminUserGuilds(userId);
		} catch (err: any) {
			addToast(err.message || 'Failed to load user guilds', 'error');
			userGuildsList = [];
		} finally {
			loadingUserGuilds = false;
		}
	}

	function severityClasses(s: AnnouncementSeverity): string {
		switch (s) {
			case 'info': return 'bg-blue-500/20 text-blue-400';
			case 'warning': return 'bg-yellow-500/20 text-yellow-400';
			case 'critical': return 'bg-red-500/20 text-red-400';
		}
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'dashboard', label: 'Dashboard' },
		{ id: 'users', label: 'Users' },
		{ id: 'guilds', label: 'Guilds' },
		{ id: 'bots', label: 'Bots' },
		{ id: 'bans', label: 'Instance Bans' },
		{ id: 'registration', label: 'Registration' },
		{ id: 'announcements', label: 'Announcements' },
		{ id: 'rate_limits', label: 'Rate Limiting' },
		{ id: 'content_safety', label: 'Content Safety' },
		{ id: 'captcha', label: 'CAPTCHA' },
		{ id: 'instance', label: 'Instance' },
		{ id: 'federation', label: 'Federation' },
		{ id: 'health', label: 'Health' },
		{ id: 'storage', label: 'Storage' },
		{ id: 'backups', label: 'Backups' },
		{ id: 'domains', label: 'Domains' },
		{ id: 'retention', label: 'Retention' },
		{ id: 'updates', label: 'Updates' }
	];
</script>

<svelte:head>
	<title>Admin — AmityVox</title>
</svelte:head>

<!-- Ban User Modal -->
<Modal open={banModalOpen} title="Instance Ban User" onclose={() => (banModalOpen = false)}>
	{#if banTargetUser}
		<p class="mb-4 text-sm text-text-secondary">
			Ban <strong class="text-text-primary">{banTargetUser.display_name ?? banTargetUser.username}</strong> from this instance? They will not be able to log in or interact.
		</p>
		<div class="mb-4">
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Reason</label>
			<textarea
				class="input w-full"
				rows="3"
				placeholder="Provide a reason for the ban..."
				bind:value={banReason}
			></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (banModalOpen = false)}>Cancel</button>
			<button
				class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
				onclick={handleInstanceBan}
				disabled={banning || !banReason.trim()}
			>
				{banning ? 'Banning...' : 'Ban User'}
			</button>
		</div>
	{/if}
</Modal>

<!-- Guild Detail Modal -->
<Modal open={guildDetailModalOpen} title="Guild Details" onclose={() => (guildDetailModalOpen = false)}>
	{#if loadingGuildDetail}
		<p class="text-sm text-text-muted">Loading guild details...</p>
	{:else if selectedGuildDetail}
		<div class="space-y-4">
			<div class="flex items-center gap-3">
				{#if selectedGuildDetail.icon_url}
					<img src={selectedGuildDetail.icon_url} alt="" class="h-12 w-12 rounded-full object-cover" />
				{:else}
					<div class="flex h-12 w-12 items-center justify-center rounded-full bg-brand-500/20 text-lg font-bold text-brand-400">
						{selectedGuildDetail.name.charAt(0).toUpperCase()}
					</div>
				{/if}
				<div>
					<h3 class="text-lg font-bold text-text-primary">{selectedGuildDetail.name}</h3>
					{#if selectedGuildDetail.description}
						<p class="text-sm text-text-muted">{selectedGuildDetail.description}</p>
					{/if}
				</div>
			</div>

			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Members</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.member_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Channels</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.channel_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Roles</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.role_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Emojis</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.emoji_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Total Messages</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.message_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Messages Today</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.messages_today.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Active Invites</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.invite_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Bans</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.ban_count.toLocaleString()}</p>
				</div>
			</div>

			<div class="space-y-1 text-xs text-text-muted">
				<p>Owner: <span class="text-text-primary">{selectedGuildDetail.owner_name}</span></p>
				<p>ID: <span class="font-mono text-text-primary">{selectedGuildDetail.id}</span></p>
				<p>Created: <span class="text-text-primary">{new Date(selectedGuildDetail.created_at).toLocaleString()}</span></p>
			</div>

			<div class="flex justify-end gap-2 pt-2">
				<button class="btn-secondary text-sm" onclick={() => (guildDetailModalOpen = false)}>Close</button>
				<button
					class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-500"
					onclick={() => handleDeleteGuild(selectedGuildDetail!.id, selectedGuildDetail!.name)}
				>
					Delete Guild
				</button>
			</div>
		</div>
	{/if}
</Modal>

<!-- Create Token Modal -->
<Modal open={createTokenModalOpen} title="Create Registration Token" onclose={() => (createTokenModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Uses</label>
			<input type="number" class="input w-full" bind:value={newTokenMaxUses} min="1" max="1000" />
			<p class="mt-1 text-xs text-text-muted">How many times this token can be used. Set to 0 for unlimited.</p>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Note</label>
			<input type="text" class="input w-full" bind:value={newTokenNote} placeholder="e.g., For team members" maxlength="200" />
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Expires In (Hours)</label>
			<input type="number" class="input w-full" bind:value={newTokenExpiryHours} min="0" max="8760" />
			<p class="mt-1 text-xs text-text-muted">Set to 0 for no expiration.</p>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createTokenModalOpen = false)}>Cancel</button>
			<button class="btn-primary text-sm" onclick={handleCreateToken} disabled={creatingToken}>
				{creatingToken ? 'Creating...' : 'Create Token'}
			</button>
		</div>
	</div>
</Modal>

<!-- Create Announcement Modal -->
<Modal open={createAnnouncementModalOpen} title="Create Announcement" onclose={() => (createAnnouncementModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
			<input type="text" class="input w-full" bind:value={announcementTitle} maxlength="200" placeholder="Announcement title" />
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Content</label>
			<textarea class="input w-full" rows="4" bind:value={announcementContent} maxlength="2000" placeholder="Announcement content..."></textarea>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Severity</label>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="info" class="accent-blue-500" />
					Info
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="warning" class="accent-yellow-500" />
					Warning
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={announcementSeverity} value="critical" class="accent-red-500" />
					Critical
				</label>
			</div>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Expires In (Hours)</label>
			<input type="number" class="input w-full" bind:value={announcementExpiryHours} min="0" max="8760" />
			<p class="mt-1 text-xs text-text-muted">Set to 0 for no automatic expiration.</p>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createAnnouncementModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleCreateAnnouncement}
				disabled={savingAnnouncement || !announcementTitle.trim() || !announcementContent.trim()}
			>
				{savingAnnouncement ? 'Creating...' : 'Create Announcement'}
			</button>
		</div>
	</div>
</Modal>

<!-- Edit Announcement Modal -->
<Modal open={editAnnouncementModalOpen} title="Edit Announcement" onclose={() => (editAnnouncementModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
			<input type="text" class="input w-full" bind:value={announcementTitle} maxlength="200" />
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Content</label>
			<textarea class="input w-full" rows="4" bind:value={announcementContent} maxlength="2000"></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (editAnnouncementModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleUpdateAnnouncement}
				disabled={savingAnnouncement || !announcementTitle.trim() || !announcementContent.trim()}
			>
				{savingAnnouncement ? 'Saving...' : 'Save Changes'}
			</button>
		</div>
	</div>
</Modal>

<!-- Create Content Scan Rule Modal -->
<Modal open={createRuleModalOpen} title="Create Content Scan Rule" onclose={() => (createRuleModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
			<input type="text" class="input w-full" bind:value={ruleName} maxlength="200" placeholder="e.g., Block executable files" />
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Regex Pattern</label>
			<input type="text" class="input w-full font-mono text-sm" bind:value={rulePattern} placeholder="e.g., \.(exe|bat|cmd|ps1)$" />
			<p class="mt-1 text-xs text-text-muted">Regular expression pattern to match against the target field.</p>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Target</label>
			<select class="input w-full" bind:value={ruleTarget}>
				<option value="filename">Filename</option>
				<option value="content_type">Content Type (MIME)</option>
				<option value="text_content">Text Content (message body)</option>
			</select>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</label>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="log" class="accent-blue-500" />
					Log Only
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="flag" class="accent-yellow-500" />
					Flag for Review
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="block" class="accent-red-500" />
					Block Upload
				</label>
			</div>
		</div>
		<div>
			<label class="flex items-center gap-2 text-sm text-text-secondary">
				<input type="checkbox" bind:checked={ruleEnabled} class="accent-brand-500" />
				Enabled
			</label>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createRuleModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleCreateRule}
				disabled={savingRule || !ruleName.trim() || !rulePattern.trim()}
			>
				{savingRule ? 'Creating...' : 'Create Rule'}
			</button>
		</div>
	</div>
</Modal>

<!-- Edit Content Scan Rule Modal -->
<Modal open={editRuleModalOpen} title="Edit Content Scan Rule" onclose={() => (editRuleModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Rule Name</label>
			<input type="text" class="input w-full" bind:value={ruleName} maxlength="200" />
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Regex Pattern</label>
			<input type="text" class="input w-full font-mono text-sm" bind:value={rulePattern} />
			<p class="mt-1 text-xs text-text-muted">Regular expression pattern to match against the target field.</p>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Target</label>
			<select class="input w-full" bind:value={ruleTarget}>
				<option value="filename">Filename</option>
				<option value="content_type">Content Type (MIME)</option>
				<option value="text_content">Text Content (message body)</option>
			</select>
		</div>
		<div>
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Action</label>
			<div class="flex gap-4">
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="log" class="accent-blue-500" />
					Log Only
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="flag" class="accent-yellow-500" />
					Flag for Review
				</label>
				<label class="flex items-center gap-2 text-sm text-text-secondary">
					<input type="radio" bind:group={ruleAction} value="block" class="accent-red-500" />
					Block Upload
				</label>
			</div>
		</div>
		<div>
			<label class="flex items-center gap-2 text-sm text-text-secondary">
				<input type="checkbox" bind:checked={ruleEnabled} class="accent-brand-500" />
				Enabled
			</label>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (editRuleModalOpen = false)}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={handleUpdateRule}
				disabled={savingRule || !ruleName.trim() || !rulePattern.trim()}
			>
				{savingRule ? 'Saving...' : 'Save Changes'}
			</button>
		</div>
	</div>
</Modal>

<div class="flex h-full">
	<!-- Sidebar -->
	<nav class="flex w-48 shrink-0 flex-col overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Administration</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
		</ul>
		<div class="mt-auto pt-4">
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
				onclick={() => goto('/app')}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
					<path d="M15 19l-7-7 7-7" />
				</svg>
				Back to App
			</button>
		</div>
	</nav>

	<!-- Content -->
	<div class="flex-1 overflow-y-auto p-8">
		<div class="max-w-3xl">
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}

			<!-- ==================== DASHBOARD ==================== -->
			{#if currentTab === 'dashboard'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Instance Dashboard</h1>
					<button class="btn-secondary text-sm" onclick={refresh} disabled={loading}>
						{loading ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loading && !stats}
					<p class="text-text-muted">Loading stats...</p>
				{:else if stats}
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Total Users</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{stats.users.toLocaleString()}</p>
							<p class="text-xs text-text-muted">{stats.online_users} online</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Total Guilds</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{stats.guilds.toLocaleString()}</p>
							<p class="text-xs text-text-muted">{stats.channels} channels</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Messages Today</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{stats.messages_today.toLocaleString()}</p>
							<p class="text-xs text-text-muted">{stats.messages.toLocaleString()} total</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Files</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{stats.files.toLocaleString()}</p>
							<p class="text-xs text-text-muted">{stats.invites} active invites</p>
						</div>
					</div>

					<div class="mt-4 grid gap-4 sm:grid-cols-3">
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Roles</p>
							<p class="mt-1 text-xl font-bold text-text-primary">{stats.roles}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Custom Emoji</p>
							<p class="mt-1 text-xl font-bold text-text-primary">{stats.emoji}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Federation Peers</p>
							<p class="mt-1 text-xl font-bold text-text-primary">{stats.federation_peers}</p>
						</div>
					</div>

					<div class="mt-8">
						<h2 class="mb-4 text-lg font-semibold text-text-primary">System Info</h2>
						<div class="rounded-lg bg-bg-secondary p-4">
							<div class="grid gap-3 text-sm">
								<div class="flex justify-between">
									<span class="text-text-muted">Software</span>
									<span class="text-text-primary">AmityVox v0.5.0</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">Go Version</span>
									<span class="text-text-primary">{stats.go_version}</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">Database Size</span>
									<span class="text-text-primary">{stats.database_size}</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">Memory (Alloc / Sys)</span>
									<span class="text-text-primary">{stats.mem_alloc_mb} MB / {stats.mem_sys_mb} MB</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">Goroutines</span>
									<span class="text-text-primary">{stats.goroutines}</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">CPUs</span>
									<span class="text-text-primary">{stats.num_cpu}</span>
								</div>
								<div class="flex justify-between">
									<span class="text-text-muted">Uptime</span>
									<span class="text-text-primary">{stats.uptime}</span>
								</div>
							</div>
						</div>
					</div>
				{/if}

			<!-- ==================== USERS ==================== -->
			{:else if currentTab === 'users'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">User Management</h1>

				<div class="mb-4">
					<input
						type="text"
						class="input w-full"
						placeholder="Search users by name..."
						bind:value={userSearch}
						oninput={handleUserSearch}
					/>
				</div>

				{#if loadingUsers}
					<p class="text-sm text-text-muted">Loading users...</p>
				{:else if users.length === 0}
					<p class="text-sm text-text-muted">No users found.</p>
				{:else}
					<div class="space-y-2">
						{#each users as user (user.id)}
							{@const isSuspended = (user.flags & 1) !== 0}
							{@const isAdmin = (user.flags & 4) !== 0}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center gap-3">
									<Avatar name={user.display_name ?? user.username} size="sm" />
									<div>
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-text-primary">{user.display_name ?? user.username}</span>
											<span class="text-xs text-text-muted">@{user.username}</span>
											{#if isAdmin}
												<span class="rounded bg-brand-500/20 px-1.5 py-0.5 text-2xs font-bold text-brand-400">Admin</span>
											{/if}
											{#if (user.flags & 32) !== 0}
												<span class="rounded bg-orange-500/20 px-1.5 py-0.5 text-2xs font-bold text-orange-400">Global Mod</span>
											{/if}
											{#if isSuspended}
												<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-2xs font-bold text-red-400">Suspended</span>
											{/if}
										</div>
										<p class="text-xs text-text-muted">
											{user.email ?? 'No email'} &middot; {user.id.slice(0, 8)}... &middot; Joined {new Date(user.created_at).toLocaleDateString()}
										</p>
									</div>
								</div>
								<div class="flex items-center gap-2">
									<button
										class="text-xs {isAdmin ? 'text-yellow-400 hover:text-yellow-300' : 'text-brand-400 hover:text-brand-300'}"
										onclick={() => handleToggleAdmin(user.id, user.flags)}
									>
										{isAdmin ? 'Remove Admin' : 'Make Admin'}
									</button>
									<button
										class="text-xs {(user.flags & 32) !== 0 ? 'text-orange-400 hover:text-orange-300' : 'text-orange-400 hover:text-orange-300'}"
										onclick={() => handleToggleGlobalMod(user.id, user.flags)}
									>
										{(user.flags & 32) !== 0 ? 'Remove Mod' : 'Make Mod'}
									</button>
									{#if isSuspended}
										<button
											class="text-xs text-green-400 hover:text-green-300"
											onclick={() => handleUnsuspend(user.id)}
										>
											Unsuspend
										</button>
									{:else}
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => handleSuspend(user.id)}
										>
											Suspend
										</button>
									{/if}
									<button
										class="text-xs text-red-500 hover:text-red-400 font-medium"
										onclick={() => openBanModal(user)}
									>
										Ban
									</button>
									<button
										class="text-xs text-text-muted hover:text-text-primary"
										onclick={() => loadUserGuilds(user.id)}
									>
										{expandedUserGuilds === user.id ? 'Hide' : 'Guilds'}
									</button>
								</div>
							</div>
							{#if expandedUserGuilds === user.id}
								<div class="ml-12 mt-1 mb-2 rounded-lg bg-bg-modifier/30 p-3">
									{#if loadingUserGuilds}
										<p class="text-xs text-text-muted">Loading guilds...</p>
									{:else if userGuildsList.length === 0}
										<p class="text-xs text-text-muted">Not a member of any guilds.</p>
									{:else}
										<p class="text-xs text-text-muted mb-2">{userGuildsList.length} guild{userGuildsList.length !== 1 ? 's' : ''}</p>
										<div class="space-y-1">
											{#each userGuildsList as ug (ug.id)}
												<div class="flex items-center justify-between rounded bg-bg-secondary/50 px-2 py-1.5">
													<div class="flex items-center gap-2">
														<span class="text-sm text-text-primary">{ug.name}</span>
														{#if ug.is_owner}
															<span class="rounded bg-yellow-500/20 px-1 py-0.5 text-2xs font-bold text-yellow-400">Owner</span>
														{/if}
													</div>
													<span class="text-xs text-text-muted">{ug.member_count} members &middot; Joined {new Date(ug.joined_at).toLocaleDateString()}</span>
												</div>
											{/each}
										</div>
									{/if}
								</div>
							{/if}
						{/each}
					</div>
				{/if}

			<!-- ==================== GUILDS ==================== -->
			{:else if currentTab === 'guilds'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Guild Management</h1>
					<button class="btn-secondary text-sm" onclick={() => { adminGuilds = []; loadGuilds(); }}>
						{loadingGuilds ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				<div class="mb-4 flex gap-3">
					<input
						type="text"
						class="input flex-1"
						placeholder="Search guilds by name..."
						bind:value={guildSearch}
						oninput={handleGuildSearch}
					/>
					<select class="input w-40" bind:value={guildSort} onchange={() => loadGuilds()}>
						<option value="newest">Newest</option>
						<option value="oldest">Oldest</option>
						<option value="name">Name A-Z</option>
						<option value="members">Most Members</option>
					</select>
				</div>

				{#if loadingGuilds}
					<p class="text-sm text-text-muted">Loading guilds...</p>
				{:else if adminGuilds.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">No guilds found.</p>
					</div>
				{:else}
					<p class="mb-3 text-xs text-text-muted">{adminGuilds.length} guild{adminGuilds.length !== 1 ? 's' : ''}</p>
					<div class="space-y-2">
						{#each adminGuilds as guild (guild.id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center gap-3">
									{#if guild.icon_url}
										<img src={guild.icon_url} alt="" class="h-10 w-10 rounded-full object-cover" />
									{:else}
										<div class="flex h-10 w-10 items-center justify-center rounded-full bg-brand-500/20 text-sm font-bold text-brand-400">
											{guild.name.charAt(0).toUpperCase()}
										</div>
									{/if}
									<div>
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-text-primary">{guild.name}</span>
											<span class="text-xs text-text-muted">{guild.id.slice(0, 8)}...</span>
										</div>
										<p class="text-xs text-text-muted">
											Owner: {guild.owner_name} &middot;
											{guild.member_count} member{guild.member_count !== 1 ? 's' : ''} &middot;
											{guild.channel_count} channel{guild.channel_count !== 1 ? 's' : ''} &middot;
											{guild.role_count} role{guild.role_count !== 1 ? 's' : ''} &middot;
											Created {new Date(guild.created_at).toLocaleDateString()}
										</p>
									</div>
								</div>
								<div class="flex items-center gap-2">
									<button
										class="text-xs text-brand-400 hover:text-brand-300"
										onclick={() => viewGuildDetail(guild.id)}
									>
										Details
									</button>
									<button
										class="text-xs text-red-400 hover:text-red-300"
										onclick={() => handleDeleteGuild(guild.id, guild.name)}
									>
										Delete
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== BOTS ==================== -->
			{:else if currentTab === 'bots'}
				<div class="mb-6 flex items-center justify-between">
					<div>
						<h1 class="text-2xl font-bold text-text-primary">Bot Management</h1>
						<p class="mt-1 text-sm text-text-muted">All bot accounts with their permissions, subscriptions, and rate limits.</p>
					</div>
					<button class="btn-secondary text-sm" onclick={() => { allBots = []; loadAllBots(); }}>
						{loadingAllBots ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingAllBots}
					<p class="text-sm text-text-muted">Loading bots...</p>
				{:else if allBots.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">No bot accounts on this instance.</p>
					</div>
				{:else}
					<div class="space-y-3">
						{#each allBots as bot (bot.id)}
							{@const isSuspended = (bot.flags & 1) !== 0}
							{@const isExpanded = expandedBotId === bot.id}
							<div class="rounded-lg border border-bg-modifier bg-bg-secondary overflow-hidden">
								<button
									class="flex w-full items-center justify-between p-4 text-left hover:bg-bg-modifier/50 transition-colors"
									onclick={() => toggleBotExpand(bot.id)}
								>
									<div class="flex items-center gap-3">
										<div class="flex h-10 w-10 items-center justify-center rounded-full bg-brand-500/20 text-brand-400">
											<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
											</svg>
										</div>
										<div>
											<div class="flex items-center gap-2">
												<span class="font-medium text-text-primary">{bot.username}</span>
												{#if bot.display_name}
													<span class="text-xs text-text-muted">({bot.display_name})</span>
												{/if}
												{#if isSuspended}
													<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-2xs font-bold text-red-400">Suspended</span>
												{:else}
													<span class="rounded bg-green-500/20 px-1.5 py-0.5 text-2xs font-bold text-green-400">Active</span>
												{/if}
												{#if bot.presence}
													<span class="rounded px-1.5 py-0.5 text-2xs font-bold {presenceStatusColor(bot.presence.status)}">
														{bot.presence.status}{#if bot.presence.activity_name} - {bot.presence.activity_type ?? ''} {bot.presence.activity_name}{/if}
													</span>
												{/if}
											</div>
											<p class="text-xs text-text-muted">
												Owner: {bot.bot_owner_id ? bot.bot_owner_id.slice(0, 12) + '...' : 'N/A'}
												&middot; ID: <span class="font-mono">{bot.id.slice(0, 12)}...</span>
												&middot; Created {new Date(bot.created_at).toLocaleDateString()}
											</p>
										</div>
									</div>
									<div class="flex items-center gap-3">
										<div class="flex gap-2 text-xs text-text-muted">
											<span>{bot.guild_permissions?.length ?? 0} guilds</span>
											<span>&middot;</span>
											<span>{bot.event_subscriptions?.length ?? 0} subs</span>
											{#if bot.rate_limit}
												<span>&middot;</span>
												<span>{bot.rate_limit.requests_per_second} req/s</span>
											{/if}
										</div>
										<svg class="h-4 w-4 text-text-muted transition-transform {isExpanded ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M19 9l-7 7-7-7" /></svg>
									</div>
								</button>

								{#if isExpanded}
									<div class="border-t border-bg-modifier p-4 space-y-4">
										<div>
											<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Guild Permissions</h4>
											{#if bot.guild_permissions && bot.guild_permissions.length > 0}
												<div class="space-y-2">
													{#each bot.guild_permissions as perm}
														<div class="rounded bg-bg-primary p-3">
															<div class="flex items-center justify-between">
																<span class="text-sm font-medium text-text-primary font-mono">{perm.guild_id.slice(0, 16)}...</span>
																<span class="text-xs text-text-muted">Max role pos: {perm.max_role_position}</span>
															</div>
															<div class="mt-1 flex flex-wrap gap-1">
																{#each perm.scopes as scope}
																	<span class="rounded bg-brand-500/10 px-1.5 py-0.5 text-2xs text-brand-400">{scope}</span>
																{/each}
															</div>
															<p class="mt-1 text-2xs text-text-muted">Updated {new Date(perm.updated_at).toLocaleString()}</p>
														</div>
													{/each}
												</div>
											{:else}
												<p class="text-xs text-text-muted">No guild-specific permissions configured.</p>
											{/if}
										</div>

										<div>
											<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Event Subscriptions</h4>
											{#if bot.event_subscriptions && bot.event_subscriptions.length > 0}
												<div class="overflow-hidden rounded border border-bg-modifier">
													<table class="w-full text-left text-xs">
														<thead class="bg-bg-primary">
															<tr>
																<th class="px-3 py-2 text-text-muted font-bold uppercase tracking-wide">Guild</th>
																<th class="px-3 py-2 text-text-muted font-bold uppercase tracking-wide">Events</th>
																<th class="px-3 py-2 text-text-muted font-bold uppercase tracking-wide">Webhook</th>
																<th class="px-3 py-2 text-text-muted font-bold uppercase tracking-wide">Created</th>
															</tr>
														</thead>
														<tbody class="divide-y divide-bg-modifier">
															{#each bot.event_subscriptions as sub}
																<tr>
																	<td class="px-3 py-2 text-text-secondary font-mono">{sub.guild_id.slice(0, 12)}...</td>
																	<td class="px-3 py-2"><div class="flex flex-wrap gap-1">{#each sub.event_types as evt}<span class="rounded bg-purple-500/10 px-1 py-0.5 text-2xs text-purple-400">{evt}</span>{/each}</div></td>
																	<td class="px-3 py-2 text-text-muted max-w-48 truncate" title={sub.webhook_url}>{sub.webhook_url.slice(0, 40)}{sub.webhook_url.length > 40 ? '...' : ''}</td>
																	<td class="px-3 py-2 text-text-muted">{new Date(sub.created_at).toLocaleDateString()}</td>
																</tr>
															{/each}
														</tbody>
													</table>
												</div>
											{:else}
												<p class="text-xs text-text-muted">No event subscriptions.</p>
											{/if}
										</div>

										<div>
											<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Rate Limit</h4>
											<div class="rounded bg-bg-primary p-3">
												<div class="grid grid-cols-2 gap-4 text-sm">
													<div>
														<span class="text-text-muted">Requests/second:</span>
														<span class="ml-2 font-medium text-text-primary">{bot.rate_limit ? bot.rate_limit.requests_per_second : '50 (default)'}</span>
													</div>
													<div>
														<span class="text-text-muted">Burst:</span>
														<span class="ml-2 font-medium text-text-primary">{bot.rate_limit ? bot.rate_limit.burst : '100 (default)'}</span>
													</div>
												</div>
												{#if bot.rate_limit}
													<p class="mt-1 text-2xs text-text-muted">Updated {new Date(bot.rate_limit.updated_at).toLocaleString()}</p>
												{/if}
											</div>
										</div>

										<div>
											<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Presence</h4>
											{#if bot.presence}
												<div class="rounded bg-bg-primary p-3">
													<div class="flex items-center gap-3 text-sm">
														<span class="rounded px-2 py-0.5 text-xs font-bold {presenceStatusColor(bot.presence.status)}">{bot.presence.status}</span>
														{#if bot.presence.activity_type && bot.presence.activity_name}
															<span class="text-text-secondary">{bot.presence.activity_type}: <span class="text-text-primary">{bot.presence.activity_name}</span></span>
														{:else}
															<span class="text-text-muted">No activity set</span>
														{/if}
													</div>
													<p class="mt-1 text-2xs text-text-muted">Updated {new Date(bot.presence.updated_at).toLocaleString()}</p>
												</div>
											{:else}
												<p class="text-xs text-text-muted">No presence data. Bot has not set a status.</p>
											{/if}
										</div>
									</div>
								{/if}
							</div>
						{/each}
					</div>
					<p class="mt-3 text-xs text-text-muted">{allBots.length} bot{allBots.length !== 1 ? 's' : ''} total</p>
				{/if}

			<!-- ==================== INSTANCE BANS ==================== -->
			{:else if currentTab === 'bans'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">Instance Bans</h1>

				{#if loadingBans}
					<p class="text-sm text-text-muted">Loading bans...</p>
				{:else if instanceBans.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">No instance-banned users.</p>
						<p class="mt-1 text-xs text-text-muted">Users can be instance-banned from the Users tab.</p>
					</div>
				{:else}
					<div class="overflow-hidden rounded-lg border border-bg-modifier">
						<table class="w-full text-left text-sm">
							<thead class="bg-bg-secondary">
								<tr>
									<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">User</th>
									<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Reason</th>
									<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Banned</th>
									<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">By</th>
									<th class="px-4 py-3 text-right text-xs font-bold uppercase tracking-wide text-text-muted">Actions</th>
								</tr>
							</thead>
							<tbody class="divide-y divide-bg-modifier">
								{#each instanceBans as ban (ban.user_id)}
									<tr class="hover:bg-bg-secondary/50">
										<td class="px-4 py-3">
											<div class="flex items-center gap-2">
												<Avatar name={ban.display_name ?? ban.username} size="sm" />
												<div>
													<span class="font-medium text-text-primary">{ban.display_name ?? ban.username}</span>
													<span class="ml-1 text-xs text-text-muted">@{ban.username}</span>
												</div>
											</div>
										</td>
										<td class="px-4 py-3 text-text-secondary">{ban.reason ?? 'No reason provided'}</td>
										<td class="px-4 py-3 text-text-muted">{new Date(ban.created_at).toLocaleDateString()}</td>
										<td class="px-4 py-3 text-text-muted">{ban.admin_id.slice(0, 8)}...</td>
										<td class="px-4 py-3 text-right">
											<button
												class="text-xs text-green-400 hover:text-green-300"
												onclick={() => handleInstanceUnban(ban.user_id)}
											>
												Unban
											</button>
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}

			<!-- ==================== REGISTRATION ==================== -->
			{:else if currentTab === 'registration'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">Registration Settings</h1>

				{#if loadingReg}
					<p class="text-sm text-text-muted">Loading registration settings...</p>
				{:else}
					<div class="space-y-6">
						<!-- Mode -->
						<div class="rounded-lg bg-bg-secondary p-4">
							<label class="mb-3 block text-xs font-bold uppercase tracking-wide text-text-muted">Registration Mode</label>
							<div class="space-y-2">
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {regMode === 'open' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={regMode} value="open" class="accent-brand-500" />
									<div>
										<span class="font-medium">Open</span>
										<p class="text-xs text-text-muted">Anyone can register a new account.</p>
									</div>
								</label>
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {regMode === 'invite_only' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={regMode} value="invite_only" class="accent-brand-500" />
									<div>
										<span class="font-medium">Invite Only</span>
										<p class="text-xs text-text-muted">A valid registration token is required to create an account.</p>
									</div>
								</label>
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {regMode === 'closed' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={regMode} value="closed" class="accent-brand-500" />
									<div>
										<span class="font-medium">Closed</span>
										<p class="text-xs text-text-muted">No new registrations. Only existing accounts can log in.</p>
									</div>
								</label>
							</div>
						</div>

						<!-- Custom Message -->
						{#if regMode !== 'open'}
							<div>
								<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Custom Message</label>
								<textarea
									class="input w-full"
									rows="3"
									bind:value={regMessage}
									placeholder="Message shown to visitors on the registration page..."
									maxlength="500"
								></textarea>
								<p class="mt-1 text-xs text-text-muted">Displayed on the registration page when registration is not open.</p>
							</div>
						{/if}

						<button class="btn-primary" onclick={saveRegistration} disabled={savingReg}>
							{savingReg ? 'Saving...' : 'Save Registration Settings'}
						</button>

						<!-- Tokens Section -->
						<div class="border-t border-bg-modifier pt-6">
							<div class="mb-4 flex items-center justify-between">
								<h2 class="text-lg font-semibold text-text-primary">Registration Tokens</h2>
								<button class="btn-primary text-sm" onclick={openCreateTokenModal}>
									Create Token
								</button>
							</div>

							{#if regTokens.length === 0}
								<div class="rounded-lg bg-bg-secondary p-6 text-center">
									<p class="text-sm text-text-muted">No registration tokens.</p>
									<p class="mt-1 text-xs text-text-muted">Create a token to invite users when in invite-only mode.</p>
								</div>
							{:else}
								<div class="overflow-hidden rounded-lg border border-bg-modifier">
									<table class="w-full text-left text-sm">
										<thead class="bg-bg-secondary">
											<tr>
												<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Token</th>
												<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Uses</th>
												<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Note</th>
												<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Expires</th>
												<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Status</th>
												<th class="px-4 py-3 text-right text-xs font-bold uppercase tracking-wide text-text-muted">Actions</th>
											</tr>
										</thead>
										<tbody class="divide-y divide-bg-modifier">
											{#each regTokens as token (token.id)}
												{@const status = getTokenStatus(token)}
												<tr class="hover:bg-bg-secondary/50">
													<td class="px-4 py-3">
														<code class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-primary">{token.token.slice(0, 12)}...</code>
													</td>
													<td class="px-4 py-3 text-text-secondary">
														{token.uses}{token.max_uses > 0 ? `/${token.max_uses}` : '/unlimited'}
													</td>
													<td class="px-4 py-3 text-text-muted">{token.note ?? '-'}</td>
													<td class="px-4 py-3 text-text-muted">
														{token.expires_at ? new Date(token.expires_at).toLocaleString() : 'Never'}
													</td>
													<td class="px-4 py-3">
														<span class="rounded px-1.5 py-0.5 text-2xs font-bold {status === 'active' ? 'bg-green-500/20 text-green-400' : status === 'expired' ? 'bg-red-500/20 text-red-400' : 'bg-yellow-500/20 text-yellow-400'}">
															{status}
														</span>
													</td>
													<td class="px-4 py-3 text-right">
														<div class="flex items-center justify-end gap-2">
															<button
																class="text-xs text-brand-400 hover:text-brand-300"
																onclick={() => copyToken(token.token)}
															>
																Copy
															</button>
															<button
																class="text-xs text-red-400 hover:text-red-300"
																onclick={() => handleDeleteToken(token.id)}
															>
																Delete
															</button>
														</div>
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							{/if}
						</div>
					</div>
				{/if}

			<!-- ==================== ANNOUNCEMENTS ==================== -->
			{:else if currentTab === 'announcements'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Announcements</h1>
					<button class="btn-primary text-sm" onclick={openCreateAnnouncement}>
						Create Announcement
					</button>
				</div>

				{#if loadingAnnouncements}
					<p class="text-sm text-text-muted">Loading announcements...</p>
				{:else if announcements.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">No announcements yet.</p>
						<p class="mt-1 text-xs text-text-muted">Create an announcement to notify all users on this instance.</p>
					</div>
				{:else}
					<div class="space-y-3">
						{#each announcements as announcement (announcement.id)}
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="mb-2 flex items-start justify-between">
									<div class="flex items-center gap-2">
										<h3 class="text-sm font-semibold text-text-primary">{announcement.title}</h3>
										<span class="rounded px-1.5 py-0.5 text-2xs font-bold {severityClasses(announcement.severity)}">
											{severityLabel(announcement.severity)}
										</span>
										<span class="rounded px-1.5 py-0.5 text-2xs font-bold {announcement.active ? 'bg-green-500/20 text-green-400' : 'bg-gray-500/20 text-gray-400'}">
											{announcement.active ? 'Active' : 'Inactive'}
										</span>
									</div>
									<div class="flex items-center gap-2">
										<button
											class="text-xs text-brand-400 hover:text-brand-300"
											onclick={() => openEditAnnouncement(announcement)}
										>
											Edit
										</button>
										<button
											class="text-xs {announcement.active ? 'text-yellow-400 hover:text-yellow-300' : 'text-green-400 hover:text-green-300'}"
											onclick={() => handleToggleAnnouncement(announcement)}
										>
											{announcement.active ? 'Deactivate' : 'Activate'}
										</button>
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => handleDeleteAnnouncement(announcement.id)}
										>
											Delete
										</button>
									</div>
								</div>
								<p class="mb-2 text-sm text-text-secondary">{announcement.content}</p>
								<div class="flex gap-4 text-xs text-text-muted">
									<span>Created {new Date(announcement.created_at).toLocaleString()}</span>
									{#if announcement.expires_at}
										<span>Expires {new Date(announcement.expires_at).toLocaleString()}</span>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== RATE LIMITING ==================== -->
			{:else if currentTab === 'rate_limits'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Rate Limiting</h1>
					<button class="btn-secondary text-sm" onclick={loadRateLimitStats} disabled={loadingRateLimits}>
						{loadingRateLimits ? 'Loading...' : 'Refresh'}
					</button>
				</div>

				{#if loadingRateLimits && !rateLimitStats}
					<p class="text-sm text-text-muted">Loading rate limit data...</p>
				{:else if rateLimitStats}
					<!-- Summary Cards -->
					<div class="mb-6 grid gap-4 sm:grid-cols-3">
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Unique IPs (24h)</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{rateLimitStats.unique_ips_24h.toLocaleString()}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Total Entries (24h)</p>
							<p class="mt-1 text-2xl font-bold text-text-primary">{rateLimitStats.total_entries_24h.toLocaleString()}</p>
						</div>
						<div class="rounded-lg bg-bg-secondary p-4">
							<p class="text-sm text-text-muted">Blocked (24h)</p>
							<p class="mt-1 text-2xl font-bold text-red-400">{rateLimitStats.blocked_entries_24h.toLocaleString()}</p>
						</div>
					</div>

					<!-- Configuration -->
					<div class="mb-6 rounded-lg bg-bg-secondary p-4">
						<h2 class="mb-4 text-sm font-semibold text-text-primary">Configuration</h2>
						<div class="grid gap-4 sm:grid-cols-2">
							<div>
								<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Requests Per Window</label>
								<input type="text" class="input w-full" bind:value={editReqsPerWindow} placeholder="100" />
							</div>
							<div>
								<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Window (seconds)</label>
								<input type="text" class="input w-full" bind:value={editWindowSeconds} placeholder="60" />
							</div>
						</div>
						<button class="btn-primary mt-4 text-sm" onclick={saveRateLimitConfig} disabled={savingRateLimitConfig}>
							{savingRateLimitConfig ? 'Saving...' : 'Save Configuration'}
						</button>
					</div>

					<!-- Top IPs Table -->
					<div class="mb-6">
						<h2 class="mb-4 text-sm font-semibold text-text-primary">Top IPs (Last 24 Hours)</h2>
						{#if rateLimitStats.top_ips.length === 0}
							<div class="rounded-lg bg-bg-secondary p-6 text-center">
								<p class="text-sm text-text-muted">No rate limit data recorded in the last 24 hours.</p>
							</div>
						{:else}
							<div class="overflow-hidden rounded-lg border border-bg-modifier">
								<table class="w-full text-left text-sm">
									<thead class="bg-bg-secondary">
										<tr>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">IP Address</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Total Requests</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Blocks</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Last Seen</th>
										</tr>
									</thead>
									<tbody class="divide-y divide-bg-modifier">
										{#each rateLimitStats.top_ips as ip (ip.ip_address)}
											<tr class="hover:bg-bg-secondary/50">
												<td class="px-4 py-3">
													<code class="text-xs text-text-primary">{ip.ip_address}</code>
												</td>
												<td class="px-4 py-3 text-text-secondary">{ip.total_requests.toLocaleString()}</td>
												<td class="px-4 py-3">
													{#if ip.block_count > 0}
														<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-2xs font-bold text-red-400">{ip.block_count}</span>
													{:else}
														<span class="text-text-muted">0</span>
													{/if}
												</td>
												<td class="px-4 py-3 text-text-muted">{new Date(ip.last_seen).toLocaleString()}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>

					<!-- Log Viewer -->
					<div>
						<div class="mb-4 flex items-center justify-between">
							<h2 class="text-sm font-semibold text-text-primary">Rate Limit Log</h2>
							<button class="btn-secondary text-sm" onclick={loadRateLimitLog} disabled={loadingRateLimitLog}>
								{loadingRateLimitLog ? 'Loading...' : 'Load Log'}
							</button>
						</div>
						<div class="mb-4 flex gap-2">
							<input
								type="text"
								class="input flex-1"
								placeholder="Filter by IP address..."
								bind:value={rateLimitIPFilter}
								onkeydown={(e) => e.key === 'Enter' && loadRateLimitLog()}
							/>
							<select class="input w-36" bind:value={rateLimitLogFilter} onchange={() => loadRateLimitLog()}>
								<option value="all">All Entries</option>
								<option value="blocked">Blocked Only</option>
							</select>
						</div>
						{#if rateLimitLog.length > 0}
							<div class="overflow-hidden rounded-lg border border-bg-modifier">
								<table class="w-full text-left text-sm">
									<thead class="bg-bg-secondary">
										<tr>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">IP</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Endpoint</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Requests</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Status</th>
											<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Time</th>
										</tr>
									</thead>
									<tbody class="divide-y divide-bg-modifier">
										{#each rateLimitLog as entry (entry.id)}
											<tr class="hover:bg-bg-secondary/50">
												<td class="px-4 py-3"><code class="text-xs text-text-primary">{entry.ip_address}</code></td>
												<td class="px-4 py-3 text-text-muted text-xs">{entry.endpoint}</td>
												<td class="px-4 py-3 text-text-secondary">{entry.requests_count}</td>
												<td class="px-4 py-3">
													<span class="rounded px-1.5 py-0.5 text-2xs font-bold {entry.blocked ? 'bg-red-500/20 text-red-400' : 'bg-green-500/20 text-green-400'}">
														{entry.blocked ? 'Blocked' : 'Allowed'}
													</span>
												</td>
												<td class="px-4 py-3 text-text-muted text-xs">{new Date(entry.created_at).toLocaleString()}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{:else if !loadingRateLimitLog}
							<p class="text-sm text-text-muted">Click "Load Log" to view recent rate limit entries.</p>
						{/if}
					</div>
				{:else}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">Failed to load rate limit data. Make sure migration 031 has been applied.</p>
					</div>
				{/if}

			<!-- ==================== CONTENT SAFETY ==================== -->
			{:else if currentTab === 'content_safety'}
				<div class="mb-6 flex items-center justify-between">
					<h1 class="text-2xl font-bold text-text-primary">Content Safety</h1>
					<div class="flex gap-2">
						<button
							class="text-sm {contentLogSubTab === 'rules' ? 'btn-primary' : 'btn-secondary'}"
							onclick={() => (contentLogSubTab = 'rules')}
						>
							Rules
						</button>
						<button
							class="text-sm {contentLogSubTab === 'log' ? 'btn-primary' : 'btn-secondary'}"
							onclick={() => { contentLogSubTab = 'log'; if (contentScanLog.length === 0) loadContentScanLog(); }}
						>
							Scan Log
						</button>
					</div>
				</div>

				{#if contentLogSubTab === 'rules'}
					<div class="mb-4 flex items-center justify-between">
						<p class="text-sm text-text-muted">
							Define regex patterns to scan uploads and messages. Matched content can be blocked, flagged, or logged.
						</p>
						<button class="btn-primary text-sm" onclick={openCreateRuleModal}>
							Create Rule
						</button>
					</div>

					{#if loadingContentRules}
						<p class="text-sm text-text-muted">Loading content scan rules...</p>
					{:else if contentScanRules.length === 0}
						<div class="rounded-lg bg-bg-secondary p-6 text-center">
							<p class="text-sm text-text-muted">No content scan rules configured.</p>
							<p class="mt-1 text-xs text-text-muted">Create a rule to start scanning uploads and messages.</p>
						</div>
					{:else}
						<div class="space-y-3">
							{#each contentScanRules as rule (rule.id)}
								<div class="rounded-lg bg-bg-secondary p-4">
									<div class="flex items-start justify-between">
										<div class="flex-1">
											<div class="mb-1 flex items-center gap-2">
												<h3 class="text-sm font-semibold text-text-primary">{rule.name}</h3>
												<span class="rounded px-1.5 py-0.5 text-2xs font-bold {actionClasses(rule.action)}">
													{rule.action}
												</span>
												<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs text-text-muted">
													{targetLabel(rule.target)}
												</span>
												<span class="rounded px-1.5 py-0.5 text-2xs font-bold {rule.enabled ? 'bg-green-500/20 text-green-400' : 'bg-gray-500/20 text-gray-400'}">
													{rule.enabled ? 'Enabled' : 'Disabled'}
												</span>
											</div>
											<code class="text-xs text-text-muted">{rule.pattern}</code>
											<p class="mt-1 text-xs text-text-muted">Created {new Date(rule.created_at).toLocaleString()}</p>
										</div>
										<div class="flex items-center gap-2">
											<button
												class="text-xs text-brand-400 hover:text-brand-300"
												onclick={() => openEditRuleModal(rule)}
											>
												Edit
											</button>
											<button
												class="text-xs {rule.enabled ? 'text-yellow-400 hover:text-yellow-300' : 'text-green-400 hover:text-green-300'}"
												onclick={() => handleToggleRule(rule)}
											>
												{rule.enabled ? 'Disable' : 'Enable'}
											</button>
											<button
												class="text-xs text-red-400 hover:text-red-300"
												onclick={() => handleDeleteRule(rule.id)}
											>
												Delete
											</button>
										</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				{:else}
					<!-- Scan Log Sub-tab -->
					<div class="mb-4 flex items-center justify-between">
						<p class="text-sm text-text-muted">Recent content scan matches from all rules.</p>
						<button class="btn-secondary text-sm" onclick={loadContentScanLog} disabled={loadingContentLog}>
							{loadingContentLog ? 'Loading...' : 'Refresh'}
						</button>
					</div>

					{#if loadingContentLog}
						<p class="text-sm text-text-muted">Loading scan log...</p>
					{:else if contentScanLog.length === 0}
						<div class="rounded-lg bg-bg-secondary p-6 text-center">
							<p class="text-sm text-text-muted">No content scan matches recorded.</p>
							<p class="mt-1 text-xs text-text-muted">Matches will appear here when content triggers a scan rule.</p>
						</div>
					{:else}
						<div class="overflow-hidden rounded-lg border border-bg-modifier">
							<table class="w-full text-left text-sm">
								<thead class="bg-bg-secondary">
									<tr>
										<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Rule</th>
										<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">User</th>
										<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Matched</th>
										<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Action</th>
										<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Time</th>
									</tr>
								</thead>
								<tbody class="divide-y divide-bg-modifier">
									{#each contentScanLog as entry (entry.id)}
										<tr class="hover:bg-bg-secondary/50">
											<td class="px-4 py-3 text-text-primary text-xs">{entry.rule_name}</td>
											<td class="px-4 py-3 text-text-secondary text-xs">@{entry.username}</td>
											<td class="px-4 py-3">
												<code class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">{entry.content_matched.length > 50 ? entry.content_matched.slice(0, 50) + '...' : entry.content_matched}</code>
											</td>
											<td class="px-4 py-3">
												<span class="rounded px-1.5 py-0.5 text-2xs font-bold {actionClasses(entry.action_taken)}">
													{entry.action_taken}
												</span>
											</td>
											<td class="px-4 py-3 text-text-muted text-xs">{new Date(entry.created_at).toLocaleString()}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				{/if}

			<!-- ==================== CAPTCHA ==================== -->
			{:else if currentTab === 'captcha'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">CAPTCHA Settings</h1>
				<p class="mb-6 text-sm text-text-muted">
					Configure CAPTCHA verification for user registration. When enabled, new users must complete a CAPTCHA challenge before creating an account.
				</p>

				{#if loadingCaptcha}
					<p class="text-sm text-text-muted">Loading CAPTCHA settings...</p>
				{:else}
					<div class="space-y-6">
						<!-- Provider Selection -->
						<div class="rounded-lg bg-bg-secondary p-4">
							<label class="mb-3 block text-xs font-bold uppercase tracking-wide text-text-muted">CAPTCHA Provider</label>
							<div class="space-y-2">
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'none' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={captchaProvider} value="none" class="accent-brand-500" />
									<div>
										<span class="font-medium">Disabled</span>
										<p class="text-xs text-text-muted">No CAPTCHA verification on registration.</p>
									</div>
								</label>
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'hcaptcha' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={captchaProvider} value="hcaptcha" class="accent-brand-500" />
									<div>
										<span class="font-medium">hCaptcha</span>
										<p class="text-xs text-text-muted">Privacy-focused CAPTCHA. Requires an hCaptcha account.</p>
									</div>
								</label>
								<label class="flex items-center gap-3 rounded p-2 text-sm transition-colors hover:bg-bg-modifier {captchaProvider === 'recaptcha' ? 'text-text-primary' : 'text-text-secondary'}">
									<input type="radio" bind:group={captchaProvider} value="recaptcha" class="accent-brand-500" />
									<div>
										<span class="font-medium">reCAPTCHA</span>
										<p class="text-xs text-text-muted">Google reCAPTCHA v2/v3. Requires a Google reCAPTCHA account.</p>
									</div>
								</label>
							</div>
						</div>

						<!-- API Keys (shown only when a provider is selected) -->
						{#if captchaProvider !== 'none'}
							<div class="space-y-4">
								<div>
									<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Site Key</label>
									<input
										type="text"
										class="input w-full font-mono text-sm"
										bind:value={captchaSiteKey}
										placeholder="Enter your site key..."
									/>
									<p class="mt-1 text-xs text-text-muted">
										The public site key for the CAPTCHA widget. This is embedded in the registration page HTML.
									</p>
								</div>
								<div>
									<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Secret Key</label>
									<input
										type="password"
										class="input w-full font-mono text-sm"
										bind:value={captchaSecretKey}
										placeholder={captchaConfig?.secret_key ? captchaConfig.secret_key : 'Enter your secret key...'}
									/>
									<p class="mt-1 text-xs text-text-muted">
										The private secret key for server-side verification. Leave blank to keep the existing key.
									</p>
								</div>
							</div>
						{/if}

						{#if captchaConfig}
							<div class="rounded-lg bg-bg-secondary p-4">
								<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Current Configuration</h3>
								<div class="grid gap-2 text-sm">
									<div class="flex justify-between">
										<span class="text-text-muted">Provider</span>
										<span class="text-text-primary">{captchaConfig.provider === 'none' ? 'Disabled' : captchaConfig.provider}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-text-muted">Site Key</span>
										<span class="text-text-primary">{captchaConfig.site_key ? captchaConfig.site_key.slice(0, 20) + '...' : 'Not set'}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-text-muted">Secret Key</span>
										<span class="text-text-primary">{captchaConfig.secret_key || 'Not set'}</span>
									</div>
								</div>
							</div>
						{/if}

						<button class="btn-primary" onclick={saveCaptchaConfig} disabled={savingCaptcha}>
							{savingCaptcha ? 'Saving...' : 'Save CAPTCHA Settings'}
						</button>
					</div>
				{/if}

			<!-- ==================== INSTANCE ==================== -->
			{:else if currentTab === 'instance'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">Instance Settings</h1>

				{#if loadingInstance}
					<p class="text-sm text-text-muted">Loading instance settings...</p>
				{:else}
					<div class="space-y-4">
						<div>
							<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Instance Name</label>
							<input type="text" class="input w-full" bind:value={instanceName} maxlength="100" />
						</div>
						<div>
							<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
							<textarea class="input w-full" bind:value={instanceDesc} rows="3" maxlength="1024"></textarea>
						</div>
						<div>
							<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Federation Mode</label>
							<select class="input w-full" bind:value={instanceFedMode}>
								<option value="open">Open (federate with all peers)</option>
								<option value="allowlist">Allowlist (federate only with approved peers)</option>
								<option value="closed">Closed (no federation)</option>
							</select>
						</div>

						{#if instance}
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="grid gap-2 text-sm">
									<div class="flex justify-between">
										<span class="text-text-muted">Instance ID</span>
										<code class="text-xs text-text-primary">{instance.id}</code>
									</div>
									<div class="flex justify-between">
										<span class="text-text-muted">Domain</span>
										<span class="text-text-primary">{instance.domain}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-text-muted">Software</span>
										<span class="text-text-primary">{instance.software} {instance.software_version}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-text-muted">Created</span>
										<span class="text-text-primary">{new Date(instance.created_at).toLocaleDateString()}</span>
									</div>
								</div>
							</div>
						{/if}

						<button class="btn-primary" onclick={saveInstance} disabled={savingInstance}>
							{savingInstance ? 'Saving...' : 'Save Settings'}
						</button>
					</div>
				{/if}

			<!-- ==================== FEDERATION ==================== -->
			{:else if currentTab === 'federation'}
				<h1 class="mb-6 text-2xl font-bold text-text-primary">Federation Peers</h1>

				<div class="mb-6 flex gap-2">
					<input
						type="text" class="input flex-1" placeholder="Enter domain (e.g., chat.example.com)..."
						bind:value={newPeerDomain}
						onkeydown={(e) => e.key === 'Enter' && handleAddPeer()}
					/>
					<button class="btn-primary" onclick={handleAddPeer} disabled={addingPeer || !newPeerDomain.trim()}>
						{addingPeer ? 'Adding...' : 'Add Peer'}
					</button>
				</div>

				{#if loadingPeers}
					<p class="text-sm text-text-muted">Loading peers...</p>
				{:else if peers.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">No federation peers configured.</p>
						<p class="mt-1 text-xs text-text-muted">Add a peer domain above to begin federating.</p>
					</div>
				{:else}
					<div class="space-y-2">
						{#each peers as peer (peer.id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div>
									<div class="flex items-center gap-2">
										<span class="text-sm font-medium text-text-primary">{peer.domain}</span>
										<span class="rounded px-1.5 py-0.5 text-2xs font-bold {peer.status === 'active' ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'}">
											{peer.status}
										</span>
									</div>
									<p class="text-xs text-text-muted">
										{peer.software ?? 'Unknown'} {peer.software_version ?? ''} &middot;
										{peer.last_seen_at ? `Last seen ${new Date(peer.last_seen_at).toLocaleString()}` : 'Never seen'}
									</p>
								</div>
								<button
									class="text-xs text-red-400 hover:text-red-300"
									onclick={() => handleRemovePeer(peer.id)}
								>
									Remove
								</button>
							</div>
						{/each}
					</div>
				{/if}
			<!-- ==================== HEALTH ==================== -->
			{:else if currentTab === 'health'}
				<HealthMonitor />

			<!-- ==================== STORAGE ==================== -->
			{:else if currentTab === 'storage'}
				<StorageDashboard />

			<!-- ==================== BACKUPS ==================== -->
			{:else if currentTab === 'backups'}
				<BackupScheduler />

			<!-- ==================== DOMAINS ==================== -->
			{:else if currentTab === 'domains'}
				<DomainSettings />

			<!-- ==================== RETENTION ==================== -->
			{:else if currentTab === 'retention'}
				<RetentionSettings />

			<!-- ==================== UPDATES ==================== -->
			{:else if currentTab === 'updates'}
				<UpdateNotifications />
			{/if}
		</div>
	</div>
</div>
