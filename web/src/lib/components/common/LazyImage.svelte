<!-- LazyImage.svelte — Lazy-loading image component using IntersectionObserver.
     Only loads the actual image when it enters the viewport, showing a placeholder
     (blurhash or solid color) until then. Supports width/height for layout stability.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		src: string;
		alt: string;
		width?: number | null;
		height?: number | null;
		blurhash?: string | null;
		class?: string;
		/** Placeholder background color when no blurhash is available */
		placeholderColor?: string;
		/** Root margin for IntersectionObserver (how early to start loading) */
		rootMargin?: string;
		/** Loading threshold (0-1, fraction of element visible to trigger load) */
		threshold?: number;
	}

	let {
		src,
		alt,
		width = null,
		height = null,
		blurhash = null,
		class: className = '',
		placeholderColor = 'var(--bg-modifier)',
		rootMargin = '200px',
		threshold = 0.01
	}: Props = $props();

	let containerEl: HTMLDivElement | undefined = $state();
	let loaded = $state(false);
	let error = $state(false);
	let visible = $state(false);

	// Compute aspect ratio for placeholder sizing.
	let aspectRatio = $derived(
		width && height ? `${width} / ${height}` : undefined
	);

	onMount(() => {
		if (!containerEl) return;

		// Check if IntersectionObserver is available (always true in modern browsers).
		if (!('IntersectionObserver' in window)) {
			visible = true;
			return;
		}

		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) {
						visible = true;
						observer.disconnect();
					}
				}
			},
			{
				rootMargin,
				threshold
			}
		);

		observer.observe(containerEl);

		return () => observer.disconnect();
	});

	function handleLoad() {
		loaded = true;
	}

	function handleError() {
		error = true;
		loaded = true;
	}
</script>

<div
	bind:this={containerEl}
	class="lazy-image-container overflow-hidden {className}"
	style:aspect-ratio={aspectRatio}
	style:width={width ? `${width}px` : undefined}
	style:max-width="100%"
	style:background-color={!loaded ? placeholderColor : undefined}
>
	{#if visible && !error}
		<img
			{src}
			{alt}
			width={width ?? undefined}
			height={height ?? undefined}
			class="w-full h-full object-cover transition-opacity duration-300"
			class:opacity-0={!loaded}
			class:opacity-100={loaded}
			loading="lazy"
			decoding="async"
			onload={handleLoad}
			onerror={handleError}
		/>
	{/if}

	{#if error}
		<div class="flex items-center justify-center w-full h-full text-text-muted text-xs p-2">
			<svg class="w-5 h-5 mr-1 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
				<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909M3.75 21h16.5a2.25 2.25 0 002.25-2.25V5.25a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 003.75 21z" />
			</svg>
			<span>Failed to load</span>
		</div>
	{/if}

	<!-- Loading placeholder (shimmer effect) -->
	{#if !loaded && !error && visible}
		<div class="absolute inset-0 shimmer"></div>
	{/if}
</div>

<style>
	.lazy-image-container {
		position: relative;
		display: inline-block;
	}

	.shimmer {
		background: linear-gradient(
			90deg,
			transparent 25%,
			rgba(255, 255, 255, 0.05) 50%,
			transparent 75%
		);
		background-size: 200% 100%;
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% { background-position: -200% 0; }
		100% { background-position: 200% 0; }
	}
</style>
