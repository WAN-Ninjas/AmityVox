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
	class="group relative z-10 h-full w-1 shrink-0 cursor-col-resize select-none transition-colors {dragging ? 'bg-brand-500/50' : 'hover:bg-brand-500/30'}"
	onmousedown={handleMouseDown}
	ondblclick={handleDblClick}
	onkeydown={() => {}}
>
	<!-- Wider invisible hit area -->
	<div class="absolute inset-y-0 -left-1.5 -right-1.5"></div>
	<!-- Visual drag indicator (dots) on hover -->
	<div class="pointer-events-none absolute inset-x-0 top-1/2 flex -translate-y-1/2 flex-col items-center gap-1 opacity-0 transition-opacity {dragging ? 'opacity-100' : 'group-hover:opacity-100'}">
		<div class="h-1 w-1 rounded-full bg-text-muted"></div>
		<div class="h-1 w-1 rounded-full bg-text-muted"></div>
		<div class="h-1 w-1 rounded-full bg-text-muted"></div>
	</div>
</div>
