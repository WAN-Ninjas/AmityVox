<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		src: string;
		poster?: string;
		width?: number;
		height?: number;
		filename: string;
	}

	let { src, poster, width, height, filename }: Props = $props();

	let videoEl: HTMLVideoElement;
	let containerEl: HTMLDivElement;
	let progressEl: HTMLDivElement;
	let paused = $state(true);
	let currentTime = $state(0);
	let duration = $state(0);
	let volume = $state(1);
	let muted = $state(false);
	let isFullscreen = $state(false);
	let showControls = $state(true);
	let controlsTimeout: ReturnType<typeof setTimeout> | null = null;
	let seeking = false;
	let hasPlayed = $state(false);

	const progress = $derived(duration > 0 ? (currentTime / duration) * 100 : 0);

	// Attach all media event listeners via onMount — zero reactive tracking.
	onMount(() => {
		const el = videoEl;
		if (!el) return;

		const handlePlay = () => {
			paused = false;
			hasPlayed = true;
			startControlsTimer();
		};
		const handlePause = () => {
			paused = true;
			showControls = true;
			clearControlsTimer();
		};
		const handleTimeUpdate = () => {
			if (!seeking) currentTime = el.currentTime;
		};
		const handleDurationChange = () => {
			if (isFinite(el.duration)) duration = el.duration;
		};
		const handleVolumeChange = () => {
			volume = el.volume;
			muted = el.muted;
		};
		const handleEnded = () => {
			paused = true;
			hasPlayed = false;
			if (el) el.currentTime = 0;
			currentTime = 0;
			showControls = true;
			clearControlsTimer();
		};

		el.addEventListener('play', handlePlay);
		el.addEventListener('pause', handlePause);
		el.addEventListener('timeupdate', handleTimeUpdate);
		el.addEventListener('durationchange', handleDurationChange);
		el.addEventListener('loadedmetadata', handleDurationChange);
		el.addEventListener('volumechange', handleVolumeChange);
		el.addEventListener('ended', handleEnded);

		return () => {
			el.removeEventListener('play', handlePlay);
			el.removeEventListener('pause', handlePause);
			el.removeEventListener('timeupdate', handleTimeUpdate);
			el.removeEventListener('durationchange', handleDurationChange);
			el.removeEventListener('loadedmetadata', handleDurationChange);
			el.removeEventListener('volumechange', handleVolumeChange);
			el.removeEventListener('ended', handleEnded);
		};
	});

	function togglePlay() {
		if (!videoEl) return;
		if (videoEl.paused) {
			videoEl.play().catch(() => {});
		} else {
			videoEl.pause();
		}
	}

	function seekTo(clientX: number) {
		if (!progressEl || !videoEl || duration <= 0) return;
		const rect = progressEl.getBoundingClientRect();
		const pct = (clientX - rect.left) / rect.width;
		const t = Math.max(0, Math.min(pct * duration, duration));
		videoEl.currentTime = t;
		currentTime = t;
	}

	function handleProgressMouseDown(e: MouseEvent) {
		e.stopPropagation();
		seeking = true;
		seekTo(e.clientX);
		const handleMove = (ev: MouseEvent) => seekTo(ev.clientX);
		const handleUp = () => {
			seeking = false;
			window.removeEventListener('mousemove', handleMove);
			window.removeEventListener('mouseup', handleUp);
		};
		window.addEventListener('mousemove', handleMove);
		window.addEventListener('mouseup', handleUp);
	}

	function handleVolumeInput(e: Event) {
		e.stopPropagation();
		const input = e.target as HTMLInputElement;
		const v = parseFloat(input.value);
		if (videoEl) {
			videoEl.volume = v;
			videoEl.muted = v === 0;
		}
	}

	function toggleMute(e: MouseEvent) {
		e.stopPropagation();
		if (!videoEl) return;
		videoEl.muted = !videoEl.muted;
	}

	function toggleFullscreen(e: MouseEvent) {
		e.stopPropagation();
		if (!containerEl) return;
		if (!document.fullscreenElement) {
			containerEl.requestFullscreen().then(() => {
				isFullscreen = true;
			}).catch(() => {});
		} else {
			document.exitFullscreen().then(() => {
				isFullscreen = false;
			}).catch(() => {});
		}
	}

	function clearControlsTimer() {
		if (controlsTimeout) {
			clearTimeout(controlsTimeout);
			controlsTimeout = null;
		}
	}

	function startControlsTimer() {
		clearControlsTimer();
		controlsTimeout = setTimeout(() => {
			if (!paused) showControls = false;
		}, 2000);
	}

	function handleMouseEnter() {
		showControls = true;
		if (!paused) startControlsTimer();
	}

	function handleMouseMove() {
		showControls = true;
		if (!paused) startControlsTimer();
	}

	function handleMouseLeave() {
		if (!paused) startControlsTimer();
	}

	function handlePlayPauseClick(e: MouseEvent) {
		e.stopPropagation();
		togglePlay();
	}

	function formatTime(seconds: number): string {
		if (!isFinite(seconds) || seconds < 0) return '0:00';
		const s = Math.floor(seconds);
		const m = Math.floor(s / 60);
		const h = Math.floor(m / 60);
		if (h > 0) {
			return `${h}:${String(m % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
		}
		return `${m}:${String(s % 60).padStart(2, '0')}`;
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	bind:this={containerEl}
	class="group relative inline-flex max-w-lg max-h-96 overflow-hidden rounded bg-black"
	class:w-full={isFullscreen}
	class:max-h-full={isFullscreen}
	class:max-w-full={isFullscreen}
	onmouseenter={handleMouseEnter}
	onmousemove={handleMouseMove}
	onmouseleave={handleMouseLeave}
>
	<!-- svelte-ignore a11y_media_has_caption -->
	<video
		bind:this={videoEl}
		{src}
		{poster}
		{width}
		{height}
		preload="metadata"
		class="max-h-96 max-w-lg {isFullscreen ? 'max-h-full max-w-full w-full h-full object-contain' : ''}"
	></video>

	<!-- Transparent click overlay for play/pause toggle (above video, below controls) -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="absolute inset-0 z-[1] cursor-pointer"
		onclick={handlePlayPauseClick}
	></div>

	<!-- Play overlay (shown when paused and video hasn't started yet) -->
	{#if paused && !hasPlayed}
		<button
			class="absolute inset-0 z-[2] flex cursor-pointer items-center justify-center bg-black/30"
			onclick={handlePlayPauseClick}
		>
			<div class="flex h-14 w-14 items-center justify-center rounded-full bg-bg-secondary/80 shadow-lg">
				<svg class="ml-1 h-7 w-7 text-text-primary" fill="currentColor" viewBox="0 0 24 24">
					<path d="M8 5v14l11-7z" />
				</svg>
			</div>
		</button>
	{/if}

	<!-- Custom controls overlay -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="absolute inset-x-0 bottom-0 z-[3] flex flex-col gap-1 bg-gradient-to-t from-black/80 to-transparent px-3 pb-2 pt-6 transition-opacity duration-200"
		class:opacity-0={!showControls}
		class:pointer-events-none={!showControls}
		onclick={(e) => e.stopPropagation()}
	>
		<!-- Progress bar -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			bind:this={progressEl}
			class="group/progress relative h-1 w-full cursor-pointer rounded-full bg-white/20 hover:h-1.5"
			onmousedown={handleProgressMouseDown}
		>
			<div
				class="absolute inset-y-0 left-0 rounded-full bg-brand-500"
				style="width: {progress}%"
			></div>
			<div
				class="absolute top-1/2 -translate-y-1/2 h-3 w-3 rounded-full bg-brand-400 opacity-0 group-hover/progress:opacity-100 transition-opacity"
				style="left: calc({progress}% - 6px)"
			></div>
		</div>

		<!-- Controls row -->
		<div class="flex items-center gap-2">
			<!-- Play/Pause -->
			<button
				class="shrink-0 text-white hover:text-brand-400"
				onclick={handlePlayPauseClick}
				title={paused ? 'Play' : 'Pause'}
			>
				{#if !paused}
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
					</svg>
				{:else}
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M8 5v14l11-7z" />
					</svg>
				{/if}
			</button>

			<!-- Time display -->
			<span class="shrink-0 text-2xs font-mono text-white/80">
				{formatTime(currentTime)} / {formatTime(duration)}
			</span>

			<div class="flex-1"></div>

			<!-- Volume -->
			<div class="flex items-center gap-1">
				<button
					class="shrink-0 text-white hover:text-brand-400"
					onclick={toggleMute}
					title={muted ? 'Unmute' : 'Mute'}
				>
					{#if muted || volume === 0}
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
							<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
						</svg>
					{:else}
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						</svg>
					{/if}
				</button>
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<input
					type="range"
					min="0"
					max="1"
					step="0.05"
					value={muted ? 0 : volume}
					oninput={handleVolumeInput}
					onclick={(e) => e.stopPropagation()}
					class="h-1 w-16 cursor-pointer appearance-none rounded-full bg-white/20 accent-brand-500 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white"
				/>
			</div>

			<!-- Fullscreen -->
			<button
				class="shrink-0 text-white hover:text-brand-400"
				onclick={toggleFullscreen}
				title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
			>
				{#if isFullscreen}
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M8 3v3a2 2 0 01-2 2H3m18 0h-3a2 2 0 01-2-2V3m0 18v-3a2 2 0 012-2h3M3 16h3a2 2 0 012 2v3" />
					</svg>
				{:else}
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
					</svg>
				{/if}
			</button>
		</div>
	</div>

	<!-- Filename tooltip (shown on hover at top) -->
	<div
		class="absolute inset-x-0 top-0 z-[3] truncate bg-gradient-to-b from-black/60 to-transparent px-3 py-2 text-2xs text-white/70 transition-opacity duration-200"
		class:opacity-0={!showControls}
		class:pointer-events-none={!showControls}
	>
		{filename}
	</div>
</div>
