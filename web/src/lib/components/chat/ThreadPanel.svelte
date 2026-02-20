<script lang="ts">
	import type { Message, ForumTag } from '$lib/types';
	import { api } from '$lib/api/client';
	import { messagesByChannel, loadMessages, appendMessage } from '$lib/stores/messages';
	import { channels } from '$lib/stores/channels';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import Avatar from '$components/common/Avatar.svelte';
	import { tick } from 'svelte';

	interface Props {
		threadChannel: { id: string; name: string | null; guild_id: string | null; parent_channel_id?: string | null; locked?: boolean; pinned?: boolean; encrypted?: boolean };
		parentMessage?: Message | null;
		onclose: () => void;
	}

	let { threadChannel, parentMessage = null, onclose }: Props = $props();

	let content = $state('');
	let messagesContainer: HTMLDivElement;
	let loading = $state(true);
	let forumTags = $state<ForumTag[]>([]);
	let channelPassphrase = $state('');
	let hasKey = $state(false);
	let checkingKey = $state(true);

	// Encrypted threads use the parent channel's key (same passphrase + parent ID as salt)
	const encryptionChannelId = $derived(threadChannel.parent_channel_id ?? threadChannel.id);

	// Check if we already have the decryption key for this encrypted thread
	$effect(() => {
		if (threadChannel.encrypted) {
			checkingKey = true;
			e2ee.hasChannelKey(encryptionChannelId).then((has) => {
				hasKey = has;
				checkingKey = false;
			});
		} else {
			hasKey = true;
			checkingKey = false;
		}
	});

	async function unlockThread() {
		if (!channelPassphrase.trim()) return;
		try {
			await e2ee.setPassphrase(encryptionChannelId, channelPassphrase);
			hasKey = true;
			channelPassphrase = '';
		} catch {
			// Invalid passphrase
		}
	}

	// Detect if this thread belongs to a forum channel
	let isForumPost = $derived.by(() => {
		if (!threadChannel.parent_channel_id) return false;
		let parentCh: any;
		channels.subscribe((m) => (parentCh = m.get(threadChannel.parent_channel_id!)))();
		return parentCh?.channel_type === 'forum';
	});

	// Load forum tags if this is a forum post
	$effect(() => {
		if (isForumPost && threadChannel.parent_channel_id) {
			api.getForumTags(threadChannel.parent_channel_id).then((tags) => {
				// We get all tags for the forum; we'd ideally filter to just this post's tags
				// but we show all available tags as context
				forumTags = tags;
			}).catch(() => {
				forumTags = [];
			});
		} else {
			forumTags = [];
		}
	});

	const rawThreadMessages = $derived.by(() => {
		return $messagesByChannel.get(threadChannel.id) ?? [];
	});

	// Decrypt encrypted messages
	let decryptedContents = $state<Map<string, string>>(new Map());

	$effect(() => {
		if (!threadChannel.encrypted || !hasKey) return;
		const msgs = rawThreadMessages;
		for (const msg of msgs) {
			if (msg.encrypted && !decryptedContents.has(msg.id)) {
				e2ee.decryptMessage(encryptionChannelId, msg.content).then((plain) => {
					decryptedContents.set(msg.id, plain);
					decryptedContents = new Map(decryptedContents);
				}).catch(() => {
					decryptedContents.set(msg.id, '[Decryption failed]');
					decryptedContents = new Map(decryptedContents);
				});
			}
		}
	});

	function getDisplayContent(msg: Message): string {
		if (msg.encrypted && decryptedContents.has(msg.id)) {
			return decryptedContents.get(msg.id)!;
		}
		if (msg.encrypted) return '[Encrypted message]';
		return msg.content;
	}

	const threadMessages = $derived(rawThreadMessages);

	$effect(() => {
		loading = true;
		loadMessages(threadChannel.id).finally(() => {
			loading = false;
			tick().then(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			});
		});
	});

	// Auto-scroll on new messages.
	$effect(() => {
		if (threadMessages.length > 0) {
			tick().then(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			});
		}
	});

	async function sendThreadMessage() {
		if (!content.trim()) return;
		const msg = content.trim();
		content = '';

		try {
			let sendContent = msg;
			const opts: { encrypted?: boolean } = {};
			if (threadChannel.encrypted) {
				sendContent = await e2ee.encryptMessage(encryptionChannelId, msg);
				opts.encrypted = true;
			}
			const sent = await api.sendMessage(threadChannel.id, sendContent, opts);
			appendMessage(sent);
		} catch (err) {
			content = msg;
			console.error('Failed to send thread message:', err);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendThreadMessage();
		}
		if (e.key === 'Escape') onclose();
	}

	function formatTime(ts: string): string {
		return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
</script>

<div class="flex h-full w-80 shrink-0 flex-col border-l border-bg-floating bg-bg-secondary">
	<!-- Header -->
	<div class="flex h-12 items-center justify-between border-b border-bg-floating px-4">
		<div class="min-w-0">
			<h3 class="truncate text-sm font-semibold text-text-primary">{isForumPost ? 'Post' : 'Thread'}</h3>
			<p class="truncate text-2xs text-text-muted">{threadChannel.name ?? (isForumPost ? 'Forum Post' : 'Thread')}</p>
		</div>
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={onclose} title="Close Thread">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<!-- Forum post indicators -->
	{#if isForumPost}
		<div class="flex flex-wrap items-center gap-1.5 border-b border-bg-floating px-3 py-2">
			{#if threadChannel.pinned}
				<span class="inline-flex items-center gap-1 rounded-full bg-brand-500/15 px-2 py-0.5 text-[10px] font-medium text-brand-400">
					<svg class="h-3 w-3" fill="currentColor" viewBox="0 0 24 24">
						<path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
					</svg>
					Pinned
				</span>
			{/if}
			{#if threadChannel.locked}
				<span class="inline-flex items-center gap-1 rounded-full bg-red-500/15 px-2 py-0.5 text-[10px] font-medium text-red-400">
					<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
						<path d="M7 11V7a5 5 0 0110 0v4" />
					</svg>
					Closed
				</span>
			{/if}
			{#each forumTags as tag (tag.id)}
				<span
					class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium"
					style="background-color: {tag.color ? tag.color + '20' : 'var(--bg-modifier)'}; color: {tag.color || 'var(--text-secondary)'}"
				>
					{#if tag.emoji}<span>{tag.emoji}</span>{/if}
					{tag.name}
				</span>
			{/each}
		</div>
	{/if}

	<!-- Parent message -->
	{#if parentMessage}
		<div class="border-b border-bg-floating bg-bg-primary/50 p-3">
			<div class="flex items-center gap-2 text-xs text-text-muted">
				<span class="font-medium text-text-primary">
					{parentMessage.author?.display_name ?? parentMessage.author?.username ?? 'Unknown'}
				</span>
				<time>{formatTime(parentMessage.created_at)}</time>
			</div>
			<p class="mt-1 text-sm text-text-secondary line-clamp-3">{parentMessage.content}</p>
		</div>
	{/if}

	<!-- Passphrase prompt for encrypted threads -->
	{#if threadChannel.encrypted && !hasKey && !checkingKey}
		<div class="flex flex-1 flex-col items-center justify-center gap-3 p-4">
			<svg class="h-8 w-8 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
			</svg>
			<p class="text-center text-sm text-text-muted">This thread is encrypted. Enter the channel passphrase to view and send messages.</p>
			<form onsubmit={(e) => { e.preventDefault(); unlockThread(); }} class="flex w-full gap-2">
				<input
					type="password"
					bind:value={channelPassphrase}
					placeholder="Passphrase"
					class="flex-1 rounded-md bg-bg-modifier px-3 py-1.5 text-sm text-text-primary outline-none placeholder:text-text-muted"
				/>
				<button type="submit" class="rounded-md bg-brand-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-600">Unlock</button>
			</form>
		</div>
	{:else}
	<!-- Thread messages -->
	<div bind:this={messagesContainer} class="flex-1 overflow-y-auto p-2">
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if threadMessages.length === 0}
			<p class="py-8 text-center text-sm text-text-muted">No replies yet</p>
		{:else}
			{#each threadMessages as msg (msg.id)}
				<div class="flex gap-2 rounded p-2 hover:bg-bg-modifier/30">
					<div class="mt-0.5 shrink-0">
						<Avatar
							name={msg.author?.display_name ?? msg.author?.username ?? 'U'}
							src={msg.author?.avatar_id ? `/api/v1/files/${msg.author.avatar_id}` : null}
							size="sm"
						/>
					</div>
					<div class="min-w-0 flex-1">
						<div class="flex items-baseline gap-2">
							<span class="text-sm font-medium text-text-primary">
								{msg.author?.display_name ?? msg.author?.username ?? msg.author_id.slice(0, 8)}
							</span>
							<time class="text-2xs text-text-muted">{formatTime(msg.created_at)}</time>
						</div>
						<p class="text-sm text-text-secondary break-words whitespace-pre-wrap">{getDisplayContent(msg)}</p>
					</div>
				</div>
			{/each}
		{/if}
	</div>

	<!-- Thread input -->
	<div class="border-t border-bg-floating p-3">
		<div class="flex items-end gap-2 rounded-lg bg-bg-modifier px-3 py-2">
			<textarea
				bind:value={content}
				onkeydown={handleKeydown}
				placeholder="Reply to thread..."
				class="max-h-24 min-h-[20px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
				rows="1"
			></textarea>
		</div>
	</div>
	{/if}
</div>
