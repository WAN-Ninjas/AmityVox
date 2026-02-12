<script lang="ts">
	import type { Message } from '$lib/types';
	import Avatar from '$components/common/Avatar.svelte';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { startReply, startEdit } from '$lib/stores/messageInteraction';
	import { messagesByChannel } from '$lib/stores/messages';
	import { currentChannelId } from '$lib/stores/channels';

	interface Props {
		message: Message;
		isCompact?: boolean;
		onscrollto?: (messageId: string) => void;
	}

	let { message, isCompact = false, onscrollto }: Props = $props();

	let contextMenu = $state<{ x: number; y: number } | null>(null);
	let attachmentContextMenu = $state<{ x: number; y: number; attachment: any } | null>(null);
	let showQuickReactions = $state(false);

	const isOwnMessage = $derived($currentUser?.id === message.author_id);

	const displayName = $derived(
		message.masquerade_name ?? message.author?.display_name ?? message.author?.username ?? message.author_id
	);

	const timestamp = $derived(
		new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
	);

	const fullDateTime = $derived(
		new Date(message.created_at).toLocaleString()
	);

	const editedTime = $derived(
		message.edited_at ? new Date(message.edited_at).toLocaleString() : null
	);

	// Relative time for timestamps.
	const relativeTime = $derived.by(() => {
		const now = Date.now();
		const then = new Date(message.created_at).getTime();
		const diff = now - then;
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		if (days < 7) return `${days}d ago`;
		return new Date(message.created_at).toLocaleDateString();
	});

	// Find the replied-to message.
	const repliedMessage = $derived.by(() => {
		if (!message.reply_to_ids?.length) return null;
		const channelMsgs = $messagesByChannel.get(message.channel_id);
		if (!channelMsgs) return null;
		return channelMsgs.find(m => m.id === message.reply_to_ids[0]) ?? null;
	});

	const commonEmoji = ['👍', '❤️', '😂', '🎉', '😮', '😢'];

	// --- Actions ---

	function handleContextMenu(e: MouseEvent) {
		e.preventDefault();
		contextMenu = { x: e.clientX, y: e.clientY };
	}

	function handleAttachmentContextMenu(e: MouseEvent, attachment: any) {
		e.preventDefault();
		e.stopPropagation();
		attachmentContextMenu = { x: e.clientX, y: e.clientY, attachment };
	}

	function handleReply() {
		contextMenu = null;
		startReply(message);
	}

	function handleEdit() {
		contextMenu = null;
		startEdit(message);
	}

	async function handleDelete() {
		contextMenu = null;
		try {
			await api.deleteMessage(message.channel_id, message.id);
		} catch (err: any) {
			console.error('Failed to delete message:', err);
		}
	}

	async function handlePin() {
		contextMenu = null;
		try {
			if (message.pinned) {
				await api.unpinMessage(message.channel_id, message.id);
			} else {
				await api.pinMessage(message.channel_id, message.id);
			}
		} catch (err: any) {
			console.error('Failed to pin/unpin message:', err);
		}
	}

	function handleCopyText() {
		contextMenu = null;
		if (message.content) {
			navigator.clipboard.writeText(message.content);
		}
	}

	function handleCopyLink() {
		contextMenu = null;
		const url = `${window.location.origin}/app/guilds/${message.channel_id}#${message.id}`;
		navigator.clipboard.writeText(url);
	}

	async function toggleReaction(emoji: string) {
		showQuickReactions = false;
		try {
			const existing = message.reactions?.find(r => r.emoji === emoji);
			if (existing?.me) {
				await api.removeReaction(message.channel_id, message.id, emoji);
			} else {
				await api.addReaction(message.channel_id, message.id, emoji);
			}
		} catch (err: any) {
			console.error('Failed to toggle reaction:', err);
		}
	}

	// Attachment actions.
	function downloadAttachment(attachment: any) {
		attachmentContextMenu = null;
		const a = document.createElement('a');
		a.href = `/api/v1/files/${attachment.id}`;
		a.download = attachment.filename;
		a.click();
	}

	function openAttachmentInTab(attachment: any) {
		attachmentContextMenu = null;
		window.open(`/api/v1/files/${attachment.id}`, '_blank');
	}

	function copyAttachmentUrl(attachment: any) {
		attachmentContextMenu = null;
		navigator.clipboard.writeText(`${window.location.origin}/api/v1/files/${attachment.id}`);
	}
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="group relative flex gap-4 px-4 py-0.5 hover:bg-bg-modifier/30"
	class:mt-4={!isCompact}
	oncontextmenu={handleContextMenu}
	id="msg-{message.id}"
>
	<!-- Reply reference -->
	{#if repliedMessage && !isCompact}
		<div class="absolute -top-3 left-14 flex items-center gap-1 text-xs text-text-muted">
			<svg class="h-3 w-3 rotate-180" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M3 10h10a5 5 0 015 5v6M3 10l6 6m-6-6l6-6" />
			</svg>
			<button
				class="font-medium text-text-secondary hover:underline"
				onclick={() => onscrollto?.(repliedMessage.id)}
			>
				{repliedMessage.author?.display_name ?? repliedMessage.author?.username ?? 'Unknown'}
			</button>
			<span class="truncate max-w-xs">{repliedMessage.content?.slice(0, 80)}{(repliedMessage.content?.length ?? 0) > 80 ? '...' : ''}</span>
		</div>
	{/if}

	{#if isCompact}
		<div class="w-10 shrink-0 pt-1 text-right">
			<span class="hidden text-2xs text-text-muted group-hover:inline" title={fullDateTime}>{timestamp}</span>
		</div>
	{:else}
		<div class="mt-0.5 shrink-0">
			<Avatar name={displayName} size="md" />
		</div>
	{/if}

	<div class="min-w-0 flex-1" class:mt-3={repliedMessage && !isCompact}>
		{#if !isCompact}
			<div class="flex items-baseline gap-2">
				<span class="font-medium text-text-primary hover:underline">{displayName}</span>
				<time class="text-xs text-text-muted" title={fullDateTime}>{relativeTime}</time>
				{#if message.pinned}
					<span class="text-2xs text-yellow-500" title="Pinned">📌</span>
				{/if}
				{#if message.edited_at}
					<span class="text-2xs text-text-muted" title="Edited: {editedTime}">(edited)</span>
				{/if}
			</div>
		{/if}

		{#if message.content}
			<p class="text-sm text-text-secondary leading-relaxed break-words whitespace-pre-wrap">{message.content}</p>
		{/if}

		<!-- Attachments -->
		{#if message.attachments?.length > 0}
			<div class="mt-1 flex flex-wrap gap-2">
				{#each message.attachments as attachment (attachment.id)}
					{#if attachment.content_type?.startsWith('image/')}
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<img
							src="/api/v1/files/{attachment.id}"
							alt={attachment.filename}
							class="max-h-80 max-w-md rounded cursor-pointer"
							loading="lazy"
							oncontextmenu={(e) => handleAttachmentContextMenu(e, attachment)}
						/>
					{:else}
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<a
							href="/api/v1/files/{attachment.id}"
							class="flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-sm text-text-link hover:underline"
							download={attachment.filename}
							oncontextmenu={(e) => handleAttachmentContextMenu(e, attachment)}
						>
							<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
								<path d="M14 2H6c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8l-6-6zm4 18H6V4h7v5h5v11z" />
							</svg>
							{attachment.filename}
							<span class="text-xs text-text-muted">
								({(attachment.size_bytes / 1024).toFixed(0)} KB)
							</span>
						</a>
					{/if}
				{/each}
			</div>
		{/if}

		<!-- Embeds -->
		{#if message.embeds?.length > 0}
			{#each message.embeds as embed}
				<div class="mt-1 max-w-md overflow-hidden rounded border-l-4 border-brand-500 bg-bg-secondary p-3">
					{#if embed.provider_name}
						<p class="text-xs text-text-muted">{embed.provider_name}</p>
					{/if}
					{#if embed.title}
						<p class="font-semibold text-text-link">
							{#if embed.url}
								<a href={embed.url} target="_blank" rel="noopener" class="hover:underline">{embed.title}</a>
							{:else}
								{embed.title}
							{/if}
						</p>
					{/if}
					{#if embed.description}
						<p class="mt-1 text-sm text-text-secondary">{embed.description}</p>
					{/if}
					{#if embed.thumbnail_url}
						<img src={embed.thumbnail_url} alt="" class="mt-2 max-h-60 rounded" loading="lazy" />
					{/if}
				</div>
			{/each}
		{/if}

		<!-- Reactions -->
		{#if message.reactions?.length > 0}
			<div class="mt-1 flex flex-wrap gap-1">
				{#each message.reactions as reaction (reaction.emoji)}
					<button
						class="flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors {reaction.me ? 'border-brand-500 bg-brand-500/10' : 'border-bg-modifier hover:border-brand-500'}"
						onclick={() => toggleReaction(reaction.emoji)}
						title="{reaction.count} reaction{reaction.count !== 1 ? 's' : ''}"
					>
						<span>{reaction.emoji}</span>
						<span class="text-text-muted">{reaction.count}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Hover action bar -->
	{#if !contextMenu}
		<div class="absolute -top-3 right-4 hidden rounded border border-bg-modifier bg-bg-secondary shadow group-hover:flex">
			<button
				class="p-1.5 text-text-muted hover:text-text-primary"
				title="Add Reaction"
				onclick={() => (showQuickReactions = !showQuickReactions)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
			</button>
			<button
				class="p-1.5 text-text-muted hover:text-text-primary"
				title="Reply"
				onclick={handleReply}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M3 10h10a5 5 0 015 5v6M3 10l6 6m-6-6l6-6" />
				</svg>
			</button>
			{#if isOwnMessage}
				<button
					class="p-1.5 text-text-muted hover:text-text-primary"
					title="Edit"
					onclick={handleEdit}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
					</svg>
				</button>
			{/if}
		</div>
	{/if}

	<!-- Quick reaction picker -->
	{#if showQuickReactions}
		<div class="absolute right-4 top-6 z-10 flex gap-1 rounded bg-bg-floating p-2 shadow-lg">
			{#each commonEmoji as emoji}
				<button class="rounded p-1 text-lg hover:bg-bg-modifier" onclick={() => toggleReaction(emoji)}>
					{emoji}
				</button>
			{/each}
		</div>
	{/if}
</div>

<!-- Message context menu -->
{#if contextMenu}
	<ContextMenu x={contextMenu.x} y={contextMenu.y} onclose={() => contextMenu = null}>
		<ContextMenuItem label="Reply" onclick={handleReply} />
		{#if message.content}
			<ContextMenuItem label="Copy Text" onclick={handleCopyText} />
		{/if}
		{#if isOwnMessage}
			<ContextMenuItem label="Edit Message" onclick={handleEdit} />
		{/if}
		<ContextMenuItem label={message.pinned ? 'Unpin Message' : 'Pin Message'} onclick={handlePin} />
		<ContextMenuItem label="Copy Message Link" onclick={handleCopyLink} />
		{#if isOwnMessage}
			<ContextMenuDivider />
			<ContextMenuItem label="Delete Message" danger onclick={handleDelete} />
		{/if}
	</ContextMenu>
{/if}

<!-- Attachment context menu -->
{#if attachmentContextMenu}
	<ContextMenu x={attachmentContextMenu.x} y={attachmentContextMenu.y} onclose={() => attachmentContextMenu = null}>
		<ContextMenuItem label="Download" onclick={() => downloadAttachment(attachmentContextMenu?.attachment)} />
		<ContextMenuItem label="Open in New Tab" onclick={() => openAttachmentInTab(attachmentContextMenu?.attachment)} />
		<ContextMenuItem label="Copy URL" onclick={() => copyAttachmentUrl(attachmentContextMenu?.attachment)} />
	</ContextMenu>
{/if}
