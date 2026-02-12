<script lang="ts">
	import { tick } from 'svelte';
	import type { Message, Channel } from '$lib/types';
	import { currentChannelId } from '$lib/stores/channels';
	import { messagesByChannel, loadMessages, isLoadingMessages } from '$lib/stores/messages';
	import { unreadCounts, getLastReadId } from '$lib/stores/unreads';
	import MessageItem from './MessageItem.svelte';

	interface Props {
		onopenthread?: (threadChannel: Channel, parentMessage: Message) => void;
	}

	let { onopenthread }: Props = $props();

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

	// Track the last-read message ID for the current channel.
	const lastReadId = $derived.by(() => {
		const channelId = $currentChannelId;
		if (!channelId) return null;
		const store = getLastReadId(channelId);
		let value: string | null = null;
		store.subscribe((v) => { value = v; })();
		return value;
	});

	// Find the first unread message (the one right after lastReadId).
	const firstUnreadId = $derived.by(() => {
		const channelId = $currentChannelId;
		if (!channelId) return null;
		const count = $unreadCounts.get(channelId) ?? 0;
		if (count === 0) return null;
		if (!lastReadId) {
			// No read state: treat all messages as unread, first message is the first unread.
			return messages.length > 0 ? messages[0].id : null;
		}
		const idx = messages.findIndex((m) => m.id === lastReadId);
		if (idx === -1) {
			// lastReadId is not in the loaded messages; the unread starts before what's loaded.
			return messages.length > 0 ? messages[0].id : null;
		}
		if (idx + 1 < messages.length) {
			return messages[idx + 1].id;
		}
		return null;
	});

	// Whether the first unread message is above the viewport (user scrolled past it).
	let showJumpToUnread = $state(false);

	function checkUnreadVisibility() {
		if (!firstUnreadId || !messagesContainer) {
			showJumpToUnread = false;
			return;
		}
		const el = document.getElementById(`msg-${firstUnreadId}`);
		if (!el) {
			// The unread message element doesn't exist in the DOM (not loaded or not rendered).
			// If we have an unread count, the message is likely above what's loaded.
			showJumpToUnread = ($unreadCounts.get($currentChannelId ?? '') ?? 0) > 0;
			return;
		}
		const containerRect = messagesContainer.getBoundingClientRect();
		const elRect = el.getBoundingClientRect();
		// Show the button if the first unread message is above the visible area.
		showJumpToUnread = elRect.bottom < containerRect.top;
	}

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

		checkUnreadVisibility();
	}

	// Re-check unread visibility when the first unread message changes.
	$effect(() => {
		// Subscribe to firstUnreadId and messages length to re-check.
		const _dep = firstUnreadId;
		const _len = messages.length;
		tick().then(() => checkUnreadVisibility());
	});

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

	// Count of messages that arrived while scrolled up.
	let newMessageCount = $state(0);

	// Track new messages arriving while not auto-scrolling.
	$effect(() => {
		const len = messages.length;
		if (len > 0 && !shouldAutoScroll) {
			// This fires when messages array changes length.
			newMessageCount++;
		}
		if (shouldAutoScroll) {
			newMessageCount = 0;
		}
	});

	function scrollToFirstUnread() {
		if (!firstUnreadId) return;
		const el = document.getElementById(`msg-${firstUnreadId}`);
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'start' });
			el.classList.add('bg-brand-500/10');
			setTimeout(() => el.classList.remove('bg-brand-500/10'), 2000);
			showJumpToUnread = false;
		}
	}

	function scrollToBottom() {
		if (messagesContainer) {
			messagesContainer.scrollTo({ top: messagesContainer.scrollHeight, behavior: 'smooth' });
			shouldAutoScroll = true;
			newMessageCount = 0;
		}
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

<div class="relative flex-1 overflow-hidden">
	<!-- Jump to Unread banner -->
	{#if showJumpToUnread}
		<button
			class="absolute left-0 right-0 top-0 z-10 flex items-center justify-center gap-2 border-b border-brand-500 bg-brand-500/90 px-4 py-1.5 text-sm font-medium text-white shadow-md transition-all hover:bg-brand-500"
			onclick={scrollToFirstUnread}
		>
			<svg class="h-4 w-4 rotate-180" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 14l-7 7m0 0l-7-7m7 7V3" />
			</svg>
			Jump to first unread message
		</button>
	{/if}

	<div
		bind:this={messagesContainer}
		class="h-full overflow-y-auto"
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
					{onopenthread}
				/>
			{/each}
		</div>
	{/if}
	</div>

	<!-- Scroll to bottom button -->
	{#if !shouldAutoScroll}
		<button
			class="absolute bottom-4 left-1/2 z-10 flex -translate-x-1/2 items-center gap-2 rounded-full bg-bg-floating px-4 py-2 text-sm font-medium text-text-primary shadow-lg transition-all hover:bg-bg-modifier"
			onclick={scrollToBottom}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 14l-7 7m0 0l-7-7m7 7V3" />
			</svg>
			{#if newMessageCount > 0}
				<span class="flex h-5 min-w-5 items-center justify-center rounded-full bg-brand-500 px-1.5 text-xs font-bold text-white">
					{newMessageCount}
				</span>
			{/if}
		</button>
	{/if}
</div>
