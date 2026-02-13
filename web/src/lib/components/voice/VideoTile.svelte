<!-- VideoTile.svelte — Reusable tile rendering a video stream or avatar fallback with speaking indicator. -->
<script lang="ts">
	import type { VideoTrackInfo, VoiceParticipant } from '$lib/stores/voice';
	import Avatar from '$components/common/Avatar.svelte';

	interface Props {
		trackInfo: VideoTrackInfo | null;
		participant: VoiceParticipant;
		onclick?: () => void;
	}

	let { trackInfo, participant, onclick }: Props = $props();

	let videoContainer = $state<HTMLDivElement | undefined>(undefined);

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
</script>

<button
	type="button"
	class="video-tile group relative flex h-full w-full items-center justify-center overflow-hidden rounded-xl bg-bg-tertiary {participant.speaking ? 'ring-2 ring-green-500 ring-offset-0' : ''} {onclick ? 'cursor-pointer' : 'cursor-default'}"
	{onclick}
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
</button>

<style>
	@keyframes speaking-pulse {
		0%, 100% { box-shadow: inset 0 0 0 2px rgba(34, 197, 94, 0.6); }
		50% { box-shadow: inset 0 0 0 2px rgba(34, 197, 94, 0.2); }
	}
	.speaking-pulse {
		animation: speaking-pulse 1.5s ease-in-out infinite;
	}
</style>
