<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		x: number;
		y: number;
		onclose: () => void;
		children: Snippet;
	}

	let { x, y, onclose, children }: Props = $props();
	let menuEl: HTMLDivElement;

	// Adjust position to stay within viewport.
	let adjustedX = $state(x);
	let adjustedY = $state(y);

	$effect(() => {
		if (!menuEl) return;
		const rect = menuEl.getBoundingClientRect();
		const vw = window.innerWidth;
		const vh = window.innerHeight;

		adjustedX = x + rect.width > vw ? vw - rect.width - 8 : x;
		adjustedY = y + rect.height > vh ? vh - rect.height - 8 : y;
	});

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.stopPropagation();
			onclose();
		}
	}

	function handleClickOutside(e: MouseEvent) {
		if (menuEl && !menuEl.contains(e.target as Node)) {
			onclose();
		}
	}

	$effect(() => {
		document.addEventListener('click', handleClickOutside, true);
		document.addEventListener('contextmenu', handleClickOutside, true);
		return () => {
			document.removeEventListener('click', handleClickOutside, true);
			document.removeEventListener('contextmenu', handleClickOutside, true);
		};
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	bind:this={menuEl}
	class="fixed z-[100] min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg animate-in"
	style="left: {adjustedX}px; top: {adjustedY}px;"
	onkeydown={handleKeydown}
	role="menu"
	tabindex="-1"
>
	{@render children()}
</div>

<style>
	.animate-in {
		animation: context-menu-in 100ms ease-out;
	}

	@keyframes context-menu-in {
		from {
			opacity: 0;
			transform: scale(0.95);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}
</style>
