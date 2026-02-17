<script lang="ts">
	import type { Attachment } from '$lib/types';

	interface Props {
		attachment: Attachment;
		onclick: () => void;
	}

	let { attachment, onclick }: Props = $props();

	const isVideo = $derived(attachment.content_type.startsWith('video/'));
	const isImage = $derived(attachment.content_type.startsWith('image/'));
	const fileUrl = $derived(`/api/v1/files/${attachment.id}`);
</script>

<button
	class="group w-full overflow-hidden rounded-lg bg-bg-primary transition-colors hover:bg-bg-modifier"
	{onclick}
	title={attachment.filename}
>
	<!-- Media preview -->
	<div class="relative w-full">
		{#if isImage}
			<img
				class="w-full rounded-t-lg object-contain"
				style="max-height: 280px;"
				src={fileUrl}
				alt={attachment.alt_text ?? attachment.filename}
				loading="lazy"
			/>
		{:else if isVideo}
			<div class="relative w-full">
				<video
					class="w-full rounded-t-lg object-contain"
					style="max-height: 280px;"
					src={fileUrl}
					preload="metadata"
				>
					<track kind="captions" />
				</video>
				<div class="absolute inset-0 flex items-center justify-center">
					<div class="rounded-full bg-black/50 p-3">
						<svg class="h-8 w-8 text-white" fill="currentColor" viewBox="0 0 24 24">
							<path d="M8 5v14l11-7z" />
						</svg>
					</div>
				</div>
				{#if attachment.duration_seconds}
					<span class="absolute bottom-2 right-2 rounded bg-black/70 px-1.5 py-0.5 text-xs font-medium text-white">
						{Math.floor(attachment.duration_seconds / 60)}:{String(Math.floor(attachment.duration_seconds % 60)).padStart(2, '0')}
					</span>
				{/if}
			</div>
		{/if}

		{#if attachment.nsfw}
			<span class="absolute left-2 top-2 rounded bg-red-500/80 px-1.5 py-0.5 text-xs font-bold text-white">NSFW</span>
		{/if}
	</div>

	<!-- File info (always visible) -->
	<div class="px-3 py-2">
		<p class="truncate text-sm font-medium text-text-primary">{attachment.filename}</p>
		<p class="text-xs text-text-muted">
			{(attachment.size_bytes / 1024).toFixed(0)} KB
			{#if attachment.width && attachment.height}
				&middot; {attachment.width}x{attachment.height}
			{/if}
		</p>
	</div>
</button>
