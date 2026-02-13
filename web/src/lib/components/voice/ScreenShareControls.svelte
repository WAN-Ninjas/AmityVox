<script lang="ts">
	import { getRoom } from '$lib/stores/voice';

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

	let isSharing = $state(false);
	let starting = $state(false);
	let stopping = $state(false);
	let error = $state<string | null>(null);

	// Settings
	let resolution = $state<'720p' | '1080p' | '4k'>('1080p');
	let framerate = $state<15 | 30 | 60>(30);
	let audioEnabled = $state(false);
	let showSettings = $state(false);

	function getResolutionConstraints(): { width: number; height: number } {
		switch (resolution) {
			case '720p': return { width: 1280, height: 720 };
			case '4k': return { width: 3840, height: 2160 };
			default: return { width: 1920, height: 1080 };
		}
	}

	async function startScreenShare() {
		const room = getRoom();
		if (!room) {
			error = 'Not connected to voice channel';
			return;
		}

		try {
			starting = true;
			error = null;

			const res = getResolutionConstraints();
			await room.localParticipant.setScreenShareEnabled(true, {
				audio: audioEnabled,
				resolution: { width: res.width, height: res.height, frameRate: framerate },
				contentHint: 'detail'
			});

			isSharing = true;
			showSettings = false;
		} catch (err: any) {
			// User cancelled the picker — not an error
			if (err.name === 'NotAllowedError' || err.message?.includes('cancelled')) {
				error = null;
			} else {
				error = err.message || 'Failed to start screen share';
				console.error('Screen share error:', err);
			}
		} finally {
			starting = false;
		}
	}

	async function stopScreenShare() {
		const room = getRoom();
		if (!room) return;

		try {
			stopping = true;
			error = null;
			await room.localParticipant.setScreenShareEnabled(false);
			isSharing = false;
		} catch (err: any) {
			error = err.message || 'Failed to stop screen share';
		} finally {
			stopping = false;
		}
	}
</script>

{#if connected}
	<div class="flex flex-col gap-2">
		{#if isSharing}
			<div class="flex flex-col gap-2 rounded-lg border border-brand-500/30 bg-brand-500/10 px-3 py-2.5">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<span class="h-2 w-2 animate-pulse rounded-full bg-brand-500"></span>
						<span class="text-sm font-semibold text-text-primary">Screen Sharing</span>
					</div>
					<div class="flex items-center gap-2 text-xs text-text-muted">
						<span>{resolution}</span>
						<span class="opacity-50">-</span>
						<span>{framerate}fps</span>
					</div>
				</div>
				<button
					class="w-full rounded bg-red-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-red-600 disabled:cursor-not-allowed disabled:opacity-60"
					onclick={stopScreenShare}
					disabled={stopping}
				>
					{stopping ? 'Stopping...' : 'Stop Sharing'}
				</button>
			</div>
		{:else if showSettings}
			<div class="flex flex-col gap-3 rounded-lg bg-bg-secondary p-3.5">
				<h4 class="m-0 text-sm font-semibold text-text-primary">Screen Share Settings</h4>

				<div class="flex flex-col gap-1">
					<label class="text-2xs font-medium uppercase tracking-wide text-text-secondary">Resolution</label>
					<select class="rounded border border-bg-tertiary bg-bg-primary px-2.5 py-1.5 text-sm text-text-primary outline-none focus:border-brand-500" bind:value={resolution}>
						<option value="720p">720p (HD)</option>
						<option value="1080p">1080p (Full HD)</option>
						<option value="4k">4K (Ultra HD)</option>
					</select>
				</div>

				<div class="flex flex-col gap-1">
					<label class="text-2xs font-medium uppercase tracking-wide text-text-secondary">Frame Rate</label>
					<select class="rounded border border-bg-tertiary bg-bg-primary px-2.5 py-1.5 text-sm text-text-primary outline-none focus:border-brand-500" bind:value={framerate}>
						<option value={15}>15 fps (Low bandwidth)</option>
						<option value={30}>30 fps (Standard)</option>
						<option value={60}>60 fps (Smooth)</option>
					</select>
				</div>

				<label class="flex cursor-pointer items-center gap-2 text-sm text-text-primary">
					<input type="checkbox" bind:checked={audioEnabled} />
					Share System Audio
				</label>

				<div class="flex justify-end gap-2">
					<button
						class="rounded bg-bg-tertiary px-4 py-1.5 text-sm text-text-secondary hover:bg-bg-primary hover:text-text-primary"
						onclick={() => showSettings = false}
					>
						Cancel
					</button>
					<button
						class="rounded bg-brand-500 px-4 py-1.5 text-sm font-medium text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60"
						onclick={startScreenShare}
						disabled={starting}
					>
						{starting ? 'Starting...' : 'Start Sharing'}
					</button>
				</div>
			</div>
		{:else}
			<button
				class="flex w-full items-center gap-2 rounded-lg border border-dashed border-bg-tertiary bg-bg-secondary px-3 py-2 text-sm text-text-secondary transition-all hover:border-brand-500 hover:bg-bg-tertiary hover:text-text-primary"
				onclick={() => showSettings = true}
				title="Share your screen"
			>
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M21 2H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7l-2 3v1h8v-1l-2-3h7c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H3V4h18v12z"/>
				</svg>
				<span>Share Screen</span>
			</button>
		{/if}

		{#if error}
			<div class="px-2 text-xs text-red-500">{error}</div>
		{/if}
	</div>
{/if}
