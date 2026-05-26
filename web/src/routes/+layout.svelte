<script lang="ts">
	import { onMount } from 'svelte';
	import '../app.css';
	import type { Snippet } from 'svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	onMount(() => {
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js').catch(() => {
				// PWA support is optional; failed registration should not block the app.
			});
		}
	});
</script>

{@render children()}
