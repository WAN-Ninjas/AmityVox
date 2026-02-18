<script lang="ts">
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let {
		channelId,
		guildId,
		connected = false,
		currentUserId = ''
	}: {
		channelId: string;
		guildId: string;
		connected: boolean;
		currentUserId: string;
	} = $props();

	interface Broadcast {
		id: string;
		guild_id: string;
		channel_id: string;
		broadcaster_id: string;
		title: string;
		started_at: string;
		ended_at: string | null;
		listener_count: number;
	}

	let activeBroadcast = $state<Broadcast | null>(null);
	let loadOp = $state(createAsyncOp());
	let startOp = $state(createAsyncOp());
	let stopOp = $state(createAsyncOp());
	let title = $state('');
	let showStartForm = $state(false);
	let elapsed = $state('00:00');
	let elapsedInterval: ReturnType<typeof setInterval> | null = null;

	let isBroadcaster = $derived(activeBroadcast?.broadcaster_id === currentUserId);

	// Load active broadcast on mount and when channel changes
	$effect(() => {
		if (channelId && connected) {
			loadBroadcast();
		}

		return () => {
			if (elapsedInterval) clearInterval(elapsedInterval);
		};
	});

	// Update elapsed time when broadcast is active
	$effect(() => {
		if (activeBroadcast && !activeBroadcast.ended_at) {
			if (elapsedInterval) clearInterval(elapsedInterval);
			elapsedInterval = setInterval(() => {
				const start = new Date(activeBroadcast!.started_at).getTime();
				const now = Date.now();
				const diffMs = now - start;
				const mins = Math.floor(diffMs / 60000);
				const secs = Math.floor((diffMs % 60000) / 1000);
				elapsed = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
			}, 1000);
		} else {
			if (elapsedInterval) {
				clearInterval(elapsedInterval);
				elapsedInterval = null;
			}
			elapsed = '00:00';
		}
	});

	async function loadBroadcast() {
		activeBroadcast = await loadOp.run(() => api.getVoiceBroadcast(channelId)) ?? null;
	}

	async function startBroadcast() {
		const result = await startOp.run(() => api.startVoiceBroadcast(channelId, { title: title || 'Live Broadcast' }));
		if (!startOp.error) {
			activeBroadcast = result!;
			showStartForm = false;
			title = '';
		}
	}

	async function stopBroadcast() {
		await stopOp.run(() => api.stopVoiceBroadcast(channelId));
		if (!stopOp.error) {
			activeBroadcast = null;
		}
	}
</script>

{#if connected}
	<div class="broadcast-section">
		{#if loadOp.loading}
			<div class="loading">Checking broadcast...</div>
		{:else if activeBroadcast}
			<!-- Active broadcast banner -->
			<div class="broadcast-active">
				<div class="broadcast-indicator">
					<span class="live-dot"></span>
					<span class="live-text">LIVE</span>
				</div>
				<div class="broadcast-info">
					<span class="broadcast-title">{activeBroadcast.title}</span>
					<span class="broadcast-meta">
						{elapsed}
						{#if activeBroadcast.listener_count > 0}
							 - {activeBroadcast.listener_count} listener{activeBroadcast.listener_count !== 1 ? 's' : ''}
						{/if}
					</span>
				</div>
				{#if isBroadcaster}
					<button
						class="btn-stop"
						onclick={stopBroadcast}
						disabled={stopOp.loading}
					>
						{stopOp.loading ? 'Stopping...' : 'End'}
					</button>
				{/if}
			</div>
		{:else if showStartForm}
			<!-- Start broadcast form -->
			<div class="broadcast-form">
				<input
					type="text"
					class="input"
					placeholder="Broadcast title (optional)"
					bind:value={title}
					onkeydown={(e) => e.key === 'Enter' && startBroadcast()}
					maxlength={100}
				/>
				<div class="form-actions">
					<button
						class="btn-secondary"
						onclick={() => { showStartForm = false; startOp.error = null; }}
					>
						Cancel
					</button>
					<button
						class="btn-primary"
						onclick={startBroadcast}
						disabled={startOp.loading}
					>
						{startOp.loading ? 'Starting...' : 'Go Live'}
					</button>
				</div>
			</div>
		{:else}
			<!-- Start broadcast button -->
			<button
				class="btn-broadcast"
				onclick={() => showStartForm = true}
				title="Start a live broadcast"
			>
				<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
					<path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z"/>
				</svg>
				<span>Start Broadcast</span>
			</button>
		{/if}

		{#if startOp.error || stopOp.error}
			<div class="error-msg">{startOp.error || stopOp.error}</div>
		{/if}
	</div>
{/if}

<style>
	.broadcast-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.loading {
		padding: 8px;
		font-size: 12px;
		color: var(--text-muted);
		text-align: center;
	}

	.broadcast-active {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 12px;
		background: linear-gradient(135deg, rgba(239, 68, 68, 0.15), rgba(239, 68, 68, 0.05));
		border: 1px solid rgba(239, 68, 68, 0.3);
		border-radius: 8px;
	}

	.broadcast-indicator {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-shrink: 0;
	}

	.live-dot {
		width: 8px;
		height: 8px;
		background: #ef4444;
		border-radius: 50%;
		animation: livePulse 1.5s ease-in-out infinite;
	}

	@keyframes livePulse {
		0%, 100% { opacity: 1; transform: scale(1); }
		50% { opacity: 0.6; transform: scale(0.85); }
	}

	.live-text {
		font-size: 11px;
		font-weight: 700;
		color: #ef4444;
		letter-spacing: 0.05em;
	}

	.broadcast-info {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.broadcast-title {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.broadcast-meta {
		font-size: 11px;
		color: var(--text-muted);
	}

	.btn-stop {
		padding: 4px 12px;
		background: #ef4444;
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 12px;
		font-weight: 600;
		flex-shrink: 0;
	}

	.btn-stop:hover {
		background: #dc2626;
	}

	.btn-stop:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.broadcast-form {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		background: var(--bg-secondary);
		border-radius: 8px;
	}

	.form-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}

	.btn-broadcast {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		padding: 8px 12px;
		background: var(--bg-secondary);
		color: var(--text-secondary);
		border: 1px dashed var(--bg-tertiary);
		border-radius: 8px;
		cursor: pointer;
		font-size: 13px;
		transition: all 0.15s ease;
	}

	.btn-broadcast:hover {
		background: var(--bg-tertiary);
		color: var(--text-primary);
		border-color: var(--brand-500);
	}

	.error-msg {
		padding: 4px 8px;
		font-size: 12px;
		color: #ef4444;
	}

	.btn-primary {
		padding: 6px 14px;
		background: var(--brand-500);
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
		font-weight: 500;
	}

	.btn-primary:hover {
		filter: brightness(1.1);
	}

	.btn-primary:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-secondary {
		padding: 6px 14px;
		background: var(--bg-tertiary);
		color: var(--text-secondary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
	}

	.btn-secondary:hover {
		background: var(--bg-primary);
		color: var(--text-primary);
	}

	.input {
		width: 100%;
		padding: 8px 10px;
		background: var(--bg-primary);
		color: var(--text-primary);
		border: 1px solid var(--bg-tertiary);
		border-radius: 4px;
		font-size: 13px;
		outline: none;
	}

	.input:focus {
		border-color: var(--brand-500);
	}
</style>
