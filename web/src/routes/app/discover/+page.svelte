<script lang="ts">
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import type { Guild } from '$lib/types';

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
		selectedCategory; // track dependency
		loadGuilds();
	});

	let searchTimeout: ReturnType<typeof setTimeout>;
	function onSearchInput() {
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => loadGuilds(), 300);
	}

	const filteredGuilds = $derived(() => guilds);

	async function joinGuild(guild: Guild) {
		joining = guild.id;
		try {
			// Try to join via invite or direct join for public guilds
			await api.joinGuild(guild.id);
			goto(`/app/guilds/${guild.id}`);
		} catch (err: any) {
			error = err.message || 'Failed to join server';
		} finally {
			joining = null;
		}
	}

	function getInitials(name: string): string {
		return name
			.split(' ')
			.map((w) => w[0])
			.join('')
			.slice(0, 3)
			.toUpperCase();
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

	<!-- Search & Filters -->
	<div class="border-b border-bg-modifier px-6 py-4">
		<div class="mx-auto max-w-4xl">
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

	<!-- Server Grid -->
	<div class="flex-1 overflow-y-auto p-6">
		<div class="mx-auto max-w-4xl">
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
							<!-- Banner -->
							<div class="h-28 w-full bg-gradient-to-br from-brand-600/40 via-brand-500/20 to-bg-tertiary">
								{#if guild.banner_id}
									<img
										src="/api/v1/files/{guild.banner_id}"
										alt=""
										class="h-full w-full object-cover"
									/>
								{/if}
							</div>

							<!-- Info -->
							<div class="flex flex-1 flex-col p-4">
								<div class="-mt-10 mb-3 flex items-end gap-3">
									<div class="shrink-0 rounded-xl bg-bg-secondary p-1 shadow-md">
										{#if guild.icon_id}
											<img
												src="/api/v1/files/{guild.icon_id}"
												alt={guild.name}
												class="h-12 w-12 rounded-lg object-cover"
											/>
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
												<path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" />
												<circle cx="9" cy="7" r="4" />
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
		</div>
	</div>
</div>
