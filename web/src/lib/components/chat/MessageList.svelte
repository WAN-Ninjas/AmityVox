<script lang="ts">
	import { onMount, tick } from 'svelte';
	import type { Message } from '$lib/types';
	import { currentChannelId } from '$lib/stores/channels';
	import { getChannelMessages, loadMessages, isLoadingMessages } from '$lib/stores/messages';
	import MessageItem from './MessageItem.svelte';

	let messagesContainer: HTMLDivElement;
	let shouldAutoScroll = $state(true);

	// Reactive: load messages when channel changes.
	$effect(() => {
		const channelId = $currentChannelId;
		if (channelId) {
			loadMessages(channelId);
		}
	});

	// Get messages for current channel.
	const messages = $derived.by(() => {
		const channelId = $currentChannelId;
		if (!channelId) return [];
		const store = getChannelMessages(channelId);
		let value: Message[] = [];
		store.subscribe((v) => (value = v))();
		return value;
	});

	// Auto-scroll to bottom on new messages.
	$effect(() => {
		if (messages.length > 0 && shouldAutoScroll) {
			tick().then(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			});
		}
	});

	function handleScroll() {
		if (!messagesContainer) return;
		const { scrollTop, scrollHeight, clientHeight } = messagesContainer;
		shouldAutoScroll = scrollHeight - scrollTop - clientHeight < 100;

		// Load more when scrolled to top.
		if (scrollTop < 50 && !$isLoadingMessages && messages.length > 0) {
			const channelId = $currentChannelId;
			if (channelId) {
				loadMessages(channelId, messages[0]?.id);
			}
		}
	}

	// Check if two consecutive messages should be grouped (same author within 5 min).
	function isCompact(msg: Message, idx: number): boolean {
		if (idx === 0) return false;
		const prev = messages[idx - 1];
		if (prev.author_id !== msg.author_id) return false;
		const diff = new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime();
		return diff < 5 * 60 * 1000;
	}
</script>

<div
	bind:this={messagesContainer}
	class="flex-1 overflow-y-auto"
	onscroll={handleScroll}
>
	{#if $isLoadingMessages && messages.length === 0}
		<div class="flex h-full items-center justify-center">
			<p class="text-text-muted">Loading messages...</p>
		</div>
	{:else if messages.length === 0}
		<div class="flex h-full items-center justify-center">
			<div class="text-center">
				<p class="text-lg text-text-secondary">No messages yet</p>
				<p class="text-sm text-text-muted">Be the first to say something!</p>
			</div>
		</div>
	{:else}
		<div class="py-4">
			{#each messages as msg, i (msg.id)}
				<MessageItem message={msg} isCompact={isCompact(msg, i)} />
			{/each}
		</div>
	{/if}
</div>
