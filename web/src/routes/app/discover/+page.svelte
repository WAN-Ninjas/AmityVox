<script lang="ts">
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { loadGuilds as reloadGuilds } from '$lib/stores/guilds';
	import { addToast } from '$lib/stores/toast';
	import FederationBadge from '$lib/components/common/FederationBadge.svelte';
	import type { Guild, FederationPeer } from '$lib/types';

	type DiscoverTab = 'local' | 'federated' | 'instances';

	let activeTab = $state<DiscoverTab>('local');
	let guilds = $state<Guild[]>([]);
	let loading = $state(true);
	let error = $state('');
	let search = $state('');
	let joining = $state<string | null>(null);

	const categories = [
		'All', 'Gaming', 'Music', 'Education', 'Science & Tech',
		'Entertainment', 'Art & Creative', 'Community', 'Other'
	];
	let selectedCategory = $state('All');

	// Federation state
	let peers = $state<FederationPeer[]>([]);
	let peersLoading = $state(false);
	let selectedPeerId = $state<string>('__all__');
	let remoteGuilds = $state<(Guild & { instance_domain?: string })[]>([]);
	let remoteLoading = $state(false);
	let remoteError = $state('');

	async function loadGuilds() {
		loading = true;
		error = '';
		try {
			const params: Record<string, string> = { limit: '100' };
			if (search.trim()) params.q = search.trim();
			if (selectedCategory !== 'All') params.tag = selectedCategory;
			guilds = await api.discoverGuilds(params);
		} catch (err: any) {
			error = err.message || 'Failed to load servers';
		} finally {
			loading = false;
		}
	}

	// Load on mount and re-fetch when category changes.
	$effect(() => {
		if (activeTab === 'local') {
			selectedCategory; // track dependency
			loadGuilds();
		}
	});

	let searchTimeout: ReturnType<typeof setTimeout>;
	function onSearchInput() {
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			if (activeTab === 'local') loadGuilds();
			else if (activeTab === 'federated') loadRemoteGuilds();
		}, 300);
	}

	const filteredGuilds = $derived(() => guilds);

	async function joinGuild(guild: Guild) {
		joining = guild.id;
		try {
			await api.joinGuild(guild.id);
			await reloadGuilds();
			goto(`/app/guilds/${guild.id}`);
		} catch (err: any) {
			error = err.message || 'Failed to join server';
		} finally {
			joining = null;
		}
	}

	async function joinRemoteGuild(guild: Guild & { instance_domain?: string }) {
		if (!guild.instance_domain) return;
		joining = guild.id;
		try {
			await api.joinFederatedGuild(guild.instance_domain, guild.id);
			await reloadGuilds();
			addToast(`Joined ${guild.name} on ${guild.instance_domain}!`, 'success');
		} catch (err: any) {
			remoteError = err.message || 'Failed to join federated server';
		} finally {
			joining = null;
		}
	}

	async function loadPeers() {
		peersLoading = true;
		try {
			peers = await api.getPublicFederationPeers();
		} catch {
			peers = [];
		} finally {
			peersLoading = false;
		}
	}

	async function loadRemoteGuilds() {
		if (!selectedPeerId) return;
		remoteLoading = true;
		remoteError = '';
		try {
			const params: Record<string, string> = { limit: '50' };
			if (search.trim()) params.q = search.trim();
			if (selectedCategory !== 'All') params.tag = selectedCategory;

			if (selectedPeerId === '__all__') {
				// Query all peers in parallel
				const results = await Promise.allSettled(
					peers.map(peer => api.discoverRemoteGuilds(peer.id, params))
				);
				const allGuilds: (Guild & { instance_domain?: string })[] = [];
				for (const result of results) {
					if (result.status === 'fulfilled' && Array.isArray(result.value)) {
						allGuilds.push(...(result.value as any));
					}
				}
				remoteGuilds = allGuilds;
			} else {
				remoteGuilds = await api.discoverRemoteGuilds(selectedPeerId, params) as any;
			}
		} catch (err: any) {
			remoteError = err.message || 'Failed to load remote servers';
		} finally {
			remoteLoading = false;
		}
	}

	$effect(() => {
		if (activeTab === 'federated' || activeTab === 'instances') {
			if (peers.length === 0 && !peersLoading) loadPeers();
		}
	});

	$effect(() => {
		if (activeTab === 'federated' && selectedPeerId && peers.length > 0) {
			selectedCategory; // track category changes for federated filters
			loadRemoteGuilds();
		}
	});

	function selectPeerAndDiscover(peerId: string) {
		selectedPeerId = peerId;
		activeTab = 'federated';
	}

	function getInitials(name: string): string {
		return name
			.split(' ')
			.map((w) => w[0])
			.join('')
			.slice(0, 3)
			.toUpperCase();
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}
</script>

<svelte:head>
	<title>Discover Servers — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col">
	<div class="flex h-12 items-center border-b border-bg-modifier px-4">
		<svg class="mr-2 h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
		</svg>
		<h1 class="text-base font-semibold text-text-primary">Discover Servers</h1>
	</div>

	<!-- Tabs -->
	<div class="flex border-b border-bg-modifier px-6">
		<button
			class="px-4 py-2.5 text-sm font-medium transition-colors {activeTab === 'local' ? 'border-b-2 border-brand-500 text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (activeTab = 'local')}
		>
			Local Servers
		</button>
		<button
			class="px-4 py-2.5 text-sm font-medium transition-colors {activeTab === 'federated' ? 'border-b-2 border-brand-500 text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (activeTab = 'federated')}
		>
			<span class="flex items-center gap-1.5">
				<svg class="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
					<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
				</svg>
				Federated Servers
			</span>
		</button>
		<button
			class="px-4 py-2.5 text-sm font-medium transition-colors {activeTab === 'instances' ? 'border-b-2 border-brand-500 text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => (activeTab = 'instances')}
		>
			Federated Instances
		</button>
	</div>

	<!-- Search & Filters (Local + Federated tabs) -->
	{#if activeTab !== 'instances'}
		<div class="border-b border-bg-modifier px-6 py-4">
			<div class="mx-auto max-w-4xl">
				{#if activeTab === 'federated'}
					<!-- Peer selector -->
					<div class="mb-3">
						<label class="mb-1 block text-xs font-medium text-text-muted">Select Instance</label>
						<select
							class="input w-full"
							onchange={(e) => (selectedPeerId = (e.target as HTMLSelectElement).value)}
							value={selectedPeerId}
						>
							<option value="__all__">All Federated Servers</option>
							{#each peers as peer}
								<option value={peer.id}>{peer.domain}{peer.name ? ` (${peer.name})` : ''}</option>
							{/each}
						</select>
					</div>
				{/if}
				<div class="relative mb-4">
					<svg class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
					</svg>
					<input
						type="text"
						bind:value={search}
						oninput={onSearchInput}
						placeholder="Search for servers..."
						class="input w-full pl-10"
					/>
				</div>
				<div class="flex flex-wrap gap-2">
					{#each categories as cat}
						<button
							class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
							class:bg-brand-500={selectedCategory === cat}
							class:text-white={selectedCategory === cat}
							class:bg-bg-secondary={selectedCategory !== cat}
							class:text-text-muted={selectedCategory !== cat}
							class:hover:bg-bg-modifier={selectedCategory !== cat}
							onclick={() => (selectedCategory = cat)}
						>
							{cat}
						</button>
					{/each}
				</div>
			</div>
		</div>
	{/if}

	<!-- Content -->
	<div class="flex-1 overflow-y-auto p-6">
		<div class="mx-auto max-w-4xl">
			{#if activeTab === 'local'}
				<!-- LOCAL SERVERS TAB -->
				{#if loading}
					<div class="flex items-center justify-center py-20">
						<svg class="h-6 w-6 animate-spin text-text-muted" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
						</svg>
					</div>
				{:else if error}
					<p class="py-10 text-center text-sm text-red-400">{error}</p>
				{:else if filteredGuilds().length === 0}
					<div class="flex flex-col items-center justify-center py-20 text-center">
						<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
							<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
						</svg>
						<h2 class="mb-2 text-lg font-semibold text-text-primary">No servers found</h2>
						<p class="text-sm text-text-muted">
							{search ? 'Try a different search term.' : 'No public servers are available yet.'}
						</p>
					</div>
				{:else}
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each filteredGuilds() as guild (guild.id)}
							<div class="group flex flex-col overflow-hidden rounded-lg bg-bg-secondary transition-shadow hover:shadow-lg">
								<div class="h-28 w-full bg-gradient-to-br from-brand-600/40 via-brand-500/20 to-bg-tertiary">
									{#if guild.banner_id}
										<img src="/api/v1/files/{guild.banner_id}" alt="" class="h-full w-full object-cover" />
									{/if}
								</div>
								<div class="flex flex-1 flex-col p-4">
									<div class="-mt-10 mb-3 flex items-end gap-3">
										<div class="shrink-0 rounded-xl bg-bg-secondary p-1 shadow-md">
											{#if guild.icon_id}
												<img src="/api/v1/files/{guild.icon_id}" alt={guild.name} class="h-12 w-12 rounded-lg object-cover" />
											{:else}
												<div class="flex h-12 w-12 items-center justify-center rounded-lg bg-brand-600 text-sm font-bold text-white">
													{getInitials(guild.name)}
												</div>
											{/if}
										</div>
										<h3 class="truncate text-sm font-bold text-text-primary">{guild.name}</h3>
									</div>
									{#if guild.description}
										<p class="mb-2 line-clamp-2 text-xs text-text-muted">{guild.description}</p>
									{:else}
										<p class="mb-2 text-xs text-text-muted italic">No description</p>
									{/if}
									{#if guild.tags?.length}
										<div class="mb-2 flex flex-wrap gap-1">
											{#each guild.tags.slice(0, 3) as tag}
												<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-[10px] text-text-muted">{tag}</span>
											{/each}
										</div>
									{/if}
									<div class="mt-auto flex items-center justify-between">
										<div class="flex items-center gap-3 text-xs text-text-muted">
											<span class="flex items-center gap-1">
												<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
													<path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" /><circle cx="9" cy="7" r="4" />
												</svg>
												{guild.member_count ?? 0}
											</span>
										</div>
										<button
											class="rounded-md bg-brand-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
											onclick={() => joinGuild(guild)}
											disabled={joining === guild.id}
										>
											{joining === guild.id ? 'Joining...' : 'Join'}
										</button>
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			{:else if activeTab === 'federated'}
				<!-- FEDERATED SERVERS TAB -->
				{#if remoteLoading}
					<div class="flex items-center justify-center py-20">
						<svg class="h-6 w-6 animate-spin text-text-muted" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
						</svg>
					</div>
				{:else if remoteError}
					<p class="py-10 text-center text-sm text-red-400">{remoteError}</p>
				{:else if remoteGuilds.length === 0}
					<div class="flex flex-col items-center justify-center py-20 text-center">
						<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
							<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
						</svg>
						<h2 class="mb-2 text-lg font-semibold text-text-primary">No servers found</h2>
						<p class="text-sm text-text-muted">This instance has no public discoverable servers.</p>
					</div>
				{:else}
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each remoteGuilds as guild (guild.id)}
							<div class="group flex flex-col overflow-hidden rounded-lg bg-bg-secondary transition-shadow hover:shadow-lg">
								<div class="h-28 w-full bg-gradient-to-br from-blue-600/30 via-blue-500/15 to-bg-tertiary">
									{#if guild.banner_id}
										<img src="/api/v1/files/{guild.banner_id}" alt="" class="h-full w-full object-cover" />
									{/if}
								</div>
								<div class="flex flex-1 flex-col p-4">
									<div class="-mt-10 mb-3 flex items-end gap-3">
										<div class="shrink-0 rounded-xl bg-bg-secondary p-1 shadow-md">
											{#if guild.icon_id}
												<img src="/api/v1/files/{guild.icon_id}" alt={guild.name} class="h-12 w-12 rounded-lg object-cover" />
											{:else}
												<div class="flex h-12 w-12 items-center justify-center rounded-lg bg-blue-600 text-sm font-bold text-white">
													{getInitials(guild.name)}
												</div>
											{/if}
										</div>
										<div class="min-w-0 flex-1">
											<h3 class="truncate text-sm font-bold text-text-primary">{guild.name}</h3>
											{#if guild.instance_domain}
												<FederationBadge domain={guild.instance_domain} />
											{/if}
										</div>
									</div>
									{#if guild.description}
										<p class="mb-2 line-clamp-2 text-xs text-text-muted">{guild.description}</p>
									{:else}
										<p class="mb-2 text-xs text-text-muted italic">No description</p>
									{/if}
									{#if guild.tags?.length}
										<div class="mb-2 flex flex-wrap gap-1">
											{#each guild.tags.slice(0, 3) as tag}
												<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-[10px] text-text-muted">{tag}</span>
											{/each}
										</div>
									{/if}
									<div class="mt-auto flex items-center justify-between">
										<div class="flex items-center gap-3 text-xs text-text-muted">
											<span class="flex items-center gap-1">
												<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
													<path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" /><circle cx="9" cy="7" r="4" />
												</svg>
												{guild.member_count ?? 0}
											</span>
										</div>
										<button
											class="rounded-md bg-blue-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-600 disabled:opacity-50"
											onclick={() => joinRemoteGuild(guild)}
											disabled={joining === guild.id}
										>
											{joining === guild.id ? 'Joining...' : 'Join'}
										</button>
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			{:else}
				<!-- FEDERATED INSTANCES TAB -->
				{#if peersLoading}
					<div class="flex items-center justify-center py-20">
						<svg class="h-6 w-6 animate-spin text-text-muted" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
						</svg>
					</div>
				{:else if peers.length === 0}
					<div class="flex flex-col items-center justify-center py-20 text-center">
						<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" viewBox="0 0 16 16" fill="currentColor">
							<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
						</svg>
						<h2 class="mb-2 text-lg font-semibold text-text-primary">No Federated Instances</h2>
						<p class="text-sm text-text-muted">This instance is not federated with any other instances yet.</p>
					</div>
				{:else}
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#each peers as peer (peer.id)}
							<button
								class="flex flex-col overflow-hidden rounded-lg bg-bg-secondary p-5 text-left transition-all hover:shadow-lg hover:ring-1 hover:ring-blue-500/30"
								onclick={() => selectPeerAndDiscover(peer.id)}
							>
								<div class="mb-3 flex items-center gap-3">
									<div class="flex h-10 w-10 items-center justify-center rounded-full bg-blue-500/15">
										<svg class="h-5 w-5 text-blue-400" viewBox="0 0 16 16" fill="currentColor">
											<path d="M8 0a8 8 0 100 16A8 8 0 008 0zm5.3 5H11a13 13 0 00-1-3.3A6 6 0 0113.3 5zM8 1.5c.7.8 1.3 2 1.7 3.5H6.3C6.7 3.5 7.3 2.3 8 1.5zM1.5 9a6.5 6.5 0 010-2h2.8a13 13 0 000 2H1.5zm.2 1h2.5a13 13 0 001 3.3A6 6 0 011.7 10zm2.5-5H1.7A6 6 0 016 1.7 13 13 0 004.2 5zM8 14.5c-.7-.8-1.3-2-1.7-3.5h3.4c-.4 1.5-1 2.7-1.7 3.5zm2-4.5H6a12 12 0 010-4h4a12 12 0 010 4zm.1 3.3a13 13 0 001-3.3h2.5a6 6 0 01-3.5 3.3zM11.7 9a13 13 0 000-2h2.8a6.5 6.5 0 010 2h-2.8z"/>
										</svg>
									</div>
									<div class="min-w-0 flex-1">
										<h3 class="truncate text-sm font-bold text-text-primary">{peer.domain}</h3>
										{#if peer.name}
											<p class="truncate text-xs text-text-muted">{peer.name}</p>
										{/if}
									</div>
								</div>
								<div class="flex items-center gap-3 text-xs text-text-muted">
									<span class="flex items-center gap-1">
										<span class="h-2 w-2 rounded-full bg-green-500"></span>
										{peer.status}
									</span>
									<span>Since {formatDate(peer.established_at)}</span>
								</div>
								<p class="mt-3 text-xs text-blue-400">Browse servers &rarr;</p>
							</button>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>
