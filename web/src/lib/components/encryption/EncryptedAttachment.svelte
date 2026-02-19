<script lang="ts">
	import { e2ee } from '$lib/encryption/e2eeManager';
	import { addToast } from '$lib/stores/toast';
	import AudioPlayer from '$components/chat/AudioPlayer.svelte';
	import VideoPlayer from '$components/chat/VideoPlayer.svelte';

	interface Props {
		attachment: {
			id: string;
			filename: string;
			content_type: string;
			size_bytes: number;
			width?: number | null;
			height?: number | null;
			alt_text?: string | null;
		};
		channelId: string;
		onlightbox?: (src: string) => void;
		oncontextmenu?: (e: MouseEvent) => void;
	}

	let { attachment, channelId, onlightbox, oncontextmenu }: Props = $props();

	let blobUrl = $state<string | null>(null);
	let decrypting = $state(true);
	let failed = $state(false);

	/** Strip the .enc suffix to get the original filename and MIME type. */
	const originalFilename = $derived(
		attachment.filename.endsWith('.enc')
			? attachment.filename.slice(0, -4)
			: attachment.filename
	);

	const originalMime = $derived(mimeFromFilename(originalFilename));
	const isImage = $derived(originalMime.startsWith('image/'));
	const isAudio = $derived(originalMime.startsWith('audio/'));
	const isVideo = $derived(originalMime.startsWith('video/'));

	function mimeFromFilename(name: string): string {
		const ext = name.split('.').pop()?.toLowerCase() ?? '';
		const map: Record<string, string> = {
			jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
			gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml',
			avif: 'image/avif', bmp: 'image/bmp', ico: 'image/x-icon',
			mp3: 'audio/mpeg', ogg: 'audio/ogg', wav: 'audio/wav',
			flac: 'audio/flac', aac: 'audio/aac', m4a: 'audio/mp4',
			webm: 'video/webm', mp4: 'video/mp4', mov: 'video/quicktime',
			avi: 'video/x-msvideo', mkv: 'video/x-matroska',
			pdf: 'application/pdf', zip: 'application/zip',
		};
		return map[ext] ?? 'application/octet-stream';
	}

	$effect(() => {
		// Reset state when attachment changes
		const id = attachment.id;
		const ch = channelId;
		blobUrl = null;
		decrypting = true;
		failed = false;

		fetchAndDecrypt(id, ch);

		return () => {
			if (blobUrl) URL.revokeObjectURL(blobUrl);
		};
	});

	async function fetchAndDecrypt(attachmentId: string, ch: string) {
		try {
			const res = await fetch(`/api/v1/files/${attachmentId}`);
			if (!res.ok) throw new Error('fetch failed');
			const encrypted = await res.arrayBuffer();
			const decrypted = await e2ee.decryptFile(ch, encrypted);
			blobUrl = URL.createObjectURL(new Blob([decrypted], { type: originalMime }));
		} catch {
			failed = true;
		} finally {
			decrypting = false;
		}
	}

	function handleDownload() {
		if (!blobUrl) return;
		const a = document.createElement('a');
		a.href = blobUrl;
		a.download = originalFilename;
		a.click();
	}
</script>

{#if decrypting}
	<div class="flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-sm text-text-muted">
		<svg class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
			<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
			<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
		</svg>
		Decrypting {originalFilename}...
	</div>
{:else if failed}
	<div class="flex items-center gap-2 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">
		<svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
		</svg>
		Failed to decrypt {originalFilename}
	</div>
{:else if blobUrl && isImage}
	<div class="inline-flex flex-col">
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<img
			src={blobUrl}
			alt={attachment.alt_text || originalFilename}
			class="max-h-80 max-w-md rounded cursor-pointer hover:brightness-90 transition-[filter]"
			loading="lazy"
			onclick={() => onlightbox?.(blobUrl!)}
			oncontextmenu={oncontextmenu}
		/>
		{#if attachment.alt_text}
			<span class="mt-0.5 max-w-md text-2xs text-text-muted">{attachment.alt_text}</span>
		{/if}
	</div>
{:else if blobUrl && isAudio}
	<AudioPlayer src={blobUrl} />
{:else if blobUrl && isVideo}
	<VideoPlayer
		src={blobUrl}
		width={attachment.width ?? undefined}
		height={attachment.height ?? undefined}
		filename={originalFilename}
	/>
{:else if blobUrl}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<button
		class="flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-sm text-text-link hover:underline"
		onclick={handleDownload}
		oncontextmenu={oncontextmenu}
	>
		<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
			<path d="M14 2H6c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8l-6-6zm4 18H6V4h7v5h5v11z" />
		</svg>
		{originalFilename}
		<span class="text-xs text-text-muted">
			({(attachment.size_bytes / 1024).toFixed(0)} KB)
		</span>
	</button>
{/if}
