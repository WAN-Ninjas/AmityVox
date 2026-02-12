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
	async function adminPut<T>(path: string, body?: unknown): Promise<T> {
		const token = api.getToken();
		const res = await fetch(`/api/v1${path}`, {
			method: 'PUT',
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

	// --- Types ---
	interface PeerHealth {
		peer_id: string;
		peer_domain: string;
		peer_name: string | null;
		peer_software: string;
		federation_status: string;
		health_status: string;
		last_sync_at: string | null;
		last_event_at: string | null;
		event_lag_ms: number;
		events_sent: number;
		events_received: number;
		errors_24h: number;
		version: string | null;
		capabilities: string[];
		established_at: string;
	}

	interface DashboardData {
		peers: PeerHealth[];
		federation_mode: string;
		total_peers: number;
		active_peers: number;
		blocked_peers: number;
		degraded_peers: number;
		pending_deliveries: number;
		failed_deliveries: number;
		total_deliveries: number;
	}

	interface PeerControl {
		id: string;
		peer_id: string;
		peer_domain: string;
		peer_name: string | null;
		action: string;
		reason: string | null;
		created_by: string;
		created_at: string;
	}

	interface DeliveryReceipt {
		id: string;
		message_id: string;
		source_instance: string;
		target_instance: string;
		status: string;
		attempts: number;
		last_attempt_at: string | null;
		delivered_at: string | null;
		error_message: string | null;
		created_at: string;
	}

	interface SearchConfig {
		enabled: boolean;
		index_outgoing: boolean;
		index_incoming: boolean;
		allowed_peers: string[];
	}

	interface ProtocolInfo {
		protocol_version: string;
		capabilities: string[];
		supported_protocols: string[];
		default_capabilities: string[];
	}

	type Tab = 'overview' | 'controls' | 'delivery' | 'search' | 'protocol' | 'blocklist';

	// --- State ---
	let currentTab = $state<Tab>('overview');
	let loading = $state(true);
	let error = $state('');

	let dashboard = $state<DashboardData | null>(null);
	let controls = $state<PeerControl[]>([]);
	let blockedPeers = $derived(controls.filter(c => c.action === 'block'));
	let allowedPeers = $derived(controls.filter(c => c.action === 'allow'));
	let deliveryReceipts = $state<DeliveryReceipt[]>([]);
	let searchConfig = $state<SearchConfig>({ enabled: false, index_outgoing: false, index_incoming: false, allowed_peers: [] });
	let protocolInfo = $state<ProtocolInfo | null>(null);

	let loadingControls = $state(false);
	let loadingDelivery = $state(false);
	let savingSearch = $state(false);
	let deliveryFilter = $state('');

	// --- Data loading ---
	async function loadDashboard() {
		loading = true;
		error = '';
		try {
			dashboard = await adminGet<DashboardData>('/admin/federation/dashboard');
		} catch (e: any) {
			error = e.message || 'Failed to load federation dashboard';
		} finally {
			loading = false;
		}
	}

	async function loadControls() {
		loadingControls = true;
		try {
			controls = await adminGet<PeerControl[]>('/admin/federation/peers/controls');
		} catch (e: any) {
			addToast('Failed to load peer controls: ' + e.message, 'error');
		} finally {
			loadingControls = false;
		}
	}

	async function loadDeliveryReceipts() {
		loadingDelivery = true;
		try {
			const path = deliveryFilter
				? `/admin/federation/delivery-receipts?status=${deliveryFilter}`
				: '/admin/federation/delivery-receipts';
			deliveryReceipts = await adminGet<DeliveryReceipt[]>(path);
		} catch (e: any) {
			addToast('Failed to load delivery receipts: ' + e.message, 'error');
		} finally {
			loadingDelivery = false;
		}
	}

	async function loadSearchConfig() {
		try {
			searchConfig = await adminGet<SearchConfig>('/admin/federation/search-config');
		} catch {
			// Use defaults.
		}
	}

	async function loadProtocol() {
		try {
			protocolInfo = await adminGet<ProtocolInfo>('/admin/federation/protocol');
		} catch (e: any) {
			addToast('Failed to load protocol info: ' + e.message, 'error');
		}
	}

	// --- Actions ---
	async function updatePeerControl(peerId: string, action: string, reason?: string) {
		try {
			await adminPut(`/admin/federation/peers/${peerId}/control`, { action, reason });
			addToast(`Peer ${action === 'block' ? 'blocked' : action === 'allow' ? 'allowed' : 'muted'} successfully`, 'success');
			await loadDashboard();
			await loadControls();
		} catch (e: any) {
			addToast('Failed to update peer control: ' + e.message, 'error');
		}
	}

	async function retryDelivery(receiptId: string) {
		try {
			await adminPost(`/admin/federation/delivery-receipts/${receiptId}/retry`);
			addToast('Retry queued', 'success');
			await loadDeliveryReceipts();
		} catch (e: any) {
			addToast('Failed to retry: ' + e.message, 'error');
		}
	}

	async function saveSearchConfig() {
		savingSearch = true;
		try {
			await adminPatch('/admin/federation/search-config', searchConfig);
			addToast('Search config updated', 'success');
		} catch (e: any) {
			addToast('Failed to save search config: ' + e.message, 'error');
		} finally {
			savingSearch = false;
		}
	}

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const d = new Date(dateStr);
		return d.toLocaleString();
	}

	function formatLag(ms: number): string {
		if (ms < 1000) return `${ms}ms`;
		if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
		return `${(ms / 60000).toFixed(1)}m`;
	}

	function healthColor(status: string): string {
		switch (status) {
			case 'healthy': return 'text-status-online';
			case 'degraded': return 'text-status-idle';
			case 'unreachable': return 'text-status-dnd';
			default: return 'text-text-muted';
		}
	}

	function statusBadge(status: string): string {
		switch (status) {
			case 'active': return 'bg-green-500/20 text-green-400';
			case 'blocked': return 'bg-red-500/20 text-red-400';
			case 'pending': return 'bg-yellow-500/20 text-yellow-400';
			default: return 'bg-bg-modifier text-text-muted';
		}
	}

	onMount(() => {
		loadDashboard();
	});

	// Load tab-specific data.
	$effect(() => {
		if (currentTab === 'controls') loadControls();
		if (currentTab === 'delivery') loadDeliveryReceipts();
		if (currentTab === 'search') loadSearchConfig();
		if (currentTab === 'protocol') loadProtocol();
		if (currentTab === 'blocklist') loadControls();
	});
</script>

<div class="flex-1 overflow-y-auto p-6">
	<div class="max-w-6xl mx-auto">
		<!-- Header -->
		<div class="mb-6">
			<a href="/app/admin" class="text-sm text-text-muted hover:text-text-primary mb-2 inline-block">
				&larr; Back to Admin
			</a>
			<h1 class="text-2xl font-bold text-text-primary">Federation Dashboard</h1>
			<p class="text-text-muted mt-1">Manage federation peers, delivery, search, and protocol settings.</p>
		</div>

		<!-- Tab Navigation -->
		<div class="flex gap-1 mb-6 border-b border-border-primary">
			{#each [
				['overview', 'Overview'],
				['controls', 'Peer Controls'],
				['delivery', 'Delivery'],
				['search', 'Federated Search'],
				['protocol', 'Protocol'],
				['blocklist', 'Block/Allow Lists']
			] as [tab, label]}
				<button
					class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {currentTab === tab
						? 'border-brand-500 text-brand-400'
						: 'border-transparent text-text-muted hover:text-text-primary'}"
					onclick={() => currentTab = tab as Tab}
				>
					{label}
				</button>
			{/each}
		</div>

		{#if loading && currentTab === 'overview'}
			<div class="flex items-center justify-center py-16">
				<div class="animate-spin h-8 w-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
			</div>
		{:else if error}
			<div class="bg-red-500/10 text-red-400 p-4 rounded-lg">{error}</div>
		{:else if currentTab === 'overview' && dashboard}
			<!-- Stats Cards -->
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Total Peers</div>
					<div class="text-2xl font-bold text-text-primary">{dashboard.total_peers}</div>
				</div>
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Active</div>
					<div class="text-2xl font-bold text-status-online">{dashboard.active_peers}</div>
				</div>
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Blocked</div>
					<div class="text-2xl font-bold text-status-dnd">{dashboard.blocked_peers}</div>
				</div>
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Degraded</div>
					<div class="text-2xl font-bold text-status-idle">{dashboard.degraded_peers}</div>
				</div>
			</div>

			<!-- Delivery Stats -->
			<div class="grid grid-cols-3 gap-4 mb-6">
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Pending Deliveries</div>
					<div class="text-xl font-bold text-text-primary">{dashboard.pending_deliveries}</div>
				</div>
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Failed Deliveries</div>
					<div class="text-xl font-bold text-status-dnd">{dashboard.failed_deliveries}</div>
				</div>
				<div class="bg-bg-secondary p-4 rounded-lg">
					<div class="text-text-muted text-xs uppercase">Total Deliveries</div>
					<div class="text-xl font-bold text-text-primary">{dashboard.total_deliveries}</div>
				</div>
			</div>

			<!-- Federation Mode -->
			<div class="bg-bg-secondary p-4 rounded-lg mb-6">
				<div class="flex items-center gap-3">
					<span class="text-text-muted text-sm">Federation Mode:</span>
					<span class="px-3 py-1 rounded-full text-sm font-medium {dashboard.federation_mode === 'open'
						? 'bg-green-500/20 text-green-400'
						: dashboard.federation_mode === 'allowlist'
						? 'bg-yellow-500/20 text-yellow-400'
						: 'bg-red-500/20 text-red-400'}">
						{dashboard.federation_mode}
					</span>
				</div>
			</div>

			<!-- Peer List -->
			<h2 class="text-lg font-semibold text-text-primary mb-3">Connected Peers</h2>
			{#if dashboard.peers.length === 0}
				<div class="bg-bg-secondary p-8 rounded-lg text-center text-text-muted">
					No federation peers connected.
				</div>
			{:else}
				<div class="space-y-3">
					{#each dashboard.peers as peer}
						<div class="bg-bg-secondary p-4 rounded-lg">
							<div class="flex items-center justify-between mb-2">
								<div class="flex items-center gap-3">
									<div class="w-2 h-2 rounded-full {peer.health_status === 'healthy'
										? 'bg-status-online'
										: peer.health_status === 'degraded'
										? 'bg-status-idle'
										: 'bg-status-dnd'}"></div>
									<div>
										<span class="font-medium text-text-primary">{peer.peer_domain}</span>
										{#if peer.peer_name}
											<span class="text-text-muted ml-2">({peer.peer_name})</span>
										{/if}
									</div>
								</div>
								<div class="flex items-center gap-2">
									<span class="px-2 py-0.5 rounded text-xs {statusBadge(peer.federation_status)}">
										{peer.federation_status}
									</span>
									<span class="{healthColor(peer.health_status)} text-xs">{peer.health_status}</span>
								</div>
							</div>
							<div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
								<div>
									<span class="text-text-muted">Software:</span>
									<span class="text-text-secondary ml-1">{peer.peer_software}</span>
								</div>
								<div>
									<span class="text-text-muted">Event Lag:</span>
									<span class="text-text-secondary ml-1">{formatLag(peer.event_lag_ms)}</span>
								</div>
								<div>
									<span class="text-text-muted">Last Sync:</span>
									<span class="text-text-secondary ml-1">{formatDate(peer.last_sync_at)}</span>
								</div>
								<div>
									<span class="text-text-muted">Errors (24h):</span>
									<span class="ml-1 {peer.errors_24h > 0 ? 'text-status-dnd' : 'text-text-secondary'}">{peer.errors_24h}</span>
								</div>
							</div>
							<div class="flex items-center gap-4 mt-3 text-xs text-text-muted">
								<span>Sent: {peer.events_sent.toLocaleString()}</span>
								<span>Received: {peer.events_received.toLocaleString()}</span>
								{#if peer.version}
									<span>Protocol: {peer.version}</span>
								{/if}
								<span>Since: {formatDate(peer.established_at)}</span>
							</div>
							<!-- Quick Actions -->
							<div class="flex gap-2 mt-3">
								{#if peer.federation_status !== 'blocked'}
									<button
										class="px-3 py-1 text-xs bg-red-500/20 text-red-400 rounded hover:bg-red-500/30 transition-colors"
										onclick={() => updatePeerControl(peer.peer_id, 'block')}
									>
										Block
									</button>
								{:else}
									<button
										class="px-3 py-1 text-xs bg-green-500/20 text-green-400 rounded hover:bg-green-500/30 transition-colors"
										onclick={() => updatePeerControl(peer.peer_id, 'allow')}
									>
										Unblock
									</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

		{:else if currentTab === 'controls'}
			<h2 class="text-lg font-semibold text-text-primary mb-4">Peer Access Controls</h2>
			{#if loadingControls}
				<div class="flex items-center justify-center py-8">
					<div class="animate-spin h-6 w-6 border-2 border-brand-500 border-t-transparent rounded-full"></div>
				</div>
			{:else if controls.length === 0}
				<div class="bg-bg-secondary p-8 rounded-lg text-center text-text-muted">
					No peer-specific controls configured. Peers follow the instance federation mode by default.
				</div>
			{:else}
				<div class="space-y-2">
					{#each controls as ctrl}
						<div class="bg-bg-secondary p-4 rounded-lg flex items-center justify-between">
							<div>
								<span class="font-medium text-text-primary">{ctrl.peer_domain}</span>
								{#if ctrl.peer_name}
									<span class="text-text-muted ml-1">({ctrl.peer_name})</span>
								{/if}
								{#if ctrl.reason}
									<div class="text-sm text-text-muted mt-1">Reason: {ctrl.reason}</div>
								{/if}
								<div class="text-xs text-text-muted mt-1">Since {formatDate(ctrl.created_at)}</div>
							</div>
							<div class="flex items-center gap-3">
								<span class="px-2 py-0.5 rounded text-xs {ctrl.action === 'block'
									? 'bg-red-500/20 text-red-400'
									: ctrl.action === 'allow'
									? 'bg-green-500/20 text-green-400'
									: 'bg-yellow-500/20 text-yellow-400'}">
									{ctrl.action}
								</span>
								<button
									class="text-xs text-text-muted hover:text-text-primary"
									onclick={() => {
										const newAction = ctrl.action === 'block' ? 'allow' : 'block';
										updatePeerControl(ctrl.peer_id, newAction);
									}}
								>
									{ctrl.action === 'block' ? 'Unblock' : 'Block'}
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}

		{:else if currentTab === 'delivery'}
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-text-primary">Delivery Receipts</h2>
				<div class="flex gap-2">
					<select
						class="input text-sm"
						bind:value={deliveryFilter}
						onchange={() => loadDeliveryReceipts()}
					>
						<option value="">All statuses</option>
						<option value="pending">Pending</option>
						<option value="delivered">Delivered</option>
						<option value="failed">Failed</option>
						<option value="retrying">Retrying</option>
					</select>
				</div>
			</div>

			{#if loadingDelivery}
				<div class="flex items-center justify-center py-8">
					<div class="animate-spin h-6 w-6 border-2 border-brand-500 border-t-transparent rounded-full"></div>
				</div>
			{:else if deliveryReceipts.length === 0}
				<div class="bg-bg-secondary p-8 rounded-lg text-center text-text-muted">
					No delivery receipts found.
				</div>
			{:else}
				<div class="space-y-2">
					{#each deliveryReceipts as receipt}
						<div class="bg-bg-secondary p-3 rounded-lg">
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-3">
									<span class="px-2 py-0.5 rounded text-xs {receipt.status === 'delivered'
										? 'bg-green-500/20 text-green-400'
										: receipt.status === 'failed'
										? 'bg-red-500/20 text-red-400'
										: receipt.status === 'retrying'
										? 'bg-yellow-500/20 text-yellow-400'
										: 'bg-bg-modifier text-text-muted'}">
										{receipt.status}
									</span>
									<span class="text-sm text-text-primary font-mono">{receipt.message_id.slice(0, 12)}...</span>
									<span class="text-xs text-text-muted">-&gt; {receipt.target_instance}</span>
								</div>
								<div class="flex items-center gap-2">
									<span class="text-xs text-text-muted">
										Attempts: {receipt.attempts}
									</span>
									{#if receipt.status === 'failed' || receipt.status === 'pending'}
										<button
											class="px-2 py-0.5 text-xs bg-brand-500/20 text-brand-400 rounded hover:bg-brand-500/30 transition-colors"
											onclick={() => retryDelivery(receipt.id)}
										>
											Retry
										</button>
									{/if}
								</div>
							</div>
							{#if receipt.error_message}
								<div class="text-xs text-status-dnd mt-1">{receipt.error_message}</div>
							{/if}
							<div class="text-xs text-text-muted mt-1">
								Created: {formatDate(receipt.created_at)}
								{#if receipt.delivered_at}
									| Delivered: {formatDate(receipt.delivered_at)}
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

		{:else if currentTab === 'search'}
			<h2 class="text-lg font-semibold text-text-primary mb-4">Federated Search Configuration</h2>
			<div class="bg-bg-secondary p-6 rounded-lg space-y-4">
				<p class="text-sm text-text-muted mb-4">
					Configure whether this instance participates in federated search. When enabled,
					messages can be searched across connected instances (opt-in only).
				</p>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={searchConfig.enabled} class="w-4 h-4" />
					<div>
						<div class="text-text-primary text-sm font-medium">Enable Federated Search</div>
						<div class="text-text-muted text-xs">Allow search queries to include results from federated peers.</div>
					</div>
				</label>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={searchConfig.index_outgoing} class="w-4 h-4" />
					<div>
						<div class="text-text-primary text-sm font-medium">Index Outgoing Content</div>
						<div class="text-text-muted text-xs">Allow federated peers to search content hosted on this instance.</div>
					</div>
				</label>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={searchConfig.index_incoming} class="w-4 h-4" />
					<div>
						<div class="text-text-primary text-sm font-medium">Index Incoming Content</div>
						<div class="text-text-muted text-xs">Index and make searchable the content received from federated peers.</div>
					</div>
				</label>

				<div class="pt-4 border-t border-border-primary">
					<button
						class="btn-primary"
						onclick={saveSearchConfig}
						disabled={savingSearch}
					>
						{savingSearch ? 'Saving...' : 'Save Configuration'}
					</button>
				</div>
			</div>

		{:else if currentTab === 'protocol'}
			<h2 class="text-lg font-semibold text-text-primary mb-4">Protocol Information</h2>
			{#if protocolInfo}
				<div class="space-y-4">
					<div class="bg-bg-secondary p-4 rounded-lg">
						<div class="text-text-muted text-xs uppercase mb-1">Current Protocol Version</div>
						<div class="text-lg font-mono text-text-primary">{protocolInfo.protocol_version}</div>
					</div>
					<div class="bg-bg-secondary p-4 rounded-lg">
						<div class="text-text-muted text-xs uppercase mb-2">Supported Protocols</div>
						<div class="flex flex-wrap gap-2">
							{#each protocolInfo.supported_protocols as proto}
								<span class="px-3 py-1 bg-bg-modifier rounded text-sm text-text-secondary font-mono">{proto}</span>
							{/each}
						</div>
					</div>
					<div class="bg-bg-secondary p-4 rounded-lg">
						<div class="text-text-muted text-xs uppercase mb-2">Default Capabilities</div>
						<div class="flex flex-wrap gap-2">
							{#each protocolInfo.default_capabilities as cap}
								<span class="px-2 py-0.5 bg-brand-500/10 text-brand-400 rounded text-xs">{cap}</span>
							{/each}
						</div>
					</div>
					<div class="bg-bg-secondary p-4 rounded-lg">
						<div class="text-text-muted text-xs uppercase mb-2">Instance Capabilities</div>
						{#if protocolInfo.capabilities && protocolInfo.capabilities.length > 0}
							<div class="flex flex-wrap gap-2">
								{#each protocolInfo.capabilities as cap}
									<span class="px-2 py-0.5 bg-green-500/10 text-green-400 rounded text-xs">{cap}</span>
								{/each}
							</div>
						{:else}
							<div class="text-sm text-text-muted">Using default capabilities.</div>
						{/if}
					</div>
					<p class="text-xs text-text-muted">
						Protocol versioning enables capability negotiation during the federation handshake.
						Peers advertise their supported version and capabilities, and both sides agree on
						the intersection of features during peering.
					</p>
				</div>
			{:else}
				<div class="flex items-center justify-center py-8">
					<div class="animate-spin h-6 w-6 border-2 border-brand-500 border-t-transparent rounded-full"></div>
				</div>
			{/if}

		{:else if currentTab === 'blocklist'}
			<div class="space-y-6">
				<div>
					<h2 class="text-lg font-semibold text-text-primary mb-3">Blocked Instances</h2>
					{#if loadingControls}
						<div class="flex items-center justify-center py-8">
							<div class="animate-spin h-6 w-6 border-2 border-brand-500 border-t-transparent rounded-full"></div>
						</div>
					{:else if blockedPeers.length === 0}
						<div class="bg-bg-secondary p-6 rounded-lg text-center text-text-muted text-sm">
							No instances are blocked.
						</div>
					{:else}
						<div class="space-y-2">
							{#each blockedPeers as peer}
								<div class="bg-bg-secondary p-3 rounded-lg flex items-center justify-between">
									<div>
										<span class="text-text-primary font-medium">{peer.peer_domain}</span>
										{#if peer.reason}
											<span class="text-text-muted text-sm ml-2">-- {peer.reason}</span>
										{/if}
									</div>
									<button
										class="text-xs text-green-400 hover:text-green-300"
										onclick={() => updatePeerControl(peer.peer_id, 'allow')}
									>
										Unblock
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<div>
					<h2 class="text-lg font-semibold text-text-primary mb-3">Allowed Instances</h2>
					{#if allowedPeers.length === 0}
						<div class="bg-bg-secondary p-6 rounded-lg text-center text-text-muted text-sm">
							No instance-specific allowlist entries. Using federation mode default.
						</div>
					{:else}
						<div class="space-y-2">
							{#each allowedPeers as peer}
								<div class="bg-bg-secondary p-3 rounded-lg flex items-center justify-between">
									<div>
										<span class="text-text-primary font-medium">{peer.peer_domain}</span>
										{#if peer.peer_name}
											<span class="text-text-muted text-sm ml-1">({peer.peer_name})</span>
										{/if}
									</div>
									<button
										class="text-xs text-red-400 hover:text-red-300"
										onclick={() => updatePeerControl(peer.peer_id, 'block')}
									>
										Block
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>
