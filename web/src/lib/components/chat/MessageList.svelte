<script lang="ts">
	import { tick } from 'svelte';
	import type { Message } from '$lib/types';
	import { currentChannelId } from '$lib/stores/channels';
	import { messagesByChannel, loadMessages, isLoadingMessages } from '$lib/stores/messages';
	import MessageItem from './MessageItem.svelte';

	let messagesContainer: HTMLDivElement;
	let shouldAutoScroll = $state(true);

	// Load messages when channel changes.
	$effect(() => {
		const channelId = $currentChannelId;
		if (channelId) {
			loadMessages(channelId);
		}
	});

	const messages = $derived.by(() => {
		const channelId = $currentChannelId;
		if (!channelId) return [] as Message[];
		return $messagesByChannel.get(channelId) ?? ([] as Message[]);
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

	// Group: same author within 5 min.
	function isCompact(msg: Message, idx: number): boolean {
		if (idx === 0) return false;
		const prev = messages[idx - 1];
		if (prev.author_id !== msg.author_id) return false;
		// Break group if this message is a reply.
		if (msg.reply_to_ids?.length) return false;
		const diff = new Date(msg.created_at).getTime() - new Date(prev.created_at).getTime();
		return diff < 5 * 60 * 1000;
	}

	// Check if a date separator should appear before this message.
	function showDateSeparator(msg: Message, idx: number): string | null {
		const msgDate = new Date(msg.created_at);
		if (idx === 0) {
			return formatDateSeparator(msgDate);
		}
		const prevDate = new Date(messages[idx - 1].created_at);
		if (msgDate.toDateString() !== prevDate.toDateString()) {
			return formatDateSeparator(msgDate);
		}
		return null;
	}

	function formatDateSeparator(date: Date): string {
		const today = new Date();
		const yesterday = new Date(today);
		yesterday.setDate(yesterday.getDate() - 1);

		if (date.toDateString() === today.toDateString()) return 'Today';
		if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
		return date.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
	}

	function scrollToMessage(messageId: string) {
		const el = document.getElementById(`msg-${messageId}`);
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			el.classList.add('bg-brand-500/10');
			setTimeout(() => el.classList.remove('bg-brand-500/10'), 2000);
		}
	}
</script>

<div
	bind:this={messagesContainer}
	class="flex-1 overflow-y-auto"
	onscroll={handleScroll}
>
	{#if $isLoadingMessages && messages.length === 0}
		<div class="flex h-full items-center justify-center">
			<div class="text-center">
				<div class="mx-auto mb-2 h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				<p class="text-text-muted">Loading messages...</p>
			</div>
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
				{@const dateSep = showDateSeparator(msg, i)}
				{#if dateSep}
					<div class="mx-4 my-4 flex items-center gap-4">
						<div class="h-px flex-1 bg-bg-modifier"></div>
						<span class="text-xs font-medium text-text-muted">{dateSep}</span>
						<div class="h-px flex-1 bg-bg-modifier"></div>
					</div>
				{/if}
				<MessageItem
					message={msg}
					isCompact={isCompact(msg, i)}
					onscrollto={scrollToMessage}
				/>
			{/each}
		</div>
	{/if}
</div>
