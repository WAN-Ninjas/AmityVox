<script lang="ts">
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import type { MessageBookmark } from '$lib/types';

	let bookmarks = $state<MessageBookmark[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			bookmarks = await api.getBookmarks();
		} catch (err: any) {
			error = err.message || 'Failed to load bookmarks';
		} finally {
			loading = false;
		}
	});

	async function removeBookmark(messageId: string) {
		try {
			await api.deleteBookmark(messageId);
			bookmarks = bookmarks.filter(b => b.message_id !== messageId);
		} catch (err: any) {
			error = err.message || 'Failed to remove bookmark';
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<svelte:head>
	<title>Saved Messages — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col">
	<div class="flex h-12 items-center border-b border-bg-modifier px-4">
		<svg class="mr-2 h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
		</svg>
		<h1 class="text-base font-semibold text-text-primary">Saved Messages</h1>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="mx-auto max-w-2xl">
			{#if loading}
				<p class="text-sm text-text-muted">Loading saved messages...</p>
			{:else if error}
				<p class="text-sm text-red-400">{error}</p>
			{:else if bookmarks.length === 0}
				<div class="flex flex-col items-center justify-center py-20 text-center">
					<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
						<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
					</svg>
					<h2 class="mb-2 text-lg font-semibold text-text-primary">No saved messages</h2>
					<p class="text-sm text-text-muted">Right-click any message and select "Bookmark" to save it here.</p>
				</div>
			{:else}
				<div class="space-y-3">
					{#each bookmarks as bookmark (bookmark.message_id)}
						<div class="group rounded-lg bg-bg-secondary p-4">
							<div class="flex items-start justify-between">
								<div class="flex-1">
									{#if bookmark.message}
										<div class="mb-1 flex items-center gap-2">
											<span class="text-sm font-semibold text-text-primary">
												{bookmark.message.author?.display_name ?? bookmark.message.author?.username ?? 'Unknown'}
											</span>
											<span class="text-xs text-text-muted">{formatDate(bookmark.message.created_at)}</span>
										</div>
										<p class="text-sm text-text-secondary">{bookmark.message.content ?? '[No content]'}</p>
									{:else}
										<p class="text-sm text-text-muted">Message unavailable</p>
									{/if}
									{#if bookmark.note}
										<p class="mt-2 border-l-2 border-brand-500 pl-2 text-xs text-text-muted italic">{bookmark.note}</p>
									{/if}
								</div>
								<button
									class="ml-3 shrink-0 text-xs text-red-400 opacity-0 transition-opacity hover:text-red-300 group-hover:opacity-100"
									onclick={() => removeBookmark(bookmark.message_id)}
								>
									Remove
								</button>
							</div>
							<p class="mt-1 text-xs text-text-muted">Saved {formatDate(bookmark.created_at)}</p>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
