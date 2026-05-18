<script lang="ts">
	import { goto } from '$app/navigation';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import { currentUser } from '$lib/stores/auth';
	import { relationships } from '$lib/stores/relationships';
	import { getDMRecipient } from '$lib/utils/dm';
	import { isChannelMuted, muteChannel, unmuteChannel } from '$lib/stores/muting';
	import type { Channel } from '$lib/types';

	interface Props {
		x: number;
		y: number;
		channel: Channel;
		onclose: () => void;
		onmarkread: (channelId: string) => void;
		onaddfriend: (channel: Channel) => void;
		onclosedm: (channelId: string) => void;
	}

	let { x, y, channel, onclose, onmarkread, onaddfriend, onclosedm }: Props = $props();
</script>

<ContextMenu {x} {y} {onclose}>
	<ContextMenuItem label="Open Message" onclick={() => { goto(`/app/dms/${channel.id}`); onclose(); }} />
	<ContextMenuItem label="Mark as Read" onclick={() => { onmarkread(channel.id); onclose(); }} />
	{@const recipient = getDMRecipient(channel, $currentUser?.id)}
	{#if recipient}
		{@const relationship = $relationships.get(recipient.id)}
		{#if !relationship || relationship.type === 'pending_incoming'}
			<ContextMenuItem
				label={relationship?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
				onclick={() => { onaddfriend(channel); onclose(); }}
			/>
		{:else if relationship.type === 'pending_outgoing'}
			<ContextMenuItem label="Request Sent" disabled />
		{/if}
	{/if}
	<ContextMenuDivider />
	{#if isChannelMuted(channel.id)}
		<ContextMenuItem label="Unmute Conversation" onclick={() => { unmuteChannel(channel.id); onclose(); }} />
	{:else}
		<ContextMenuItem label="Mute for 15 Minutes" onclick={() => { muteChannel(channel.id, 15 * 60 * 1000); onclose(); }} />
		<ContextMenuItem label="Mute for 1 Hour" onclick={() => { muteChannel(channel.id, 60 * 60 * 1000); onclose(); }} />
		<ContextMenuItem label="Mute for 8 Hours" onclick={() => { muteChannel(channel.id, 8 * 60 * 60 * 1000); onclose(); }} />
		<ContextMenuItem label="Mute for 24 Hours" onclick={() => { muteChannel(channel.id, 24 * 60 * 60 * 1000); onclose(); }} />
		<ContextMenuItem label="Mute Until I Turn It Back On" onclick={() => { muteChannel(channel.id); onclose(); }} />
	{/if}
	<ContextMenuDivider />
	<ContextMenuItem label="Close DM" danger onclick={() => { onclosedm(channel.id); onclose(); }} />
</ContextMenu>
