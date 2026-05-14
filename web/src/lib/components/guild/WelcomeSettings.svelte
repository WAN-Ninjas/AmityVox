<script lang="ts">
	import { api, type WelcomeConfig } from '$lib/api/client';
	import type { Channel } from '$lib/types';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let { guildId }: { guildId: string } = $props();

	let loadOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());
	let error = $state('');
	let success = $state('');

	let config = $state<WelcomeConfig>({
		guild_id: '',
		enabled: false,
		channel_id: null,
		message: 'Welcome to the server, {user}!',
		dm_enabled: false,
		dm_message: 'Welcome to {guild}! Please read the rules.',
		embed_enabled: false,
		embed_color: '#5865F2',
		embed_title: 'Welcome!',
		embed_image_url: null
	});
	let channels = $state<Channel[]>([]);

	async function loadConfig() {
		error = '';
		const result = await loadOp.run(() => api.getWelcomeConfig(guildId));
		if (!loadOp.error) {
			config = result!;
		} else {
			error = loadOp.error;
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
		const result = await saveOp.run(() => api.updateWelcomeConfig(guildId, {
			enabled: config.enabled,
			channel_id: config.channel_id,
			message: config.message,
			dm_enabled: config.dm_enabled,
			dm_message: config.dm_message,
			embed_enabled: config.embed_enabled,
			embed_color: config.embed_color,
			embed_title: config.embed_title,
			embed_image_url: config.embed_image_url
		}));
		if (!saveOp.error) {
			config = result!;
			success = 'Settings saved';
			setTimeout(() => (success = ''), 3000);
		} else {
			error = saveOp.error;
		}
	}

	$effect(() => {
		if (guildId) {
			loadConfig();
			loadChannels();
		}
	});

	// Preview the welcome message with example values
	let previewMessage = $derived(
		config.message
			.replace('{user}', '@NewUser')
			.replace('{username}', 'NewUser')
			.replace('{guild}', 'My Server')
			.replace('{membercount}', '42')
	);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Welcome Messages</h2>
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
		<div class="space-y-4">
			<!-- Enable toggle -->
			<label class="flex items-center gap-3">
				<input type="checkbox" bind:checked={config.enabled} class="h-4 w-4 rounded" />
				<span class="text-sm text-text-primary">Enable welcome messages</span>
			</label>

			<!-- Channel Welcome -->
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Channel Welcome Message</h3>

				<div class="space-y-3">
						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted" for="welcome-channel-id">
								Channel
							</label>
							<select id="welcome-channel-id" class="input w-full" bind:value={config.channel_id}>
							<option value={null}>Select a channel...</option>
							{#each channels as ch}
								<option value={ch.id}>#{ch.name}</option>
							{/each}
						</select>
					</div>

						<div>
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted" for="welcome-message">
								Message
							</label>
							<textarea id="welcome-message" class="input w-full" rows="3" bind:value={config.message}></textarea>
						<p class="mt-1 text-xs text-text-muted">
							Variables: {'{user}'} (mention), {'{username}'} (plain), {'{guild}'} (server name), {'{membercount}'}
						</p>
					</div>

					<!-- Preview -->
					<div class="rounded bg-bg-tertiary p-3">
						<div class="text-xs font-bold uppercase tracking-wide text-text-muted">Preview</div>
						<div class="mt-1 text-sm text-text-primary">{previewMessage}</div>
					</div>
				</div>
			</div>

			<!-- DM Welcome -->
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Direct Message</h3>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.dm_enabled} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Send a DM to new members</span>
				</label>

				{#if config.dm_enabled}
						<div class="mt-3">
							<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted" for="welcome-dm-message">
								DM Message
							</label>
							<textarea id="welcome-dm-message" class="input w-full" rows="3" bind:value={config.dm_message}></textarea>
						<p class="mt-1 text-xs text-text-muted">
							Variables: {'{user}'}, {'{username}'}, {'{guild}'}
						</p>
					</div>
				{/if}
			</div>

			<!-- Embed Options -->
			<div class="rounded-lg bg-bg-secondary p-4">
				<h3 class="mb-3 text-sm font-semibold text-text-primary">Embed Styling</h3>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.embed_enabled} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Use rich embed for welcome message</span>
				</label>

				{#if config.embed_enabled}
					<div class="mt-3 space-y-3">
							<div>
								<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted" for="welcome-embed-title">
									Embed Title
								</label>
								<input id="welcome-embed-title" type="text" class="input w-full" bind:value={config.embed_title} />
						</div>

							<div>
								<div class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
									Embed Color
								</div>
								<div class="flex items-center gap-2">
									<input type="color" bind:value={config.embed_color} aria-label="Embed color picker"
										class="h-8 w-8 cursor-pointer rounded border-0" />
									<input type="text" class="input w-28" bind:value={config.embed_color} aria-label="Embed color hex"
										placeholder="#5865F2" />
								</div>
							</div>

							<div>
								<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted" for="welcome-embed-image-url">
									Image URL (optional)
								</label>
								<input id="welcome-embed-image-url" type="text" class="input w-full" bind:value={config.embed_image_url}
								placeholder="https://example.com/welcome.png" />
						</div>
					</div>
				{/if}
			</div>

			<button class="btn-primary" onclick={saveConfig} disabled={saveOp.loading}>
				{saveOp.loading ? 'Saving...' : 'Save Settings'}
			</button>
		</div>
	{/if}
</div>
