<script lang="ts">
	import DragHandle from '$components/common/DragHandle.svelte';
	import { currentChannelId } from '$lib/stores/channels';
	import { mentionCounts, unreadCounts } from '$lib/stores/unreads';
	import type { Channel } from '$lib/types';
	import type { DragController } from '$lib/utils/dragDrop';

	interface Props {
		channels: Channel[];
		kind: 'forum' | 'gallery';
		canManageChannels: boolean;
		dragController: DragController | null;
		onchannelclick: (channelId: string) => void;
		oncontextmenu: (event: MouseEvent, channel: Channel) => void;
	}

	let { channels, kind, canManageChannels, dragController, onchannelclick, oncontextmenu }: Props = $props();
</script>

{#each channels as channel (channel.id)}
	{@const isActive = $currentChannelId === channel.id}
	{@const unread = $unreadCounts.get(channel.id) ?? 0}
	{@const mentions = $mentionCounts.get(channel.id) ?? 0}
	<div
		class="group/drag flex items-center"
		data-channel-id={channel.id}
		onpointerdown={(e) => dragController?.handlePointerDown(e, channel.id)}
		role="listitem"
	>
		<DragHandle visible={canManageChannels} />
		<button
			class="flex flex-1 items-center gap-1.5 rounded px-1.5 py-1 text-left text-sm transition-colors
				{isActive
					? 'bg-bg-modifier text-text-primary'
					: unread > 0
						? 'text-text-primary hover:bg-bg-modifier/50'
						: 'text-text-muted hover:bg-bg-modifier/50 hover:text-text-secondary'}"
			onclick={() => onchannelclick(channel.id)}
			oncontextmenu={(e) => oncontextmenu(e, channel)}
		>
			{#if kind === 'forum'}
				<svg class="h-5 w-5 shrink-0 {isActive ? 'text-text-primary' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
				</svg>
			{:else}
				<svg class="h-5 w-5 shrink-0 {isActive ? 'text-text-primary' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<rect x="3" y="3" width="7" height="7" rx="1" />
					<rect x="14" y="3" width="7" height="7" rx="1" />
					<rect x="3" y="14" width="7" height="7" rx="1" />
					<rect x="14" y="14" width="7" height="7" rx="1" />
				</svg>
			{/if}
			<span class="truncate {unread > 0 ? 'font-semibold' : ''}">{channel.name ?? kind}</span>
			{#if mentions > 0}
				<span class="ml-auto flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">{mentions}</span>
			{:else if unread > 0}
				<span class="ml-auto h-2 w-2 rounded-full bg-text-primary"></span>
			{/if}
		</button>
	</div>
{/each}
