<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		open?: boolean;
		title?: string;
		persistent?: boolean;
		onclose?: () => void;
		children: Snippet;
	}

	let { open = false, title = '', persistent = false, onclose, children }: Props = $props();

	// Track where mousedown started so we only close when both press and
	// release occur on the backdrop — prevents accidental dismiss when
	// the user drags from inside the modal content to outside.
	let mouseDownOnBackdrop = false;

	function handleMouseDown(e: MouseEvent) {
		mouseDownOnBackdrop = e.target === e.currentTarget;
	}

	function handleMouseUp(e: MouseEvent) {
		if (!persistent && mouseDownOnBackdrop && e.target === e.currentTarget) {
			onclose?.();
		}
		mouseDownOnBackdrop = false;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && !persistent) onclose?.();
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
		onmousedown={handleMouseDown}
		onmouseup={handleMouseUp}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="w-full max-w-md rounded-lg bg-bg-secondary p-6 shadow-xl" onmousedown={(e) => e.stopPropagation()}>
			{#if title}
				<h2 class="mb-4 text-xl font-semibold text-text-primary">{title}</h2>
			{/if}
			{@render children()}
		</div>
	</div>
{/if}
