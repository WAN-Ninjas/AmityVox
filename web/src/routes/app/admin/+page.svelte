<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { AdminStats } from '$lib/types';

	let stats = $state<AdminStats | null>(null);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			stats = await api.getAdminStats();
		} catch (err: any) {
			error = err.message || 'Failed to load stats. You may not have admin access.';
		} finally {
			loading = false;
		}
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
</script>

<svelte:head>
	<title>Admin — AmityVox</title>
</svelte:head>

<div class="p-8">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold text-text-primary">Instance Dashboard</h1>
		<button class="btn-secondary text-sm" onclick={refresh} disabled={loading}>
			{loading ? 'Loading...' : 'Refresh'}
		</button>
	</div>

	{#if error}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}

	{#if loading && !stats}
		<p class="text-text-muted">Loading stats...</p>
	{:else if stats}
		<!-- Key Metrics -->
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

		<!-- Additional Counts -->
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

		<!-- System Info -->
		<div class="mt-8">
			<h2 class="mb-4 text-lg font-semibold text-text-primary">System Info</h2>
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="grid gap-3 text-sm">
					<div class="flex justify-between">
						<span class="text-text-muted">Software</span>
						<span class="text-text-primary">AmityVox v0.3.0</span>
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
</div>
