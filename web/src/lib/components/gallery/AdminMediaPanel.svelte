<script lang="ts">
	import type { Attachment } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import GalleryItem from './GalleryItem.svelte';
	import MediaPreviewModal from './MediaPreviewModal.svelte';

	let items = $state<Attachment[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let selectedItem = $state<Attachment | null>(null);
	let showPreview = $state(false);

	$effect(() => {
		api.getAdminMedia()
			.then((data) => { items = data; hasMore = data.length >= 50; })
			.catch(() => {})
			.finally(() => { loading = false; });
	});

	async function loadMore() {
		if (loadingMore || items.length === 0) return;
		loadingMore = true;
		try {
			const data = await api.getAdminMedia(items[items.length - 1].id);
			items = [...items, ...data];
			hasMore = data.length >= 50;
		} catch {
			// ignore
		} finally {
			loadingMore = false;
		}
	}

	function openPreview(item: Attachment) {
		selectedItem = item;
		showPreview = true;
	}

	async function handleAdminDelete() {
		if (!selectedItem) return;
		try {
			await api.deleteAdminMedia(selectedItem.id);
			items = items.filter((i) => i.id !== selectedItem!.id);
			selectedItem = null;
			showPreview = false;
			addToast('File deleted by admin', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete', 'error');
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center gap-2">
		<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
		</svg>
		<h3 class="text-sm font-semibold text-text-primary">Instance Media</h3>
		<span class="text-xs text-text-muted">({items.length} files)</span>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-8">
			<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if items.length === 0}
		<p class="py-4 text-sm text-text-muted">No media uploaded yet.</p>
	{:else}
		<div class="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
			{#each items as item (item.id)}
				<GalleryItem attachment={item} onclick={() => openPreview(item)} />
			{/each}
		</div>

		{#if hasMore}
			<div class="flex justify-center">
				<button class="btn-secondary text-sm" onclick={loadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load More'}
				</button>
			</div>
		{/if}
	{/if}
</div>

<MediaPreviewModal
	attachment={selectedItem}
	bind:open={showPreview}
	onclose={() => (showPreview = false)}
	ondelete={handleAdminDelete}
	canManage={true}
/>
