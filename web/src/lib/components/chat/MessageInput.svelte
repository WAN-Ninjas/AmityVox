<script lang="ts">
	import { currentChannelId, currentChannel } from '$lib/stores/channels';
	import { api } from '$lib/api/client';
	import { getGatewayClient } from '$lib/stores/gateway';
	import { appendMessage } from '$lib/stores/messages';
	import { replyingTo, editingMessage, cancelReply, cancelEdit } from '$lib/stores/messageInteraction';
	import { messagesByChannel } from '$lib/stores/messages';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { getDMDisplayName } from '$lib/utils/dm';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import EmojiPicker from '$components/common/EmojiPicker.svelte';
	import GiphyPicker from '$components/common/GiphyPicker.svelte';
	import StickerPicker from '$components/common/StickerPicker.svelte';
	import VoiceMessageRecorder from '$components/chat/VoiceMessageRecorder.svelte';
	import type { Sticker } from '$lib/types';

	let content = $state('');
	let inputEl: HTMLTextAreaElement;
	let typingTimeout: ReturnType<typeof setTimeout> | null = null;
	let showEmojiPicker = $state(false);
	let showGiphyPicker = $state(false);
	let showStickerPicker = $state(false);
	let silentMode = $state(false);
	let showSchedulePicker = $state(false);
	let customDatetime = $state('');
	let showVoiceRecorder = $state(false);

	// --- E2EE passphrase prompt ---
	let needsPassphrase = $state(false);
	let channelPassphrase = $state('');
	let settingPassphrase = $state(false);

	$effect(() => {
		const ch = $currentChannel;
		if (ch?.encrypted) {
			const channelId = ch.id;
			e2ee.hasChannelKey(channelId).then((has) => {
				if ($currentChannel?.id === channelId) {
					needsPassphrase = !has;
				}
			});
		} else {
			needsPassphrase = false;
		}
	});

	async function handleSetPassphrase() {
		const channelId = $currentChannelId;
		if (!channelId || !channelPassphrase.trim()) return;
		settingPassphrase = true;
		try {
			await e2ee.setPassphrase(channelId, channelPassphrase);
			needsPassphrase = false;
			channelPassphrase = '';
			addToast('Channel unlocked', 'success');
		} catch {
			addToast('Failed to set passphrase', 'error');
		} finally {
			settingPassphrase = false;
		}
	}

	// --- File attachment state ---
	let pendingFiles = $state<File[]>([]);
	let pendingAltTexts = $state<Record<number, string>>({});
	let uploading = $state(false);

	// Default max upload size in bytes (25 MB). This could be overridden by instance config.
	const MAX_FILE_SIZE_BYTES = 25 * 1024 * 1024;

	/**
	 * Format a byte count into a human-readable string (KB, MB, GB).
	 */
	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
		return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
	}

	/**
	 * Check whether a file exceeds the upload size limit.
	 */
	function isFileOverLimit(file: File): boolean {
		return file.size > MAX_FILE_SIZE_BYTES;
	}

	const hasOversizedFiles = $derived(pendingFiles.some(isFileOverLimit));

	function removePendingFile(index: number) {
		pendingFiles = pendingFiles.filter((_, i) => i !== index);
		// Re-index alt texts after removal.
		const newAlts: Record<number, string> = {};
		let j = 0;
		for (let i = 0; i < pendingFiles.length + 1; i++) {
			if (i === index) continue;
			if (pendingAltTexts[i]) newAlts[j] = pendingAltTexts[i];
			j++;
		}
		pendingAltTexts = newAlts;
	}

	function clearPendingFiles() {
		pendingFiles = [];
		pendingAltTexts = {};
	}

	// When entering edit mode, populate the input with the message content.
	$effect(() => {
		if ($editingMessage) {
			content = $editingMessage.content ?? '';
			inputEl?.focus();
		}
	});

	const isEditing = $derived(!!$editingMessage);
	const isReplying = $derived(!!$replyingTo);
	const isDM = $derived($currentChannel?.channel_type === 'dm' || $currentChannel?.channel_type === 'group');
	const channelName = $derived(
		isDM && $currentChannel
			? getDMDisplayName($currentChannel, $currentUser?.id)
			: $currentChannel?.name ?? 'channel'
	);

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
		const opts: { reply_to_ids?: string[]; silent?: boolean; encrypted?: boolean } = {};
		if (isReplying && $replyingTo) {
			opts.reply_to_ids = [$replyingTo.id];
		}
		if (silentMode) {
			opts.silent = true;
		}

		cancelReply();

		try {
			let sendContent = msg;

			// Encrypt the message if the channel is encrypted
			if ($currentChannel?.encrypted) {
				try {
					sendContent = await e2ee.encryptMessage(channelId, msg);
					opts.encrypted = true;
				} catch (encErr) {
					content = msg;
					addToast('Failed to encrypt message. Do you have the channel key?', 'error');
					return;
				}
			}

			const sent = await api.sendMessage(channelId, sendContent, opts);
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
			if (pendingFiles.length > 0) {
				uploadPendingFiles();
			} else {
				handleSubmit();
			}
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
			if (showStickerPicker) {
				showStickerPicker = false;
				return;
			}
			if (showGiphyPicker) {
				showGiphyPicker = false;
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

	function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const files = target.files;
		if (!files || files.length === 0) return;

		for (const file of files) {
			pendingFiles = [...pendingFiles, file];
		}
		target.value = '';
	}

	async function uploadPendingFiles() {
		const channelId = $currentChannelId;
		if (!channelId || pendingFiles.length === 0) return;

		// Check for oversized files.
		const oversized = pendingFiles.filter(isFileOverLimit);
		if (oversized.length > 0) {
			addToast(`${oversized.length} file(s) exceed the ${formatFileSize(MAX_FILE_SIZE_BYTES)} limit`, 'error');
			return;
		}

		uploading = true;
		try {
			const ids: string[] = [];
			for (let i = 0; i < pendingFiles.length; i++) {
				const file = pendingFiles[i];
				const altText = pendingAltTexts[i]?.trim() || undefined;
				const uploaded = await api.uploadFile(file, altText);
				ids.push(uploaded.id);
			}
			const msg = content.trim();
			const sent = await api.sendMessage(channelId, msg, { attachment_ids: ids });
			appendMessage(sent);
			content = '';
			if (inputEl) inputEl.style.height = 'auto';
			pendingFiles = [];
			pendingAltTexts = {};
		} catch (err) {
			addToast('Upload failed', 'error');
		} finally {
			uploading = false;
		}
	}

	async function handleFileUpload(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file || !$currentChannelId) return;

		if (isFileOverLimit(file)) {
			addToast(`File exceeds the ${formatFileSize(MAX_FILE_SIZE_BYTES)} limit`, 'error');
			target.value = '';
			return;
		}

		try {
			const uploaded = await api.uploadFile(file);
			const sent = await api.sendMessage($currentChannelId, '', { attachment_ids: [uploaded.id] });
			appendMessage(sent);
		} catch (err) {
			addToast('Upload failed', 'error');
		}
		target.value = '';
	}

	function handlePaste(e: ClipboardEvent) {
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

		// Add pasted images to pending files instead of auto-sending.
		pendingFiles = [...pendingFiles, ...imageFiles];
		addToast(`${imageFiles.length} image${imageFiles.length > 1 ? 's' : ''} added — press Send to upload`, 'info');
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

	async function sendSticker(sticker: Sticker) {
		showStickerPicker = false;
		const channelId = $currentChannelId;
		if (!channelId) return;
		try {
			// Send the sticker as a message with the sticker image file as an attachment.
			const sent = await api.sendMessage(channelId, '', { attachment_ids: [sticker.file_id] });
			appendMessage(sent);
		} catch (e) {
			addToast('Failed to send sticker', 'error');
		}
	}

	async function sendVoiceMessage(audioBlob: Blob, waveform: number[], durationMs: number) {
		const channelId = $currentChannelId;
		if (!channelId) return;

		showVoiceRecorder = false;

		try {
			const ext = audioBlob.type.includes('webm') ? 'webm' : audioBlob.type.includes('ogg') ? 'ogg' : 'mp4';
			const file = new File([audioBlob], `voice-message.${ext}`, { type: audioBlob.type });
			const uploaded = await api.uploadFile(file);
			const sent = await api.sendMessage(channelId, '', {
				attachment_ids: [uploaded.id],
				voice_duration_ms: durationMs,
				voice_waveform: waveform
			});
			appendMessage(sent);
		} catch (err) {
			addToast('Failed to send voice message', 'error');
		}
	}
</script>

{#if $currentChannelId}
	<div class="border-t border-bg-floating px-4 pb-4 pt-2">
		<!-- Passphrase prompt for encrypted channels without a key -->
		{#if needsPassphrase}
			<div class="mb-2 flex items-center gap-2 rounded-lg bg-yellow-500/10 border border-yellow-500/30 px-3 py-2">
				<svg class="h-4 w-4 shrink-0 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
				</svg>
				<span class="text-xs text-yellow-400">Enter passphrase to send messages</span>
				<input
					type="password"
					class="ml-auto min-w-0 flex-1 max-w-48 rounded border border-bg-modifier bg-bg-primary px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
					placeholder="Channel passphrase"
					bind:value={channelPassphrase}
					onkeydown={(e) => e.key === 'Enter' && handleSetPassphrase()}
				/>
				<button
					class="shrink-0 rounded bg-brand-500 px-2.5 py-1 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50"
					onclick={handleSetPassphrase}
					disabled={settingPassphrase || !channelPassphrase.trim()}
				>
					{settingPassphrase ? '...' : 'Unlock'}
				</button>
			</div>
		{/if}

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

		<!-- Pending files preview -->
		{#if pendingFiles.length > 0}
			<div class="mb-2 rounded-lg bg-bg-secondary p-3">
				<div class="mb-2 flex items-center justify-between">
					<span class="text-xs font-semibold text-text-muted">
						{pendingFiles.length} file{pendingFiles.length > 1 ? 's' : ''} attached
						<span class="ml-1 font-normal text-text-muted">
							(max {formatFileSize(MAX_FILE_SIZE_BYTES)})
						</span>
					</span>
					<div class="flex items-center gap-2">
						<button
							class="text-xs text-text-muted hover:text-text-primary"
							onclick={clearPendingFiles}
						>
							Clear all
						</button>
						<button
							class="btn-primary text-xs px-3 py-1"
							onclick={uploadPendingFiles}
							disabled={uploading || hasOversizedFiles}
						>
							{uploading ? 'Uploading...' : 'Send'}
						</button>
					</div>
				</div>
				<div class="space-y-1.5">
					{#each pendingFiles as file, i (file.name + i)}
						{@const overLimit = isFileOverLimit(file)}
						{@const isImage = file.type.startsWith('image/')}
						<div class="rounded {overLimit ? 'bg-red-500/10' : 'bg-bg-primary'}">
							<div class="flex items-center gap-2 px-2 py-1.5">
								<svg class="h-4 w-4 shrink-0 {overLimit ? 'text-red-400' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
									<polyline points="14 2 14 8 20 8" />
								</svg>
								<span class="flex-1 truncate text-xs {overLimit ? 'text-red-400' : 'text-text-primary'}">
									{file.name}
								</span>
								<span class="shrink-0 text-2xs {overLimit ? 'font-semibold text-red-400' : 'text-text-muted'}">
									{formatFileSize(file.size)}
									{#if overLimit}
										-- exceeds limit
									{/if}
								</span>
								<button
									class="shrink-0 text-text-muted hover:text-text-primary"
									onclick={() => removePendingFile(i)}
									title="Remove file"
								>
									<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M6 18L18 6M6 6l12 12" />
									</svg>
								</button>
							</div>
							{#if isImage}
								<div class="px-2 pb-1.5">
									<input
										type="text"
										class="w-full rounded border border-bg-floating bg-bg-secondary px-2 py-1 text-2xs text-text-primary outline-none placeholder:text-text-muted focus:border-text-link"
										placeholder="Alt text (describe this image for accessibility)"
										value={pendingAltTexts[i] ?? ''}
										oninput={(e) => { pendingAltTexts = { ...pendingAltTexts, [i]: (e.target as HTMLInputElement).value }; }}
									/>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Voice recorder (replaces the input bar when active) -->
		{#if showVoiceRecorder}
			<VoiceMessageRecorder
				onsend={sendVoiceMessage}
				oncancel={() => (showVoiceRecorder = false)}
			/>
		{:else}
			<div
				class="flex items-end gap-2 rounded border border-bg-modifier px-4 py-2 {isEditing ? 'bg-yellow-900/20 ring-1 ring-yellow-500/30' : 'bg-bg-modifier'}"
			>
				<!-- File upload -->
				{#if !isEditing}
					<label class="flex cursor-pointer items-center justify-center text-text-muted hover:text-text-primary">
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48" />
						</svg>
						<input type="file" class="hidden" onchange={handleFileSelect} multiple />
					</label>
				{/if}

				<!-- Text input -->
				<textarea
					bind:this={inputEl}
					bind:value={content}
					onkeydown={handleKeydown}
					oninput={handleInput}
					onpaste={handlePaste}
					placeholder={isEditing ? 'Edit your message...' : isReplying ? 'Reply...' : silentMode ? `Message ${isDM ? '@' : '#'}${channelName} (silent)` : `Message ${isDM ? '@' : '#'}${channelName}`}
					class="max-h-[200px] min-h-[24px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted placeholder:font-mono"
					rows="1"
				></textarea>

				<!-- Right-side icon toolbar -->
				{#if !isEditing}
				<div class="flex items-center gap-2">
					<button
						class="flex items-center justify-center transition-colors {silentMode ? 'text-yellow-500 hover:text-yellow-400' : 'text-text-muted hover:text-text-primary'}"
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

				<!-- Schedule message button -->
					<div class="relative">
						<button
							class="flex items-center justify-center text-text-muted hover:text-text-primary"
							title="Schedule message"
							onclick={() => {
								showSchedulePicker = !showSchedulePicker;
								showEmojiPicker = false;
								showGiphyPicker = false;
								showStickerPicker = false;
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

				<!-- GIF picker button -->
					<div class="giphy-picker relative">
						<button
							class="flex h-5 items-center rounded border border-text-muted px-1 text-[10px] font-bold leading-none text-text-muted hover:border-text-primary hover:text-text-primary"
							title="GIF"
							onclick={(e) => { e.stopPropagation(); showGiphyPicker = !showGiphyPicker; showEmojiPicker = false; showStickerPicker = false; showSchedulePicker = false; }}
						>
							GIF
						</button>
						{#if showGiphyPicker}
							<GiphyPicker onselect={insertGif} onclose={() => (showGiphyPicker = false)} />
						{/if}
					</div>

				<!-- Sticker picker button -->
					<div class="sticker-picker relative">
						<button
							class="flex items-center justify-center text-text-muted hover:text-text-primary"
							title="Stickers"
							onclick={(e) => { e.stopPropagation(); showStickerPicker = !showStickerPicker; showEmojiPicker = false; showGiphyPicker = false; showSchedulePicker = false; }}
						>
							<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
								<path d="M15 2v5a2 2 0 002 2h5" />
							</svg>
						</button>
						{#if showStickerPicker}
							<StickerPicker onselect={sendSticker} onclose={() => (showStickerPicker = false)} />
						{/if}
					</div>

				<!-- Emoji picker button -->
					<div class="emoji-picker relative">
						<button
							class="flex items-center justify-center text-text-muted hover:text-text-primary"
							title="Emoji"
							onclick={(e) => { e.stopPropagation(); showEmojiPicker = !showEmojiPicker; showGiphyPicker = false; showStickerPicker = false; showSchedulePicker = false; }}
						>
							<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<circle cx="12" cy="12" r="9" />
								<path d="M8.5 14.5c1 1.5 5.5 1.5 7 0" stroke-linecap="round" />
								<circle cx="9" cy="10" r="1" fill="currentColor" stroke="none" />
								<circle cx="15" cy="10" r="1" fill="currentColor" stroke="none" />
							</svg>
						</button>
						{#if showEmojiPicker}
							<EmojiPicker onselect={insertEmoji} onclose={() => (showEmojiPicker = false)} />
						{/if}
					</div>

				<!-- Voice message button -->
					<button
						class="flex items-center justify-center text-text-muted hover:text-text-primary"
						title="Record voice message"
						onclick={() => {
							showVoiceRecorder = true;
							showEmojiPicker = false;
							showGiphyPicker = false;
							showStickerPicker = false;
							showSchedulePicker = false;
						}}
					>
						<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
							<path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
						</svg>
					</button>
				</div>
				{/if}

				<!-- Submit hint -->
				{#if isEditing}
					<span class="text-2xs text-text-muted">Esc cancel · Enter save</span>
				{/if}
			</div>
		{/if}
	</div>
{/if}
