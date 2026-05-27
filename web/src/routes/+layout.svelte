<script lang="ts">
	import { onMount } from 'svelte';
	import '../app.css';
	import type { Snippet } from 'svelte';
	import DesktopInstanceBar from '$lib/components/desktop/DesktopInstanceBar.svelte';
	import { isTauriRuntime } from '$lib/desktop/instances';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
	let desktopMode = $state(false);

	onMount(() => {
		desktopMode = isTauriRuntime();
		if (!desktopMode && 'serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js').catch(() => {
				// PWA support is optional; failed registration should not block the app.
			});
		}
	});
</script>

<DesktopInstanceBar />
<div
	class:pt-11={desktopMode}
	style={`--amityvox-shell-height: ${desktopMode ? 'calc(100vh - 2.75rem)' : '100vh'};`}
>
	{@render children()}
</div>
