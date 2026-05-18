<script lang="ts">
	import AudioPlayer from '$components/chat/AudioPlayer.svelte';
	import VideoPlayer from '$components/chat/VideoPlayer.svelte';
	import EncryptedAttachment from '$components/encryption/EncryptedAttachment.svelte';
	import { fileUrl } from '$lib/utils/avatar';
	import type { Attachment, Message } from '$lib/types';

	interface Props {
		message: Message;
		hasEncryptionKey: boolean | null;
		isStickerMessage: boolean;
		shouldBlurImage: (attachmentId: string) => boolean;
		onrevealimage: (attachmentId: string) => void;
		onlightbox: (src: string) => void;
		oncontextmenu: (event: MouseEvent, attachment: Attachment) => void;
	}

	let { message, hasEncryptionKey, isStickerMessage, shouldBlurImage, onrevealimage, onlightbox, oncontextmenu }: Props = $props();
</script>

{#if message.attachments?.length > 0 && (!message.encrypted || hasEncryptionKey === true)}
	<div class="mt-1 flex flex-wrap gap-2">
		{#each message.attachments as attachment (attachment.id)}
			{#if message.encrypted && attachment.filename?.endsWith('.enc')}
				<EncryptedAttachment
					{attachment}
					channelId={message.channel_id}
					onlightbox={onlightbox}
					oncontextmenu={(e) => oncontextmenu(e, attachment)}
				/>
			{:else if attachment.content_type?.startsWith('image/')}
				{#if shouldBlurImage(attachment.id)}
					<button
						type="button"
						class="relative max-h-80 max-w-md cursor-pointer overflow-hidden rounded"
						onclick={() => onrevealimage(attachment.id)}
						aria-label="Reveal NSFW image"
					>
						<img
							src={fileUrl(attachment.id, attachment.instance_id || undefined)}
							alt={attachment.alt_text || attachment.filename}
							class="max-h-80 max-w-md rounded transition-[filter]"
							style="filter: blur(20px);"
							loading="lazy"
						/>
						<div class="absolute inset-0 flex items-center justify-center bg-black/30">
							<span class="rounded bg-bg-floating/80 px-3 py-1.5 text-xs font-medium text-text-primary">Click to reveal NSFW image</span>
						</div>
					</button>
				{:else if isStickerMessage}
					<button
						type="button"
						class="block"
						onclick={() => onlightbox(fileUrl(attachment.id, attachment.instance_id || undefined))}
						oncontextmenu={(e) => oncontextmenu(e, attachment)}
						aria-label="Open {attachment.alt_text || attachment.filename}"
					>
						<img
							src={fileUrl(attachment.id, attachment.instance_id || undefined)}
							alt={attachment.alt_text || attachment.filename}
							class="h-40 w-40 object-contain transition-transform hover:scale-105"
							loading="lazy"
						/>
					</button>
				{:else}
					<div class="inline-flex flex-col">
						<button
							type="button"
							class="block"
							onclick={() => onlightbox(fileUrl(attachment.id, attachment.instance_id || undefined))}
							oncontextmenu={(e) => oncontextmenu(e, attachment)}
							aria-label="Open {attachment.alt_text || attachment.filename}"
						>
							<img
								src={fileUrl(attachment.id, attachment.instance_id || undefined)}
								alt={attachment.alt_text || attachment.filename}
								class="max-h-80 max-w-md rounded transition-[filter] hover:brightness-90"
								loading="lazy"
							/>
						</button>
						{#if attachment.alt_text}
							<span class="mt-0.5 max-w-md text-2xs text-text-muted">{attachment.alt_text}</span>
						{/if}
					</div>
				{/if}
			{:else if attachment.content_type?.startsWith('audio/')}
				<AudioPlayer src={fileUrl(attachment.id, attachment.instance_id || undefined)} waveform={message.voice_waveform} durationMs={message.voice_duration_ms} />
			{:else if attachment.content_type?.startsWith('video/')}
				<VideoPlayer
					src={fileUrl(attachment.id, attachment.instance_id || undefined)}
					width={attachment.width ?? undefined}
					height={attachment.height ?? undefined}
					filename={attachment.filename}
				/>
			{:else}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<a
					href={fileUrl(attachment.id, attachment.instance_id || undefined)}
					class="flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-sm text-text-link hover:underline"
					download={attachment.filename}
					oncontextmenu={(e) => oncontextmenu(e, attachment)}
				>
					<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
						<path d="M14 2H6c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8l-6-6zm4 18H6V4h7v5h5v11z" />
					</svg>
					{attachment.filename}
					<span class="text-xs text-text-muted">({(attachment.size_bytes / 1024).toFixed(0)} KB)</span>
				</a>
			{/if}
		{/each}
	</div>
{/if}
