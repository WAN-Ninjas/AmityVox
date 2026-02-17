<!-- VideoTile.svelte — Reusable tile rendering a video stream or avatar fallback with speaking indicator. -->
<script lang="ts">
	import type { VideoTrackInfo, VoiceParticipant } from '$lib/stores/voice';
	import Avatar from '$components/common/Avatar.svelte';
	import { currentUser } from '$lib/stores/auth';
	import { isNoiseReductionEnabled, setNoiseReduction } from '$lib/utils/noiseReduction';

	interface Props {
		trackInfo: VideoTrackInfo | null;
		participant: VoiceParticipant;
		pinned?: boolean;
		onclick?: () => void;
	}

	let { trackInfo, participant, pinned = false, onclick }: Props = $props();

	let videoContainer = $state<HTMLDivElement | undefined>(undefined);
	let tileElement = $state<HTMLDivElement | undefined>(undefined);
	let isFullscreen = $state(false);
	let contextMenu = $state<{ x: number; y: number } | null>(null);
	let noiseEnabled = $state(false);

	$effect(() => {
		noiseEnabled = isNoiseReductionEnabled(participant.userId);
	});

	$effect(() => {
		if (!videoContainer) return;
		// Clear previous content
		videoContainer.innerHTML = '';
		if (trackInfo?.videoElement) {
			videoContainer.appendChild(trackInfo.videoElement);
			// Ensure playback starts (fallback if autoplay is blocked)
			trackInfo.videoElement.play().catch(() => {});
		}
	});

	function handleFullscreen(e: MouseEvent) {
		e.stopPropagation();
		if (!tileElement) return;
		if (document.fullscreenElement) {
			document.exitFullscreen();
		} else {
			tileElement.requestFullscreen().catch(() => {});
		}
	}

	function handleFocus(e: MouseEvent) {
		e.stopPropagation();
		onclick?.();
	}

	function onFullscreenChange() {
		isFullscreen = document.fullscreenElement === tileElement;
	}

	function handleContextMenu(e: MouseEvent) {
		if (participant.userId === $currentUser?.id) return;
		e.preventDefault();
		contextMenu = { x: e.clientX, y: e.clientY };
	}

	function toggleNoise() {
		noiseEnabled = !noiseEnabled;
		setNoiseReduction(participant.userId, noiseEnabled);
	}
</script>

<svelte:document onfullscreenchange={onFullscreenChange} />

<div
	bind:this={tileElement}
	class="video-tile group relative flex h-full w-full items-center justify-center overflow-hidden rounded-xl bg-bg-tertiary {participant.speaking ? 'ring-2 ring-green-500 ring-offset-0' : ''}"
	oncontextmenu={handleContextMenu}
>
	{#if trackInfo}
		<div bind:this={videoContainer} class="absolute inset-0 [&>video]:h-full [&>video]:w-full [&>video]:object-cover"></div>
	{:else}
		<div class="flex items-center justify-center">
			<Avatar
				name={participant.displayName ?? participant.username}
				size="lg"
			/>
		</div>
	{/if}

	<!-- Pin icon (top-right) -->
	{#if pinned}
		<div class="absolute right-2 top-2 z-10 flex h-6 w-6 items-center justify-center rounded-full bg-brand-500/80 text-white">
			<svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 24 24">
				<path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
			</svg>
		</div>
	{/if}

	<!-- Noise reduction indicator -->
	{#if noiseEnabled}
		<div class="absolute left-2 top-2 z-10 flex h-5 items-center gap-0.5 rounded-full bg-blue-500/80 px-1.5 text-[10px] font-medium text-white" title="Noise reduction active">
			<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
			</svg>
			NR
		</div>
	{/if}

	<!-- Hover actions: Focus + Fullscreen -->
	<div class="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 rounded-xl bg-black/0 opacity-0 transition-all group-hover:bg-black/30 group-hover:opacity-100">
		{#if onclick}
			<button
				type="button"
				class="pointer-events-auto rounded-md bg-black/70 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-black/90"
				onclick={handleFocus}
			>
				{pinned ? 'Unfocus' : 'Focus'}
			</button>
		{/if}
		<button
			type="button"
			class="pointer-events-auto rounded-md bg-black/70 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-black/90"
			onclick={handleFullscreen}
		>
			{isFullscreen ? 'Exit Fullscreen' : 'Fullscreen'}
		</button>
	</div>

	<!-- Speaking pulse animation -->
	{#if participant.speaking}
		<div class="speaking-pulse pointer-events-none absolute inset-0 rounded-xl"></div>
	{/if}

	<!-- Bottom overlay: name + badges -->
	<div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent px-2.5 pb-1.5 pt-6">
		<div class="flex items-center gap-1.5">
			<span class="truncate text-xs font-medium text-white">
				{participant.displayName ?? participant.username}
			</span>
			{#if trackInfo?.source === 'screenshare'}
				<span class="shrink-0 rounded-full bg-blue-500/80 px-1.5 py-0.5 text-[10px] font-semibold text-white">
					Screen
				</span>
			{/if}
			{#if participant.deafened}
				<svg class="h-3.5 w-3.5 shrink-0 text-red-400" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
					<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
					<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
				</svg>
			{:else if participant.muted}
				<svg class="h-3.5 w-3.5 shrink-0 text-red-400" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
					<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
				</svg>
			{/if}
		</div>
	</div>
</div>

<!-- Participant context menu (noise reduction toggle) -->
{#if contextMenu}
	<svelte:window onclick={() => (contextMenu = null)} />
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed z-[60] min-w-[180px] rounded-lg bg-bg-floating p-1 shadow-xl"
		style="left: {contextMenu.x}px; top: {contextMenu.y}px;"
		onclick={(e) => e.stopPropagation()}
		onkeydown={(e) => { if (e.key === 'Escape') contextMenu = null; }}
	>
		<p class="mb-1 truncate px-2 py-1 text-xs font-semibold text-text-primary">{participant.displayName ?? participant.username}</p>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { toggleNoise(); contextMenu = null; }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
			</svg>
			{noiseEnabled ? 'Disable' : 'Enable'} Noise Reduction
		</button>
	</div>
{/if}

<style>
	@keyframes speaking-pulse {
		0%, 100% { box-shadow: inset 0 0 0 2px rgba(34, 197, 94, 0.6); }
		50% { box-shadow: inset 0 0 0 2px rgba(34, 197, 94, 0.2); }
	}
	.speaking-pulse {
		animation: speaking-pulse 1.5s ease-in-out infinite;
	}
</style>
