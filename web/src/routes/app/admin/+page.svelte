<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import Avatar from '$components/common/Avatar.svelte';
	import type { AdminStats, InstanceInfo, User, FederationPeer } from '$lib/types';

	type Tab = 'dashboard' | 'users' | 'instance' | 'federation';
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
		if (currentTab === 'instance' && !instance) loadInstance();
		if (currentTab === 'federation' && peers.length === 0) loadPeers();
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

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'dashboard', label: 'Dashboard' },
		{ id: 'users', label: 'Users' },
		{ id: 'instance', label: 'Instance' },
		{ id: 'federation', label: 'Federation' }
	];
</script>

<svelte:head>
	<title>Admin — AmityVox</title>
</svelte:head>

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
