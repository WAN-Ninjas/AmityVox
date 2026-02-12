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
	let silentMode = $state(false);
	let showSchedulePicker = $state(false);
	let customDatetime = $state('');

	// When entering edit mode, populate the input with the message content.
	$effect(() => {
		if ($editingMessage) {
			content = $editingMessage.content ?? '';
			inputEl?.focus();
		}
	});

	const isEditing = $derived(!!$editingMessage);
	const isReplying = $derived(!!$replyingTo);
	const channelName = $derived($currentChannel?.name ?? 'channel');

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
		const opts: { reply_to_ids?: string[]; silent?: boolean } = {};
		if (isReplying && $replyingTo) {
			opts.reply_to_ids = [$replyingTo.id];
		}
		if (silentMode) {
			opts.silent = true;
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

	function getSchedulePresets(): { label: string; getTime: () => Date }[] {
		return [
			{
				label: 'In 15 minutes',
				getTime: () => new Date(Date.now() + 15 * 60 * 1000)
			},
			{
				label: 'In 30 minutes',
				getTime: () => new Date(Date.now() + 30 * 60 * 1000)
			},
			{
				label: 'In 1 hour',
				getTime: () => new Date(Date.now() + 60 * 60 * 1000)
			},
			{
				label: 'In 4 hours',
				getTime: () => new Date(Date.now() + 4 * 60 * 60 * 1000)
			},
			{
				label: 'Tomorrow 9:00 AM',
				getTime: () => {
					const d = new Date();
					d.setDate(d.getDate() + 1);
					d.setHours(9, 0, 0, 0);
					return d;
				}
			}
		];
	}

	async function handleSchedule(scheduledFor: Date) {
		const channelId = $currentChannelId;
		if (!channelId || !content.trim()) {
			addToast('Enter a message to schedule', 'error');
			return;
		}

		const msg = content.trim();
		content = '';
		if (inputEl) inputEl.style.height = 'auto';
		showSchedulePicker = false;
		customDatetime = '';

		try {
			await api.scheduleMessage(channelId, msg, scheduledFor.toISOString());
			addToast(`Message scheduled for ${scheduledFor.toLocaleString()}`, 'success');
		} catch (e) {
			content = msg;
			addToast('Failed to schedule message', 'error');
		}
	}

	function handleCustomSchedule() {
		if (!customDatetime) {
			addToast('Pick a date and time', 'error');
			return;
		}
		const date = new Date(customDatetime);
		if (date.getTime() <= Date.now() + 60 * 1000) {
			addToast('Scheduled time must be at least 1 minute in the future', 'error');
			return;
		}
		handleSchedule(date);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSubmit();
			return;
		}

		if (e.key === 'Escape') {
			if (showSchedulePicker) {
				showSchedulePicker = false;
				return;
			}
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

	async function handlePaste(e: ClipboardEvent) {
		const items = e.clipboardData?.items;
		if (!items || !$currentChannelId) return;

		const imageFiles: File[] = [];
		for (const item of items) {
			if (item.type.startsWith('image/')) {
				const file = item.getAsFile();
				if (file) imageFiles.push(file);
			}
		}

		if (imageFiles.length === 0) return;

		// Prevent default paste behavior for images.
		e.preventDefault();

		try {
			const ids: string[] = [];
			for (const file of imageFiles) {
				const uploaded = await api.uploadFile(file);
				ids.push(uploaded.id);
			}
			const msg = content.trim();
			const sent = await api.sendMessage($currentChannelId, msg, { attachment_ids: ids });
			appendMessage(sent);
			content = '';
			if (inputEl) inputEl.style.height = 'auto';
			addToast(`${ids.length} image${ids.length > 1 ? 's' : ''} uploaded`, 'success');
		} catch (err) {
			addToast('Failed to upload pasted image', 'error');
		}
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

		<!-- Silent mode indicator -->
		{#if silentMode && !isEditing}
			<div class="mb-2 flex items-center gap-2 rounded-t-lg bg-bg-secondary px-3 py-1.5 text-xs text-text-muted">
				<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
					<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
				</svg>
				<span>Silent mode -- recipients will not be notified</span>
				<button
					class="ml-auto text-text-muted hover:text-text-primary"
					onclick={() => (silentMode = false)}
					title="Disable silent mode"
				>
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
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
				onpaste={handlePaste}
				placeholder={isEditing ? 'Edit your message...' : isReplying ? 'Reply...' : silentMode ? `Message #${channelName} (silent)` : `Message #${channelName}`}
				class="max-h-[200px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
				rows="1"
			></textarea>

			<!-- Silent mode toggle button -->
			{#if !isEditing}
				<button
					class="self-center transition-colors {silentMode ? 'text-yellow-500 hover:text-yellow-400' : 'text-text-muted hover:text-text-primary'}"
					title={silentMode ? 'Silent mode on (click to disable)' : 'Send silently (no notifications)'}
					onclick={() => {
						silentMode = !silentMode;
						showSchedulePicker = false;
					}}
				>
					{#if silentMode}
						<!-- Bell with slash (silent active) -->
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
							<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
						</svg>
					{:else}
						<!-- Bell (silent inactive) -->
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						</svg>
					{/if}
				</button>
			{/if}

			<!-- Schedule message button -->
			{#if !isEditing}
				<div class="relative self-center">
					<button
						class="text-text-muted hover:text-text-primary"
						title="Schedule message"
						onclick={() => {
							showSchedulePicker = !showSchedulePicker;
							showEmojiPicker = false;
							showGiphyPicker = false;
						}}
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</button>
					{#if showSchedulePicker}
						<div class="absolute bottom-10 right-0 z-50 w-64 rounded-lg border border-bg-floating bg-bg-primary p-3 shadow-lg">
							<div class="mb-2 text-xs font-semibold uppercase text-text-muted">Schedule Message</div>
							<div class="flex flex-col gap-1">
								{#each getSchedulePresets() as preset}
									<button
										class="rounded px-3 py-1.5 text-left text-sm text-text-primary hover:bg-bg-modifier"
										onclick={() => handleSchedule(preset.getTime())}
									>
										{preset.label}
									</button>
								{/each}
							</div>
							<div class="my-2 border-t border-bg-floating"></div>
							<div class="text-xs font-semibold uppercase text-text-muted mb-1.5">Custom</div>
							<input
								type="datetime-local"
								bind:value={customDatetime}
								class="mb-2 w-full rounded border border-bg-floating bg-bg-secondary px-2 py-1 text-sm text-text-primary outline-none focus:border-text-link"
							/>
							<button
								class="w-full rounded bg-text-link px-3 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
								disabled={!customDatetime}
								onclick={handleCustomSchedule}
							>
								Schedule
							</button>
						</div>
					{/if}
				</div>
			{/if}

			<!-- GIF picker button -->
			{#if !isEditing}
				<div class="relative self-center">
					<button
						class="text-text-muted hover:text-text-primary"
						title="GIF"
						onclick={() => { showGiphyPicker = !showGiphyPicker; showEmojiPicker = false; showSchedulePicker = false; }}
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
						onclick={() => { showEmojiPicker = !showEmojiPicker; showGiphyPicker = false; showSchedulePicker = false; }}
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
