<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type RateLimitLogEntry, type RateLimitStats } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let rateLimitStats = $state<RateLimitStats | null>(null);
	let rateLimitLog = $state<RateLimitLogEntry[]>([]);
	let statsOp = $state(createAsyncOp());
	let logOp = $state(createAsyncOp());
	let rateLimitLogFilter = $state<'all' | 'blocked'>('all');
	let rateLimitIPFilter = $state('');
	let saveOp = $state(createAsyncOp());
	let editReqsPerWindow = $state('100');
	let editWindowSeconds = $state('60');

	onMount(() => {
		loadRateLimitStats();
	});

	async function loadRateLimitStats() {
		const result = await statsOp.run(() => api.getRateLimitStats());
		if (result) {
			rateLimitStats = result;
			editReqsPerWindow = rateLimitStats?.requests_per_window ?? '100';
			editWindowSeconds = rateLimitStats?.window_seconds ?? '60';
		} else {
			rateLimitStats = null;
		}
	}

	async function loadRateLimitLog() {
		const result = await logOp.run(() => api.getRateLimitLog({
				limit: 50,
				blocked: rateLimitLogFilter === 'blocked',
				ip: rateLimitIPFilter.trim() || undefined
			}));
		if (result) {
			rateLimitLog = result;
		} else {
			rateLimitLog = [];
		}
	}

	async function saveRateLimitConfig() {
		const result = await saveOp.run(
			() => api.updateRateLimitConfig({
				requests_per_window: editReqsPerWindow,
				window_seconds: editWindowSeconds
			}),
			msg => addToast(msg, 'error'),
			'Failed to save rate limit configuration'
		);
		if (result !== undefined) {
			if (rateLimitStats) {
				rateLimitStats = {
					...rateLimitStats,
					requests_per_window: editReqsPerWindow,
					window_seconds: editWindowSeconds
				};
			}
			addToast('Rate limit configuration saved', 'success');
		}
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<h1 class="text-2xl font-bold text-text-primary">Rate Limiting</h1>
	<button class="btn-secondary text-sm" onclick={loadRateLimitStats} disabled={statsOp.loading}>
		{statsOp.loading ? 'Loading...' : 'Refresh'}
	</button>
</div>

{#if statsOp.loading && !rateLimitStats}
	<p class="text-sm text-text-muted">Loading rate limit data...</p>
{:else if rateLimitStats}
	<!-- Summary Cards -->
	<div class="mb-6 grid gap-4 sm:grid-cols-3">
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Unique IPs (24h)</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">{rateLimitStats.unique_ips_24h.toLocaleString()}</p>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Total Entries (24h)</p>
			<p class="mt-1 text-2xl font-bold text-text-primary">{rateLimitStats.total_entries_24h.toLocaleString()}</p>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<p class="text-sm text-text-muted">Blocked (24h)</p>
			<p class="mt-1 text-2xl font-bold text-red-400">{rateLimitStats.blocked_entries_24h.toLocaleString()}</p>
		</div>
	</div>

	<!-- Configuration -->
	<div class="mb-6 rounded-lg bg-bg-secondary p-4">
		<h2 class="mb-4 text-sm font-semibold text-text-primary">Configuration</h2>
		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label for="admin-rate-limit-requests" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Requests Per Window</label>
				<input id="admin-rate-limit-requests" type="text" class="input w-full" bind:value={editReqsPerWindow} placeholder="100" />
			</div>
			<div>
				<label for="admin-rate-limit-window" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Window (seconds)</label>
				<input id="admin-rate-limit-window" type="text" class="input w-full" bind:value={editWindowSeconds} placeholder="60" />
			</div>
		</div>
		<button class="btn-primary mt-4 text-sm" onclick={saveRateLimitConfig} disabled={saveOp.loading}>
			{saveOp.loading ? 'Saving...' : 'Save Configuration'}
		</button>
	</div>

	<!-- Top IPs Table -->
	<div class="mb-6">
		<h2 class="mb-4 text-sm font-semibold text-text-primary">Top IPs (Last 24 Hours)</h2>
		{#if rateLimitStats.top_ips.length === 0}
			<div class="rounded-lg bg-bg-secondary p-6 text-center">
				<p class="text-sm text-text-muted">No rate limit data recorded in the last 24 hours.</p>
			</div>
		{:else}
			<div class="overflow-hidden rounded-lg border border-bg-modifier">
				<table class="w-full text-left text-sm">
					<thead class="bg-bg-secondary">
						<tr>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">IP Address</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Total Requests</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Blocks</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Last Seen</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-bg-modifier">
						{#each rateLimitStats.top_ips as ip (ip.ip_address)}
							<tr class="hover:bg-bg-secondary/50">
								<td class="px-4 py-3">
									<code class="text-xs text-text-primary">{ip.ip_address}</code>
								</td>
								<td class="px-4 py-3 text-text-secondary">{ip.total_requests.toLocaleString()}</td>
								<td class="px-4 py-3">
									{#if ip.block_count > 0}
										<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-2xs font-bold text-red-400">{ip.block_count}</span>
									{:else}
										<span class="text-text-muted">0</span>
									{/if}
								</td>
								<td class="px-4 py-3 text-text-muted">{new Date(ip.last_seen).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Log Viewer -->
	<div>
		<div class="mb-4 flex items-center justify-between">
			<h2 class="text-sm font-semibold text-text-primary">Rate Limit Log</h2>
			<button class="btn-secondary text-sm" onclick={loadRateLimitLog} disabled={logOp.loading}>
				{logOp.loading ? 'Loading...' : 'Load Log'}
			</button>
		</div>
		<div class="mb-4 flex gap-2">
			<input
				type="text"
				class="input flex-1"
				aria-label="Filter rate limit log by IP address"
				placeholder="Filter by IP address..."
				bind:value={rateLimitIPFilter}
				onkeydown={(e) => e.key === 'Enter' && loadRateLimitLog()}
			/>
			<select class="input w-36" bind:value={rateLimitLogFilter} aria-label="Filter rate limit log entries" onchange={() => loadRateLimitLog()}>
				<option value="all">All Entries</option>
				<option value="blocked">Blocked Only</option>
			</select>
		</div>
		{#if rateLimitLog.length > 0}
			<div class="overflow-hidden rounded-lg border border-bg-modifier">
				<table class="w-full text-left text-sm">
					<thead class="bg-bg-secondary">
						<tr>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">IP</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Endpoint</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Requests</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Status</th>
							<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Time</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-bg-modifier">
						{#each rateLimitLog as entry (entry.id)}
							<tr class="hover:bg-bg-secondary/50">
								<td class="px-4 py-3"><code class="text-xs text-text-primary">{entry.ip_address}</code></td>
								<td class="px-4 py-3 text-text-muted text-xs">{entry.endpoint}</td>
								<td class="px-4 py-3 text-text-secondary">{entry.requests_count}</td>
								<td class="px-4 py-3">
									<span class="rounded px-1.5 py-0.5 text-2xs font-bold {entry.blocked ? 'bg-red-500/20 text-red-400' : 'bg-green-500/20 text-green-400'}">
										{entry.blocked ? 'Blocked' : 'Allowed'}
									</span>
								</td>
								<td class="px-4 py-3 text-text-muted text-xs">{new Date(entry.created_at).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if !logOp.loading}
			<p class="text-sm text-text-muted">Click "Load Log" to view recent rate limit entries.</p>
		{/if}
	</div>
{:else}
	<div class="rounded-lg bg-bg-secondary p-6 text-center">
		<p class="text-sm text-text-muted">Failed to load rate limit data. Make sure migration 031 has been applied.</p>
	</div>
{/if}
