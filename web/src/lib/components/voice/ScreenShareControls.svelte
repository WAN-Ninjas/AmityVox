<script lang="ts">
	import { api } from '$lib/api/client';

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

	interface ScreenShareSession {
		id: string;
		channel_id: string;
		user_id: string;
		share_type: 'screen' | 'window';
		resolution: '720p' | '1080p' | '4k';
		framerate: 15 | 30 | 60;
		audio_enabled: boolean;
		max_viewers: number;
		started_at: string;
		ended_at: string | null;
	}

	let activeShares = $state<ScreenShareSession[]>([]);
	let myShare = $derived(activeShares.find(s => s.user_id === currentUserId));
	let isSharing = $derived(!!myShare);

	let loading = $state(false);
	let starting = $state(false);
	let stopping = $state(false);
	let showSettings = $state(false);
	let error = $state<string | null>(null);

	// Settings state
	let shareType = $state<'screen' | 'window'>('screen');
	let resolution = $state<'720p' | '1080p' | '4k'>('1080p');
	let framerate = $state<15 | 30 | 60>(30);
	let audioEnabled = $state(false);
	let maxViewers = $state(50);

	// Load active screen shares
	$effect(() => {
		if (channelId && connected) {
			loadScreenShares();
		}
	});

	async function loadScreenShares() {
		try {
			loading = true;
			const res = await fetch(`/api/v1/voice/${channelId}/screen-shares`, {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				const json = await res.json();
				activeShares = json.data || [];
			}
		} catch (err) {
			console.error('Failed to load screen shares:', err);
		} finally {
			loading = false;
		}
	}

	async function startScreenShare() {
		try {
			starting = true;
			error = null;

			const res = await fetch(`/api/v1/voice/${channelId}/screen-share/start`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					share_type: shareType,
					resolution,
					framerate,
					audio_enabled: audioEnabled,
					max_viewers: maxViewers
				})
			});

			if (!res.ok) {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to start screen share';
				return;
			}

			const json = await res.json();
			const session = json.data.session;
			activeShares = [...activeShares, session];
			showSettings = false;
		} catch (err) {
			error = 'Failed to start screen share';
			console.error('Screen share start error:', err);
		} finally {
			starting = false;
		}
	}

	async function stopScreenShare() {
		try {
			stopping = true;
			error = null;

			const res = await fetch(`/api/v1/voice/${channelId}/screen-share/stop`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				}
			});

			if (!res.ok) {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to stop screen share';
				return;
			}

			activeShares = activeShares.filter(s => s.user_id !== currentUserId);
		} catch (err) {
			error = 'Failed to stop screen share';
			console.error('Screen share stop error:', err);
		} finally {
			stopping = false;
		}
	}

	async function updateSettings() {
		if (!myShare) return;

		try {
			error = null;
			const res = await fetch(`/api/v1/voice/${channelId}/screen-share`, {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({
					resolution,
					framerate,
					audio_enabled: audioEnabled
				})
			});

			if (!res.ok) {
				const errJson = await res.json();
				error = errJson.error?.message || 'Failed to update settings';
				return;
			}

			// Update local state
			activeShares = activeShares.map(s => {
				if (s.user_id === currentUserId) {
					return { ...s, resolution, framerate, audio_enabled: audioEnabled };
				}
				return s;
			});
		} catch (err) {
			error = 'Failed to update screen share settings';
		}
	}

	function resolutionLabel(res: string): string {
		switch (res) {
			case '720p': return '720p (HD)';
			case '1080p': return '1080p (Full HD)';
			case '4k': return '4K (Ultra HD)';
			default: return res;
		}
	}
</script>

{#if connected}
	<div class="screen-share-controls">
		{#if isSharing}
			<!-- Active screen share controls -->
			<div class="sharing-active">
				<div class="sharing-header">
					<div class="sharing-indicator">
						<span class="share-dot"></span>
						<span class="share-text">Screen Sharing</span>
					</div>
					<div class="sharing-details">
						<span class="detail">{myShare?.resolution}</span>
						<span class="detail-sep">-</span>
						<span class="detail">{myShare?.framerate}fps</span>
						{#if myShare?.audio_enabled}
							<span class="detail-sep">-</span>
							<span class="detail">Audio</span>
						{/if}
					</div>
				</div>

				<div class="sharing-actions">
					<button
						class="btn-settings"
						onclick={() => showSettings = !showSettings}
						title="Screen Share Settings"
					>
						<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
							<path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58z"/>
						</svg>
					</button>
					<button
						class="btn-stop-share"
						onclick={stopScreenShare}
						disabled={stopping}
					>
						{stopping ? 'Stopping...' : 'Stop Sharing'}
					</button>
				</div>
			</div>

			{#if showSettings}
				<div class="settings-panel">
					<div class="setting-row">
						<label class="setting-label">Resolution</label>
						<select
							class="input setting-select"
							bind:value={resolution}
							onchange={updateSettings}
						>
							<option value="720p">720p (HD)</option>
							<option value="1080p">1080p (Full HD)</option>
							<option value="4k">4K (Ultra HD)</option>
						</select>
					</div>
					<div class="setting-row">
						<label class="setting-label">Frame Rate</label>
						<select
							class="input setting-select"
							bind:value={framerate}
							onchange={updateSettings}
						>
							<option value={15}>15 fps</option>
							<option value={30}>30 fps</option>
							<option value={60}>60 fps</option>
						</select>
					</div>
					<div class="setting-row">
						<label class="setting-label">
							<input
								type="checkbox"
								bind:checked={audioEnabled}
								onchange={updateSettings}
							/>
							Share Audio
						</label>
					</div>
				</div>
			{/if}
		{:else}
			<!-- Start screen share -->
			{#if showSettings}
				<div class="start-panel">
					<h4 class="panel-title">Screen Share Settings</h4>

					<div class="setting-row">
						<label class="setting-label">Share Type</label>
						<div class="share-type-btns">
							<button
								class="type-btn"
								class:active={shareType === 'screen'}
								onclick={() => shareType = 'screen'}
							>
								<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
									<path d="M21 2H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7l-2 3v1h8v-1l-2-3h7c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H3V4h18v12z"/>
								</svg>
								<span>Entire Screen</span>
							</button>
							<button
								class="type-btn"
								class:active={shareType === 'window'}
								onclick={() => shareType = 'window'}
							>
								<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
									<path d="M20 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V8h16v10z"/>
								</svg>
								<span>Window</span>
							</button>
						</div>
					</div>

					<div class="setting-row">
						<label class="setting-label">Resolution</label>
						<select class="input setting-select" bind:value={resolution}>
							<option value="720p">720p (HD)</option>
							<option value="1080p">1080p (Full HD)</option>
							<option value="4k">4K (Ultra HD)</option>
						</select>
					</div>

					<div class="setting-row">
						<label class="setting-label">Frame Rate</label>
						<select class="input setting-select" bind:value={framerate}>
							<option value={15}>15 fps (Low bandwidth)</option>
							<option value={30}>30 fps (Standard)</option>
							<option value={60}>60 fps (Smooth)</option>
						</select>
					</div>

					<div class="setting-row">
						<label class="setting-label checkbox-label">
							<input type="checkbox" bind:checked={audioEnabled} />
							Share System Audio
						</label>
					</div>

					<div class="setting-row">
						<label class="setting-label">Max Viewers</label>
						<input
							type="number"
							class="input"
							bind:value={maxViewers}
							min={1}
							max={50}
						/>
					</div>

					<div class="panel-actions">
						<button class="btn-secondary" onclick={() => showSettings = false}>
							Cancel
						</button>
						<button
							class="btn-primary"
							onclick={startScreenShare}
							disabled={starting}
						>
							{starting ? 'Starting...' : 'Start Sharing'}
						</button>
					</div>
				</div>
			{:else}
				<button
					class="btn-share"
					onclick={() => showSettings = true}
					title="Share your screen"
				>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
						<path d="M21 2H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7l-2 3v1h8v-1l-2-3h7c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H3V4h18v12z"/>
					</svg>
					<span>Share Screen</span>
				</button>
			{/if}
		{/if}

		<!-- Show other active screen shares -->
		{#if activeShares.filter(s => s.user_id !== currentUserId).length > 0}
			<div class="other-shares">
				{#each activeShares.filter(s => s.user_id !== currentUserId) as share (share.id)}
					<div class="other-share">
						<span class="share-dot small"></span>
						<span class="share-user">User sharing screen</span>
						<span class="share-quality">{share.resolution} {share.framerate}fps</span>
					</div>
				{/each}
			</div>
		{/if}

		{#if error}
			<div class="error-msg">{error}</div>
		{/if}
	</div>
{/if}

<style>
	.screen-share-controls {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.sharing-active {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		background: linear-gradient(135deg, rgba(59, 130, 246, 0.15), rgba(59, 130, 246, 0.05));
		border: 1px solid rgba(59, 130, 246, 0.3);
		border-radius: 8px;
	}

	.sharing-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.sharing-indicator {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.share-dot {
		width: 8px;
		height: 8px;
		background: #3b82f6;
		border-radius: 50%;
		animation: sharePulse 2s ease-in-out infinite;
	}

	.share-dot.small {
		width: 6px;
		height: 6px;
	}

	@keyframes sharePulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.5; }
	}

	.share-text {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.sharing-details {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 12px;
		color: var(--text-muted);
	}

	.detail-sep {
		color: var(--text-muted);
		opacity: 0.5;
	}

	.sharing-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
	}

	.btn-settings {
		padding: 6px;
		background: var(--bg-tertiary);
		color: var(--text-secondary);
		border: none;
		border-radius: 4px;
		cursor: pointer;
	}

	.btn-settings:hover {
		background: var(--bg-primary);
		color: var(--text-primary);
	}

	.btn-stop-share {
		padding: 6px 14px;
		background: #ef4444;
		color: white;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		font-size: 12px;
		font-weight: 600;
	}

	.btn-stop-share:hover {
		background: #dc2626;
	}

	.btn-stop-share:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.settings-panel {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		background: var(--bg-tertiary);
		border-radius: 8px;
	}

	.start-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 14px;
		background: var(--bg-secondary);
		border-radius: 8px;
	}

	.panel-title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.setting-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.setting-label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 8px;
		text-transform: none;
		font-size: 13px;
		color: var(--text-primary);
		cursor: pointer;
	}

	.setting-select {
		padding: 6px 10px;
		font-size: 13px;
	}

	.share-type-btns {
		display: flex;
		gap: 8px;
	}

	.type-btn {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
		padding: 12px 8px;
		background: var(--bg-tertiary);
		color: var(--text-secondary);
		border: 2px solid transparent;
		border-radius: 8px;
		cursor: pointer;
		font-size: 12px;
		transition: all 0.15s ease;
	}

	.type-btn:hover {
		background: var(--bg-primary);
		color: var(--text-primary);
	}

	.type-btn.active {
		border-color: var(--brand-500);
		color: var(--brand-400);
	}

	.panel-actions {
		display: flex;
		gap: 8px;
		justify-content: flex-end;
		margin-top: 4px;
	}

	.btn-share {
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

	.btn-share:hover {
		background: var(--bg-tertiary);
		color: var(--text-primary);
		border-color: var(--brand-500);
	}

	.other-shares {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.other-share {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 10px;
		background: var(--bg-tertiary);
		border-radius: 6px;
		font-size: 12px;
		color: var(--text-secondary);
	}

	.share-user {
		flex: 1;
	}

	.share-quality {
		color: var(--text-muted);
		font-size: 11px;
	}

	.error-msg {
		padding: 4px 8px;
		font-size: 12px;
		color: #ef4444;
	}

	.btn-primary {
		padding: 8px 16px;
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
		padding: 8px 16px;
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
		box-sizing: border-box;
	}

	.input:focus {
		border-color: var(--brand-500);
	}
</style>
