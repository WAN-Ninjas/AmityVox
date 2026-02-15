<script lang="ts">
	import type { Message, Channel } from '$lib/types';
	import Avatar from '$components/common/Avatar.svelte';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import UserPopover from '$components/common/UserPopover.svelte';
	import ImageLightbox from '$components/common/ImageLightbox.svelte';
	import EditHistoryModal from '$components/chat/EditHistoryModal.svelte';
	import MarkdownRenderer from '$components/chat/MarkdownRenderer.svelte';
	import AudioPlayer from '$components/chat/AudioPlayer.svelte';
	import VideoPlayer from '$components/chat/VideoPlayer.svelte';
	import TranslateButton from '$components/chat/TranslateButton.svelte';
	import CrossChannelQuote from '$components/chat/CrossChannelQuote.svelte';
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { presenceMap } from '$lib/stores/presence';
	import { memberTimeouts } from '$lib/stores/members';
	import { startReply, startEdit } from '$lib/stores/messageInteraction';
	import { messagesByChannel } from '$lib/stores/messages';
	import { currentChannelId, currentChannel } from '$lib/stores/channels';
	import { addToast } from '$lib/stores/toast';
	import { blockedUserIds } from '$lib/stores/blocked';

	interface Props {
		message: Message;
		isCompact?: boolean;
		onscrollto?: (messageId: string) => void;
		onopenthread?: (threadChannel: Channel, parentMessage: Message) => void;
	}

	let { message, isCompact = false, onscrollto, onopenthread }: Props = $props();

	let contextMenu = $state<{ x: number; y: number } | null>(null);
	let attachmentContextMenu = $state<{ x: number; y: number; attachment: any } | null>(null);
	let showQuickReactions = $state(false);
	let userPopover = $state<{ x: number; y: number } | null>(null);
	let lightboxSrc = $state<string | null>(null);
	let showEditHistory = $state(false);
	let showForward = $state(false);
	let forwardTargetId = $state('');
	let forwarding = $state(false);
	let showQuoteInChannel = $state(false);
	let quoteTargetChannelId = $state('');
	let quotingInChannel = $state(false);
	let showCreateThread = $state(false);
	let newThreadName = $state('');
	let creatingThread = $state(false);

	// --- Blocked user support ---
	const isAuthorBlocked = $derived($blockedUserIds.has(message.author_id));
	let showBlockedContent = $state(false);

	// --- NSFW content filter ---
	const isNsfwChannel = $derived($currentChannel?.nsfw ?? false);
	function getStoredNsfwFilter(): 'blur_all' | 'blur_suspicious' | 'show_all' {
		try {
			const stored = localStorage.getItem('av-nsfw-filter');
			if (stored === 'blur_all' || stored === 'blur_suspicious' || stored === 'show_all') return stored;
		} catch {}
		return 'blur_all';
	}
	let nsfwFilterSetting = $state(getStoredNsfwFilter());
	let revealedImages = $state<Set<string>>(new Set());

	function shouldBlurImage(attachmentId: string): boolean {
		if (!isNsfwChannel) return false;
		if (nsfwFilterSetting !== 'blur_all') return false;
		return !revealedImages.has(attachmentId);
	}

	function revealImage(attachmentId: string) {
		revealedImages = new Set([...revealedImages, attachmentId]);
	}

	const authorPresence = $derived($presenceMap.get(message.author_id) ?? undefined);

	const isOwnMessage = $derived($currentUser?.id === message.author_id);

	// Detect messages that are just a single image/GIF URL (e.g. from Giphy picker).
	const IMAGE_URL_RE = /^https?:\/\/\S+\.(?:gif|png|jpe?g|webp)(?:\?[^\s]*)?$/i;
	const GIPHY_RE = /^https?:\/\/(?:media\d*\.giphy\.com|i\.giphy\.com)\//i;
	const TENOR_RE = /^https?:\/\/(?:media\.tenor\.com|c\.tenor\.com)\//i;
	const imageOnlyUrl = $derived.by(() => {
		const text = message.content?.trim();
		if (!text || message.attachments?.length) return null;
		if (IMAGE_URL_RE.test(text) || GIPHY_RE.test(text) || TENOR_RE.test(text)) return text;
		return null;
	});

	// Detect sticker-like messages: no text content with a single image attachment.
	const isStickerMessage = $derived(
		!message.content?.trim() &&
		message.attachments?.length === 1 &&
		message.attachments[0].content_type?.startsWith('image/') &&
		message.message_type === 'default'
	);

	const isAuthorTimedOut = $derived.by(() => {
		const until = $memberTimeouts.get(message.author_id);
		if (!until) return false;
		return new Date(until).getTime() > Date.now();
	});

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
			addToast('Message deleted', 'success');
		} catch (err: any) {
			addToast('Failed to delete message', 'error');
		}
	}

	async function handlePin() {
		contextMenu = null;
		try {
			if (message.pinned) {
				await api.unpinMessage(message.channel_id, message.id);
				addToast('Message unpinned', 'success');
			} else {
				await api.pinMessage(message.channel_id, message.id);
				addToast('Message pinned', 'success');
			}
		} catch (err: any) {
			addToast('Failed to pin/unpin message', 'error');
		}
	}

	function handleCopyText() {
		contextMenu = null;
		if (message.content) {
			navigator.clipboard.writeText(message.content);
			addToast('Text copied', 'info');
		}
	}

	function handleCopyLink() {
		contextMenu = null;
		const url = `${window.location.origin}/app/guilds/${message.channel_id}#${message.id}`;
		navigator.clipboard.writeText(url);
		addToast('Link copied', 'info');
	}

	function handleCreateThread() {
		contextMenu = null;
		newThreadName = message.content?.slice(0, 50)?.trim() || 'New Thread';
		showCreateThread = true;
	}

	async function submitCreateThread() {
		const name = newThreadName.trim();
		if (!name) return;
		creatingThread = true;
		try {
			const threadChannel = await api.createThread(message.channel_id, message.id, name);
			showCreateThread = false;
			addToast('Thread created', 'success');
			onopenthread?.(threadChannel, message);
		} catch (err: any) {
			addToast('Failed to create thread', 'error');
		} finally {
			creatingThread = false;
		}
	}

	function handleViewThread() {
		contextMenu = null;
		if (message.thread_id) {
			api.getChannel(message.thread_id).then((ch) => {
				onopenthread?.(ch, message);
			}).catch(() => {
				addToast('Failed to open thread', 'error');
			});
		}
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
			addToast('Failed to toggle reaction', 'error');
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
		addToast('URL copied', 'info');
	}

	async function handleBookmark() {
		contextMenu = null;
		try {
			await api.createBookmark(message.id);
			addToast('Message bookmarked', 'success');
		} catch (err: any) {
			addToast('Failed to bookmark message', 'error');
		}
	}

	function handleForward() {
		contextMenu = null;
		showForward = true;
	}

	function handleQuoteInChannel() {
		contextMenu = null;
		showQuoteInChannel = true;
	}

	let showReportModal = $state(false);
	let reportReason = $state('');
	let reportSubmitting = $state(false);

	function handleReportMessage() {
		contextMenu = null;
		showReportModal = true;
		reportReason = '';
	}

	async function submitReportMessage() {
		if (!reportReason.trim()) return;
		reportSubmitting = true;
		try {
			await api.reportMessageToAdmins(message.channel_id, message.id, reportReason.trim());
			addToast('Message reported to moderators', 'success');
			showReportModal = false;
		} catch {
			addToast('Failed to report message', 'error');
		} finally {
			reportSubmitting = false;
		}
	}

	async function submitQuoteInChannel() {
		if (!quoteTargetChannelId.trim()) return;
		quotingInChannel = true;
		try {
			await api.createMessage(quoteTargetChannelId.trim(), {
				content: `> **Quoted from <#${message.channel_id}>:**\n> ${message.content?.slice(0, 500) ?? ''}\n\n`,
				quote_message_id: message.id,
				quote_channel_id: message.channel_id
			});
			addToast('Quote sent', 'success');
			showQuoteInChannel = false;
			quoteTargetChannelId = '';
		} catch (err: any) {
			addToast('Failed to send quote', 'error');
		} finally {
			quotingInChannel = false;
		}
	}

	async function submitForward() {
		if (!forwardTargetId.trim()) return;
		forwarding = true;
		try {
			await api.forwardMessage(message.channel_id, message.id, forwardTargetId.trim());
			addToast('Message forwarded', 'success');
			showForward = false;
			forwardTargetId = '';
		} catch (err: any) {
			addToast('Failed to forward message', 'error');
		} finally {
			forwarding = false;
		}
	}
</script>

<!-- System lockdown message: prominent alert display -->
{#if message.message_type === 'system_lockdown'}
<div
	class="mx-4 my-2 flex items-center gap-3 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-4 py-3"
	id="msg-{message.id}"
>
	<div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-yellow-500/20">
		<svg class="h-5 w-5 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
		</svg>
	</div>
	<div class="flex-1">
		<p class="text-sm font-semibold text-yellow-400">Raid Protection Lockdown</p>
		<p class="text-xs text-yellow-300/80">{message.content}</p>
	</div>
	<time class="text-xs text-yellow-500/60" title={new Date(message.created_at).toLocaleString()}>
		{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
	</time>
</div>
{:else}
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
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="mt-0.5 shrink-0 cursor-pointer" onclick={(e) => { userPopover = { x: e.clientX, y: e.clientY }; }}>
			<Avatar name={displayName} status={authorPresence} />
		</div>
	{/if}

	<div class="min-w-0 flex-1" class:mt-3={repliedMessage && !isCompact}>
		{#if isAuthorBlocked && !showBlockedContent}
			<!-- Blocked user placeholder -->
			<div class="flex items-center gap-3 rounded bg-bg-modifier/50 px-3 py-2">
				<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<circle cx="12" cy="12" r="10" />
					<path d="M4.93 4.93l14.14 14.14" />
				</svg>
				<span class="text-sm text-text-muted">Blocked message</span>
				<button
					class="ml-auto text-xs text-text-muted hover:text-text-secondary"
					onclick={() => (showBlockedContent = true)}
				>
					Show message
				</button>
			</div>
		{:else}
			{#if !isCompact}
				<div class="flex items-baseline gap-2">
					<button class="font-medium text-text-primary hover:underline" onclick={(e) => { userPopover = { x: e.clientX, y: e.clientY }; }}>{displayName}</button>
					{#if isAuthorBlocked}
						<span class="text-2xs text-red-400">(blocked)</span>
					{/if}
					{#if isAuthorTimedOut}
						<span class="inline-flex items-center" title="This user is timed out">
							<svg class="h-3.5 w-3.5 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<circle cx="12" cy="12" r="10" />
								<path d="M12 6v6l4 2" />
							</svg>
						</span>
					{/if}
					<time class="text-xs text-text-muted" title={fullDateTime}>{relativeTime}</time>
					{#if message.pinned}
						<span class="text-2xs text-yellow-500" title="Pinned">📌</span>
					{/if}
					{#if message.edited_at}
						<button
							class="text-2xs text-text-muted hover:underline"
							title="Edited: {editedTime}"
							onclick={() => (showEditHistory = true)}
						>(edited)</button>
					{/if}
					{#if message.flags & 1}
						<span class="flex items-center gap-0.5 text-2xs text-green-400" title="Published to followers">
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M11 5.882V19.24a1.76 1.76 0 01-3.417.592l-2.147-6.15M18 13a3 3 0 100-6M5.436 13.683A4.001 4.001 0 017 6h1.832c4.1 0 7.625-1.234 9.168-3v14c-1.543-1.766-5.067-3-9.168-3H7a3.988 3.988 0 01-1.564-.317z" />
							</svg>
							Published
						</span>
					{/if}
					{#if message.thread_id}
						<button
							class="flex items-center gap-1 text-2xs text-brand-400 hover:underline"
							onclick={handleViewThread}
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
							</svg>
							Thread
						</button>
					{/if}
					{#if isAuthorBlocked}
						<button
							class="text-2xs text-text-muted hover:text-text-secondary"
							onclick={() => (showBlockedContent = false)}
						>
							Hide
						</button>
					{/if}
				</div>
			{/if}

			{#if imageOnlyUrl}
				<!-- Message is a single image/GIF URL — render inline -->
				<button class="mt-1 block" onclick={() => (lightboxSrc = imageOnlyUrl)}>
					<img
						src={imageOnlyUrl}
						alt="Linked image"
						class="max-h-72 max-w-full rounded"
						loading="lazy"
						onerror={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
					/>
				</button>
			{:else if message.content}
				<div class="text-sm text-text-secondary leading-relaxed break-words whitespace-pre-wrap">
					<MarkdownRenderer content={message.content} />
				</div>
				<TranslateButton channelId={message.channel_id} messageId={message.id} />
			{/if}

			<!-- Cross-channel quote embed -->
			{#if message.embeds?.some(e => e.type === 'cross_channel_quote')}
				{@const quoteEmbed = message.embeds.find(e => e.type === 'cross_channel_quote')}
				{#if quoteEmbed?.quote_message_id && quoteEmbed?.quote_channel_id}
					<CrossChannelQuote
						quoteMessageId={quoteEmbed.quote_message_id}
						quoteChannelId={quoteEmbed.quote_channel_id}
					/>
				{/if}
			{/if}

			<!-- Attachments -->
			{#if message.attachments?.length > 0}
				<div class="mt-1 flex flex-wrap gap-2">
					{#each message.attachments as attachment (attachment.id)}
						{#if attachment.content_type?.startsWith('image/')}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							{#if shouldBlurImage(attachment.id)}
								<div
									class="relative max-h-80 max-w-md cursor-pointer overflow-hidden rounded"
									onclick={() => revealImage(attachment.id)}
								>
									<img
										src="/api/v1/files/{attachment.id}"
										alt={attachment.alt_text || attachment.filename}
										class="max-h-80 max-w-md rounded transition-[filter]"
										style="filter: blur(20px);"
										loading="lazy"
									/>
									<div class="absolute inset-0 flex items-center justify-center bg-black/30">
										<span class="rounded bg-bg-floating/80 px-3 py-1.5 text-xs font-medium text-text-primary">
											Click to reveal NSFW image
										</span>
									</div>
								</div>
							{:else if isStickerMessage}
								<img
									src="/api/v1/files/{attachment.id}"
									alt={attachment.alt_text || attachment.filename}
									class="h-40 w-40 object-contain cursor-pointer hover:scale-105 transition-transform"
									loading="lazy"
									onclick={() => (lightboxSrc = `/api/v1/files/${attachment.id}`)}
									oncontextmenu={(e) => handleAttachmentContextMenu(e, attachment)}
								/>
							{:else}
								<div class="inline-flex flex-col">
									<img
										src="/api/v1/files/{attachment.id}"
										alt={attachment.alt_text || attachment.filename}
										class="max-h-80 max-w-md rounded cursor-pointer hover:brightness-90 transition-[filter]"
										loading="lazy"
										onclick={() => (lightboxSrc = `/api/v1/files/${attachment.id}`)}
										oncontextmenu={(e) => handleAttachmentContextMenu(e, attachment)}
									/>
									{#if attachment.alt_text}
										<span class="mt-0.5 max-w-md text-2xs text-text-muted">{attachment.alt_text}</span>
									{/if}
								</div>
							{/if}
						{:else if attachment.content_type?.startsWith('audio/')}
							<AudioPlayer
								src="/api/v1/files/{attachment.id}"
								waveform={message.voice_waveform}
								durationMs={message.voice_duration_ms}
							/>
						{:else if attachment.content_type?.startsWith('video/')}
							<VideoPlayer
								src="/api/v1/files/{attachment.id}"
								width={attachment.width ?? undefined}
								height={attachment.height ?? undefined}
								filename={attachment.filename}
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
			<button
				class="p-1.5 text-text-muted hover:text-text-primary"
				title="Create Thread"
				onclick={handleCreateThread}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
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
{/if}

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
		{#if !message.thread_id}
			<ContextMenuItem label="Create Thread" onclick={handleCreateThread} />
		{:else}
			<ContextMenuItem label="View Thread" onclick={handleViewThread} />
		{/if}
		<ContextMenuItem label="Copy Message Link" onclick={handleCopyLink} />
		<ContextMenuItem label="Bookmark" onclick={handleBookmark} />
		<ContextMenuItem label="Forward" onclick={handleForward} />
		{#if message.content}
			<ContextMenuItem label="Quote in Channel" onclick={handleQuoteInChannel} />
		{/if}
		{#if !isOwnMessage}
			<ContextMenuDivider />
			<ContextMenuItem label="Report Message" danger onclick={handleReportMessage} />
		{/if}
		{#if isOwnMessage}
			<ContextMenuDivider />
			<ContextMenuItem label="Delete Message" danger onclick={handleDelete} />
		{/if}
	</ContextMenu>
{/if}

<!-- Report message modal -->
{#if showReportModal}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onclick={() => showReportModal = false} onkeydown={(e) => e.key === 'Escape' && (showReportModal = false)} role="dialog" tabindex="-1">
		<div class="w-96 rounded-lg bg-bg-secondary p-4 shadow-xl" onclick={(e) => e.stopPropagation()} onkeydown={() => {}} role="document" tabindex="-1">
			<h3 class="mb-3 text-lg font-semibold text-text-primary">Report Message</h3>
			<p class="mb-2 text-sm text-text-muted">This will be sent to instance moderators for review.</p>
			<textarea
				class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
				placeholder="Why are you reporting this message?"
				rows="3"
				bind:value={reportReason}
			></textarea>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm text-text-muted hover:text-text-primary"
					onclick={() => showReportModal = false}
				>Cancel</button>
				<button
					class="rounded-md bg-red-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-600 disabled:opacity-50"
					disabled={!reportReason.trim() || reportSubmitting}
					onclick={submitReportMessage}
				>{reportSubmitting ? 'Submitting...' : 'Report'}</button>
			</div>
		</div>
	</div>
{/if}

<!-- Attachment context menu -->
{#if attachmentContextMenu}
	<ContextMenu x={attachmentContextMenu.x} y={attachmentContextMenu.y} onclose={() => attachmentContextMenu = null}>
		<ContextMenuItem label="Download" onclick={() => downloadAttachment(attachmentContextMenu?.attachment)} />
		<ContextMenuItem label="Open in New Tab" onclick={() => openAttachmentInTab(attachmentContextMenu?.attachment)} />
		<ContextMenuItem label="Copy URL" onclick={() => copyAttachmentUrl(attachmentContextMenu?.attachment)} />
	</ContextMenu>
{/if}

<!-- User popover -->
{#if userPopover}
	<UserPopover
		userId={message.author_id}
		x={userPopover.x}
		y={userPopover.y}
		onclose={() => (userPopover = null)}
	/>
{/if}

<!-- Image lightbox -->
{#if lightboxSrc}
	<ImageLightbox src={lightboxSrc} onclose={() => (lightboxSrc = null)} />
{/if}

<!-- Edit history modal -->
<EditHistoryModal
	open={showEditHistory}
	channelId={message.channel_id}
	messageId={message.id}
	onclose={() => (showEditHistory = false)}
/>

<!-- Forward message modal -->
{#if showForward}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onclick={() => (showForward = false)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="w-96 rounded-lg bg-bg-floating p-6 shadow-xl" onclick={(e) => e.stopPropagation()}>
			<h3 class="mb-4 text-lg font-semibold text-text-primary">Forward Message</h3>
			<p class="mb-3 text-sm text-text-muted">
				Enter the channel ID to forward this message to:
			</p>
			<div class="mb-3 rounded bg-bg-secondary p-2 text-sm text-text-muted">
				{message.content?.slice(0, 100)}{(message.content?.length ?? 0) > 100 ? '...' : ''}
			</div>
			<input
				type="text"
				class="input mb-4 w-full"
				bind:value={forwardTargetId}
				placeholder="Target channel ID"
				onkeydown={(e) => e.key === 'Enter' && submitForward()}
			/>
			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={() => (showForward = false)}>Cancel</button>
				<button class="btn-primary" onclick={submitForward} disabled={forwarding || !forwardTargetId.trim()}>
					{forwarding ? 'Forwarding...' : 'Forward'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Quote in channel modal -->
{#if showQuoteInChannel}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onclick={() => (showQuoteInChannel = false)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="w-96 rounded-lg bg-bg-floating p-6 shadow-xl" onclick={(e) => e.stopPropagation()}>
			<h3 class="mb-4 text-lg font-semibold text-text-primary">Quote in Another Channel</h3>
			<p class="mb-3 text-sm text-text-muted">
				Enter the channel ID to send this quote to:
			</p>
			<div class="mb-3 rounded border-l-4 border-purple-500 bg-bg-secondary p-2 text-sm text-text-muted">
				<div class="mb-1 text-xs font-medium text-text-primary">{displayName}</div>
				{message.content?.slice(0, 150)}{(message.content?.length ?? 0) > 150 ? '...' : ''}
			</div>
			<input
				type="text"
				class="input mb-4 w-full"
				bind:value={quoteTargetChannelId}
				placeholder="Target channel ID"
				onkeydown={(e) => e.key === 'Enter' && submitQuoteInChannel()}
			/>
			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={() => (showQuoteInChannel = false)}>Cancel</button>
				<button class="btn-primary" onclick={submitQuoteInChannel} disabled={quotingInChannel || !quoteTargetChannelId.trim()}>
					{quotingInChannel ? 'Quoting...' : 'Send Quote'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Create Thread Modal -->
<Modal open={showCreateThread} title="Create Thread" onclose={() => (showCreateThread = false)}>
	<div class="mb-4">
		<label for="threadName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Thread Name
		</label>
		<input
			id="threadName"
			type="text"
			class="input w-full"
			bind:value={newThreadName}
			placeholder="Give this thread a name"
			maxlength="100"
			onkeydown={(e) => e.key === 'Enter' && submitCreateThread()}
		/>
	</div>
	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showCreateThread = false)}>Cancel</button>
		<button class="btn-primary" onclick={submitCreateThread} disabled={creatingThread || !newThreadName.trim()}>
			{creatingThread ? 'Creating...' : 'Create Thread'}
		</button>
	</div>
</Modal>
