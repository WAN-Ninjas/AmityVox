<script lang="ts">
	import type { Channel, GalleryPost, GalleryTag } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { channels } from '$lib/stores/channels';
	import GalleryPostCard from './GalleryPostCard.svelte';
	import GalleryPostCreate from './GalleryPostCreate.svelte';

	interface Props {
		channelId: string;
		onopenthread?: (channel: Channel, parentMessage?: null) => void;
	}

	let { channelId, onopenthread }: Props = $props();

	let posts = $state<GalleryPost[]>([]);
	let galleryTags = $state<GalleryTag[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');
	let hasMore = $state(false);
	let showNewPostForm = $state(false);

	// Sort & filter state
	let sortBy = $state<'newest' | 'oldest' | 'most_comments'>('newest');
	let filterTagId = $state<string | null>(null);

	// Get the current gallery channel for metadata
	let galleryChannel = $derived.by(() => {
		let ch: Channel | undefined;
		channels.subscribe((m) => (ch = m.get(channelId)))();
		return ch;
	});

	let guidelines = $derived(galleryChannel?.gallery_post_guidelines ?? null);
	let requireTags = $derived(galleryChannel?.gallery_require_tags ?? false);

	// Reload when channelId, sort, or filter changes
	$effect(() => {
		const _id = channelId;
		const _sort = sortBy;
		const _tag = filterTagId;
		loadPosts(true);
		loadTags();
	});

	async function loadTags() {
		try {
			galleryTags = await api.getGalleryTags(channelId);
		} catch {
			galleryTags = [];
		}
	}

	async function loadPosts(reset: boolean) {
		if (reset) {
			loading = true;
			error = '';
			posts = [];
		} else {
			loadingMore = true;
		}

		try {
			const cursor = !reset && posts.length > 0 ? posts[posts.length - 1].created_at : undefined;
			const result = await api.getGalleryPosts(channelId, {
				sort: sortBy,
				tag: filterTagId ?? undefined,
				before: cursor,
				limit: 25
			});
			if (reset) {
				posts = result;
			} else {
				posts = [...posts, ...result];
			}
			hasMore = result.length === 25;
		} catch (err: any) {
			if (reset) {
				error = err.message || 'Failed to load gallery posts';
			} else {
				addToast('Failed to load more posts', 'error');
			}
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	function openPost(post: GalleryPost) {
		// Gallery posts are thread channels — look up the full Channel from the store
		let channel: Channel | undefined;
		channels.subscribe((m) => (channel = m.get(post.id)))();

		if (channel) {
			onopenthread?.(channel, null);
		} else {
			// Fallback: construct a minimal channel-like object
			onopenthread?.({
				id: post.id,
				guild_id: galleryChannel?.guild_id ?? null,
				category_id: null,
				channel_type: 'text',
				name: post.name,
				topic: null,
				position: 0,
				slowmode_seconds: 0,
				nsfw: false,
				encrypted: false,
				last_message_id: null,
				owner_id: post.owner_id,
				user_limit: 0,
				bitrate: 0,
				locked: post.locked,
				locked_by: null,
				locked_at: null,
				archived: false,
				parent_channel_id: channelId,
				last_activity_at: post.last_activity_at,
				pinned: post.pinned,
				reply_count: post.reply_count,
				created_at: post.created_at
			} as Channel, null);
		}
	}

	function handlePostCreated() {
		showNewPostForm = false;
		loadPosts(true);
	}

	function setFilterTag(tagId: string | null) {
		filterTagId = filterTagId === tagId ? null : tagId;
	}

	// Separate pinned from regular posts for display
	let pinnedPosts = $derived(posts.filter((p) => p.pinned));
	let regularPosts = $derived(posts.filter((p) => !p.pinned));
</script>

<div class="flex h-full flex-col bg-bg-primary">
	<!-- Header -->
	<div class="flex min-h-12 flex-wrap items-center gap-2 border-b border-bg-floating px-4 py-2">
		<div class="flex items-center gap-2">
			<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<rect x="3" y="3" width="7" height="7" rx="1" />
				<rect x="14" y="3" width="7" height="7" rx="1" />
				<rect x="3" y="14" width="7" height="7" rx="1" />
				<rect x="14" y="14" width="7" height="7" rx="1" />
			</svg>
			<h2 class="text-sm font-semibold text-text-primary">
				{galleryChannel?.name ?? 'Gallery'}
			</h2>
		</div>

		<div class="ml-auto flex items-center gap-2">
			<!-- Sort dropdown -->
			<select
				class="rounded-md border border-bg-modifier bg-bg-tertiary px-2 py-1 text-xs text-text-secondary outline-none focus:border-brand-500"
				bind:value={sortBy}
			>
				<option value="newest">Newest</option>
				<option value="oldest">Oldest</option>
				<option value="most_comments">Most Comments</option>
			</select>

			<button
				class="rounded-md bg-brand-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600"
				onclick={() => (showNewPostForm = !showNewPostForm)}
			>
				{showNewPostForm ? 'Cancel' : 'New Post'}
			</button>
		</div>
	</div>

	<!-- Tag filter bar -->
	{#if galleryTags.length > 0}
		<div class="flex items-center gap-1.5 border-b border-bg-floating px-4 py-2 overflow-x-auto">
			<span class="shrink-0 text-xs text-text-muted">Filter:</span>
			{#each galleryTags as tag (tag.id)}
				<button
					class="inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium transition-colors {filterTagId === tag.id ? 'border-brand-500 bg-brand-500/15 text-brand-400' : 'border-bg-modifier text-text-secondary hover:border-text-muted'}"
					onclick={() => setFilterTag(tag.id)}
				>
					{#if tag.emoji}<span>{tag.emoji}</span>{/if}
					{tag.name}
				</button>
			{/each}
			{#if filterTagId}
				<button
					class="shrink-0 text-[11px] text-text-muted hover:text-text-primary"
					onclick={() => (filterTagId = null)}
				>
					Clear
				</button>
			{/if}
		</div>
	{/if}

	<!-- New post form -->
	{#if showNewPostForm}
		<GalleryPostCreate
			{channelId}
			tags={galleryTags}
			{requireTags}
			{guidelines}
			oncreated={handlePostCreated}
			oncancel={() => (showNewPostForm = false)}
		/>
	{/if}

	<!-- Post grid -->
	<div class="flex-1 overflow-y-auto">
		{#if loading}
			<div class="flex items-center justify-center py-16">
				<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if error}
			<div class="flex flex-col items-center justify-center py-16">
				<svg class="mb-2 h-10 w-10 text-red-400" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
				</svg>
				<p class="text-sm text-red-400">{error}</p>
				<button
					class="mt-2 text-xs text-brand-400 hover:underline"
					onclick={() => loadPosts(true)}
				>
					Try again
				</button>
			</div>
		{:else if posts.length === 0}
			<div class="flex flex-col items-center justify-center py-16">
				<svg class="mb-3 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1" viewBox="0 0 24 24">
					<rect x="3" y="3" width="7" height="7" rx="1" />
					<rect x="14" y="3" width="7" height="7" rx="1" />
					<rect x="3" y="14" width="7" height="7" rx="1" />
					<rect x="14" y="14" width="7" height="7" rx="1" />
				</svg>
				<p class="text-sm font-medium text-text-secondary">
					{filterTagId ? 'No posts match this filter' : 'No gallery posts yet'}
				</p>
				<p class="mt-1 text-xs text-text-muted">
					{filterTagId ? 'Try removing the tag filter.' : 'Be the first to share an image or video.'}
				</p>
			</div>
		{:else}
			<div class="p-4">
				<!-- Pinned posts -->
				{#if pinnedPosts.length > 0}
					<div class="mb-4">
						<h3 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-text-muted">
							<svg class="h-3 w-3" fill="currentColor" viewBox="0 0 24 24">
								<path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
							</svg>
							Pinned
						</h3>
						<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
							{#each pinnedPosts as post (post.id)}
								<GalleryPostCard {post} onclick={() => openPost(post)} />
							{/each}
						</div>
					</div>
				{/if}

				<!-- Regular posts -->
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
					{#each regularPosts as post (post.id)}
						<GalleryPostCard {post} onclick={() => openPost(post)} />
					{/each}
				</div>

				<!-- Load more -->
				{#if hasMore}
					<div class="flex justify-center py-6">
						<button
							class="rounded-md bg-bg-secondary px-4 py-2 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-tertiary disabled:opacity-50"
							onclick={() => loadPosts(false)}
							disabled={loadingMore}
						>
							{#if loadingMore}
								<span class="flex items-center gap-1.5">
									<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-text-muted border-t-transparent"></span>
									Loading...
								</span>
							{:else}
								Load More
							{/if}
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>
