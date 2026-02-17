<!-- ParticipantContextMenu.svelte — Right-click menu for voice participants with volume slider. -->
<script lang="ts">
	import { getUserVolume, setUserVolume } from '$lib/utils/voiceVolume';
	import { isNoiseReductionEnabled, setNoiseReduction } from '$lib/utils/noiseReduction';

	interface Props {
		userId: string;
		displayName: string;
		x: number;
		y: number;
		onclose: () => void;
	}

	let { userId, displayName, x, y, onclose }: Props = $props();

	let volume = $state(getUserVolume(userId));
	let noiseEnabled = $state(isNoiseReductionEnabled(userId));

	function handleVolumeChange(e: Event) {
		const val = parseInt((e.target as HTMLInputElement).value, 10);
		volume = val;
		setUserVolume(userId, val);
	}

	function resetVolume() {
		volume = 100;
		setUserVolume(userId, 100);
	}

	function toggleNoise() {
		noiseEnabled = !noiseEnabled;
		setNoiseReduction(userId, noiseEnabled);
	}

	// Position clamping
	const menuWidth = 220;
	const menuHeight = 140;
	const left = $derived(x + menuWidth > window.innerWidth ? x - menuWidth : x);
	const top = $derived(y + menuHeight > window.innerHeight ? y - menuHeight : y);
</script>

<svelte:window onclick={onclose} oncontextmenu={onclose} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="fixed z-[60] w-[220px] rounded-lg bg-bg-floating p-2 shadow-xl"
	style="left: {left}px; top: {top}px;"
	onclick={(e) => e.stopPropagation()}
	oncontextmenu={(e) => e.stopPropagation()}
	onkeydown={(e) => { if (e.key === 'Escape') onclose(); }}
>
	<p class="mb-2 truncate text-xs font-semibold text-text-primary">{displayName}</p>

	<div class="mb-1">
		<div class="flex items-center justify-between">
			<label for="vol-{userId}" class="text-2xs font-medium uppercase tracking-wide text-text-muted">Volume</label>
			<span class="text-2xs font-mono text-text-muted">{volume}%</span>
		</div>
		<input
			id="vol-{userId}"
			type="range"
			min="0"
			max="200"
			step="5"
			value={volume}
			oninput={handleVolumeChange}
			class="mt-1 w-full accent-brand-500"
		/>
		<div class="mt-0.5 flex justify-between text-[10px] text-text-muted">
			<span>0%</span>
			<span>100%</span>
			<span>200%</span>
		</div>
	</div>

	{#if volume !== 100}
		<button
			class="mt-1 w-full rounded px-2 py-1 text-xs text-text-muted hover:bg-bg-modifier hover:text-text-primary"
			onclick={resetVolume}
		>
			Reset to 100%
		</button>
	{/if}

	<hr class="my-1.5 border-bg-modifier" />

	<button
		class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
		onclick={toggleNoise}
	>
		<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
		</svg>
		{noiseEnabled ? 'Disable' : 'Enable'} Noise Reduction
	</button>
</div>
