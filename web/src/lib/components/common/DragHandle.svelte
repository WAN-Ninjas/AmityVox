<script lang="ts">
	interface Props {
		visible?: boolean;
	}
	let { visible = true }: Props = $props();
</script>

{#if visible}
	<div
		class="drag-handle flex shrink-0 cursor-grab items-center justify-center opacity-0 transition-opacity group-hover/drag:opacity-60 hover:!opacity-100 active:cursor-grabbing"
		aria-hidden="true"
	>
		<svg class="h-4 w-3 text-text-muted" viewBox="0 0 12 16" fill="currentColor">
			<circle cx="3.5" cy="2.5" r="1.5" />
			<circle cx="8.5" cy="2.5" r="1.5" />
			<circle cx="3.5" cy="7.5" r="1.5" />
			<circle cx="8.5" cy="7.5" r="1.5" />
			<circle cx="3.5" cy="12.5" r="1.5" />
			<circle cx="8.5" cy="12.5" r="1.5" />
		</svg>
	</div>
{/if}
