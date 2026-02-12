<script lang="ts">
	import { api } from '$lib/api/client';

	let { guildId, integrationId }: { guildId: string; integrationId: string } = $props();

	// --- Types ---
	interface CalendarConnection {
		id: string;
		integration_id: string;
		provider: string;
		calendar_url: string | null;
		calendar_name: string | null;
		sync_direction: string;
		last_synced_at: string | null;
		created_at: string;
	}

	// --- State ---
	let connections = $state<CalendarConnection[]>([]);
	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	// Add connection form.
	let showAddForm = $state(false);
	let newProvider = $state<string>('ical_url');
	let newCalendarUrl = $state('');
	let newCalendarName = $state('');
	let newSyncDirection = $state<string>('import');
	let adding = $state(false);

	// Sync state.
	let syncing = $state<string | null>(null);

	const providers = [
		{ value: 'ical_url', label: 'iCal URL (read-only)' },
		{ value: 'caldav', label: 'CalDAV Server' },
		{ value: 'google', label: 'Google Calendar' },
	];

	const syncDirections = [
		{ value: 'import', label: 'Import only (calendar to AmityVox)' },
		{ value: 'export', label: 'Export only (AmityVox to calendar)' },
		{ value: 'both', label: 'Bidirectional sync' },
	];

	// --- Data Loading ---
	async function loadConnections() {
		loading = true;
		error = '';
		try {
			connections = await api.request('GET', `/guilds/${guildId}/integrations/${integrationId}/calendar/connections`);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load calendar connections';
		} finally {
			loading = false;
		}
	}

	// --- Actions ---
	async function addConnection() {
		adding = true;
		error = '';
		try {
			const conn: CalendarConnection = await api.request(
				'POST',
				`/guilds/${guildId}/integrations/${integrationId}/calendar/connections`,
				{
					provider: newProvider,
					calendar_url: newCalendarUrl.trim() || null,
					calendar_name: newCalendarName.trim() || null,
					sync_direction: newSyncDirection,
				}
			);
			connections = [conn, ...connections];
			showAddForm = false;
			newCalendarUrl = '';
			newCalendarName = '';
			newProvider = 'ical_url';
			newSyncDirection = 'import';
			success = 'Calendar connection created';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create calendar connection';
		} finally {
			adding = false;
		}
	}

	async function deleteConnection(connectionId: string) {
		if (!confirm('Remove this calendar connection?')) return;
		try {
			await api.request(
				'DELETE',
				`/guilds/${guildId}/integrations/${integrationId}/calendar/connections/${connectionId}`
			);
			connections = connections.filter(c => c.id !== connectionId);
			success = 'Calendar connection removed';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to remove calendar connection';
		}
	}

	async function syncNow(connectionId: string) {
		syncing = connectionId;
		try {
			await api.request(
				'POST',
				`/guilds/${guildId}/integrations/${integrationId}/calendar/connections/${connectionId}/sync`
			);
			// Update last_synced_at in the local state.
			connections = connections.map(c =>
				c.id === connectionId ? { ...c, last_synced_at: new Date().toISOString() } : c
			);
			success = 'Calendar sync triggered';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to trigger sync';
		} finally {
			syncing = null;
		}
	}

	// --- Helpers ---
	function providerLabel(provider: string): string {
		const p = providers.find(pr => pr.value === provider);
		return p?.label || provider;
	}

	function directionLabel(dir: string): string {
		const d = syncDirections.find(sd => sd.value === dir);
		return d?.label || dir;
	}

	function providerIcon(provider: string): string {
		if (provider === 'google') return 'GCal';
		if (provider === 'caldav') return 'DAV';
		return 'iCal';
	}

	function timeAgo(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const date = new Date(dateStr);
		const diff = Date.now() - date.getTime();
		if (diff < 60000) return 'Just now';
		if (diff < 3600000) return `${Math.round(diff / 60000)}m ago`;
		if (diff < 86400000) return `${Math.round(diff / 3600000)}h ago`;
		return `${Math.round(diff / 86400000)}d ago`;
	}

	// Load on mount.
	$effect(() => {
		loadConnections();
	});
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h4 class="font-medium text-text-primary">Calendar Connections</h4>
		<button class="btn-secondary text-xs" onclick={() => showAddForm = !showAddForm}>
			{showAddForm ? 'Cancel' : 'Add Calendar'}
		</button>
	</div>

	{#if error}
		<div class="bg-red-500/10 border border-red-500/30 text-red-400 px-3 py-2 rounded text-sm">{error}</div>
	{/if}
	{#if success}
		<div class="bg-green-500/10 border border-green-500/30 text-green-400 px-3 py-2 rounded text-sm">{success}</div>
	{/if}

	{#if showAddForm}
		<div class="bg-bg-tertiary rounded-lg p-4 space-y-3">
			<div>
				<label class="block text-sm text-text-secondary mb-1">Provider</label>
				<select class="input w-full" bind:value={newProvider}>
					{#each providers as p}
						<option value={p.value}>{p.label}</option>
					{/each}
				</select>
			</div>

			{#if newProvider === 'google'}
				<div class="bg-bg-primary rounded p-3 text-sm text-text-muted">
					Google Calendar integration requires OAuth setup. After creating this connection,
					you will need to authorize access through Google's consent screen. Contact your
					instance administrator to configure OAuth credentials.
				</div>
			{/if}

			{#if newProvider === 'ical_url' || newProvider === 'caldav'}
				<div>
					<label class="block text-sm text-text-secondary mb-1">
						{newProvider === 'ical_url' ? 'iCal Feed URL' : 'CalDAV Server URL'}
					</label>
					<input
						class="input w-full"
						bind:value={newCalendarUrl}
						placeholder={newProvider === 'ical_url'
							? 'https://calendar.example.com/feed.ics'
							: 'https://calendar.example.com/dav/'}
						type="url"
					/>
				</div>
			{/if}

			<div>
				<label class="block text-sm text-text-secondary mb-1">Calendar Name (optional)</label>
				<input
					class="input w-full"
					bind:value={newCalendarName}
					placeholder="Team Calendar"
					maxlength="100"
				/>
			</div>

			<div>
				<label class="block text-sm text-text-secondary mb-1">Sync Direction</label>
				<select class="input w-full" bind:value={newSyncDirection}>
					{#each syncDirections as d}
						<option value={d.value}>{d.label}</option>
					{/each}
				</select>
				{#if newProvider === 'ical_url' && newSyncDirection !== 'import'}
					<p class="text-xs text-yellow-400 mt-1">iCal URLs are read-only. Only import is supported.</p>
				{/if}
			</div>

			<button
				class="btn-primary text-sm"
				onclick={addConnection}
				disabled={adding}
			>
				{adding ? 'Creating...' : 'Add Calendar Connection'}
			</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-text-muted text-sm">Loading calendar connections...</div>
	{:else if connections.length === 0}
		<div class="bg-bg-tertiary rounded-lg p-4 text-center text-text-muted text-sm">
			No calendar connections configured. Connect a Google Calendar, CalDAV server, or iCal feed.
		</div>
	{:else}
		<div class="space-y-2">
			{#each connections as conn}
				<div class="bg-bg-tertiary rounded-lg p-3 space-y-2">
					<div class="flex items-start justify-between">
						<div class="flex items-center gap-2">
							<span class="bg-accent/20 text-accent px-1.5 py-0.5 rounded text-xs font-mono">
								{providerIcon(conn.provider)}
							</span>
							<div>
								<div class="font-medium text-text-primary text-sm">
									{conn.calendar_name || providerLabel(conn.provider)}
								</div>
								{#if conn.calendar_url}
									<div class="text-xs text-text-muted truncate max-w-sm">{conn.calendar_url}</div>
								{/if}
							</div>
						</div>
						<div class="flex items-center gap-2">
							<button
								class="btn-secondary text-xs"
								onclick={() => syncNow(conn.id)}
								disabled={syncing === conn.id}
							>
								{syncing === conn.id ? 'Syncing...' : 'Sync Now'}
							</button>
							<button
								class="text-xs text-red-400 hover:text-red-300"
								onclick={() => deleteConnection(conn.id)}
							>
								Remove
							</button>
						</div>
					</div>

					<div class="flex flex-wrap gap-3 text-xs text-text-muted">
						<span>{directionLabel(conn.sync_direction)}</span>
						<span>Last synced: {timeAgo(conn.last_synced_at)}</span>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<div class="text-xs text-text-muted">
		Calendar events will be synced with guild events. Import creates AmityVox events from your
		calendar. Export sends guild events to your external calendar. The sync worker runs periodically.
	</div>
</div>
