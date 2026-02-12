<!-- ConnectionIndicator.svelte — Shows WebSocket connection quality in the UI.
     Displays a colored dot with tooltip indicating connection status:
     - Green: Connected and healthy
     - Yellow: Degraded (high latency or reconnecting)
     - Red: Disconnected
-->
<script lang="ts">
	import { connectionQuality, connectionLatency, reconnectAttempt } from '$lib/stores/gateway.reconnect';

	let showTooltip = $state(false);

	let statusColor = $derived(
		$connectionQuality === 'good' ? 'bg-status-online' :
		$connectionQuality === 'degraded' ? 'bg-status-idle' :
		'bg-status-dnd'
	);

	let statusLabel = $derived(
		$connectionQuality === 'good' ? 'Connected' :
		$connectionQuality === 'degraded' ? 'Degraded' :
		'Disconnected'
	);

	let statusDetail = $derived(() => {
		if ($connectionQuality === 'good') {
			return $connectionLatency > 0 ? `Latency: ${$connectionLatency}ms` : 'Connection healthy';
		}
		if ($connectionQuality === 'degraded') {
			if ($reconnectAttempt > 0) {
				return `Reconnecting (attempt ${$reconnectAttempt})...`;
			}
			return `High latency: ${$connectionLatency}ms`;
		}
		if ($reconnectAttempt > 0) {
			return `Reconnecting (attempt ${$reconnectAttempt})...`;
		}
		return 'Connection lost';
	});

	let pulseClass = $derived(
		$connectionQuality === 'disconnected' ? 'animate-pulse' : ''
	);
</script>

<div
	class="relative inline-flex items-center"
	role="status"
	aria-label="Connection status: {statusLabel}"
	onmouseenter={() => showTooltip = true}
	onmouseleave={() => showTooltip = false}
>
	<!-- Status dot -->
	<div class="relative flex items-center justify-center w-3 h-3">
		{#if $connectionQuality !== 'good'}
			<div class="absolute inset-0 rounded-full {statusColor} opacity-40 {pulseClass}"></div>
		{/if}
		<div class="relative w-2 h-2 rounded-full {statusColor}"></div>
	</div>

	<!-- Tooltip -->
	{#if showTooltip}
		<div
			class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-3 py-2 rounded-lg
			       bg-bg-floating text-text-primary text-xs whitespace-nowrap shadow-lg z-50
			       border border-white/10"
		>
			<div class="font-medium">{statusLabel}</div>
			<div class="text-text-muted mt-0.5">{statusDetail()}</div>
			{#if $connectionLatency > 0 && $connectionQuality !== 'disconnected'}
				<div class="flex items-center gap-1 mt-1 text-text-muted">
					<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
					<span>{$connectionLatency}ms</span>
				</div>
			{/if}
			<!-- Tooltip arrow -->
			<div class="absolute top-full left-1/2 -translate-x-1/2 -mt-px">
				<div class="w-2 h-2 bg-bg-floating border-r border-b border-white/10 rotate-45 -translate-y-1"></div>
			</div>
		</div>
	{/if}
</div>
