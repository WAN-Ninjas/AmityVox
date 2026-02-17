<script lang="ts">
	interface Props {
		typeFilter: string;
		onfilter: (type: string) => void;
	}

	let { typeFilter, onfilter }: Props = $props();
</script>

<div class="flex items-center gap-2">
	<button
		class="rounded-full px-3 py-1 text-xs font-medium transition-colors {typeFilter === 'all' ? 'bg-brand-500 text-white' : 'bg-bg-modifier text-text-muted hover:text-text-secondary'}"
		onclick={() => onfilter('all')}
	>
		All
	</button>
	<button
		class="rounded-full px-3 py-1 text-xs font-medium transition-colors {typeFilter === 'image' ? 'bg-brand-500 text-white' : 'bg-bg-modifier text-text-muted hover:text-text-secondary'}"
		onclick={() => onfilter('image')}
	>
		Images
	</button>
	<button
		class="rounded-full px-3 py-1 text-xs font-medium transition-colors {typeFilter === 'video' ? 'bg-brand-500 text-white' : 'bg-bg-modifier text-text-muted hover:text-text-secondary'}"
		onclick={() => onfilter('video')}
	>
		Videos
	</button>
</div>
