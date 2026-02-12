<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Channel } from '$lib/types';
	import RSSFeedSettings from './RSSFeedSettings.svelte';
	import CalendarSync from './CalendarSync.svelte';

	let { guildId, channels = [] }: { guildId: string; channels: Channel[] } = $props();

	// --- Types ---
	interface Integration {
		id: string;
		guild_id: string;
		integration_type: string;
		channel_id: string;
		name: string;
		enabled: boolean;
		config: Record<string, unknown>;
		created_by: string;
		created_at: string;
		updated_at: string;
	}

	interface BridgeConnection {
		id: string;
		guild_id: string;
		bridge_type: string;
		channel_id: string;
		remote_id: string;
		enabled: boolean;
		config: Record<string, unknown>;
		status: string;
		last_error: string | null;
		created_by: string;
		created_at: string;
		updated_at: string;
	}

	// --- State ---
	let integrations = $state<Integration[]>([]);
	let bridges = $state<BridgeConnection[]>([]);
	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	// Create integration form.
	let showCreateForm = $state(false);
	let newType = $state<string>('rss');
	let newName = $state('');
	let newChannelId = $state('');
	let creating = $state(false);

	// Create bridge form.
	let showBridgeForm = $state(false);
	let newBridgeType = $state<string>('telegram');
	let newBridgeChannelId = $state('');
	let newRemoteId = $state('');
	let creatingBridge = $state(false);

	// Detail view.
	let selectedIntegration = $state<Integration | null>(null);
	let selectedTab = $state<'integrations' | 'bridges' | 'log'>('integrations');

	// Log.
	let logEntries = $state<Array<{
		id: string;
		integration_id: string | null;
		bridge_connection_id: string | null;
		direction: string;
		source_id: string | null;
		amityvox_message_id: string | null;
		channel_id: string;
		status: string;
		error_message: string | null;
		created_at: string;
	}>>([]);
	let loadingLog = $state(false);

	const integrationTypes = [
		{ value: 'activitypub', label: 'ActivityPub (Mastodon/Lemmy)' },
		{ value: 'rss', label: 'RSS Feed' },
		{ value: 'calendar', label: 'Calendar Sync' },
		{ value: 'email', label: 'Email Gateway' },
		{ value: 'sms', label: 'SMS Bridge' },
	];

	const bridgeTypes = [
		{ value: 'telegram', label: 'Telegram' },
		{ value: 'slack', label: 'Slack' },
		{ value: 'irc', label: 'IRC' },
	];

	// --- Data loading ---
	async function loadIntegrations() {
		loading = true;
		error = '';
		try {
			integrations = await api.request('GET', `/guilds/${guildId}/integrations`);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load integrations';
		} finally {
			loading = false;
		}
	}

	async function loadBridges() {
		loading = true;
		error = '';
		try {
			bridges = await api.request('GET', `/guilds/${guildId}/bridge-connections`);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load bridge connections';
		} finally {
			loading = false;
		}
	}

	async function loadLog() {
		loadingLog = true;
		try {
			logEntries = await api.request('GET', `/guilds/${guildId}/integrations/log`);
		} catch {
			// Silently fail for log.
		} finally {
			loadingLog = false;
		}
	}

	// --- Integration CRUD ---
	async function createIntegration() {
		if (!newName.trim() || !newChannelId) return;
		creating = true;
		error = '';
		try {
			const integration: Integration = await api.request('POST', `/guilds/${guildId}/integrations`, {
				integration_type: newType,
				channel_id: newChannelId,
				name: newName.trim(),
				config: {},
			});
			integrations = [integration, ...integrations];
			showCreateForm = false;
			newName = '';
			newChannelId = '';
			success = 'Integration created successfully';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create integration';
		} finally {
			creating = false;
		}
	}

	async function toggleIntegration(integration: Integration) {
		try {
			const updated: Integration = await api.request('PATCH', `/guilds/${guildId}/integrations/${integration.id}`, {
				enabled: !integration.enabled,
			});
			integrations = integrations.map(i => i.id === updated.id ? updated : i);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to update integration';
		}
	}

	async function deleteIntegration(id: string) {
		if (!confirm('Delete this integration? All associated feeds, connections, and settings will be removed.')) return;
		try {
			await api.request('DELETE', `/guilds/${guildId}/integrations/${id}`);
			integrations = integrations.filter(i => i.id !== id);
			if (selectedIntegration?.id === id) selectedIntegration = null;
			success = 'Integration deleted';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to delete integration';
		}
	}

	// --- Bridge CRUD ---
	async function createBridge() {
		if (!newBridgeChannelId || !newRemoteId.trim()) return;
		creatingBridge = true;
		error = '';
		try {
			const bridge: BridgeConnection = await api.request('POST', `/guilds/${guildId}/bridge-connections`, {
				bridge_type: newBridgeType,
				channel_id: newBridgeChannelId,
				remote_id: newRemoteId.trim(),
				config: {},
			});
			bridges = [bridge, ...bridges];
			showBridgeForm = false;
			newRemoteId = '';
			newBridgeChannelId = '';
			success = 'Bridge connection created';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create bridge connection';
		} finally {
			creatingBridge = false;
		}
	}

	async function toggleBridge(bridge: BridgeConnection) {
		try {
			const updated: BridgeConnection = await api.request('PATCH', `/guilds/${guildId}/bridge-connections/${bridge.id}`, {
				enabled: !bridge.enabled,
			});
			bridges = bridges.map(b => b.id === updated.id ? updated : b);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to update bridge';
		}
	}

	async function deleteBridge(id: string) {
		if (!confirm('Delete this bridge connection?')) return;
		try {
			await api.request('DELETE', `/guilds/${guildId}/bridge-connections/${id}`);
			bridges = bridges.filter(b => b.id !== id);
			success = 'Bridge connection deleted';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to delete bridge';
		}
	}

	// --- Helpers ---
	function typeIcon(type_: string): string {
		const icons: Record<string, string> = {
			activitypub: 'AP',
			rss: 'RSS',
			calendar: 'CAL',
			email: 'EM',
			sms: 'SMS',
			telegram: 'TG',
			slack: 'SL',
			irc: 'IRC',
		};
		return icons[type_] || type_.toUpperCase().slice(0, 3);
	}

	function statusColor(status: string): string {
		if (status === 'connected') return 'text-green-400';
		if (status === 'error') return 'text-red-400';
		return 'text-text-muted';
	}

	function channelName(id: string): string {
		const ch = channels.find(c => c.id === id);
		return ch?.name || id;
	}

	// Load data on mount.
	$effect(() => {
		loadIntegrations();
		loadBridges();
	});
</script>

<div class="space-y-6">
	<!-- Tab Bar -->
	<div class="flex gap-2 border-b border-bg-tertiary pb-2">
		<button
			class="px-3 py-1.5 rounded-t text-sm font-medium transition-colors
				{selectedTab === 'integrations' ? 'bg-bg-tertiary text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => { selectedTab = 'integrations'; }}
		>
			Integrations
		</button>
		<button
			class="px-3 py-1.5 rounded-t text-sm font-medium transition-colors
				{selectedTab === 'bridges' ? 'bg-bg-tertiary text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => { selectedTab = 'bridges'; }}
		>
			Bridge Connections
		</button>
		<button
			class="px-3 py-1.5 rounded-t text-sm font-medium transition-colors
				{selectedTab === 'log' ? 'bg-bg-tertiary text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
			onclick={() => { selectedTab = 'log'; loadLog(); }}
		>
			Message Log
		</button>
	</div>

	{#if error}
		<div class="bg-red-500/10 border border-red-500/30 text-red-400 px-4 py-2 rounded text-sm">{error}</div>
	{/if}
	{#if success}
		<div class="bg-green-500/10 border border-green-500/30 text-green-400 px-4 py-2 rounded text-sm">{success}</div>
	{/if}

	<!-- Integrations Tab -->
	{#if selectedTab === 'integrations'}
		{#if selectedIntegration}
			<!-- Integration Detail View -->
			<div class="space-y-4">
				<button class="text-sm text-accent hover:underline" onclick={() => selectedIntegration = null}>
					Back to list
				</button>
				<div class="bg-bg-secondary rounded-lg p-4 space-y-3">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-3">
							<span class="bg-accent/20 text-accent px-2 py-1 rounded text-xs font-mono">
								{typeIcon(selectedIntegration.integration_type)}
							</span>
							<h3 class="font-semibold text-text-primary">{selectedIntegration.name}</h3>
						</div>
						<span class="text-xs px-2 py-0.5 rounded {selectedIntegration.enabled ? 'bg-green-500/20 text-green-400' : 'bg-bg-tertiary text-text-muted'}">
							{selectedIntegration.enabled ? 'Enabled' : 'Disabled'}
						</span>
					</div>
					<p class="text-sm text-text-muted">
						Channel: #{channelName(selectedIntegration.channel_id)}
					</p>
				</div>

				<!-- Type-specific settings -->
				{#if selectedIntegration.integration_type === 'rss'}
					<RSSFeedSettings {guildId} integrationId={selectedIntegration.id} />
				{:else if selectedIntegration.integration_type === 'calendar'}
					<CalendarSync {guildId} integrationId={selectedIntegration.id} />
				{:else if selectedIntegration.integration_type === 'activitypub'}
					<div class="bg-bg-secondary rounded-lg p-4">
						<h4 class="font-medium text-text-primary mb-2">ActivityPub Follows</h4>
						<p class="text-sm text-text-muted">
							Follow Mastodon, Lemmy, and other ActivityPub accounts. Posts from followed accounts
							will appear as messages in the linked channel.
						</p>
						<p class="text-xs text-text-muted mt-3">
							Use the API to manage follows: POST /guilds/{guildId}/integrations/{selectedIntegration.id}/activitypub/follows
						</p>
					</div>
				{:else if selectedIntegration.integration_type === 'email'}
					<div class="bg-bg-secondary rounded-lg p-4">
						<h4 class="font-medium text-text-primary mb-2">Email Gateway</h4>
						<p class="text-sm text-text-muted">
							Receive emails as messages in the linked channel. A unique email address will be
							generated for this integration.
						</p>
						<p class="text-xs text-text-muted mt-3">
							Configure via API: POST /guilds/{guildId}/integrations/{selectedIntegration.id}/email/gateway
						</p>
					</div>
				{:else if selectedIntegration.integration_type === 'sms'}
					<div class="bg-bg-secondary rounded-lg p-4">
						<h4 class="font-medium text-text-primary mb-2">SMS Bridge</h4>
						<p class="text-sm text-text-muted">
							Send and receive SMS messages via Twilio or Vonage. Inbound SMS will appear as
							messages in the linked channel.
						</p>
						<p class="text-xs text-text-muted mt-3">
							Configure via API: POST /guilds/{guildId}/integrations/{selectedIntegration.id}/sms/bridge
						</p>
					</div>
				{/if}
			</div>
		{:else}
			<!-- Integration List -->
			<div class="flex items-center justify-between">
				<h3 class="text-lg font-semibold text-text-primary">Service Integrations</h3>
				<button class="btn-primary text-sm" onclick={() => showCreateForm = !showCreateForm}>
					{showCreateForm ? 'Cancel' : 'Add Integration'}
				</button>
			</div>

			{#if showCreateForm}
				<div class="bg-bg-secondary rounded-lg p-4 space-y-3">
					<div>
						<label class="block text-sm text-text-secondary mb-1">Type</label>
						<select class="input w-full" bind:value={newType}>
							{#each integrationTypes as t}
								<option value={t.value}>{t.label}</option>
							{/each}
						</select>
					</div>
					<div>
						<label class="block text-sm text-text-secondary mb-1">Name</label>
						<input class="input w-full" bind:value={newName} placeholder="My RSS Feed" maxlength="100" />
					</div>
					<div>
						<label class="block text-sm text-text-secondary mb-1">Channel</label>
						<select class="input w-full" bind:value={newChannelId}>
							<option value="">Select a channel</option>
							{#each channels.filter(c => c.channel_type === 'text') as ch}
								<option value={ch.id}>#{ch.name}</option>
							{/each}
						</select>
					</div>
					<button
						class="btn-primary text-sm"
						onclick={createIntegration}
						disabled={creating || !newName.trim() || !newChannelId}
					>
						{creating ? 'Creating...' : 'Create Integration'}
					</button>
				</div>
			{/if}

			{#if loading}
				<div class="text-text-muted text-sm">Loading integrations...</div>
			{:else if integrations.length === 0}
				<div class="bg-bg-secondary rounded-lg p-6 text-center text-text-muted">
					<p>No integrations configured yet.</p>
					<p class="text-sm mt-1">Add ActivityPub, RSS, Calendar, Email, or SMS integrations.</p>
				</div>
			{:else}
				<div class="space-y-2">
					{#each integrations as integration}
						<div class="bg-bg-secondary rounded-lg p-3 flex items-center justify-between group hover:bg-bg-tertiary transition-colors">
							<button class="flex items-center gap-3 flex-1 text-left" onclick={() => selectedIntegration = integration}>
								<span class="bg-accent/20 text-accent px-2 py-1 rounded text-xs font-mono">
									{typeIcon(integration.integration_type)}
								</span>
								<div>
									<div class="font-medium text-text-primary text-sm">{integration.name}</div>
									<div class="text-xs text-text-muted">#{channelName(integration.channel_id)}</div>
								</div>
							</button>
							<div class="flex items-center gap-2">
								<button
									class="text-xs px-2 py-1 rounded transition-colors
										{integration.enabled ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30' : 'bg-bg-tertiary text-text-muted hover:bg-bg-primary'}"
									onclick={() => toggleIntegration(integration)}
								>
									{integration.enabled ? 'On' : 'Off'}
								</button>
								<button
									class="text-xs text-red-400 hover:text-red-300 opacity-0 group-hover:opacity-100 transition-opacity"
									onclick={() => deleteIntegration(integration.id)}
								>
									Delete
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}

	<!-- Bridges Tab -->
	{#if selectedTab === 'bridges'}
		<div class="flex items-center justify-between">
			<h3 class="text-lg font-semibold text-text-primary">Bridge Connections</h3>
			<button class="btn-primary text-sm" onclick={() => showBridgeForm = !showBridgeForm}>
				{showBridgeForm ? 'Cancel' : 'Add Bridge'}
			</button>
		</div>

		{#if showBridgeForm}
			<div class="bg-bg-secondary rounded-lg p-4 space-y-3">
				<div>
					<label class="block text-sm text-text-secondary mb-1">Bridge Type</label>
					<select class="input w-full" bind:value={newBridgeType}>
						{#each bridgeTypes as t}
							<option value={t.value}>{t.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label class="block text-sm text-text-secondary mb-1">AmityVox Channel</label>
					<select class="input w-full" bind:value={newBridgeChannelId}>
						<option value="">Select a channel</option>
						{#each channels.filter(c => c.channel_type === 'text') as ch}
							<option value={ch.id}>#{ch.name}</option>
						{/each}
					</select>
				</div>
				<div>
					<label class="block text-sm text-text-secondary mb-1">
						{#if newBridgeType === 'telegram'}
							Telegram Chat ID
						{:else if newBridgeType === 'slack'}
							Slack Channel ID
						{:else}
							IRC Channel (e.g., #amityvox)
						{/if}
					</label>
					<input
						class="input w-full"
						bind:value={newRemoteId}
						placeholder={newBridgeType === 'irc' ? '#channel' : 'Channel or Chat ID'}
					/>
				</div>
				<button
					class="btn-primary text-sm"
					onclick={createBridge}
					disabled={creatingBridge || !newBridgeChannelId || !newRemoteId.trim()}
				>
					{creatingBridge ? 'Creating...' : 'Create Bridge'}
				</button>
			</div>
		{/if}

		{#if loading}
			<div class="text-text-muted text-sm">Loading bridges...</div>
		{:else if bridges.length === 0}
			<div class="bg-bg-secondary rounded-lg p-6 text-center text-text-muted">
				<p>No bridge connections configured yet.</p>
				<p class="text-sm mt-1">Connect Telegram, Slack, or IRC channels.</p>
			</div>
		{:else}
			<div class="space-y-2">
				{#each bridges as bridge}
					<div class="bg-bg-secondary rounded-lg p-3 flex items-center justify-between group hover:bg-bg-tertiary transition-colors">
						<div class="flex items-center gap-3 flex-1">
							<span class="bg-accent/20 text-accent px-2 py-1 rounded text-xs font-mono">
								{typeIcon(bridge.bridge_type)}
							</span>
							<div>
								<div class="font-medium text-text-primary text-sm">
									#{channelName(bridge.channel_id)}
								</div>
								<div class="text-xs text-text-muted flex items-center gap-2">
									<span>{bridge.remote_id}</span>
									<span class={statusColor(bridge.status)}>{bridge.status}</span>
								</div>
								{#if bridge.last_error}
									<div class="text-xs text-red-400 mt-0.5">{bridge.last_error}</div>
								{/if}
							</div>
						</div>
						<div class="flex items-center gap-2">
							<button
								class="text-xs px-2 py-1 rounded transition-colors
									{bridge.enabled ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30' : 'bg-bg-tertiary text-text-muted hover:bg-bg-primary'}"
								onclick={() => toggleBridge(bridge)}
							>
								{bridge.enabled ? 'On' : 'Off'}
							</button>
							<button
								class="text-xs text-red-400 hover:text-red-300 opacity-0 group-hover:opacity-100 transition-opacity"
								onclick={() => deleteBridge(bridge.id)}
							>
								Delete
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}

	<!-- Message Log Tab -->
	{#if selectedTab === 'log'}
		<h3 class="text-lg font-semibold text-text-primary">Integration Message Log</h3>

		{#if loadingLog}
			<div class="text-text-muted text-sm">Loading log...</div>
		{:else if logEntries.length === 0}
			<div class="bg-bg-secondary rounded-lg p-6 text-center text-text-muted">
				<p>No messages have been relayed yet.</p>
			</div>
		{:else}
			<div class="bg-bg-secondary rounded-lg overflow-hidden">
				<table class="w-full text-sm">
					<thead class="bg-bg-tertiary">
						<tr>
							<th class="text-left px-3 py-2 text-text-muted font-medium">Direction</th>
							<th class="text-left px-3 py-2 text-text-muted font-medium">Channel</th>
							<th class="text-left px-3 py-2 text-text-muted font-medium">Status</th>
							<th class="text-left px-3 py-2 text-text-muted font-medium">Time</th>
						</tr>
					</thead>
					<tbody>
						{#each logEntries as entry}
							<tr class="border-t border-bg-tertiary">
								<td class="px-3 py-2">
									<span class="text-xs px-1.5 py-0.5 rounded {entry.direction === 'inbound' ? 'bg-blue-500/20 text-blue-400' : 'bg-purple-500/20 text-purple-400'}">
										{entry.direction}
									</span>
								</td>
								<td class="px-3 py-2 text-text-secondary">#{channelName(entry.channel_id)}</td>
								<td class="px-3 py-2">
									<span class="text-xs {entry.status === 'delivered' ? 'text-green-400' : entry.status === 'failed' ? 'text-red-400' : 'text-text-muted'}">
										{entry.status}
									</span>
									{#if entry.error_message}
										<span class="text-xs text-red-400 ml-1">({entry.error_message})</span>
									{/if}
								</td>
								<td class="px-3 py-2 text-text-muted text-xs">
									{new Date(entry.created_at).toLocaleString()}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>
