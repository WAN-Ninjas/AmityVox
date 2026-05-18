<script lang="ts">
	import DragHandle from '$components/common/DragHandle.svelte';
	import { activeThreadId, currentChannelId } from '$lib/stores/channels';
	import { mentionCounts, unreadCounts } from '$lib/stores/unreads';
	import { unlockedChannels } from '$lib/encryption/e2eeManager';
	import { isChannelMuted } from '$lib/stores/muting';
	import type { Channel } from '$lib/types';
	import type { DragController } from '$lib/utils/dragDrop';

	interface Props {
		channels: Channel[];
		canManageChannels: boolean;
		dragController: DragController | null;
		getfilteredthreads: (channelId: string) => Channel[];
		onchannelclick: (channelId: string) => void;
		onthreadclick: (thread: Channel) => void;
		oncontextmenu: (event: MouseEvent, channel: Channel) => void;
		onthreadcontextmenu: (event: MouseEvent, thread: Channel) => void;
	}

	let {
		channels,
		canManageChannels,
		dragController,
		getfilteredthreads,
		onchannelclick,
		onthreadclick,
		oncontextmenu,
		onthreadcontextmenu
	}: Props = $props();
</script>

{#each channels as channel (channel.id)}
	{@const unread = $unreadCounts.get(channel.id) ?? 0}
	{@const mentions = $mentionCounts.get(channel.id) ?? 0}
	{@const chMuted = isChannelMuted(channel.id)}
	<div
		class="group/drag flex items-center"
		data-channel-id={channel.id}
		onpointerdown={(e) => dragController?.handlePointerDown(e, channel.id)}
		role="listitem"
	>
		<DragHandle visible={canManageChannels} />
		<button
			class="mb-0.5 flex flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {chMuted ? 'opacity-60' : ''} {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : unread > 0 && !chMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
			onclick={() => onchannelclick(channel.id)}
			oncontextmenu={(e) => oncontextmenu(e, channel)}
		>
			{#if channel.encrypted}
				{@const unlocked = $unlockedChannels.has(channel.id)}
				<svg class="h-4 w-4 shrink-0 {unlocked ? 'text-green-400' : 'text-red-400'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<title>{unlocked ? 'Encrypted (unlocked)' : 'Encrypted (locked)'}</title>
					{#if unlocked}
						<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
					{:else}
						<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
					{/if}
				</svg>
			{:else}
				<span class="text-lg leading-none text-brand-500 font-mono">#</span>
			{/if}
			<span class="flex-1 truncate font-mono">{channel.name}</span>
			{#if chMuted}
				<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<title>Muted</title>
					<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
					<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
				</svg>
			{/if}
			{#if mentions > 0 && $currentChannelId !== channel.id}
				<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {chMuted ? 'bg-text-muted/50' : 'bg-red-500'} px-1 text-2xs font-bold text-white" title="{mentions} mention{mentions !== 1 ? 's' : ''}">
					@{mentions > 99 ? '99+' : mentions}
				</span>
			{:else if unread > 0 && $currentChannelId !== channel.id}
				<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {chMuted ? 'bg-text-muted/30' : 'bg-text-muted'} px-1 text-2xs font-bold text-white">
					{unread > 99 ? '99+' : unread}
				</span>
			{/if}
		</button>
	</div>
	{@const filteredThreads = getfilteredthreads(channel.id)}
	{#if filteredThreads.length > 0}
		<div class="ml-3 border-l border-bg-floating/50 pl-1">
			{#each filteredThreads as thread (thread.id)}
				{@const threadUnread = $unreadCounts.get(thread.id) ?? 0}
				{@const threadMentions = $mentionCounts.get(thread.id) ?? 0}
				<button
					class="mb-0.5 flex w-full items-center gap-1 rounded px-1.5 py-1 text-left text-xs transition-colors {$activeThreadId === thread.id ? 'bg-bg-modifier text-text-primary' : threadUnread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
					onclick={() => onthreadclick(thread)}
					oncontextmenu={(e) => onthreadcontextmenu(e, thread)}
				>
					<svg class="h-3.5 w-3.5 shrink-0 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
					</svg>
					<span class="flex-1 truncate">{thread.name}</span>
					{#if threadMentions > 0 && $activeThreadId !== thread.id}
						<span class="ml-auto flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-red-500 px-0.5 text-2xs font-bold text-white">
							@{threadMentions > 99 ? '99+' : threadMentions}
						</span>
					{:else if threadUnread > 0 && $activeThreadId !== thread.id}
						<span class="ml-auto flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-text-muted px-0.5 text-2xs font-bold text-white">
							{threadUnread > 99 ? '99+' : threadUnread}
						</span>
					{/if}
				</button>
			{/each}
		</div>
	{/if}
{/each}
