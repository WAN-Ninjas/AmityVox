<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';
	import type {
		AdminStats, InstanceInfo, User, FederationPeer,
		InstanceBan, RegistrationSettings, RegistrationToken,
		Announcement, AnnouncementSeverity
	} from '$lib/types';

	type Tab = 'dashboard' | 'users' | 'bans' | 'registration' | 'announcements' | 'instance' | 'federation';
	let currentTab = $state<Tab>('dashboard');

	// --- Dashboard ---
	let stats = $state<AdminStats | null>(null);
	let loading = $state(true);
	let error = $state('');

	// --- Users ---
	let users = $state<User[]>([]);
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

	// --- Announcements ---
	let announcements = $state<Announcement[]>([]);
	let loadingAnnouncements = $state(false);
	let createAnnouncementModalOpen = $state(false);
	let editAnnouncementModalOpen = $state(false);
	let editingAnnouncement = $state<Announcement | null>(null);
	let announcementTitle = $state('');
	let announcementContent = $state('');
	let announcementSeverity = $state<AnnouncementSeverity>('info');
	let announcementExpiryHours = $state(0);
	let savingAnnouncement = $state(false);

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
		if (currentTab === 'users' && users.length === 0) loadUsers();
		if (currentTab === 'bans' && instanceBans.length === 0) loadInstanceBans();
		if (currentTab === 'instance' && !instance) loadInstance();
		if (currentTab === 'federation' && peers.length === 0) loadPeers();
		if (currentTab === 'registration' && !regSettings) loadRegistration();
		if (currentTab === 'announcements' && announcements.length === 0) loadAnnouncements();
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
		} catch { users = []; }
		finally { loadingUsers = false; }
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
		try { instanceBans = await api.getInstanceBans(); } catch { instanceBans = []; }
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
			instanceFedMode = instance?.federation_mode ?? 'allow';
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
		try { peers = await api.getFederationPeers(); } catch { peers = []; }
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
		try { announcements = await api.getAdminAnnouncements(); } catch { announcements = []; }
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

	function severityLabel(s: AnnouncementSeverity): string {
		return s.charAt(0).toUpperCase() + s.slice(1);
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
		{ id: 'bans', label: 'Instance Bans' },
		{ id: 'registration', label: 'Registration' },
		{ id: 'announcements', label: 'Announcements' },
		{ id: 'instance', label: 'Instance' },
		{ id: 'federation', label: 'Federation' }
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

<div class="flex h-full">
	<!-- Sidebar -->
	<nav class="w-48 shrink-0 overflow-y-auto bg-bg-secondary p-4">
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
									<span class="text-text-primary">AmityVox v0.4.0</span>
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
								</div>
							</div>
						{/each}
					</div>
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
								<option value="allow">Allow (federate with all peers)</option>
								<option value="allowlist">Allowlist (federate only with approved peers)</option>
								<option value="deny">Deny (no federation)</option>
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
			{/if}
		</div>
	</div>
</div>
