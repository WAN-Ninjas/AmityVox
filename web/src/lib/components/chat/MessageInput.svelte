<script lang="ts">
	import { currentChannelId, currentChannel } from '$lib/stores/channels';
	import { api } from '$lib/api/client';
	import { getGatewayClient } from '$lib/stores/gateway';
	import { appendMessage } from '$lib/stores/messages';
	import { replyingTo, editingMessage, cancelReply, cancelEdit } from '$lib/stores/messageInteraction';
	import { messagesByChannel } from '$lib/stores/messages';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import EmojiPicker from '$components/common/EmojiPicker.svelte';
	import GiphyPicker from '$components/common/GiphyPicker.svelte';

	let content = $state('');
	let inputEl: HTMLTextAreaElement;
	let typingTimeout: ReturnType<typeof setTimeout> | null = null;
	let showEmojiPicker = $state(false);
	let showGiphyPicker = $state(false);

	// When entering edit mode, populate the input with the message content.
	$effect(() => {
		if ($editingMessage) {
			content = $editingMessage.content ?? '';
			inputEl?.focus();
		}
	});

	const isEditing = $derived(!!$editingMessage);
	const isReplying = $derived(!!$replyingTo);

	async function handleSubmit() {
		const channelId = $currentChannelId;
		if (!channelId || !content.trim()) return;

		const msg = content.trim();
		content = '';
		if (inputEl) inputEl.style.height = 'auto';

		if (isEditing && $editingMessage) {
			// Edit mode: update existing message.
			try {
				await api.editMessage($editingMessage.channel_id, $editingMessage.id, msg);
				cancelEdit();
			} catch (e) {
				content = msg;
				addToast('Failed to edit message', 'error');
			}
			return;
		}

		// Normal send (possibly with reply).
		const opts: { reply_to_ids?: string[] } = {};
		if (isReplying && $replyingTo) {
			opts.reply_to_ids = [$replyingTo.id];
		}

		cancelReply();

		try {
			const sent = await api.sendMessage(channelId, msg, opts);
			appendMessage(sent);
		} catch (e) {
			content = msg;
			addToast('Failed to send message', 'error');
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSubmit();
			return;
		}

		if (e.key === 'Escape') {
			if (showEmojiPicker) {
				showEmojiPicker = false;
				return;
			}
			if (isEditing) {
				cancelEdit();
				content = '';
				return;
			}
			if (isReplying) {
				cancelReply();
				return;
			}
		}

		// Up arrow on empty input: edit last own message.
		if (e.key === 'ArrowUp' && !content.trim() && !isEditing && !isReplying) {
			const channelId = $currentChannelId;
			const userId = $currentUser?.id;
			if (channelId && userId) {
				const msgs = $messagesByChannel.get(channelId) ?? [];
				for (let i = msgs.length - 1; i >= 0; i--) {
					if (msgs[i].author_id === userId && msgs[i].content) {
						e.preventDefault();
						editingMessage.set(msgs[i]);
						return;
					}
				}
			}
		}

		// Send typing indicator (throttled).
		const channelId = $currentChannelId;
		if (channelId && !typingTimeout && !isEditing) {
			getGatewayClient()?.sendTyping(channelId);
			typingTimeout = setTimeout(() => {
				typingTimeout = null;
			}, 5000);
		}
	}

	function handleInput() {
		if (inputEl) {
			inputEl.style.height = 'auto';
			inputEl.style.height = Math.min(inputEl.scrollHeight, 200) + 'px';
		}
	}

	async function handleFileUpload(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file || !$currentChannelId) return;

		try {
			const uploaded = await api.uploadFile(file);
			const sent = await api.sendMessage($currentChannelId, '', { attachment_ids: [uploaded.id] });
			appendMessage(sent);
		} catch (err) {
			addToast('Upload failed', 'error');
		}
		target.value = '';
	}

	function insertEmoji(emoji: string) {
		content += emoji;
		showEmojiPicker = false;
		inputEl?.focus();
	}

	async function insertGif(gifUrl: string) {
		showGiphyPicker = false;
		const channelId = $currentChannelId;
		if (!channelId) return;
		try {
			const sent = await api.sendMessage(channelId, gifUrl);
			appendMessage(sent);
		} catch (e) {
			addToast('Failed to send GIF', 'error');
		}
	}
</script>

{#if $currentChannelId}
	<div class="border-t border-bg-floating px-4 pb-4 pt-2">
		<!-- Reply bar -->
		{#if $replyingTo}
			<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-bg-secondary px-3 py-2 text-sm">
				<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M3 10h10a5 5 0 015 5v6M3 10l6 6m-6-6l6-6" />
				</svg>
				<span class="text-text-muted">Replying to</span>
				<span class="font-medium text-text-primary">
					{$replyingTo.author?.display_name ?? $replyingTo.author?.username ?? 'Unknown'}
				</span>
				<span class="flex-1 truncate text-text-muted">{$replyingTo.content?.slice(0, 60)}</span>
				<button
					class="shrink-0 text-text-muted hover:text-text-primary"
					onclick={cancelReply}
					title="Cancel reply"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
		{/if}

		<!-- Edit bar -->
		{#if $editingMessage}
			<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-yellow-500/10 px-3 py-2 text-sm">
				<svg class="h-4 w-4 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
				</svg>
				<span class="text-yellow-500">Editing message</span>
				<span class="flex-1"></span>
				<button
					class="shrink-0 text-text-muted hover:text-text-primary"
					onclick={() => { cancelEdit(); content = ''; }}
					title="Cancel edit"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
		{/if}

		<div
			class="flex items-end gap-2 rounded-lg px-4 py-2 {isEditing ? 'bg-yellow-900/20 ring-1 ring-yellow-500/30' : 'bg-bg-modifier'}"
		>
			<!-- File upload -->
			{#if !isEditing}
				<label class="cursor-pointer self-center text-text-muted hover:text-text-primary">
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48" />
					</svg>
					<input type="file" class="hidden" onchange={handleFileUpload} />
				</label>
			{/if}

			<!-- Text input -->
			<textarea
				bind:this={inputEl}
				bind:value={content}
				onkeydown={handleKeydown}
				oninput={handleInput}
				placeholder={isEditing ? 'Edit your message...' : isReplying ? 'Reply...' : `Message #{$currentChannel?.name ?? 'channel'}`}
				class="max-h-[200px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
				rows="1"
			></textarea>

			<!-- GIF picker button -->
			{#if !isEditing}
				<div class="relative self-center">
					<button
						class="text-text-muted hover:text-text-primary"
						title="GIF"
						onclick={() => { showGiphyPicker = !showGiphyPicker; showEmojiPicker = false; }}
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
						</svg>
					</button>
					{#if showGiphyPicker}
						<GiphyPicker onselect={insertGif} onclose={() => (showGiphyPicker = false)} />
					{/if}
				</div>
			{/if}

			<!-- Emoji picker button -->
			{#if !isEditing}
				<div class="relative self-center">
					<button
						class="text-text-muted hover:text-text-primary"
						title="Emoji"
						onclick={() => { showEmojiPicker = !showEmojiPicker; showGiphyPicker = false; }}
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</button>
					{#if showEmojiPicker}
						<EmojiPicker onselect={insertEmoji} onclose={() => (showEmojiPicker = false)} />
					{/if}
				</div>
			{/if}

			<!-- Submit hint -->
			{#if isEditing}
				<span class="self-center text-2xs text-text-muted">Esc cancel · Enter save</span>
			{/if}
		</div>
	</div>
{/if}
