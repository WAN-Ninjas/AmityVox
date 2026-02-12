<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

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

	interface GuildPlugin {
		id: string;
		guild_id: string;
		plugin_id: string;
		enabled: boolean;
		config: Record<string, unknown>;
		installed_by: string;
		installed_at: string;
		updated_at: string;
		name: string;
		description: string | null;
		author: string;
		version: string;
		icon_url: string | null;
		category: string;
	}

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let plugins = $state<GuildPlugin[]>([]);
	let loading = $state(true);
	let error = $state('');
	let configuring = $state<string | null>(null);
	let configJson = $state('');

	const categoryColors: Record<string, string> = {
		utility: 'bg-blue-500/10 text-blue-400',
		moderation: 'bg-red-500/10 text-red-400',
		fun: 'bg-yellow-500/10 text-yellow-400',
		integration: 'bg-green-500/10 text-green-400'
	};

	$effect(() => {
		const id = guildId;
		if (id) loadPlugins(id);
	});

	async function loadPlugins(gId: string) {
		loading = true;
		error = '';
		try {
			const resp = await apiRequest<GuildPlugin[]>('GET', `/guilds/${gId}/plugins`);
			plugins = resp;
		} catch (err: any) {
			error = err.message || 'Failed to load plugins';
		} finally {
			loading = false;
		}
	}

	async function togglePlugin(plugin: GuildPlugin) {
		try {
			await apiRequest('PATCH', `/guilds/${guildId}/plugins/${plugin.id}`, {
				enabled: !plugin.enabled
			});
			plugins = plugins.map((p) =>
				p.id === plugin.id ? { ...p, enabled: !p.enabled } : p
			);
			addToast(`Plugin ${plugin.enabled ? 'disabled' : 'enabled'}`, 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to update plugin', 'error');
		}
	}

	async function uninstallPlugin(plugin: GuildPlugin) {
		if (!confirm(`Uninstall "${plugin.name}"? This will remove all plugin configuration.`)) return;
		try {
			await apiRequest('DELETE', `/guilds/${guildId}/plugins/${plugin.id}`);
			plugins = plugins.filter((p) => p.id !== plugin.id);
			addToast('Plugin uninstalled', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to uninstall plugin', 'error');
		}
	}

	function startConfigure(plugin: GuildPlugin) {
		configuring = plugin.id;
		configJson = JSON.stringify(plugin.config, null, 2);
	}

	async function saveConfig(plugin: GuildPlugin) {
		try {
			const config = JSON.parse(configJson);
			await apiRequest('PATCH', `/guilds/${guildId}/plugins/${plugin.id}`, { config });
			plugins = plugins.map((p) =>
				p.id === plugin.id ? { ...p, config } : p
			);
			configuring = null;
			addToast('Plugin configuration saved', 'success');
		} catch (err: any) {
			if (err instanceof SyntaxError) {
				addToast('Invalid JSON configuration', 'error');
			} else {
				addToast(err.message || 'Failed to save configuration', 'error');
			}
		}
	}
</script>

<div class="flex flex-col gap-4">
	<div class="flex items-center justify-between">
		<div>
			<h3 class="text-lg font-semibold text-text-primary">Installed Plugins</h3>
			<p class="text-sm text-text-muted">Manage plugins installed in this server.</p>
		</div>
		<a
			href="/app/plugins"
			class="rounded-md bg-brand-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-600"
		>
			Browse Plugins
		</a>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<span class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if plugins.length === 0}
		<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-8 text-center">
			<svg class="mx-auto h-12 w-12 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
				<path d="M14 10l-2 1m0 0l-2-1m2 1v2.5M20 7l-2 1m2-1l-2-1m2 1v2.5M14 4l-2-1-2 1M4 7l2-1M4 7l2 1M4 7v2.5M12 21l-2-1m2 1l2-1m-2 1v-2.5M6 18l-2-1v-2.5M18 18l2-1v-2.5" />
			</svg>
			<p class="mt-3 text-sm text-text-muted">No plugins installed. Browse the plugin marketplace to get started.</p>
		</div>
	{:else}
		<div class="flex flex-col gap-3">
			{#each plugins as plugin (plugin.id)}
				<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-4">
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
								<h4 class="text-sm font-semibold text-text-primary">{plugin.name}</h4>
								<span class="text-xs text-text-muted">v{plugin.version}</span>
								<span class="rounded-full px-2 py-0.5 text-xs {categoryColors[plugin.category] || 'bg-bg-modifier text-text-muted'}">
									{plugin.category}
								</span>
							</div>
							{#if plugin.description}
								<p class="mt-1 text-xs text-text-secondary">{plugin.description}</p>
							{/if}
							<p class="mt-1 text-xs text-text-muted">by {plugin.author}</p>
						</div>
						<div class="flex items-center gap-2">
							<!-- Toggle enabled -->
							<button
								class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
								class:bg-brand-500={plugin.enabled}
								class:bg-bg-modifier={!plugin.enabled}
								onclick={() => togglePlugin(plugin)}
								role="switch"
								aria-checked={plugin.enabled}
							>
								<span
									class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
									class:translate-x-5={plugin.enabled}
									class:translate-x-0={!plugin.enabled}
								></span>
							</button>
							<!-- Configure -->
							<button
								class="rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
								onclick={() => startConfigure(plugin)}
								title="Configure"
							>
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.573-1.066z" />
									<circle cx="12" cy="12" r="3" />
								</svg>
							</button>
							<!-- Uninstall -->
							<button
								class="rounded p-1.5 text-text-muted transition-colors hover:bg-red-500/10 hover:text-red-400"
								onclick={() => uninstallPlugin(plugin)}
								title="Uninstall"
							>
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						</div>
					</div>

					{#if configuring === plugin.id}
						<div class="mt-4 border-t border-bg-modifier pt-4">
							<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
								Plugin Configuration (JSON)
							</label>
							<textarea
								class="input w-full font-mono text-xs"
								rows="6"
								bind:value={configJson}
								spellcheck="false"
							></textarea>
							<div class="mt-2 flex justify-end gap-2">
								<button
									class="btn-secondary"
									onclick={() => (configuring = null)}
								>
									Cancel
								</button>
								<button
									class="btn-primary"
									onclick={() => saveConfig(plugin)}
								>
									Save Config
								</button>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
