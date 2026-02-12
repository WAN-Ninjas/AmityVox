<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	// Typed fetch helpers using the existing api client's token.
	async function adminGet<T>(path: string): Promise<T> {
		const token = api.getToken();
		const res = await fetch(`/api/v1${path}`, {
			headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
			throw new Error(err.error?.message || res.statusText);
		}
		const json = await res.json();
		return json.data as T;
	}
	async function adminPost<T>(path: string, body?: unknown): Promise<T> {
		const token = api.getToken();
		const res = await fetch(`/api/v1${path}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
			body: body ? JSON.stringify(body) : undefined,
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
			throw new Error(err.error?.message || res.statusText);
		}
		if (res.status === 204) return undefined as T;
		const json = await res.json();
		return json.data as T;
	}
	async function adminPatch<T>(path: string, body?: unknown): Promise<T> {
		const token = api.getToken();
		const res = await fetch(`/api/v1${path}`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
			body: body ? JSON.stringify(body) : undefined,
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
			throw new Error(err.error?.message || res.statusText);
		}
		if (res.status === 204) return undefined as T;
		const json = await res.json();
		return json.data as T;
	}
	async function adminDel(path: string): Promise<void> {
		const token = api.getToken();
		const res = await fetch(`/api/v1${path}`, {
			method: 'DELETE',
			headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) }
		});
		if (!res.ok && res.status !== 204) {
			const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
			throw new Error(err.error?.message || res.statusText);
		}
	}

	// --- Types ---
	interface BridgeConfig {
		id: string;
		bridge_type: string;
		enabled: boolean;
		display_name: string;
		config: Record<string, unknown>;
		status: string;
		last_sync_at: string | null;
		error_message: string | null;
		channel_count: number;
		virtual_user_count: number;
		created_at: string;
		updated_at: string;
	}

	interface ChannelMapping {
		id: string;
		local_channel_id: string;
		local_channel_name: string | null;
		remote_channel_id: string;
		remote_channel_name: string | null;
		direction: string;
		active: boolean;
		last_message_at: string | null;
		message_count: number;
		created_at: string;
	}

	interface VirtualUser {
		id: string;
		remote_user_id: string;
		remote_username: string;
		remote_avatar: string | null;
		platform: string;
		last_active_at: string | null;
		created_at: string;
	}

	const BRIDGE_TYPES = [
		{ id: 'matrix', name: 'Matrix', icon: 'M', description: 'Bridge to Matrix/Element rooms via Appservice' },
		{ id: 'discord', name: 'Discord', icon: 'D', description: 'Bridge to Discord channels via Bot' },
		{ id: 'telegram', name: 'Telegram', icon: 'T', description: 'Bridge to Telegram groups and channels' },
		{ id: 'slack', name: 'Slack', icon: 'S', description: 'Bridge to Slack workspaces and channels' },
		{ id: 'irc', name: 'IRC', icon: '#', description: 'Bridge to IRC networks and channels' },
		{ id: 'xmpp', name: 'XMPP', icon: 'X', description: 'Bridge to XMPP/Jabber MUCs' },
	];

	// --- State ---
	let bridges = $state<BridgeConfig[]>([]);
	let loading = $state(true);
	let error = $state('');

	let selectedBridge = $state<BridgeConfig | null>(null);
	let channelMappings = $state<ChannelMapping[]>([]);
	let virtualUsers = $state<VirtualUser[]>([]);
	let loadingMappings = $state(false);
	let loadingUsers = $state(false);

	// Create bridge form
	let showCreateForm = $state(false);
	let newBridgeType = $state('matrix');
	let newBridgeName = $state('');
	let creating = $state(false);

	// Add mapping form
	let showAddMapping = $state(false);
	let newLocalChannel = $state('');
	let newRemoteChannel = $state('');
	let newRemoteName = $state('');
	let newDirection = $state('bidirectional');
	let addingMapping = $state(false);

	// --- Data loading ---
	async function loadBridges() {
		loading = true;
		error = '';
		try {
			bridges = await adminGet<BridgeConfig[]>('/admin/bridges');
		} catch (e: any) {
			error = e.message || 'Failed to load bridges';
		} finally {
			loading = false;
		}
	}

	async function loadBridgeDetails(bridge: BridgeConfig) {
		selectedBridge = bridge;
		loadingMappings = true;
		loadingUsers = true;

		try {
			channelMappings = await adminGet<ChannelMapping[]>(`/admin/bridges/${bridge.id}/mappings`);
		} catch {
			channelMappings = [];
		} finally {
			loadingMappings = false;
		}

		try {
			virtualUsers = await adminGet<VirtualUser[]>(`/admin/bridges/${bridge.id}/virtual-users`);
		} catch {
			virtualUsers = [];
		} finally {
			loadingUsers = false;
		}
	}

	// --- Actions ---
	async function createBridge() {
		creating = true;
		try {
			await adminPost('/admin/bridges', {
				bridge_type: newBridgeType,
				display_name: newBridgeName || newBridgeType,
			});
			addToast('Bridge created successfully', 'success');
			showCreateForm = false;
			newBridgeName = '';
			await loadBridges();
		} catch (e: any) {
			addToast('Failed to create bridge: ' + e.message, 'error');
		} finally {
			creating = false;
		}
	}

	async function toggleBridge(bridge: BridgeConfig) {
		try {
			await adminPatch(`/admin/bridges/${bridge.id}`, { enabled: !bridge.enabled });
			addToast(`Bridge ${bridge.enabled ? 'disabled' : 'enabled'}`, 'success');
			await loadBridges();
			if (selectedBridge?.id === bridge.id) {
				selectedBridge = { ...bridge, enabled: !bridge.enabled };
			}
		} catch (e: any) {
			addToast('Failed to toggle bridge: ' + e.message, 'error');
		}
	}

	async function deleteBridge(bridgeId: string) {
		if (!confirm('Are you sure? This will remove all channel mappings and virtual users.')) return;
		try {
			await adminDel(`/admin/bridges/${bridgeId}`);
			addToast('Bridge deleted', 'success');
			if (selectedBridge?.id === bridgeId) selectedBridge = null;
			await loadBridges();
		} catch (e: any) {
			addToast('Failed to delete bridge: ' + e.message, 'error');
		}
	}

	async function addChannelMapping() {
		if (!selectedBridge || !newLocalChannel || !newRemoteChannel) return;
		addingMapping = true;
		try {
			await adminPost(`/admin/bridges/${selectedBridge.id}/mappings`, {
				local_channel_id: newLocalChannel,
				remote_channel_id: newRemoteChannel,
				remote_channel_name: newRemoteName || undefined,
				direction: newDirection,
			});
			addToast('Channel mapping added', 'success');
			showAddMapping = false;
			newLocalChannel = '';
			newRemoteChannel = '';
			newRemoteName = '';
			await loadBridgeDetails(selectedBridge);
		} catch (e: any) {
			addToast('Failed to add mapping: ' + e.message, 'error');
		} finally {
			addingMapping = false;
		}
	}

	async function deleteMapping(mappingId: string) {
		if (!selectedBridge) return;
		try {
			await adminDel(`/admin/bridges/${selectedBridge.id}/mappings/${mappingId}`);
			addToast('Mapping removed', 'success');
			await loadBridgeDetails(selectedBridge);
		} catch (e: any) {
			addToast('Failed to remove mapping: ' + e.message, 'error');
		}
	}

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		return new Date(dateStr).toLocaleString();
	}

	function bridgeIcon(type: string): string {
		return BRIDGE_TYPES.find(b => b.id === type)?.icon || '?';
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'connected': return 'bg-green-500/20 text-green-400';
			case 'disconnected': return 'bg-bg-modifier text-text-muted';
			case 'error': return 'bg-red-500/20 text-red-400';
			case 'configuring': return 'bg-yellow-500/20 text-yellow-400';
			default: return 'bg-bg-modifier text-text-muted';
		}
	}

	function directionLabel(dir: string): string {
		switch (dir) {
			case 'bidirectional': return 'Bidirectional';
			case 'inbound': return 'Inbound only';
			case 'outbound': return 'Outbound only';
			default: return dir;
		}
	}

	onMount(loadBridges);
</script>

<div class="flex-1 overflow-y-auto p-6">
	<div class="max-w-6xl mx-auto">
		<!-- Header -->
		<div class="mb-6">
			<a href="/app/admin" class="text-sm text-text-muted hover:text-text-primary mb-2 inline-block">
				&larr; Back to Admin
			</a>
			<div class="flex items-center justify-between">
				<div>
					<h1 class="text-2xl font-bold text-text-primary">Bridge Management</h1>
					<p class="text-text-muted mt-1">Connect AmityVox to external platforms via bridges.</p>
				</div>
				<button
					class="btn-primary"
					onclick={() => showCreateForm = !showCreateForm}
				>
					{showCreateForm ? 'Cancel' : 'Add Bridge'}
				</button>
			</div>
		</div>

		<!-- Create Bridge Form -->
		{#if showCreateForm}
			<div class="bg-bg-secondary p-6 rounded-lg mb-6">
				<h3 class="text-text-primary font-medium mb-4">Add New Bridge</h3>
				<div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
					{#each BRIDGE_TYPES as bt}
						<button
							class="p-4 rounded-lg border-2 text-left transition-colors {newBridgeType === bt.id
								? 'border-brand-500 bg-brand-500/10'
								: 'border-border-primary bg-bg-tertiary hover:border-text-muted'}"
							onclick={() => newBridgeType = bt.id}
						>
							<div class="flex items-center gap-3 mb-2">
								<div class="w-8 h-8 rounded bg-bg-modifier flex items-center justify-center text-text-primary font-bold text-sm">
									{bt.icon}
								</div>
								<span class="font-medium text-text-primary">{bt.name}</span>
							</div>
							<p class="text-xs text-text-muted">{bt.description}</p>
						</button>
					{/each}
				</div>
				<div class="flex items-end gap-4">
					<div class="flex-1">
						<label class="block text-sm text-text-muted mb-1">Display Name</label>
						<input
							type="text"
							class="input w-full"
							placeholder={`My ${BRIDGE_TYPES.find(b => b.id === newBridgeType)?.name} Bridge`}
							bind:value={newBridgeName}
						/>
					</div>
					<button
						class="btn-primary"
						onclick={createBridge}
						disabled={creating}
					>
						{creating ? 'Creating...' : 'Create Bridge'}
					</button>
				</div>
			</div>
		{/if}

		{#if loading}
			<div class="flex items-center justify-center py-16">
				<div class="animate-spin h-8 w-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
			</div>
		{:else if error}
			<div class="bg-red-500/10 text-red-400 p-4 rounded-lg">{error}</div>
		{:else}
			<div class="flex gap-6">
				<!-- Bridge List -->
				<div class="w-80 shrink-0 space-y-3">
					{#if bridges.length === 0}
						<div class="bg-bg-secondary p-6 rounded-lg text-center text-text-muted text-sm">
							No bridges configured. Click "Add Bridge" to get started.
						</div>
					{:else}
						{#each bridges as bridge}
							<button
								class="w-full p-4 rounded-lg text-left transition-colors {selectedBridge?.id === bridge.id
									? 'bg-brand-500/10 border border-brand-500/30'
									: 'bg-bg-secondary hover:bg-bg-modifier border border-transparent'}"
								onclick={() => loadBridgeDetails(bridge)}
							>
								<div class="flex items-center gap-3 mb-2">
									<div class="w-10 h-10 rounded-lg bg-bg-tertiary flex items-center justify-center text-text-primary font-bold">
										{bridgeIcon(bridge.bridge_type)}
									</div>
									<div class="flex-1 min-w-0">
										<div class="font-medium text-text-primary truncate">{bridge.display_name}</div>
										<div class="text-xs text-text-muted capitalize">{bridge.bridge_type}</div>
									</div>
								</div>
								<div class="flex items-center justify-between text-xs">
									<span class="px-2 py-0.5 rounded {statusColor(bridge.status)}">{bridge.status}</span>
									<span class="text-text-muted">
										{bridge.channel_count} channel{bridge.channel_count !== 1 ? 's' : ''}
									</span>
								</div>
							</button>
						{/each}
					{/if}
				</div>

				<!-- Bridge Detail -->
				<div class="flex-1">
					{#if !selectedBridge}
						<div class="bg-bg-secondary p-12 rounded-lg text-center text-text-muted">
							Select a bridge from the list to view details.
						</div>
					{:else}
						<div class="space-y-6">
							<!-- Bridge Header -->
							<div class="bg-bg-secondary p-6 rounded-lg">
								<div class="flex items-center justify-between mb-4">
									<div class="flex items-center gap-4">
										<div class="w-12 h-12 rounded-lg bg-bg-tertiary flex items-center justify-center text-text-primary font-bold text-lg">
											{bridgeIcon(selectedBridge.bridge_type)}
										</div>
										<div>
											<h2 class="text-lg font-semibold text-text-primary">{selectedBridge.display_name}</h2>
											<div class="flex items-center gap-2 mt-1">
												<span class="px-2 py-0.5 rounded text-xs {statusColor(selectedBridge.status)}">{selectedBridge.status}</span>
												<span class="text-xs text-text-muted capitalize">{selectedBridge.bridge_type}</span>
											</div>
										</div>
									</div>
									<div class="flex gap-2">
										<button
											class="px-4 py-2 text-sm rounded transition-colors {selectedBridge.enabled
												? 'bg-yellow-500/20 text-yellow-400 hover:bg-yellow-500/30'
												: 'bg-green-500/20 text-green-400 hover:bg-green-500/30'}"
											onclick={() => toggleBridge(selectedBridge!)}
										>
											{selectedBridge.enabled ? 'Disable' : 'Enable'}
										</button>
										<button
											class="px-4 py-2 text-sm bg-red-500/20 text-red-400 rounded hover:bg-red-500/30 transition-colors"
											onclick={() => deleteBridge(selectedBridge!.id)}
										>
											Delete
										</button>
									</div>
								</div>

								{#if selectedBridge.error_message}
									<div class="bg-red-500/10 text-red-400 p-3 rounded text-sm mb-4">
										{selectedBridge.error_message}
									</div>
								{/if}

								<div class="grid grid-cols-3 gap-4 text-sm">
									<div>
										<span class="text-text-muted">Channels:</span>
										<span class="text-text-primary ml-1">{selectedBridge.channel_count}</span>
									</div>
									<div>
										<span class="text-text-muted">Virtual Users:</span>
										<span class="text-text-primary ml-1">{selectedBridge.virtual_user_count}</span>
									</div>
									<div>
										<span class="text-text-muted">Last Sync:</span>
										<span class="text-text-primary ml-1">{formatDate(selectedBridge.last_sync_at)}</span>
									</div>
								</div>
							</div>

							<!-- Channel Mappings -->
							<div class="bg-bg-secondary p-6 rounded-lg">
								<div class="flex items-center justify-between mb-4">
									<h3 class="text-text-primary font-medium">Channel Mappings</h3>
									<button
										class="text-sm text-brand-400 hover:text-brand-300"
										onclick={() => showAddMapping = !showAddMapping}
									>
										{showAddMapping ? 'Cancel' : '+ Add Mapping'}
									</button>
								</div>

								{#if showAddMapping}
									<div class="bg-bg-tertiary p-4 rounded mb-4 space-y-3">
										<div class="grid grid-cols-2 gap-3">
											<div>
												<label class="block text-xs text-text-muted mb-1">Local Channel ID</label>
												<input type="text" class="input w-full text-sm" placeholder="Channel ULID" bind:value={newLocalChannel} />
											</div>
											<div>
												<label class="block text-xs text-text-muted mb-1">Remote Channel ID</label>
												<input type="text" class="input w-full text-sm" placeholder="e.g. !room:matrix.org" bind:value={newRemoteChannel} />
											</div>
										</div>
										<div class="grid grid-cols-2 gap-3">
											<div>
												<label class="block text-xs text-text-muted mb-1">Remote Channel Name</label>
												<input type="text" class="input w-full text-sm" placeholder="Optional display name" bind:value={newRemoteName} />
											</div>
											<div>
												<label class="block text-xs text-text-muted mb-1">Direction</label>
												<select class="input w-full text-sm" bind:value={newDirection}>
													<option value="bidirectional">Bidirectional</option>
													<option value="inbound">Inbound Only</option>
													<option value="outbound">Outbound Only</option>
												</select>
											</div>
										</div>
										<button
											class="btn-primary text-sm"
											onclick={addChannelMapping}
											disabled={addingMapping || !newLocalChannel || !newRemoteChannel}
										>
											{addingMapping ? 'Adding...' : 'Add Mapping'}
										</button>
									</div>
								{/if}

								{#if loadingMappings}
									<div class="flex items-center justify-center py-4">
										<div class="animate-spin h-5 w-5 border-2 border-brand-500 border-t-transparent rounded-full"></div>
									</div>
								{:else if channelMappings.length === 0}
									<p class="text-text-muted text-sm">No channel mappings configured.</p>
								{:else}
									<div class="space-y-2">
										{#each channelMappings as mapping}
											<div class="bg-bg-tertiary p-3 rounded flex items-center justify-between">
												<div class="flex items-center gap-4 text-sm">
													<div>
														<span class="text-text-muted">Local:</span>
														<span class="text-text-primary ml-1">{mapping.local_channel_name || mapping.local_channel_id.slice(0, 12)}</span>
													</div>
													<span class="text-text-muted">{mapping.direction === 'bidirectional' ? '<->' : mapping.direction === 'inbound' ? '<-' : '->'}</span>
													<div>
														<span class="text-text-muted">Remote:</span>
														<span class="text-text-primary ml-1">{mapping.remote_channel_name || mapping.remote_channel_id}</span>
													</div>
												</div>
												<div class="flex items-center gap-3">
													<span class="text-xs text-text-muted">{mapping.message_count.toLocaleString()} msgs</span>
													<button
														class="text-xs text-red-400 hover:text-red-300"
														onclick={() => deleteMapping(mapping.id)}
													>
														Remove
													</button>
												</div>
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Virtual Users -->
							<div class="bg-bg-secondary p-6 rounded-lg">
								<h3 class="text-text-primary font-medium mb-4">Virtual Users (Puppets)</h3>
								{#if loadingUsers}
									<div class="flex items-center justify-center py-4">
										<div class="animate-spin h-5 w-5 border-2 border-brand-500 border-t-transparent rounded-full"></div>
									</div>
								{:else if virtualUsers.length === 0}
									<p class="text-text-muted text-sm">No virtual users created yet. They appear automatically when remote users send messages.</p>
								{:else}
									<div class="grid grid-cols-1 md:grid-cols-2 gap-2">
										{#each virtualUsers as vu}
											<div class="bg-bg-tertiary p-3 rounded flex items-center gap-3">
												<div class="w-8 h-8 rounded-full bg-bg-modifier flex items-center justify-center text-xs font-medium text-text-primary">
													{vu.remote_username.charAt(0).toUpperCase()}
												</div>
												<div class="flex-1 min-w-0">
													<div class="text-sm text-text-primary truncate">{vu.remote_username}</div>
													<div class="text-xs text-text-muted">{vu.platform} | Last active: {formatDate(vu.last_active_at)}</div>
												</div>
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<!-- Bridge-Specific Info -->
							<div class="bg-bg-secondary p-6 rounded-lg">
								<h3 class="text-text-primary font-medium mb-3">Bridge Information</h3>
								<div class="text-sm space-y-2">
									{#if selectedBridge.bridge_type === 'matrix'}
										<p class="text-text-muted">
											The Matrix bridge uses the Application Service (Appservice) API to relay messages
											between AmityVox channels and Matrix rooms. Configure your homeserver to register
											the appservice using the config section above.
										</p>
									{:else if selectedBridge.bridge_type === 'discord'}
										<p class="text-text-muted">
											The Discord bridge uses a bot token to connect to Discord and relay messages
											between mapped channels. The bot must be invited to the Discord server with
											appropriate permissions (Read Messages, Send Messages, Manage Webhooks).
										</p>
									{:else if selectedBridge.bridge_type === 'telegram'}
										<p class="text-text-muted">
											The Telegram bridge connects to the Telegram Bot API. Create a bot via @BotFather
											and add it to your target groups. Configure the bot token in the bridge settings.
										</p>
									{:else if selectedBridge.bridge_type === 'slack'}
										<p class="text-text-muted">
											The Slack bridge uses the Slack Events API and Web API. Create a Slack app with
											the necessary scopes (channels:read, chat:write, users:read) and configure the
											OAuth token and signing secret.
										</p>
									{:else if selectedBridge.bridge_type === 'irc'}
										<p class="text-text-muted">
											The IRC bridge connects to an IRC network and joins configured channels. Messages
											are relayed bidirectionally. Configure the IRC server, port, nickname, and channels.
										</p>
									{:else if selectedBridge.bridge_type === 'xmpp'}
										<p class="text-text-muted">
											The XMPP bridge connects to XMPP Multi-User Chat (MUC) rooms. Configure the
											JID, password, and conference server to relay messages between platforms.
										</p>
									{/if}
									<div class="mt-3 pt-3 border-t border-border-primary text-xs text-text-muted">
										Created: {formatDate(selectedBridge.created_at)} | Updated: {formatDate(selectedBridge.updated_at)}
									</div>
								</div>
							</div>
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>
