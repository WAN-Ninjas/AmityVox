<script lang="ts">
	import type { Attachment } from '$lib/types';
	import { api } from '$lib/api/client';
	import GalleryItem from './GalleryItem.svelte';
	import GalleryFilters from './GalleryFilters.svelte';
	import MediaPreviewModal from './MediaPreviewModal.svelte';

	interface Props {
		channelId?: string;
		guildId?: string;
		canManage?: boolean;
		onclose?: () => void;
	}

	let { channelId, guildId, canManage = false, onclose }: Props = $props();

	let items = $state<Attachment[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let typeFilter = $state('all');
	let scope = $state<'channel' | 'server'>(channelId ? 'channel' : 'server');
	let selectedItem = $state<Attachment | null>(null);
	let showPreview = $state(false);

	async function loadGallery(append = false) {
		if (append) {
			loadingMore = true;
		} else {
			loading = true;
			items = [];
		}

		try {
			const before = append && items.length > 0 ? items[items.length - 1].id : undefined;
			const opts = { before, type: typeFilter !== 'all' ? typeFilter : undefined };

			let data: Attachment[];
			if (scope === 'channel' && channelId) {
				data = await api.getChannelGallery(channelId, opts);
			} else if (guildId) {
				data = await api.getGuildGallery(guildId, opts);
			} else if (channelId) {
				data = await api.getChannelGallery(channelId, opts);
			} else {
				data = [];
			}

			if (append) {
				items = [...items, ...data];
			} else {
				items = data;
			}
			hasMore = data.length >= 50;
		} catch {
			// Silently fail
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	$effect(() => {
		// Re-load when channelId, guildId, filter, or scope changes.
		channelId; guildId; typeFilter; scope;
		loadGallery();
	});

	function handleFilterChange(type: string) {
		typeFilter = type;
	}

	function openPreview(item: Attachment) {
		selectedItem = item;
		showPreview = true;
	}

	function handleDelete() {
		if (selectedItem) {
			items = items.filter((i) => i.id !== selectedItem!.id);
			selectedItem = null;
		}
	}
</script>

<div class="flex h-full flex-col">
	<!-- Header -->
	<div class="border-b border-bg-modifier px-4 py-3">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				{#if onclose}
					<button
						class="rounded p-0.5 text-text-muted transition-colors hover:text-text-primary"
						onclick={onclose}
						title="Close gallery"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
							<path d="M15 19l-7-7 7-7" />
						</svg>
					</button>
				{/if}
				<h2 class="text-sm font-semibold text-text-primary">Gallery</h2>
				<span class="text-xs text-text-muted">({items.length})</span>
			</div>
			<GalleryFilters {typeFilter} onfilter={handleFilterChange} />
		</div>

		<!-- Scope toggle (channel vs server) -->
		{#if channelId && guildId}
			<div class="mt-2 flex rounded-lg bg-bg-primary p-0.5">
				<button
					class="flex-1 rounded-md px-3 py-1 text-xs font-medium transition-colors {scope === 'channel' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
					onclick={() => (scope = 'channel')}
				>
					Channel
				</button>
				<button
					class="flex-1 rounded-md px-3 py-1 text-xs font-medium transition-colors {scope === 'server' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
					onclick={() => (scope = 'server')}
				>
					Server
				</button>
			</div>
		{/if}
	</div>

	<!-- Gallery grid -->
	<div class="flex-1 overflow-y-auto p-4">
		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if items.length === 0}
			<div class="flex flex-col items-center justify-center py-12 text-text-muted">
				<svg class="mb-2 h-12 w-12 opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909M18 18.75h.008v.008H18v-.008zm-1.5-7.5h.008v.008H16.5v-.008z" />
				</svg>
				<p class="text-sm">No media found.</p>
			</div>
		{:else}
			<div class="flex flex-col gap-3">
				{#each items as item (item.id)}
					<GalleryItem attachment={item} onclick={() => openPreview(item)} />
				{/each}
			</div>

			{#if hasMore}
				<div class="flex justify-center py-4">
					<button
						class="btn-secondary text-sm"
						onclick={() => loadGallery(true)}
						disabled={loadingMore}
					>
						{loadingMore ? 'Loading...' : 'Load More'}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>

<MediaPreviewModal
	attachment={selectedItem}
	bind:open={showPreview}
	onclose={() => (showPreview = false)}
	ondelete={handleDelete}
	{canManage}
/>
