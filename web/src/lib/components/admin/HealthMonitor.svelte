<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface ServiceHealth {
		name: string;
		status: string;
		response_time_ms: number;
		details: string;
		last_checked: string;
	}

	interface HealthDashboard {
		services: ServiceHealth[];
		database: {
			size: string;
			total_conns: number;
			idle_conns: number;
			acquired_conns: number;
			max_conns: number;
		};
		runtime: {
			go_version: string;
			goroutines: number;
			mem_alloc_mb: number;
			mem_sys_mb: number;
			mem_gc_cycles: number;
			num_cpu: number;
			uptime: string;
		};
		activity: {
			active_sessions: number;
			messages_last_hour: number;
			messages_last_day: number;
		};
		trends: Record<string, { time: string; status: string; response_time_ms: number }[]>;
	}

	let loading = $state(true);
	let health = $state<HealthDashboard | null>(null);
	let autoRefresh = $state(true);
	let refreshInterval: ReturnType<typeof setInterval> | null = null;

	async function loadHealth() {
		try {
			const res = await fetch('/api/v1/admin/health', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				health = json.data;
			}
		} catch (e) {
			addToast('Failed to load health data', 'error');
		}
		loading = false;
	}

	function startAutoRefresh() {
		if (refreshInterval) clearInterval(refreshInterval);
		if (autoRefresh) {
			refreshInterval = setInterval(loadHealth, 30000);
		}
	}

	$effect(() => {
		startAutoRefresh();
		return () => {
			if (refreshInterval) clearInterval(refreshInterval);
		};
	});

	onMount(() => {
		loadHealth();
	});

	function statusColor(status: string): string {
		switch (status) {
			case 'healthy': return 'text-status-online';
			case 'degraded': return 'text-status-idle';
			case 'unhealthy': return 'text-status-dnd';
			default: return 'text-text-muted';
		}
	}

	function statusBg(status: string): string {
		switch (status) {
			case 'healthy': return 'bg-status-online/20';
			case 'degraded': return 'bg-status-idle/20';
			case 'unhealthy': return 'bg-status-dnd/20';
			default: return 'bg-bg-modifier';
		}
	}

	function formatUptime(uptime: string): string {
		if (!uptime) return 'N/A';
		// Go duration format: "72h30m15s"
		const hours = uptime.match(/(\d+)h/);
		const minutes = uptime.match(/(\d+)m/);
		if (hours) {
			const h = parseInt(hours[1]);
			const d = Math.floor(h / 24);
			const remainder = h % 24;
			if (d > 0) return `${d}d ${remainder}h`;
			return `${h}h ${minutes ? minutes[1] : '0'}m`;
		}
		return uptime;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-bold text-text-primary">Health Monitor</h2>
			<p class="text-text-muted text-sm">Real-time system health and performance metrics</p>
		</div>
		<div class="flex items-center gap-3">
			<label class="flex items-center gap-2 text-sm text-text-secondary">
				<input type="checkbox" bind:checked={autoRefresh} />
				Auto-refresh
			</label>
			<button class="btn-secondary text-sm px-3 py-1.5" onclick={loadHealth} disabled={loading}>
				{loading ? 'Refreshing...' : 'Refresh'}
			</button>
		</div>
	</div>

	{#if loading && !health}
		<div class="flex justify-center py-12">
			<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
		</div>
	{:else if health}
		<!-- Overall Status -->
		{@const overallHealthy = health.services.every(s => s.status === 'healthy')}
		<div class="rounded-lg p-4 {overallHealthy ? 'bg-status-online/10 border border-status-online/20' : 'bg-status-idle/10 border border-status-idle/20'}">
			<div class="flex items-center gap-3">
				<div class="w-3 h-3 rounded-full {overallHealthy ? 'bg-status-online' : 'bg-status-idle'} animate-pulse"></div>
				<span class="font-medium text-text-primary">
					{overallHealthy ? 'All Systems Operational' : 'Degraded Performance Detected'}
				</span>
				<span class="text-text-muted text-sm ml-auto">
					Uptime: {formatUptime(health.runtime.uptime)}
				</span>
			</div>
		</div>

		<!-- Service Cards -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each health.services as service}
				<div class="bg-bg-tertiary rounded-lg p-4">
					<div class="flex items-center justify-between mb-3">
						<h3 class="font-medium text-text-primary capitalize">{service.name}</h3>
						<span class="text-xs px-2 py-0.5 rounded-full {statusBg(service.status)} {statusColor(service.status)}">
							{service.status}
						</span>
					</div>
					<div class="space-y-1 text-sm">
						<div class="flex justify-between">
							<span class="text-text-muted">Response</span>
							<span class="text-text-secondary">{service.response_time_ms}ms</span>
						</div>
						<div class="text-text-muted text-xs truncate" title={service.details}>
							{service.details}
						</div>
					</div>
				</div>
			{/each}
		</div>

		<!-- Metrics Grid -->
		<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
			<!-- Database -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Database</h3>
				<div class="space-y-3">
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Size</span>
						<span class="text-text-primary text-sm font-medium">{health.database.size}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Connections</span>
						<span class="text-text-primary text-sm font-medium">
							{health.database.acquired_conns}/{health.database.max_conns}
						</span>
					</div>
					<div>
						<div class="flex justify-between text-xs text-text-muted mb-1">
							<span>Pool Usage</span>
							<span>{Math.round((health.database.acquired_conns / health.database.max_conns) * 100)}%</span>
						</div>
						<div class="h-2 bg-bg-modifier rounded-full overflow-hidden">
							<div
								class="h-full bg-brand-500 rounded-full transition-all"
								style="width: {Math.round((health.database.acquired_conns / health.database.max_conns) * 100)}%"
							></div>
						</div>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Idle</span>
						<span class="text-text-primary text-sm">{health.database.idle_conns}</span>
					</div>
				</div>
			</div>

			<!-- Runtime -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Runtime</h3>
				<div class="space-y-3">
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Go Version</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.go_version}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Goroutines</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.goroutines}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Memory (Alloc)</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.mem_alloc_mb} MB</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Memory (Sys)</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.mem_sys_mb} MB</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">CPUs</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.num_cpu}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">GC Cycles</span>
						<span class="text-text-primary text-sm font-medium">{health.runtime.mem_gc_cycles}</span>
					</div>
				</div>
			</div>

			<!-- Activity -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Activity</h3>
				<div class="space-y-3">
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Active Sessions</span>
						<span class="text-text-primary text-sm font-medium">{health.activity.active_sessions}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Messages (1h)</span>
						<span class="text-text-primary text-sm font-medium">{health.activity.messages_last_hour}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-text-muted text-sm">Messages (24h)</span>
						<span class="text-text-primary text-sm font-medium">{health.activity.messages_last_day}</span>
					</div>
				</div>
			</div>

			<!-- Response Time Trend -->
			<div class="bg-bg-tertiary rounded-lg p-5">
				<h3 class="text-sm font-semibold text-text-secondary mb-4">Recent Trends (24h)</h3>
				{#if health.trends && Object.keys(health.trends).length > 0}
					{#each Object.entries(health.trends) as [svc, points]}
						<div class="mb-3">
							<div class="flex justify-between text-xs mb-1">
								<span class="text-text-muted capitalize">{svc}</span>
								<span class="text-text-secondary">
									Avg: {Math.round(points.reduce((a, p) => a + p.response_time_ms, 0) / points.length)}ms
								</span>
							</div>
							<div class="flex items-end gap-px h-8">
								{#each points.slice(-30) as point}
									{@const maxMs = Math.max(...points.slice(-30).map(p => p.response_time_ms), 1)}
									<div
										class="flex-1 rounded-t-sm transition-all {point.status === 'healthy' ? 'bg-status-online/60' : 'bg-status-dnd/60'}"
										style="height: {Math.max(2, (point.response_time_ms / maxMs) * 100)}%"
										title="{point.response_time_ms}ms at {new Date(point.time).toLocaleTimeString()}"
									></div>
								{/each}
							</div>
						</div>
					{/each}
				{:else}
					<p class="text-text-muted text-sm">No trend data yet. Data will appear after a few health checks.</p>
				{/if}
			</div>
		</div>
	{/if}
</div>
