<script lang="ts">
	import { page } from '$app/stores';
	import { setChannel, currentChannelId } from '$lib/stores/channels';
	import { currentGuildId, setGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';

	// Ensure we're not in a guild context for DMs.
	$effect(() => {
		if ($currentGuildId) {
			setGuild(null);
		}
	});

	// Set current channel when route params change.
	$effect(() => {
		const channelId = $page.params.channelId;
		if (channelId) {
			setChannel(channelId);
			ackChannel(channelId);
		}
	});
</script>

<svelte:head>
	<title>DM — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col">
	<header class="flex h-12 items-center border-b border-bg-floating bg-bg-tertiary px-4">
		<svg class="mr-2 h-5 w-5 text-text-muted" fill="currentColor" viewBox="0 0 24 24">
			<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5 0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29 1.94-3.5 3.22-6 3.22z" />
		</svg>
		<h1 class="font-semibold text-text-primary">Direct Message</h1>
	</header>
	<MessageList />
	<TypingIndicator typingUsers={$currentTypingUsers} />
	<MessageInput />
</div>
