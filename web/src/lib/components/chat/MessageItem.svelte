<script lang="ts">
	import type { Message, User } from '$lib/types';
	import Avatar from '$components/common/Avatar.svelte';

	interface Props {
		message: Message;
		isCompact?: boolean;
	}

	let { message, isCompact = false }: Props = $props();

	const timestamp = $derived(
		new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
	);

	const fullDate = $derived(new Date(message.created_at).toLocaleDateString());

	const displayName = $derived(message.masquerade_name ?? message.author_id);
</script>

<div class="group flex gap-4 px-4 py-0.5 hover:bg-bg-modifier/30" class:mt-4={!isCompact}>
	{#if isCompact}
		<div class="w-10 shrink-0 pt-1 text-right">
			<span class="hidden text-2xs text-text-muted group-hover:inline">{timestamp}</span>
		</div>
	{:else}
		<div class="mt-0.5 shrink-0">
			<Avatar name={displayName} size="md" />
		</div>
	{/if}

	<div class="min-w-0 flex-1">
		{#if !isCompact}
			<div class="flex items-baseline gap-2">
				<span class="font-medium text-text-primary hover:underline">{displayName}</span>
				<time class="text-xs text-text-muted" title={fullDate}>{timestamp}</time>
				{#if message.edited_at}
					<span class="text-2xs text-text-muted">(edited)</span>
				{/if}
			</div>
		{/if}

		{#if message.content}
			<p class="text-sm text-text-secondary leading-relaxed break-words">{message.content}</p>
		{/if}

		<!-- Attachments -->
		{#if message.attachments?.length > 0}
			<div class="mt-1 flex flex-wrap gap-2">
				{#each message.attachments as attachment (attachment.id)}
					{#if attachment.content_type?.startsWith('image/')}
						<img
							src="/api/v1/files/{attachment.id}"
							alt={attachment.filename}
							class="max-h-80 max-w-md rounded"
							loading="lazy"
						/>
					{:else}
						<a
							href="/api/v1/files/{attachment.id}"
							class="flex items-center gap-2 rounded bg-bg-secondary px-3 py-2 text-sm text-text-link hover:underline"
							download={attachment.filename}
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
				<div
					class="mt-1 max-w-md overflow-hidden rounded border-l-4 border-brand-500 bg-bg-secondary p-3"
				>
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
						<img
							src={embed.thumbnail_url}
							alt=""
							class="mt-2 max-h-60 rounded"
							loading="lazy"
						/>
					{/if}
				</div>
			{/each}
		{/if}
	</div>
</div>
