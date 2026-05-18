<script lang="ts">
	import type { Channel } from '$lib/types';

	interface Props {
		x: number;
		y: number;
		thread: Channel;
		canManageThreads: boolean;
		onopen: (thread: Channel) => void;
		onhide: (thread: Channel) => void;
		onarchive: (thread: Channel, archive: boolean) => void;
		ondelete: (thread: Channel) => void;
	}

	let { x, y, thread, canManageThreads, onopen, onhide, onarchive, ondelete }: Props = $props();
</script>

<div
	class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
	style="left: {x}px; top: {y}px;"
	onclick={(e) => e.stopPropagation()}
	onkeydown={(e) => e.stopPropagation()}
	role="menu"
	tabindex="-1"
>
	<button
		class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
		onclick={() => onopen(thread)}
	>
		<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
		</svg>
		Open Thread
	</button>
	<button
		class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
		onclick={() => onhide(thread)}
	>
		<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
		</svg>
		Hide Thread
	</button>
	{#if canManageThreads}
		{#if thread.archived}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => onarchive(thread, false)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
				</svg>
				Unarchive Thread
			</button>
		{:else}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => onarchive(thread, true)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
				</svg>
				Archive Thread
			</button>
		{/if}
		<div class="my-1 border-t border-bg-modifier"></div>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
			onclick={() => ondelete(thread)}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
			Delete Thread
		</button>
	{/if}
</div>
