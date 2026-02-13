<!-- CameraSettings.svelte — Resolution, frame rate, and facing mode settings for camera. -->
<script lang="ts">
	import { selfCamera, getRoom } from '$lib/stores/voice';

	let resolution = $state<'360p' | '720p' | '1080p'>('720p');
	let framerate = $state<15 | 30 | 60>(30);
	let facingMode = $state<'user' | 'environment'>('user');
	let applying = $state(false);
	let error = $state<string | null>(null);

	function getResolutionConstraints(): { width: number; height: number } {
		switch (resolution) {
			case '360p': return { width: 640, height: 360 };
			case '1080p': return { width: 1920, height: 1080 };
			default: return { width: 1280, height: 720 };
		}
	}

	async function applySettings() {
		const room = getRoom();
		if (!room || !$selfCamera) return;

		applying = true;
		error = null;
		try {
			// Toggle off then on with new constraints
			await room.localParticipant.setCameraEnabled(false);
			const res = getResolutionConstraints();
			await room.localParticipant.setCameraEnabled(true, {
				resolution: { width: res.width, height: res.height, frameRate: framerate },
				facingMode
			});
		} catch (err: any) {
			error = err.message || 'Failed to apply camera settings';
			console.error('[Camera] Settings error:', err);
		} finally {
			applying = false;
		}
	}
</script>

<div class="flex flex-col gap-3">
	<div class="flex flex-col gap-1">
		<label class="text-2xs font-medium uppercase tracking-wide text-text-secondary">Resolution</label>
		<select class="rounded border border-bg-tertiary bg-bg-primary px-2.5 py-1.5 text-sm text-text-primary outline-none focus:border-brand-500" bind:value={resolution}>
			<option value="360p">360p (Low bandwidth)</option>
			<option value="720p">720p (HD)</option>
			<option value="1080p">1080p (Full HD)</option>
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

	<div class="flex flex-col gap-1">
		<label class="text-2xs font-medium uppercase tracking-wide text-text-secondary">Camera</label>
		<select class="rounded border border-bg-tertiary bg-bg-primary px-2.5 py-1.5 text-sm text-text-primary outline-none focus:border-brand-500" bind:value={facingMode}>
			<option value="user">Front Camera</option>
			<option value="environment">Rear Camera</option>
		</select>
	</div>

	<button
		class="rounded bg-brand-500 px-4 py-1.5 text-sm font-medium text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60"
		onclick={applySettings}
		disabled={applying || !$selfCamera}
	>
		{applying ? 'Applying...' : 'Apply Settings'}
	</button>

	{#if !$selfCamera}
		<p class="text-xs text-text-muted">Enable your camera to change settings.</p>
	{/if}

	{#if error}
		<div class="text-xs text-red-500">{error}</div>
	{/if}
</div>
