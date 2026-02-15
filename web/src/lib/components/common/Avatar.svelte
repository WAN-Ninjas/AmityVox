<script lang="ts">
	interface Props {
		src?: string | null;
		name?: string;
		size?: 'sm' | 'md' | 'lg';
		status?: string | null;
	}

	let { src = null, name = '?', size = 'md', status = null }: Props = $props();

	const sizes = { sm: 'w-8 h-8 text-xs', md: 'w-10 h-10 text-sm', lg: 'w-20 h-20 text-xl' };
	const statusColors: Record<string, string> = {
		online: 'bg-status-online',
		idle: 'bg-status-idle',
		dnd: 'bg-status-dnd',
		offline: 'bg-status-offline'
	};

	const initials = $derived(
		name
			.split(' ')
			.map((w) => w[0])
			.join('')
			.slice(0, 2)
			.toUpperCase()
	);
</script>

<div class="relative inline-flex shrink-0">
	{#if src}
		<img class="{sizes[size]} rounded-md object-cover" {src} alt={name} />
	{:else}
		<div
			class="{sizes[size]} flex items-center justify-center rounded-md bg-brand-600 font-semibold text-white"
		>
			{initials}
		</div>
	{/if}

	{#if status}
		<span
			class="absolute bottom-0 right-0 block h-3 w-3 rounded-full ring-2 ring-bg-secondary {statusColors[
				status
			] ?? statusColors.offline}"
		></span>
	{/if}
</div>
