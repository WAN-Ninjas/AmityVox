<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import HealthMonitor from '$lib/components/admin/HealthMonitor.svelte';
	import StorageDashboard from '$lib/components/admin/StorageDashboard.svelte';
	import BackupScheduler from '$lib/components/admin/BackupScheduler.svelte';
	import DomainSettings from '$lib/components/admin/DomainSettings.svelte';
	import RetentionSettings from '$lib/components/admin/RetentionSettings.svelte';
	import UpdateNotifications from '$lib/components/admin/UpdateNotifications.svelte';
	import AdminCaptchaTab from '$lib/components/admin/AdminCaptchaTab.svelte';
	import AdminInstanceTab from '$lib/components/admin/AdminInstanceTab.svelte';
	import AdminFederationPeersTab from '$lib/components/admin/AdminFederationPeersTab.svelte';
	import AdminAnnouncementsTab from '$lib/components/admin/AdminAnnouncementsTab.svelte';
	import AdminRateLimitsTab from '$lib/components/admin/AdminRateLimitsTab.svelte';
	import AdminContentSafetyTab from '$lib/components/admin/AdminContentSafetyTab.svelte';
	import AdminRegistrationTab from '$lib/components/admin/AdminRegistrationTab.svelte';
	import AdminBotsTab from '$lib/components/admin/AdminBotsTab.svelte';
	import AdminUsersTab from '$lib/components/admin/AdminUsersTab.svelte';
	import AdminGuildsTab from '$lib/components/admin/AdminGuildsTab.svelte';
	import AdminInstanceBansTab from '$lib/components/admin/AdminInstanceBansTab.svelte';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { AdminStats } from '$lib/types';

	type Tab = 'dashboard' | 'users' | 'guilds' | 'bots' | 'bans' | 'registration' | 'announcements' | 'instance' | 'federation' | 'rate_limits' | 'content_safety' | 'captcha' | 'health' | 'storage' | 'backups' | 'domains' | 'retention' | 'updates';
	let currentTab = $state<Tab>('dashboard');

	// --- Dashboard ---
	let stats = $state<AdminStats | null>(null);
	let statsOp = $state(createAsyncOp(true));

	async function loadStats(fallback = 'Failed to load stats. You may not have admin access.') {
		const result = await statsOp.run(() => api.getAdminStats(), undefined, fallback);
		if (result) {
			stats = result;
		}
	}

	onMount(() => {
		loadStats();
	});

	async function refresh() {
		await loadStats('Failed to refresh stats');
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'dashboard', label: 'Dashboard' },
		{ id: 'users', label: 'Users' },
		{ id: 'guilds', label: 'Servers' },
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

<div class="flex h-full">
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
				Back to App
			</button>
		</div>
	</nav>

	<div class="flex-1 overflow-y-auto p-6">
		{#if currentTab === 'dashboard'}
			<div class="mb-6 flex items-center justify-between">
				<h1 class="text-2xl font-bold text-text-primary">Admin Dashboard</h1>
				<button class="btn-secondary text-sm" onclick={refresh} disabled={statsOp.loading}>
					{statsOp.loading ? 'Refreshing...' : 'Refresh'}
				</button>
			</div>

			{#if statsOp.error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{statsOp.error}</div>
			{/if}

			{#if statsOp.loading}
				<p class="text-sm text-text-muted">Loading statistics...</p>
			{:else if stats}
				<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
					<div class="rounded-lg bg-bg-secondary p-4">
						<div class="text-2xl font-bold text-text-primary">{stats.users.toLocaleString()}</div>
						<div class="text-sm text-text-muted">Users</div>
					</div>
					<div class="rounded-lg bg-bg-secondary p-4">
						<div class="text-2xl font-bold text-green-400">{stats.online_users.toLocaleString()}</div>
						<div class="text-sm text-text-muted">Online</div>
					</div>
					<div class="rounded-lg bg-bg-secondary p-4">
						<div class="text-2xl font-bold text-text-primary">{stats.guilds.toLocaleString()}</div>
						<div class="text-sm text-text-muted">Servers</div>
					</div>
					<div class="rounded-lg bg-bg-secondary p-4">
						<div class="text-2xl font-bold text-text-primary">{stats.messages.toLocaleString()}</div>
						<div class="text-sm text-text-muted">Messages</div>
					</div>
				</div>
				<div class="mt-6 grid gap-4 md:grid-cols-3">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-3 text-sm font-semibold text-text-primary">Content</h3>
						<div class="space-y-2 text-sm">
							<div class="flex justify-between"><span class="text-text-muted">Channels</span><span>{stats.channels.toLocaleString()}</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Files</span><span>{stats.files.toLocaleString()}</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Emoji</span><span>{stats.emoji.toLocaleString()}</span></div>
						</div>
					</div>
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-3 text-sm font-semibold text-text-primary">Federation</h3>
						<div class="space-y-2 text-sm">
							<div class="flex justify-between"><span class="text-text-muted">Peers</span><span>{stats.federation_peers.toLocaleString()}</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Invites</span><span>{stats.invites.toLocaleString()}</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Roles</span><span>{stats.roles.toLocaleString()}</span></div>
						</div>
					</div>
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-3 text-sm font-semibold text-text-primary">Runtime</h3>
						<div class="space-y-2 text-sm">
							<div class="flex justify-between"><span class="text-text-muted">Uptime</span><span>{stats.uptime}</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Memory</span><span>{stats.mem_alloc_mb} MB</span></div>
							<div class="flex justify-between"><span class="text-text-muted">Goroutines</span><span>{stats.goroutines.toLocaleString()}</span></div>
						</div>
					</div>
				</div>
			{/if}
		{:else if currentTab === 'users'}
			<AdminUsersTab />
		{:else if currentTab === 'guilds'}
			<AdminGuildsTab />
		{:else if currentTab === 'bots'}
			<AdminBotsTab />
		{:else if currentTab === 'bans'}
			<AdminInstanceBansTab />
		{:else if currentTab === 'registration'}
			<AdminRegistrationTab />
		{:else if currentTab === 'announcements'}
			<AdminAnnouncementsTab />
		{:else if currentTab === 'rate_limits'}
			<AdminRateLimitsTab />
		{:else if currentTab === 'content_safety'}
			<AdminContentSafetyTab />
		{:else if currentTab === 'captcha'}
			<AdminCaptchaTab />
		{:else if currentTab === 'instance'}
			<AdminInstanceTab />
		{:else if currentTab === 'federation'}
			<AdminFederationPeersTab />
		{:else if currentTab === 'health'}
			<HealthMonitor />
		{:else if currentTab === 'storage'}
			<StorageDashboard />
		{:else if currentTab === 'backups'}
			<BackupScheduler />
		{:else if currentTab === 'domains'}
			<DomainSettings />
		{:else if currentTab === 'retention'}
			<RetentionSettings />
		{:else if currentTab === 'updates'}
			<UpdateNotifications />
		{/if}
	</div>
</div>
