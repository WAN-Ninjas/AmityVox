<script lang="ts">
	let { readOnly = false }: { readOnly: boolean } = $props();
</script>

{#if readOnly}
	<div class="flex items-center gap-2 border-b border-bg-modifier bg-yellow-500/5 px-4 py-2">
		<svg class="h-4 w-4 shrink-0 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
			<path d="M7 11V7a5 5 0 0110 0v4" />
		</svg>
		<span class="text-xs text-yellow-400">This channel is read-only. Only users with specific roles can post messages.</span>
	</div>
{/if}
