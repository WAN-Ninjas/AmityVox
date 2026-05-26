<script lang="ts">
	import { page } from '$app/stores';
	import { api, type PluginListing } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { currentGuildId } from '$lib/stores/guilds';
	import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';

	let plugins = $state<PluginListing[]>([]);
	let loadOp = $state(createAsyncOp(true));
	let search = $state('');
	let selectedCategory = $state('');
	let installing = $state<string | null>(null);
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	let selectedPlugin = $state<(PluginListing & { manifest: unknown }) | null>(null);
	let detailOp = $state(createAsyncOp());
	const hasWidgets = $derived(isFeatureEnabled($clientConfig, 'widgets'));

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
		if (hasWidgets) {
			loadPlugins();
		} else {
			plugins = [];
			selectedPlugin = null;
		}
	});

	async function loadPlugins() {
		if (!hasWidgets) return;
		const result = await loadOp.run(() =>
			api.listPlugins({
				q: search.trim() || undefined,
				category: selectedCategory || undefined,
				limit: 50
			})
		);
		if (result) plugins = result;
	}

	async function installPlugin(plugin: PluginListing) {
		if (!hasWidgets) {
			addToast('Plugins are disabled on this instance', 'error');
			return;
		}
		const guildId = $page.url.searchParams.get('guild') || $currentGuildId;
		if (!guildId) {
			addToast('Select a server first to install plugins', 'error');
			return;
		}

		installing = plugin.id;
		try {
			await api.installPlugin(guildId, plugin.id);
			plugins = plugins.map((entry) =>
				entry.id === plugin.id ? { ...entry, install_count: entry.install_count + 1 } : entry
			);
			addToast(`${plugin.name} installed successfully`, 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to install plugin'), 'error');
		} finally {
			installing = null;
		}
	}

	async function showPluginDetails(plugin: PluginListing) {
		if (!hasWidgets) return;
		const detail = await detailOp.run(
			() => api.getPlugin(plugin.id),
			(message) => addToast(message, 'error'),
			'Failed to load plugin details'
		);
		if (detail) selectedPlugin = detail;
	}

	function installSelectedPlugin() {
		if (!selectedPlugin) return;
		installPlugin(selectedPlugin);
	}

	function handleSearch() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(loadPlugins, 250);
	}

	function formatInstalls(count: number): string {
		if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
		if (count >= 1000) return `${(count / 1000).toFixed(1)}K`;
		return count.toString();
	}

	function formatManifest(manifest: unknown): string {
		try {
			return JSON.stringify(manifest, null, 2);
		} catch {
			return String(manifest);
		}
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

	{#if hasWidgets}
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
					oninput={handleSearch}
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
	{/if}

	<!-- Plugin grid -->
	{#if !hasWidgets}
		<div class="rounded-lg border border-bg-modifier bg-bg-secondary px-6 py-5 text-sm text-text-muted">
			Plugins are disabled on this instance.
		</div>
	{:else if loadOp.loading}
		<div class="flex items-center justify-center py-16">
			<span class="inline-block h-8 w-8 animate-spin rounded-full border-3 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if loadOp.error}
		<div class="rounded-lg bg-red-500/10 px-6 py-4 text-sm text-red-400">{loadOp.error}</div>
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
									<svg class="h-4 w-4 shrink-0 text-brand-400" fill="currentColor" viewBox="0 0 20 20">
										<title>Verified</title>
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
						<div class="flex items-center gap-2">
							<button
								class="rounded-md border border-bg-modifier px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
								onclick={() => showPluginDetails(plugin)}
								disabled={detailOp.loading}
							>
								Details
							</button>
							<button
								class="rounded-md bg-brand-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
								onclick={() => installPlugin(plugin)}
								disabled={installing === plugin.id}
							>
								{installing === plugin.id ? 'Installing...' : 'Install'}
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

{#if selectedPlugin}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
		onclick={() => (selectedPlugin = null)}
		onkeydown={(e) => e.key === 'Escape' && (selectedPlugin = null)}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-lg bg-bg-floating p-5 shadow-xl"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="document"
			tabindex="-1"
		>
			<div class="flex items-start justify-between gap-4">
				<div class="min-w-0">
					<h2 class="truncate text-lg font-semibold text-text-primary">{selectedPlugin.name}</h2>
					<p class="text-xs text-text-muted">by {selectedPlugin.author} · v{selectedPlugin.version}</p>
				</div>
				<button class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary" onclick={() => (selectedPlugin = null)} aria-label="Close">
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			{#if selectedPlugin.description}
				<p class="mt-4 text-sm text-text-secondary">{selectedPlugin.description}</p>
			{/if}

			<div class="mt-4 flex flex-wrap items-center gap-2 text-xs">
				<span class="rounded-full border px-2 py-0.5 {categoryColors[selectedPlugin.category] || 'border-bg-modifier bg-bg-modifier text-text-muted'}">
					{selectedPlugin.category}
				</span>
				<span class="text-text-muted">{formatInstalls(selectedPlugin.install_count)} installs</span>
				{#if selectedPlugin.verified}
					<span class="rounded-full bg-brand-500/10 px-2 py-0.5 text-brand-400">Verified</span>
				{/if}
				{#if selectedPlugin.homepage_url}
					<a class="text-brand-400 hover:underline" href={selectedPlugin.homepage_url} target="_blank" rel="noopener noreferrer">Homepage</a>
				{/if}
			</div>

			<div class="mt-5">
				<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Manifest</h3>
				<pre class="max-h-80 overflow-auto rounded-md bg-bg-primary p-3 text-xs text-text-secondary">{formatManifest(selectedPlugin.manifest)}</pre>
			</div>

			<div class="mt-5 flex justify-end gap-2">
				<button class="btn-secondary text-sm" onclick={() => (selectedPlugin = null)}>Close</button>
				<button class="btn-primary text-sm" onclick={installSelectedPlugin} disabled={installing === selectedPlugin.id}>
					{installing === selectedPlugin.id ? 'Installing...' : 'Install'}
				</button>
			</div>
		</div>
	</div>
{/if}
