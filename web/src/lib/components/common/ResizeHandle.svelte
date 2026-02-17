<!-- ResizeHandle.svelte — Invisible drag handle for resizing panes. -->
<script lang="ts">
	interface Props {
		/** Current width of the pane being resized. */
		width: number;
		/** Called with the new width during drag. */
		onresize: (width: number) => void;
		/** Called on double-click to reset to default width. */
		onreset?: () => void;
		/** 'left' means the pane is to the left (drag right = wider). 'right' means opposite. */
		side?: 'left' | 'right';
	}

	let { width, onresize, onreset, side = 'left' }: Props = $props();

	let dragging = $state(false);
	let startX = 0;
	let startWidth = 0;

	function handleMouseDown(e: MouseEvent) {
		e.preventDefault();
		dragging = true;
		startX = e.clientX;
		startWidth = width;
		document.addEventListener('mousemove', handleMouseMove);
		document.addEventListener('mouseup', handleMouseUp);
	}

	function handleMouseMove(e: MouseEvent) {
		const delta = e.clientX - startX;
		const newWidth = side === 'left' ? startWidth + delta : startWidth - delta;
		onresize(newWidth);
	}

	function handleMouseUp() {
		dragging = false;
		document.removeEventListener('mousemove', handleMouseMove);
		document.removeEventListener('mouseup', handleMouseUp);
	}

	function handleDblClick() {
		onreset?.();
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="group relative z-10 w-1 shrink-0 cursor-col-resize select-none {dragging ? 'bg-brand-500/40' : 'hover:bg-brand-500/20'}"
	onmousedown={handleMouseDown}
	ondblclick={handleDblClick}
	onkeydown={() => {}}
>
	<!-- Wider invisible hit area -->
	<div class="absolute inset-y-0 -left-1 -right-1"></div>
</div>
