<script lang="ts">
	interface Props {
		src: string;
		waveform?: number[] | null;
		durationMs?: number | null;
	}
	let { src, waveform = null, durationMs = null }: Props = $props();

	let audioEl: HTMLAudioElement;
	let playing = $state(false);
	let currentTime = $state(0);
	let duration = $state(0);
	let progress = $derived(duration > 0 ? currentTime / duration : 0);

	const bars = $derived(waveform ?? generateDefaultBars(40));

	function generateDefaultBars(count: number): number[] {
		return Array.from({ length: count }, () => Math.floor(Math.random() * 180 + 30));
	}

	function togglePlay() {
		if (!audioEl) return;
		if (playing) {
			audioEl.pause();
		} else {
			audioEl.play();
		}
	}

	function handleTimeUpdate() {
		currentTime = audioEl.currentTime;
	}

	function handleLoadedMetadata() {
		duration = audioEl.duration;
		if (durationMs && duration === Infinity) {
			duration = durationMs / 1000;
		}
	}

	function handleEnded() {
		playing = false;
		currentTime = 0;
	}

	function seekTo(e: MouseEvent) {
		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const pct = (e.clientX - rect.left) / rect.width;
		if (audioEl && duration > 0) {
			audioEl.currentTime = pct * duration;
		}
	}

	function formatTime(seconds: number): string {
		const s = Math.floor(seconds);
		const m = Math.floor(s / 60);
		return `${m}:${String(s % 60).padStart(2, '0')}`;
	}
</script>

<div class="flex items-center gap-3 rounded-lg bg-bg-secondary px-3 py-2 max-w-sm">
	<audio
		bind:this={audioEl}
		{src}
		preload="metadata"
		ontimeupdate={handleTimeUpdate}
		onloadedmetadata={handleLoadedMetadata}
		onended={handleEnded}
		onplay={() => (playing = true)}
		onpause={() => (playing = false)}
	></audio>

	<!-- Play/Pause button -->
	<button
		class="shrink-0 rounded-full bg-brand-500 p-2 text-white hover:bg-brand-600"
		onclick={togglePlay}
	>
		{#if playing}
			<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
				<path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
			</svg>
		{:else}
			<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
				<path d="M8 5v14l11-7z" />
			</svg>
		{/if}
	</button>

	<!-- Waveform with progress -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="flex h-8 flex-1 cursor-pointer items-center gap-px" onclick={seekTo}>
		{#each bars as bar, i}
			{@const barProgress = i / bars.length}
			<div
				class="w-1 shrink-0 rounded-full transition-colors {barProgress <= progress ? 'bg-brand-400' : 'bg-text-muted/30'}"
				style="height: {Math.max(3, (bar / 255) * 28)}px"
			></div>
		{/each}
	</div>

	<!-- Duration -->
	<span class="shrink-0 text-xs font-mono text-text-muted">
		{#if playing || currentTime > 0}
			{formatTime(currentTime)}
		{:else if duration > 0}
			{formatTime(duration)}
		{:else if durationMs}
			{formatTime(durationMs / 1000)}
		{:else}
			0:00
		{/if}
	</span>
</div>
