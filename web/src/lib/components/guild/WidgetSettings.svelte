<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Channel } from '$lib/types';

	interface GuildWidgetConfig {
		guild_id: string;
		enabled: boolean;
		invite_channel_id: string | null;
		style: string;
		updated_at: string;
	}

	interface Props {
		guildId: string;
		channels?: Channel[];
	}

	let { guildId, channels = [] }: Props = $props();

	let config = $state<GuildWidgetConfig | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let copied = $state(false);

	let enabled = $state(false);
	let inviteChannelId = $state('');
	let style = $state('banner_1');

	const styleOptions = [
		{ value: 'banner_1', label: 'Banner (Large)', description: 'Full-width banner with server info and member list' },
		{ value: 'banner_2', label: 'Banner (Compact)', description: 'Compact banner with server name and member count' },
		{ value: 'shield', label: 'Shield Badge', description: 'Small badge showing online member count' }
	];

	$effect(() => {
		const id = guildId;
		if (id) loadWidget(id);
	});

	async function loadWidget(gId: string) {
		loading = true;
		error = '';
		try {
			const resp = await api.getGuildWidget(gId) as GuildWidgetConfig;
			config = resp;
			enabled = resp.enabled;
			inviteChannelId = resp.invite_channel_id || '';
			style = resp.style || 'banner_1';
		} catch (err: any) {
			error = err.message || 'Failed to load widget settings';
		} finally {
			loading = false;
		}
	}

	async function saveWidget() {
		saving = true;
		try {
			const resp = await api.updateGuildWidget(guildId, {
				enabled,
				invite_channel_id: inviteChannelId || null,
				style
			}) as GuildWidgetConfig;
			config = resp;
			addToast('Widget settings saved', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to save widget settings', 'error');
		} finally {
			saving = false;
		}
	}

	function getEmbedUrl() {
		return `${location.origin}/app/embed/${guildId}`;
	}

	function getJsonUrl() {
		return `${location.origin}/api/v1/guilds/${guildId}/widget.json`;
	}

	function copyEmbed() {
		const html = `<iframe src="${getEmbedUrl()}" width="350" height="500" allowtransparency="true" frameborder="0" sandbox="allow-popups allow-popups-to-escape-sandbox allow-same-origin allow-scripts"></iframe>`;
		navigator.clipboard.writeText(html).then(() => {
			copied = true;
			setTimeout(() => (copied = false), 2000);
		});
	}

	let textChannels = $derived(
		channels.filter((c) => c.channel_type === 'text' && c.guild_id === guildId)
	);
</script>

<div class="flex flex-col gap-6">
	<div>
		<h3 class="text-lg font-semibold text-text-primary">Server Widget</h3>
		<p class="text-sm text-text-muted">
			Allow other websites to embed a widget showing your server info.
		</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<span class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else}
		<!-- Enable toggle -->
		<div class="flex items-center justify-between rounded-lg border border-bg-modifier bg-bg-secondary p-4">
			<div>
				<p class="text-sm font-medium text-text-primary">Enable Server Widget</p>
				<p class="text-xs text-text-muted">Allow external websites to show a widget with your server information.</p>
			</div>
			<button
				class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out"
				class:bg-brand-500={enabled}
				class:bg-bg-modifier={!enabled}
				onclick={() => (enabled = !enabled)}
				role="switch"
				aria-checked={enabled}
			>
				<span
					class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
					class:translate-x-5={enabled}
					class:translate-x-0={!enabled}
				></span>
			</button>
		</div>

		{#if enabled}
			<!-- Style selector -->
			<div>
				<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Widget Style
				</label>
				<div class="flex flex-col gap-2">
					{#each styleOptions as opt}
						<label
							class="flex cursor-pointer items-center gap-3 rounded-lg border border-bg-modifier bg-bg-secondary p-3 transition-colors hover:border-brand-500/30"
							class:border-brand-500={style === opt.value}
						>
							<input
								type="radio"
								name="style"
								value={opt.value}
								bind:group={style}
								class="h-4 w-4 accent-brand-500"
							/>
							<div>
								<p class="text-sm font-medium text-text-primary">{opt.label}</p>
								<p class="text-xs text-text-muted">{opt.description}</p>
							</div>
						</label>
					{/each}
				</div>
			</div>

			<!-- Invite channel -->
			<div>
				<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Invite Channel
				</label>
				<p class="mb-2 text-xs text-text-muted">
					Select a channel for the widget's invite button. An active invite must exist for this channel.
				</p>
				<select class="input w-full" bind:value={inviteChannelId}>
					<option value="">None (no invite button)</option>
					{#each textChannels as ch}
						<option value={ch.id}>#{ch.name}</option>
					{/each}
				</select>
			</div>

			<!-- Embed code -->
			<div class="rounded-lg border border-bg-modifier bg-bg-secondary p-4">
				<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Embed Code</h4>
				<div class="rounded bg-bg-primary p-3 font-mono text-xs text-text-secondary">
					&lt;iframe src="{getEmbedUrl()}" width="350" height="500"&gt;&lt;/iframe&gt;
				</div>
				<div class="mt-3 flex gap-2">
					<button class="btn-primary text-xs" onclick={copyEmbed}>
						{copied ? 'Copied!' : 'Copy Embed Code'}
					</button>
					<a
						href={getJsonUrl()}
						target="_blank"
						rel="noopener"
						class="btn-secondary text-xs"
					>
						JSON API
					</a>
				</div>
			</div>
		{/if}

		<div class="flex justify-end">
			<button class="btn-primary" onclick={saveWidget} disabled={saving}>
				{saving ? 'Saving...' : 'Save Changes'}
			</button>
		</div>
	{/if}
</div>
