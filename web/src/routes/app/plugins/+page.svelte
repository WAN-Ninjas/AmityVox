<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { currentGuildId } from '$lib/stores/guilds';

	const API_BASE = '/api/v1';

	async function apiRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
		const headers: Record<string, string> = { 'Content-Type': 'application/json' };
		const token = api.getToken();
		if (token) headers['Authorization'] = `Bearer ${token}`;
		const res = await fetch(`${API_BASE}${path}`, {
			method,
			headers,
			body: body ? JSON.stringify(body) : undefined
		});
		if (res.status === 204) return undefined as T;
		const json = await res.json();
		if (!res.ok) {
			const err = json as { error?: { message?: string; code?: string } };
			throw new ApiRequestError(
				err.error?.message || res.statusText,
				err.error?.code || 'unknown',
				res.status
			);
		}
		return (json as { data: T }).data;
	}

	interface Plugin {
		id: string;
		name: string;
		description: string | null;
		author: string;
		version: string;
		homepage_url: string | null;
		icon_url: string | null;
		category: string;
		public: boolean;
		verified: boolean;
		install_count: number;
		created_at: string;
		updated_at: string;
	}

	let plugins = $state<Plugin[]>([]);
	let loading = $state(true);
	let error = $state('');
	let search = $state('');
	let selectedCategory = $state('');
	let installing = $state<string | null>(null);

	const categories = [
		{ value: '', label: 'All Categories' },
		{ value: 'utility', label: 'Utility' },
		{ value: 'moderation', label: 'Moderation' },
		{ value: 'fun', label: 'Fun' },
		{ value: 'integration', label: 'Integration' }
	];

	const categoryColors: Record<string, string> = {
		utility: 'bg-blue-500/10 text-blue-400 border-blue-500/20',
		moderation: 'bg-red-500/10 text-red-400 border-red-500/20',
		fun: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/20',
		integration: 'bg-green-500/10 text-green-400 border-green-500/20'
	};

	$effect(() => {
		loadPlugins();
	});

	async function loadPlugins() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams();
			if (search) params.set('q', search);
			if (selectedCategory) params.set('category', selectedCategory);
			params.set('limit', '50');

			const resp = await apiRequest<Plugin[]>('GET', `/plugins?${params.toString()}`);
			plugins = resp;
		} catch (err: any) {
			error = err.message || 'Failed to load plugins';
		} finally {
			loading = false;
		}
	}

	async function installPlugin(plugin: Plugin) {
		const guildId = $currentGuildId;
		if (!guildId) {
			addToast('Select a server first to install plugins', 'error');
			return;
		}

		installing = plugin.id;
		try {
			await apiRequest('POST', `/guilds/${guildId}/plugins`, {
				plugin_id: plugin.id,
				config: {}
			});
			addToast(`${plugin.name} installed successfully`, 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to install plugin', 'error');
		} finally {
			installing = null;
		}
	}

	function handleSearch() {
		loadPlugins();
	}

	function formatInstalls(count: number): string {
		if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
		if (count >= 1000) return `${(count / 1000).toFixed(1)}K`;
		return count.toString();
	}
</script>

<svelte:head>
	<title>Plugin Marketplace - AmityVox</title>
</svelte:head>

<div class="mx-auto max-w-5xl p-6">
	<!-- Header -->
	<div class="mb-8">
		<h1 class="text-2xl font-bold text-text-primary">Plugin Marketplace</h1>
		<p class="mt-1 text-sm text-text-muted">
			Discover and install plugins to extend your server's functionality.
		</p>
	</div>

	<!-- Search and filters -->
	<div class="mb-6 flex flex-col gap-3 sm:flex-row">
		<div class="relative flex-1">
			<svg class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
			</svg>
			<input
				type="text"
				class="input w-full pl-10"
				placeholder="Search plugins..."
				bind:value={search}
				onkeydown={(e) => e.key === 'Enter' && handleSearch()}
			/>
		</div>
		<select
			class="input w-full sm:w-48"
			bind:value={selectedCategory}
			onchange={loadPlugins}
		>
			{#each categories as cat}
				<option value={cat.value}>{cat.label}</option>
			{/each}
		</select>
	</div>

	<!-- Plugin grid -->
	{#if loading}
		<div class="flex items-center justify-center py-16">
			<span class="inline-block h-8 w-8 animate-spin rounded-full border-3 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if error}
		<div class="rounded-lg bg-red-500/10 px-6 py-4 text-sm text-red-400">{error}</div>
	{:else if plugins.length === 0}
		<div class="flex flex-col items-center justify-center py-16">
			<svg class="h-16 w-16 text-text-muted" fill="none" stroke="currentColor" stroke-width="1" viewBox="0 0 24 24">
				<path d="M14 10l-2 1m0 0l-2-1m2 1v2.5M20 7l-2 1m2-1l-2-1m2 1v2.5M14 4l-2-1-2 1M4 7l2-1M4 7l2 1M4 7v2.5M12 21l-2-1m2 1l2-1m-2 1v-2.5M6 18l-2-1v-2.5M18 18l2-1v-2.5" />
			</svg>
			<p class="mt-4 text-sm text-text-muted">
				{search ? 'No plugins found matching your search.' : 'No plugins available yet.'}
			</p>
		</div>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each plugins as plugin (plugin.id)}
				<div class="flex flex-col rounded-lg border border-bg-modifier bg-bg-secondary p-4 transition-colors hover:border-brand-500/30">
					<!-- Header -->
					<div class="flex items-start gap-3">
						{#if plugin.icon_url}
							<img src={plugin.icon_url} alt="" class="h-10 w-10 rounded-lg" />
						{:else}
							<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-bg-tertiary">
								<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M14 10l-2 1m0 0l-2-1m2 1v2.5M20 7l-2 1m2-1l-2-1m2 1v2.5M14 4l-2-1-2 1M4 7l2-1M4 7l2 1M4 7v2.5M12 21l-2-1m2 1l2-1m-2 1v-2.5M6 18l-2-1v-2.5M18 18l2-1v-2.5" />
								</svg>
							</div>
						{/if}
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<h3 class="truncate text-sm font-semibold text-text-primary">{plugin.name}</h3>
								{#if plugin.verified}
									<svg class="h-4 w-4 shrink-0 text-brand-400" fill="currentColor" viewBox="0 0 20 20" title="Verified">
										<path fill-rule="evenodd" d="M6.267 3.455a3.066 3.066 0 001.745-.723 3.066 3.066 0 013.976 0 3.066 3.066 0 001.745.723 3.066 3.066 0 012.812 2.812c.051.643.304 1.254.723 1.745a3.066 3.066 0 010 3.976 3.066 3.066 0 00-.723 1.745 3.066 3.066 0 01-2.812 2.812 3.066 3.066 0 00-1.745.723 3.066 3.066 0 01-3.976 0 3.066 3.066 0 00-1.745-.723 3.066 3.066 0 01-2.812-2.812 3.066 3.066 0 00-.723-1.745 3.066 3.066 0 010-3.976 3.066 3.066 0 00.723-1.745 3.066 3.066 0 012.812-2.812zm7.44 5.252a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
									</svg>
								{/if}
							</div>
							<p class="text-xs text-text-muted">by {plugin.author}</p>
						</div>
					</div>

					<!-- Description -->
					{#if plugin.description}
						<p class="mt-3 flex-1 text-xs text-text-secondary line-clamp-2">{plugin.description}</p>
					{:else}
						<div class="flex-1"></div>
					{/if}

					<!-- Footer -->
					<div class="mt-4 flex items-center justify-between">
						<div class="flex items-center gap-3">
							<span class="rounded-full border px-2 py-0.5 text-xs {categoryColors[plugin.category] || 'border-bg-modifier bg-bg-modifier text-text-muted'}">
								{plugin.category}
							</span>
							<span class="text-xs text-text-muted" title="{plugin.install_count} installs">
								{formatInstalls(plugin.install_count)} installs
							</span>
						</div>
						<button
							class="rounded-md bg-brand-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
							onclick={() => installPlugin(plugin)}
							disabled={installing === plugin.id}
						>
							{installing === plugin.id ? 'Installing...' : 'Install'}
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
