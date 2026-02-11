<script lang="ts">
	import { page } from '$app/stores';
	import { setChannel, currentChannel } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';
	import TopBar from '$components/layout/TopBar.svelte';
	import MemberList from '$components/layout/MemberList.svelte';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';

	let showMembers = $state(true);

	// Set current channel when route params change.
	$effect(() => {
		const channelId = $page.params.channelId;
		if (channelId) {
			setChannel(channelId);
		}
	});
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
		<TopBar onToggleMembers={() => (showMembers = !showMembers)} />
		<MessageList />
		<TypingIndicator />
		<MessageInput />
	</div>

	{#if showMembers}
		<MemberList />
	{/if}
</div>
