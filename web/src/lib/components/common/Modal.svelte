<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		open?: boolean;
		title?: string;
		onclose?: () => void;
		children: Snippet;
	}

	let { open = false, title = '', onclose, children }: Props = $props();

	function handleBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget) onclose?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose?.();
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
		onclick={handleBackdrop}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<div class="w-full max-w-md rounded-lg bg-bg-secondary p-6 shadow-xl">
			{#if title}
				<h2 class="mb-4 text-xl font-semibold text-text-primary">{title}</h2>
			{/if}
			{@render children()}
		</div>
	</div>
{/if}
