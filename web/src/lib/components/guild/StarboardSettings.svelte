<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Channel } from '$lib/types';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let { guildId }: { guildId: string } = $props();

	interface StarboardConfig {
		guild_id: string;
		enabled: boolean;
		channel_id: string | null;
		emoji: string;
		threshold: number;
		self_star: boolean;
		nsfw_allowed: boolean;
	}

	interface StarboardEntry {
		id: string;
		guild_id: string;
		source_message_id: string;
		source_channel_id: string;
		starboard_message_id: string | null;
		star_count: number;
		author_id: string;
		created_at: string;
	}

	let loadOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());
	let error = $state('');
	let success = $state('');

	let config = $state<StarboardConfig>({
		guild_id: '',
		enabled: false,
		channel_id: null,
		emoji: 'star',
		threshold: 3,
		self_star: false,
		nsfw_allowed: false
	});
	let entries = $state<StarboardEntry[]>([]);
	let channels = $state<Channel[]>([]);
	let loadEntriesOp = $state(createAsyncOp());
	let activeTab = $state<'settings' | 'entries'>('settings');

	async function loadConfig() {
		error = '';
		const result = await loadOp.run(
			() => api.request<StarboardConfig>('GET', `/guilds/${guildId}/starboard`)
		);
		if (loadOp.error) {
			error = loadOp.error;
		} else if (result) {
			config = result;
		}
	}

	async function loadChannels() {
		try {
			const allChannels = await api.getGuildChannels(guildId);
			channels = allChannels.filter((ch: Channel) => ch.channel_type === 'text');
		} catch { /* ignore */ }
	}

	async function saveConfig() {
		error = '';
		success = '';
		const result = await saveOp.run(
			() => api.request<StarboardConfig>(
				'PATCH', `/guilds/${guildId}/starboard`, {
					enabled: config.enabled,
					channel_id: config.channel_id,
					emoji: config.emoji,
					threshold: config.threshold,
					self_star: config.self_star,
					nsfw_allowed: config.nsfw_allowed
				}
			)
		);
		if (saveOp.error) {
			error = saveOp.error;
		} else if (result) {
			config = result;
			success = 'Settings saved';
			setTimeout(() => (success = ''), 3000);
		}
	}

	async function loadEntries() {
		const result = await loadEntriesOp.run(
			() => api.request<StarboardEntry[]>(
				'GET', `/guilds/${guildId}/starboard/entries?limit=50`
			)
		);
		if (loadEntriesOp.error) {
			error = loadEntriesOp.error;
		} else if (result) {
			entries = result;
		}
	}

	function getChannelName(chId: string): string {
		return channels.find(c => c.id === chId)?.name ?? 'unknown';
	}

	function emojiDisplay(emoji: string): string {
		const emojiMap: Record<string, string> = {
			'star': '\u2B50',
			'heart': '\u2764\uFE0F',
			'fire': '\uD83D\uDD25',
			'thumbsup': '\uD83D\uDC4D',
			'rocket': '\uD83D\uDE80'
		};
		return emojiMap[emoji] ?? emoji;
	}

	$effect(() => {
		if (guildId) {
			loadConfig();
			loadChannels();
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Starboard</h2>
	</div>

	{#if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded bg-green-500/10 px-4 py-3 text-sm text-green-400">{success}</div>
	{/if}

	{#if loadOp.loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else}
		<!-- Tabs -->
		<div class="flex gap-2 border-b border-bg-tertiary pb-2">
			<button
				class="rounded px-3 py-1.5 text-sm font-medium transition-colors"
				class:bg-brand-500={activeTab === 'settings'}
				class:text-white={activeTab === 'settings'}
				class:text-text-muted={activeTab !== 'settings'}
				onclick={() => (activeTab = 'settings')}
			>Settings</button>
			<button
				class="rounded px-3 py-1.5 text-sm font-medium transition-colors"
				class:bg-brand-500={activeTab === 'entries'}
				class:text-white={activeTab === 'entries'}
				class:text-text-muted={activeTab !== 'entries'}
				onclick={() => { activeTab = 'entries'; loadEntries(); }}
			>Starred Messages</button>
		</div>

		{#if activeTab === 'settings'}
			<div class="space-y-4">
				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.enabled} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Enable starboard</span>
				</label>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Starboard Channel
					</label>
					<select class="input w-full" bind:value={config.channel_id}>
						<option value={null}>Select a channel...</option>
						{#each channels as ch}
							<option value={ch.id}>#{ch.name}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-text-muted">
						Messages that reach the star threshold will be reposted here
					</p>
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Emoji
					</label>
					<select class="input w-40" bind:value={config.emoji}>
						<option value="star">{emojiDisplay('star')} Star</option>
						<option value="heart">{emojiDisplay('heart')} Heart</option>
						<option value="fire">{emojiDisplay('fire')} Fire</option>
						<option value="thumbsup">{emojiDisplay('thumbsup')} Thumbs Up</option>
						<option value="rocket">{emojiDisplay('rocket')} Rocket</option>
					</select>
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Threshold
					</label>
					<input type="number" class="input w-24" bind:value={config.threshold}
						min="1" max="100" />
					<p class="mt-1 text-xs text-text-muted">
						Number of reactions needed to post to the starboard
					</p>
				</div>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.self_star} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Allow self-starring</span>
				</label>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.nsfw_allowed} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Allow NSFW channel messages on starboard</span>
				</label>

				<button class="btn-primary" onclick={saveConfig} disabled={saveOp.loading}>
					{saveOp.loading ? 'Saving...' : 'Save Settings'}
				</button>
			</div>

		{:else if activeTab === 'entries'}
			{#if loadEntriesOp.loading}
				<div class="flex items-center justify-center py-8">
					<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				</div>
			{:else if entries.length === 0}
				<div class="rounded bg-bg-secondary px-4 py-6 text-center text-sm text-text-muted">
					No starred messages yet
				</div>
			{:else}
				<div class="space-y-2">
					{#each entries as entry}
						<div class="rounded bg-bg-secondary p-3">
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-2">
									<span class="text-lg">{emojiDisplay(config.emoji)}</span>
									<span class="font-bold text-yellow-400">{entry.star_count}</span>
									<span class="text-sm text-text-muted">
										in #{getChannelName(entry.source_channel_id)}
									</span>
								</div>
								<span class="text-xs text-text-muted">
									{new Date(entry.created_at).toLocaleDateString()}
								</span>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>
