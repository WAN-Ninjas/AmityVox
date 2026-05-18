<script lang="ts">
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import type { Message } from '$lib/types';

	interface Props {
		x: number;
		y: number;
		message: Message;
		isOwnMessage: boolean;
		canManageMessages: boolean;
		canCreateThreads: boolean;
		canModerateAuthor: boolean;
		canTimeoutMembers: boolean;
		canKickMembers: boolean;
		canBanMembers: boolean;
		onclose: () => void;
		onviewprofile: () => void;
		onreply: () => void;
		oncopytext: () => void;
		onedit: () => void;
		onpin: () => void;
		oncreatethread: () => void;
		onviewthread: () => void;
		oncopylink: () => void;
		oncopyuserid: () => void;
		onbookmark: () => void;
		onforward: () => void;
		onquote: () => void;
		onreport: () => void;
		ontimeout: (seconds: number) => void;
		onkick: () => void;
		onban: () => void;
		ondelete: () => void;
	}

	let props: Props = $props();
	let showTimeoutSubmenu = $state(false);

	const timeoutPresets = [
		{ label: '1 minute', seconds: 60 },
		{ label: '5 minutes', seconds: 300 },
		{ label: '15 minutes', seconds: 900 },
		{ label: '1 hour', seconds: 3600 }
	];
</script>

<ContextMenu x={props.x} y={props.y} onclose={props.onclose}>
	<ContextMenuItem label="View Profile" onclick={props.onviewprofile} />
	<ContextMenuDivider />
	<ContextMenuItem label="Reply" onclick={props.onreply} />
	{#if props.message.content}
		<ContextMenuItem label="Copy Text" onclick={props.oncopytext} />
	{/if}
	{#if props.isOwnMessage}
		<ContextMenuItem label="Edit Message" onclick={props.onedit} />
	{/if}
	{#if props.isOwnMessage || props.canManageMessages}
		<ContextMenuItem label={props.message.pinned ? 'Unpin Message' : 'Pin Message'} onclick={props.onpin} />
	{/if}
	{#if !props.message.thread_id}
		{#if props.canCreateThreads}
			<ContextMenuItem label="Create Thread" onclick={props.oncreatethread} />
		{/if}
	{:else}
		<ContextMenuItem label="View Thread" onclick={props.onviewthread} />
	{/if}
	<ContextMenuItem label="Copy Message Link" onclick={props.oncopylink} />
	<ContextMenuItem label="Copy User ID" onclick={props.oncopyuserid} />
	<ContextMenuItem label="Bookmark" onclick={props.onbookmark} />
	<ContextMenuItem label="Forward" onclick={props.onforward} />
	{#if props.message.content}
		<ContextMenuItem label="Quote in Channel" onclick={props.onquote} />
	{/if}
	{#if !props.isOwnMessage}
		<ContextMenuDivider />
		<ContextMenuItem label="Report Message" danger onclick={props.onreport} />
	{/if}
	{#if props.canModerateAuthor && (props.canTimeoutMembers || props.canKickMembers || props.canBanMembers)}
		<ContextMenuDivider />
		{#if props.canTimeoutMembers}
			<div class="relative">
				<button
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
					onclick={(e) => { e.stopPropagation(); showTimeoutSubmenu = !showTimeoutSubmenu; }}
				>
					Timeout User
					<svg class="ml-auto h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
				{#if showTimeoutSubmenu}
					{@const submenuLeft = props.x + 360 < (typeof window === 'undefined' ? 1024 : window.innerWidth)}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="absolute top-0 min-w-[140px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}"
						onclick={(e) => e.stopPropagation()}
						onkeydown={() => {}}
					>
						{#each timeoutPresets as preset}
							<button
								class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
								onclick={() => props.ontimeout(preset.seconds)}
							>
								{preset.label}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		{#if props.canKickMembers}
			<ContextMenuItem label="Kick User" danger onclick={props.onkick} />
		{/if}
		{#if props.canBanMembers}
			<ContextMenuItem label="Ban User" danger onclick={props.onban} />
		{/if}
	{/if}
	{#if props.isOwnMessage || props.canManageMessages}
		<ContextMenuDivider />
		<ContextMenuItem label="Delete Message" danger onclick={props.ondelete} />
	{/if}
</ContextMenu>
