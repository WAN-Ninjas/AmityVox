<script lang="ts">
	import { page } from '$app/stores';
	import type { Channel, Message } from '$lib/types';
	import { setChannel, currentChannel } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import TopBar from '$components/layout/TopBar.svelte';
	import MemberList from '$components/layout/MemberList.svelte';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';
	import PinnedMessages from '$components/chat/PinnedMessages.svelte';
	import ThreadPanel from '$components/chat/ThreadPanel.svelte';

	let showMembers = $state(true);
	let showPins = $state(false);
	let activeThread = $state<{ channel: Channel; parentMessage: Message | null } | null>(null);

	// Set current channel when route params change and ack unreads.
	$effect(() => {
		const channelId = $page.params.channelId;
		if (channelId) {
			setChannel(channelId);
			ackChannel(channelId);
		}
	});

	function scrollToMessage(messageId: string) {
		const el = document.getElementById(`msg-${messageId}`);
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			el.classList.add('bg-brand-500/10');
			setTimeout(() => el.classList.remove('bg-brand-500/10'), 2000);
		}
	}

	function openThread(threadChannel: Channel, parentMessage: Message) {
		activeThread = { channel: threadChannel, parentMessage };
		showPins = false;
	}
</script>

<svelte:head>
	<title>
		{$currentChannel?.name ? `#${$currentChannel.name}` : 'Channel'}
		{$currentGuild ? ` — ${$currentGuild.name}` : ''}
		— AmityVox
	</title>
</svelte:head>

<div class="flex h-full">
	<div class="flex min-w-0 flex-1 flex-col">
		<TopBar
			onToggleMembers={() => (showMembers = !showMembers)}
			onTogglePins={() => { showPins = !showPins; if (showPins) activeThread = null; }}
			{showPins}
		/>
		<MessageList onopenthread={openThread} />
		<TypingIndicator typingUsers={$currentTypingUsers} />
		<MessageInput />
	</div>

	{#if activeThread}
		<ThreadPanel
			threadChannel={activeThread.channel}
			parentMessage={activeThread.parentMessage}
			onclose={() => (activeThread = null)}
		/>
	{/if}

	{#if showPins && !activeThread}
		<PinnedMessages onclose={() => (showPins = false)} onscrollto={scrollToMessage} />
	{/if}

	{#if showMembers && !showPins && !activeThread}
		<MemberList />
	{/if}
</div>
