<script lang="ts">
	import { fade } from 'svelte/transition';

	interface Props {
		src: string;
		alt?: string;
		onclose: () => void;
	}

	let { src, alt = '', onclose }: Props = $props();
	let scale = $state(1);

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
		if (e.key === '+' || e.key === '=') scale = Math.min(scale + 0.25, 5);
		if (e.key === '-') scale = Math.max(scale - 0.25, 0.25);
		if (e.key === '0') scale = 1;
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		if (e.deltaY < 0) {
			scale = Math.min(scale + 0.1, 5);
		} else {
			scale = Math.max(scale - 0.1, 0.25);
		}
	}

	function handleBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget) onclose();
	}

	function download() {
		const a = document.createElement('a');
		a.href = src;
		a.download = alt || 'image';
		a.click();
	}
</script>

<svelte:document onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="fixed inset-0 z-[150] flex items-center justify-center bg-black/80"
	onclick={handleBackdrop}
	onwheel={handleWheel}
	transition:fade={{ duration: 150 }}
	role="dialog"
	aria-modal="true"
>
	<!-- Toolbar -->
	<div class="absolute left-1/2 top-4 z-10 flex -translate-x-1/2 items-center gap-2 rounded-lg bg-bg-floating/80 px-3 py-1.5 backdrop-blur">
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={() => (scale = Math.min(scale + 0.25, 5))} title="Zoom In">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v6m3-3H7" /></svg>
		</button>
		<span class="min-w-[3rem] text-center text-xs text-text-muted">{Math.round(scale * 100)}%</span>
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={() => (scale = Math.max(scale - 0.25, 0.25))} title="Zoom Out">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM7 10h6" /></svg>
		</button>
		<div class="mx-1 h-4 w-px bg-bg-modifier"></div>
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={download} title="Download">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
		</button>
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={onclose} title="Close">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M6 18L18 6M6 6l12 12" /></svg>
		</button>
	</div>

	<img
		{src}
		{alt}
		class="max-h-[90vh] max-w-[90vw] object-contain transition-transform duration-100"
		style="transform: scale({scale})"
		draggable="false"
	/>
</div>
