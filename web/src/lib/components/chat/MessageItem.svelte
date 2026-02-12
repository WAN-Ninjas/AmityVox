<script lang="ts">
	import type { Message } from '$lib/types';
	import Avatar from '$components/common/Avatar.svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	interface Props {
		message: Message;
		isCompact?: boolean;
	}

	let { message, isCompact = false }: Props = $props();

	let editing = $state(false);
	let editContent = $state('');
	let showEmojiPicker = $state(false);

	const timestamp = $derived(
		new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
	);

	const fullDate = $derived(new Date(message.created_at).toLocaleDateString());

	const displayName = $derived(
		message.masquerade_name ?? message.author?.display_name ?? message.author?.username ?? message.author_id
	);

	const isOwnMessage = $derived($currentUser?.id === message.author_id);

	const commonEmoji = ['👍', '❤️', '😂', '🎉', '😮', '😢'];

	function startEdit() {
		editContent = message.content ?? '';
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editContent = '';
	}

	async function saveEdit() {
		if (!editContent.trim()) return;
		try {
			await api.editMessage(message.channel_id, message.id, editContent.trim());
			editing = false;
		} catch (err: any) {
			console.error('Failed to edit message:', err);
		}
	}

	async function handleDelete() {
		if (!confirm('Delete this message?')) return;
		try {
			await api.deleteMessage(message.channel_id, message.id);
		} catch (err: any) {
			console.error('Failed to delete message:', err);
		}
	}

	async function addReaction(emoji: string) {
		showEmojiPicker = false;
		try {
			await api.addReaction(message.channel_id, message.id, emoji);
		} catch (err: any) {
			console.error('Failed to add reaction:', err);
		}
	}

	function handleEditKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			saveEdit();
		} else if (e.key === 'Escape') {
			cancelEdit();
		}
	}
</script>

<div class="group relative flex gap-4 px-4 py-0.5 hover:bg-bg-modifier/30" class:mt-4={!isCompact}>
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

		{#if editing}
			<div class="mt-1">
				<textarea
					class="input w-full text-sm"
					bind:value={editContent}
					onkeydown={handleEditKeydown}
					rows="2"
				></textarea>
				<div class="mt-1 flex items-center gap-2">
					<button class="text-xs text-text-link hover:underline" onclick={saveEdit}>save</button>
					<span class="text-xs text-text-muted">&middot;</span>
					<button class="text-xs text-text-muted hover:text-text-secondary" onclick={cancelEdit}>cancel</button>
					<span class="text-2xs text-text-muted ml-2">Enter to save, Escape to cancel</span>
				</div>
			</div>
		{:else if message.content}
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

	<!-- Action buttons (shown on hover) -->
	{#if !editing}
		<div class="absolute -top-3 right-4 hidden rounded border border-bg-modifier bg-bg-secondary shadow group-hover:flex">
			<!-- Reaction picker toggle -->
			<div class="relative">
				<button
					class="p-1.5 text-text-muted hover:text-text-primary"
					title="Add Reaction"
					onclick={() => (showEmojiPicker = !showEmojiPicker)}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
				</button>
				{#if showEmojiPicker}
					<div class="absolute right-0 top-full z-10 mt-1 flex gap-1 rounded bg-bg-floating p-2 shadow-lg">
						{#each commonEmoji as emoji}
							<button
								class="rounded p-1 text-lg hover:bg-bg-modifier"
								onclick={() => addReaction(emoji)}
							>
								{emoji}
							</button>
						{/each}
					</div>
				{/if}
			</div>

			{#if isOwnMessage}
				<button
					class="p-1.5 text-text-muted hover:text-text-primary"
					title="Edit"
					onclick={startEdit}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
					</svg>
				</button>
			{/if}

			{#if isOwnMessage}
				<button
					class="p-1.5 text-text-muted hover:text-red-400"
					title="Delete"
					onclick={handleDelete}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
					</svg>
				</button>
			{/if}
		</div>
	{/if}
</div>
