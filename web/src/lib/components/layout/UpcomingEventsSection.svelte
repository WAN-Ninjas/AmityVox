<script lang="ts">
	import type { GuildEvent } from '$lib/types';

	interface Props {
		events: GuildEvent[];
		collapsed: boolean;
		ontoggle: () => void;
		onviewall: () => void;
	}

	let { events, collapsed, ontoggle, onviewall }: Props = $props();

	function formatEventDate(dateStr: string): string {
		const d = new Date(dateStr);
		const now = new Date();
		const diffMs = d.getTime() - now.getTime();
		const diffH = Math.floor(diffMs / 3600000);
		if (diffH < 1) return 'Starting soon';
		if (diffH < 24) return `In ${diffH}h`;
		const diffD = Math.floor(diffH / 24);
		if (diffD === 1) return 'Tomorrow';
		return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
	}
</script>

{#if events.length > 0}
	<div class="mb-1 flex items-center justify-between px-1 pt-4">
		<button
			class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary"
			onclick={ontoggle}
			title={collapsed ? 'Expand Upcoming Events' : 'Collapse Upcoming Events'}
		>
			<svg
				class="h-3 w-3 shrink-0 transition-transform duration-200 {collapsed ? '-rotate-90' : ''}"
				fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
			>
				<path d="M19 9l-7 7-7-7" />
			</svg>
			Upcoming Events
		</button>
		<button
			class="text-text-muted hover:text-text-primary"
			onclick={onviewall}
			title="View All Events"
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
			</svg>
		</button>
	</div>
	{#if !collapsed}
		{#each events as event (event.id)}
			<button
				class="mb-0.5 flex w-full items-start gap-2 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
				onclick={onviewall}
			>
				<svg class="mt-0.5 h-4 w-4 shrink-0 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 012 2v12a2 2 0 002 2z" />
				</svg>
				<div class="min-w-0 flex-1">
					<span class="block truncate text-xs font-medium text-text-primary">{event.name}</span>
					<span class="text-2xs text-text-muted">{formatEventDate(event.scheduled_start)}</span>
				</div>
			</button>
		{/each}
	{/if}
{/if}
