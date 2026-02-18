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
	let dialogEl: HTMLDivElement | undefined = $state();
	let previouslyFocusedElement: HTMLElement | null = null;

	const focusableSelector = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

	function trapFocus(e: KeyboardEvent) {
		if (e.key !== 'Tab' || !dialogEl) return;

		const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>(focusableSelector));
		if (focusable.length === 0) return;

		const first = focusable[0];
		const last = focusable[focusable.length - 1];

		if (e.shiftKey) {
			if (document.activeElement === first) {
				e.preventDefault();
				last.focus();
			}
		} else {
			if (document.activeElement === last) {
				e.preventDefault();
				first.focus();
			}
		}
	}

	// Save previous focus and focus first element on open; restore on close.
	$effect(() => {
		if (open && dialogEl) {
			previouslyFocusedElement = document.activeElement as HTMLElement;
			// Focus the first focusable element, or the dialog itself as fallback.
			const first = dialogEl.querySelector<HTMLElement>(focusableSelector);
			if (first) {
				first.focus();
			} else {
				dialogEl.focus();
			}
		}

		return () => {
			if (previouslyFocusedElement && typeof previouslyFocusedElement.focus === 'function') {
				previouslyFocusedElement.focus();
				previouslyFocusedElement = null;
			}
		};
	});

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
		trapFocus(e);
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={dialogEl}
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
		onmousedown={handleMouseDown}
		onmouseup={handleMouseUp}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		aria-labelledby={title ? 'modal-title' : undefined}
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="w-full max-w-md rounded-lg bg-bg-secondary p-6 shadow-xl" onmousedown={(e) => e.stopPropagation()}>
			{#if title}
				<h2 id="modal-title" class="mb-4 text-xl font-semibold text-text-primary">{title}</h2>
			{/if}
			{@render children()}
		</div>
	</div>
{/if}
