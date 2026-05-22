<script lang="ts">
	import { onMount } from 'svelte';
	import { currentChannelId, currentChannel } from '$lib/stores/channels';
	import { api, ApiRequestError } from '$lib/api/client';
	import { getGatewayClient } from '$lib/stores/gateway';
	import { appendMessage } from '$lib/stores/messages';
	import { replyingTo, editingMessage, cancelReply, cancelEdit } from '$lib/stores/messageInteraction';
	import { messagesByChannel } from '$lib/stores/messages';
	import { currentUser } from '$lib/stores/auth';
	import { canManageChannels, canManageMessages, isAdministrator } from '$lib/stores/permissions';
	import { addToast } from '$lib/stores/toast';
	import { getDMDisplayName } from '$lib/utils/dm';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import EmojiPicker from '$components/common/EmojiPicker.svelte';
	import GiphyPicker from '$components/common/GiphyPicker.svelte';
	import StickerPicker from '$components/common/StickerPicker.svelte';
	import VoiceMessageRecorder from '$components/chat/VoiceMessageRecorder.svelte';
	import MentionAutocomplete from '$components/chat/MentionAutocomplete.svelte';
	import ChannelPassphrasePrompt from '$components/chat/ChannelPassphrasePrompt.svelte';
	import MessageInputStatusBars from '$components/chat/MessageInputStatusBars.svelte';
	import PendingFilesPreview from '$components/chat/PendingFilesPreview.svelte';
	import ScheduleMessagePicker from '$components/chat/ScheduleMessagePicker.svelte';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { Sticker } from '$lib/types';

	let content = $state('');
	let inputEl = $state<HTMLTextAreaElement>();
	let typingTimeout: ReturnType<typeof setTimeout> | null = null;
	let showEmojiPicker = $state(false);
	let showGiphyPicker = $state(false);
	let showStickerPicker = $state(false);
	let silentMode = $state(false);
	let showSchedulePicker = $state(false);
	let customDatetime = $state('');
	let showVoiceRecorder = $state(false);
	let showInputMore = $state(false);
	let slowmodeNow = $state(Date.now());
	let localLastSentAtByChannel = $state<Record<string, number>>({});

	// --- Mention autocomplete ---
	let showMentionAutocomplete = $state(false);
	let mentionQuery = $state('');
	let mentionStartIndex = $state(0);
	let mentionAutocomplete = $state<{ handleKeydown: (e: KeyboardEvent) => boolean }>();
	/** Maps display text (e.g. "@Horatio") → wire syntax (e.g. "<@01KH...>") for mentions inserted via autocomplete. */
	let mentionMap = new Map<string, string>();

	// --- E2EE passphrase prompt ---
	let needsPassphrase = $state(false);
	let channelPassphrase = $state('');
	let passphraseOp = $state(createAsyncOp());

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
		await passphraseOp.run(
			async () => {
			await e2ee.setPassphrase(channelId, channelPassphrase);
			needsPassphrase = false;
			channelPassphrase = '';
			addToast('Channel unlocked', 'success');
			},
			() => addToast('Failed to set passphrase', 'error')
		);
	}

	// --- File attachment state ---
	let pendingFiles = $state<File[]>([]);
	let pendingAltTexts = $state<Record<number, string>>({});
	let uploadOp = $state(createAsyncOp());

	const FALLBACK_MAX_FILE_SIZE_BYTES = 25 * 1024 * 1024;
	let maxFileSizeBytes = $state(FALLBACK_MAX_FILE_SIZE_BYTES);
	let fileUploadsEnabled = $state(true);

	onMount(() => {
		api.getClientConfig()
			.then((config) => {
				fileUploadsEnabled = config.file_uploads_enabled;
				if (config.max_upload_bytes > 0) {
					maxFileSizeBytes = config.max_upload_bytes;
				}
			})
			.catch(() => {
				fileUploadsEnabled = true;
				maxFileSizeBytes = FALLBACK_MAX_FILE_SIZE_BYTES;
			});
	});

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
		return file.size > maxFileSizeBytes;
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

	export function addPendingFiles(files: File[]) {
		if (files.length === 0) return;
		if (!fileUploadsEnabled) {
			addToast('File uploads are unavailable on this instance', 'error');
			return;
		}
		pendingFiles = [...pendingFiles, ...files];
		addToast(`${files.length} file${files.length > 1 ? 's' : ''} attached — press Send to upload`, 'info');
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
	const slowmodeRemainingSeconds = $derived.by(() => {
		if (isEditing || $canManageMessages || $canManageChannels || $isAdministrator) return 0;
		const channelId = $currentChannelId;
		const slowmodeSeconds = $currentChannel?.slowmode_seconds ?? 0;
		if (!channelId || slowmodeSeconds <= 0) return 0;

		const ownMessageSentAt = latestOwnMessageSentAt(channelId);
		const localSentAt = localLastSentAtByChannel[channelId] ?? 0;
		const lastSentAt = Math.max(ownMessageSentAt, localSentAt);
		if (lastSentAt === 0) return 0;

		const nextAllowedAt = lastSentAt + slowmodeSeconds * 1000;
		return Math.max(0, Math.ceil((nextAllowedAt - slowmodeNow) / 1000));
	});
	const slowmodeBlocked = $derived(slowmodeRemainingSeconds > 0);

	$effect(() => {
		if (($currentChannel?.slowmode_seconds ?? 0) <= 0) return;
		slowmodeNow = Date.now();
		const interval = window.setInterval(() => {
			slowmodeNow = Date.now();
		}, 1000);
		return () => window.clearInterval(interval);
	});

	function latestOwnMessageSentAt(channelId: string): number {
		const userId = $currentUser?.id;
		if (!userId) return 0;
		const messages = $messagesByChannel.get(channelId) ?? [];
		let latest = 0;
		for (const message of messages) {
			if (message.author_id !== userId) continue;
			const sentAt = Date.parse(message.created_at);
			if (!Number.isNaN(sentAt)) {
				latest = Math.max(latest, sentAt);
			}
		}
		return latest;
	}

	function recordSuccessfulSend(channelId: string) {
		if (($currentChannel?.slowmode_seconds ?? 0) <= 0) return;
		localLastSentAtByChannel = { ...localLastSentAtByChannel, [channelId]: Date.now() };
	}

	function guardSlowmode(): boolean {
		if (!slowmodeBlocked) return false;
		addToast(`Slowmode active. Try again in ${slowmodeRemainingSeconds}s`, 'error');
		return true;
	}

	function handleSendError(err: unknown, fallback: string) {
		if (err instanceof ApiRequestError && err.code === 'slowmode') {
			addToast(getErrorMessage(err, fallback), 'error');
			return;
		}
		addToast(fallback, 'error');
	}

	async function handleSubmit() {
		const channelId = $currentChannelId;
		if (!channelId || !content.trim()) return;

		const msg = resolveMentions(content.trim());
		content = '';
		mentionMap.clear();
		if (inputEl) inputEl.style.height = 'auto';

		if (isEditing && $editingMessage) {
			// Edit mode: update existing message.
			try {
				let editContent = msg;
				if ($currentChannel?.encrypted) {
					try {
						editContent = await e2ee.encryptMessage(channelId, msg);
					} catch {
						content = msg;
						addToast('Failed to encrypt message. Do you have the channel key?', 'error');
						return;
					}
				}
				await api.editMessage($editingMessage.channel_id, $editingMessage.id, editContent);
				cancelEdit();
			} catch (e) {
				content = msg;
				addToast('Failed to edit message', 'error');
			}
			return;
		}

		if (guardSlowmode()) return;

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
			recordSuccessfulSend(channelId);
		} catch (e) {
			content = msg;
			handleSendError(e, 'Failed to send message');
		}
	}

	async function handleSchedule(scheduledFor: Date) {
		if ($currentChannel?.encrypted) {
			addToast('Scheduling is not yet supported in encrypted channels', 'error');
			showSchedulePicker = false;
			customDatetime = '';
			return;
		}
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
		// Let mention autocomplete handle keys when it's open.
		if (showMentionAutocomplete && mentionAutocomplete?.handleKeydown(e)) {
			return;
		}

		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			if (uploadOp.loading) return;
			if (isEditing) {
				handleSubmit();
			} else if (pendingFiles.length > 0) {
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

		// Detect @ mention trigger.
		if (inputEl) {
			const cursorPos = inputEl.selectionStart ?? 0;
			const textBeforeCursor = content.slice(0, cursorPos);

			// Find the last @ that's either at the start or preceded by a space/newline.
			const atMatch = textBeforeCursor.match(/(?:^|[\s\n])@(\w*)$/);
			if (atMatch) {
				showMentionAutocomplete = true;
				mentionQuery = atMatch[1];
				mentionStartIndex = cursorPos - atMatch[1].length - 1; // position of @
			} else {
				showMentionAutocomplete = false;
			}
		}
	}

	function handleMentionSelect(syntax: string, displayText: string) {
		// Replace @query with the friendly display text; track the mapping for send-time replacement.
		const before = content.slice(0, mentionStartIndex);
		const after = content.slice(mentionStartIndex + 1 + mentionQuery.length); // +1 for @
		mentionMap.set(displayText, syntax);
		content = before + displayText + ' ' + after;
		showMentionAutocomplete = false;

		// Refocus and position cursor after inserted mention.
		requestAnimationFrame(() => {
			if (inputEl) {
				const newPos = before.length + displayText.length + 1;
				inputEl.focus();
				inputEl.setSelectionRange(newPos, newPos);
			}
		});
	}

	/** Replace display-text mentions with wire syntax before sending. */
	function resolveMentions(text: string): string {
		let resolved = text;
		for (const [display, syntax] of mentionMap) {
			// Replace all occurrences of the display text with the wire syntax.
			// Use split+join for literal replacement (no regex escaping needed).
			resolved = resolved.split(display).join(syntax);
		}
		return resolved;
	}

	function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const files = target.files;
		if (!files || files.length === 0) return;

		addPendingFiles(Array.from(files));
		target.value = '';
	}

	async function uploadPendingFiles() {
		const channelId = $currentChannelId;
		if (!channelId || pendingFiles.length === 0) return;
		if (!fileUploadsEnabled) {
			addToast('File uploads are unavailable on this instance', 'error');
			return;
		}
		if (guardSlowmode()) return;

		// Check for oversized files.
		const oversized = pendingFiles.filter(isFileOverLimit);
		if (oversized.length > 0) {
			addToast(`${oversized.length} file(s) exceed the ${formatFileSize(maxFileSizeBytes)} limit`, 'error');
			return;
		}

		await uploadOp.run(
			async () => {
				const isEncrypted = !!$currentChannel?.encrypted;
				const ids: string[] = [];
				for (let i = 0; i < pendingFiles.length; i++) {
					let file = pendingFiles[i];
					const altText = isEncrypted ? undefined : (pendingAltTexts[i]?.trim() || undefined);
					if (isEncrypted) {
						try {
							const buf = await file.arrayBuffer();
							const encBuf = await e2ee.encryptFile(channelId, buf);
							file = new File([encBuf], file.name + '.enc', { type: 'application/octet-stream' });
						} catch {
							addToast('Failed to encrypt file. Do you have the channel key?', 'error');
							return;
						}
					}
					const uploaded = await api.uploadFile(file, altText);
					ids.push(uploaded.id);
				}
				const msg = content.trim();
				let sendContent = msg;
				const opts: Record<string, any> = { attachment_ids: ids };
				if (isReplying && $replyingTo) {
					opts.reply_to_ids = [$replyingTo.id];
				}
				if (silentMode) {
					opts.silent = true;
				}
				if (isEncrypted) {
					opts.encrypted = true;
					if (msg) {
						try {
							sendContent = await e2ee.encryptMessage(channelId, msg);
						} catch {
							addToast('Failed to encrypt message. Do you have the channel key?', 'error');
							return;
						}
					}
				}
				const sent = await api.sendMessage(channelId, sendContent, opts);
				appendMessage(sent);
				recordSuccessfulSend(channelId);
				cancelReply();
				content = '';
				if (inputEl) inputEl.style.height = 'auto';
				pendingFiles = [];
				pendingAltTexts = {};
			},
			(message) => addToast(message, 'error'),
			'Upload failed'
		);
	}

	async function handleFileUpload(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file || !$currentChannelId) return;
		if (!fileUploadsEnabled) {
			addToast('File uploads are unavailable on this instance', 'error');
			target.value = '';
			return;
		}
		if (guardSlowmode()) {
			target.value = '';
			return;
		}

		if (isFileOverLimit(file)) {
			addToast(`File exceeds the ${formatFileSize(maxFileSizeBytes)} limit`, 'error');
			target.value = '';
			return;
		}

		try {
			let uploadFile: File = file;
			const opts: Record<string, any> = {};
			if (isReplying && $replyingTo) {
				opts.reply_to_ids = [$replyingTo.id];
			}
			if (silentMode) {
				opts.silent = true;
			}
			if ($currentChannel?.encrypted) {
				opts.encrypted = true;
				try {
					const buf = await file.arrayBuffer();
					const encBuf = await e2ee.encryptFile($currentChannelId, buf);
					uploadFile = new File([encBuf], file.name + '.enc', { type: 'application/octet-stream' });
				} catch {
					addToast('Failed to encrypt file. Do you have the channel key?', 'error');
					target.value = '';
					return;
				}
			}
			const uploaded = await api.uploadFile(uploadFile);
			opts.attachment_ids = [uploaded.id];
			const sent = await api.sendMessage($currentChannelId, '', opts);
			appendMessage(sent);
			recordSuccessfulSend($currentChannelId);
			cancelReply();
		} catch (err) {
			handleSendError(err, 'Upload failed');
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
		addPendingFiles(imageFiles);
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
		if (guardSlowmode()) return;
		try {
			let sendContent = gifUrl;
			const opts: Record<string, any> = {};
			if ($currentChannel?.encrypted) {
				try {
					sendContent = await e2ee.encryptMessage(channelId, gifUrl);
					opts.encrypted = true;
				} catch {
					addToast('Failed to encrypt message. Do you have the channel key?', 'error');
					return;
				}
			}
			const sent = await api.sendMessage(channelId, sendContent, opts);
			appendMessage(sent);
			recordSuccessfulSend(channelId);
		} catch (e) {
			handleSendError(e, 'Failed to send GIF');
		}
	}

	async function sendSticker(sticker: Sticker) {
		showStickerPicker = false;
		const channelId = $currentChannelId;
		if (!channelId) return;
		if (guardSlowmode()) return;
		if ($currentChannel?.encrypted) {
			addToast('Stickers are not supported in encrypted channels', 'error');
			return;
		}
		try {
			// Send the sticker as a message with the sticker image file as an attachment.
			const opts: Record<string, any> = { attachment_ids: [sticker.file_id] };
			const sent = await api.sendMessage(channelId, '', opts);
			appendMessage(sent);
			recordSuccessfulSend(channelId);
		} catch (e) {
			handleSendError(e, 'Failed to send sticker');
		}
	}

	async function sendVoiceMessage(audioBlob: Blob, waveform: number[], durationMs: number) {
		const channelId = $currentChannelId;
		if (!channelId) return;
		if (guardSlowmode()) return;

		showVoiceRecorder = false;

		try {
			const ext = audioBlob.type.includes('webm') ? 'webm' : audioBlob.type.includes('ogg') ? 'ogg' : 'mp4';
			let file: File = new File([audioBlob], `voice-message.${ext}`, { type: audioBlob.type });
			if ($currentChannel?.encrypted) {
				try {
					const buf = await audioBlob.arrayBuffer();
					const encBuf = await e2ee.encryptFile(channelId, buf);
					file = new File([encBuf], `voice-message.${ext}.enc`, { type: 'application/octet-stream' });
				} catch {
					addToast('Failed to encrypt voice message. Do you have the channel key?', 'error');
					return;
				}
			}
			const uploaded = await api.uploadFile(file);
			const opts: Record<string, any> = {
				attachment_ids: [uploaded.id]
			};
			if ($currentChannel?.encrypted) {
				opts.encrypted = true;
				// Omit voice metadata in encrypted channels to avoid leaking duration/waveform
			} else {
				opts.voice_duration_ms = durationMs;
				opts.voice_waveform = waveform;
			}
			const sent = await api.sendMessage(channelId, '', opts);
			appendMessage(sent);
			recordSuccessfulSend(channelId);
		} catch (err) {
			handleSendError(err, 'Failed to send voice message');
		}
	}
</script>

{#if $currentChannelId}
	<div class="border-t border-bg-floating px-4 pb-4 pt-2">
		<!-- Passphrase prompt for encrypted channels without a key -->
		{#if needsPassphrase}
			<ChannelPassphrasePrompt bind:passphrase={channelPassphrase} loading={passphraseOp.loading} onunlock={handleSetPassphrase} />
		{/if}

		<MessageInputStatusBars
			replyingTo={$replyingTo}
			editingMessage={$editingMessage}
			bind:silentMode
			{slowmodeRemainingSeconds}
			oncancelreply={cancelReply}
			oncanceledit={() => { cancelEdit(); content = ''; }}
		/>

		<!-- Pending files preview -->
		{#if pendingFiles.length > 0}
			<PendingFilesPreview
				files={pendingFiles}
				bind:altTexts={pendingAltTexts}
				{maxFileSizeBytes}
				uploading={uploadOp.loading}
				sendDisabled={uploadOp.loading || hasOversizedFiles || slowmodeBlocked || !fileUploadsEnabled}
				sendLabel={slowmodeBlocked ? `${slowmodeRemainingSeconds}s` : 'Send'}
				onclear={clearPendingFiles}
				onremove={removePendingFile}
				onsend={uploadPendingFiles}
			/>
		{/if}

		<!-- Voice recorder (replaces the input bar when active) -->
		{#if showVoiceRecorder}
			<VoiceMessageRecorder
				onsend={sendVoiceMessage}
				oncancel={() => (showVoiceRecorder = false)}
			/>
		{:else}
			<div
				class="relative flex items-end gap-2 rounded border border-bg-modifier px-4 py-2 {isEditing ? 'bg-yellow-900/20 ring-1 ring-yellow-500/30' : 'bg-bg-modifier'}"
			>
				<!-- Mention autocomplete popup -->
				{#if showMentionAutocomplete}
					<MentionAutocomplete
						bind:this={mentionAutocomplete}
						query={mentionQuery}
						onSelect={handleMentionSelect}
						onClose={() => showMentionAutocomplete = false}
					/>
				{/if}

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
					aria-label="Message input"
					autocomplete="off"
					name="chat-message"
				></textarea>

				<!-- Right-side icon toolbar -->
				{#if !isEditing}
				<div class="flex items-center gap-2">
					<button
						class="hidden items-center justify-center transition-colors md:flex {silentMode ? 'text-yellow-500 hover:text-yellow-400' : 'text-text-muted hover:text-text-primary'}"
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

				<!-- Schedule message button — desktop only -->
					<div class="relative hidden md:block">
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
						<ScheduleMessagePicker bind:open={showSchedulePicker} bind:customDatetime onpreset={handleSchedule} oncustom={handleCustomSchedule} />
					</div>

				<!-- GIF picker button — desktop only -->
					<div class="giphy-picker relative hidden md:block">
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

				<!-- Sticker picker button — desktop only -->
					<div class="sticker-picker relative hidden md:block">
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

				<!-- Voice message button — desktop only -->
					<button
						class="hidden items-center justify-center text-text-muted hover:text-text-primary md:flex"
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

			<!-- Mobile "+" overflow button -->
					<div class="relative md:hidden">
						<button
							class="flex items-center justify-center text-text-muted hover:text-text-primary"
							title="More"
							onclick={() => (showInputMore = !showInputMore)}
						>
							<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
								<path d="M12 5v14m-7-7h14" />
							</svg>
						</button>
						{#if showInputMore}
								<button class="fixed inset-0 z-40 cursor-default" aria-label="Close input actions" onclick={() => (showInputMore = false)}></button>
							<div class="absolute bottom-8 right-0 z-50 flex gap-2 rounded-lg bg-bg-floating p-2 shadow-xl">
								<button
									class="flex items-center justify-center rounded p-1.5 transition-colors {silentMode ? 'text-yellow-500' : 'text-text-muted hover:text-text-primary'}"
									title={silentMode ? 'Silent on' : 'Silent'}
									onclick={() => { silentMode = !silentMode; showInputMore = false; }}
								>
									<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										{#if silentMode}
											<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
											<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
										{:else}
											<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
										{/if}
									</svg>
								</button>
								<button
									class="flex items-center justify-center rounded p-1.5 text-text-muted hover:text-text-primary"
									title="Schedule"
									onclick={() => { showSchedulePicker = !showSchedulePicker; showInputMore = false; }}
								>
									<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
									</svg>
								</button>
								<div class="giphy-picker">
									<button
										class="flex h-7 items-center rounded border border-text-muted px-1.5 text-[10px] font-bold text-text-muted hover:border-text-primary hover:text-text-primary"
										title="GIF"
										onclick={(e) => { e.stopPropagation(); showGiphyPicker = !showGiphyPicker; showInputMore = false; }}
									>
										GIF
									</button>
								</div>
								<div class="sticker-picker">
									<button
										class="flex items-center justify-center rounded p-1.5 text-text-muted hover:text-text-primary"
										title="Stickers"
										onclick={(e) => { e.stopPropagation(); showStickerPicker = !showStickerPicker; showInputMore = false; }}
									>
										<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
											<path d="M15 2v5a2 2 0 002 2h5" />
										</svg>
									</button>
								</div>
								<button
									class="flex items-center justify-center rounded p-1.5 text-text-muted hover:text-text-primary"
									title="Voice"
									onclick={() => { showVoiceRecorder = true; showInputMore = false; }}
								>
									<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
										<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
										<path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
									</svg>
								</button>
							</div>
						{/if}
					</div>
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
