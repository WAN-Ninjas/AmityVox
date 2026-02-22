<script lang="ts">
	import type { GalleryPost } from '$lib/types';
	import { avatarUrl, fileUrl as buildFileUrl } from '$lib/utils/avatar';

	interface Props {
		post: GalleryPost;
		onclick?: () => void;
	}

	let { post, onclick }: Props = $props();

	let isVideo = $derived(post.thumbnail?.content_type?.startsWith('video/') ?? false);
	let thumbnailUrl = $derived(post.thumbnail ? buildFileUrl(post.thumbnail.id, post.thumbnail.instance_id || undefined) : null);

	function formatDate(iso: string): string {
		const date = new Date(iso);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffHours = diffMs / (1000 * 60 * 60);

		if (diffHours < 1) {
			const mins = Math.floor(diffMs / (1000 * 60));
			return mins <= 1 ? 'just now' : `${mins}m ago`;
		}
		if (diffHours < 24) {
			return `${Math.floor(diffHours)}h ago`;
		}
		if (diffHours < 24 * 7) {
			return `${Math.floor(diffHours / 24)}d ago`;
		}
		return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	}
</script>

<button
	class="group relative flex flex-col overflow-hidden rounded-lg border border-bg-modifier bg-bg-secondary transition-all hover:border-brand-500/40 hover:shadow-lg"
	{onclick}
>
	<!-- Thumbnail -->
	<div class="relative aspect-square w-full overflow-hidden bg-bg-tertiary">
		{#if thumbnailUrl}
			{#if post.thumbnail?.nsfw}
				<div class="flex h-full w-full items-center justify-center bg-bg-tertiary">
					<div class="text-center">
						<svg class="mx-auto h-8 w-8 text-red-400" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
							<path d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126z" />
							<path d="M12 15.75h.007v.008H12v-.008z" />
						</svg>
						<span class="mt-1 block text-xs text-red-400">NSFW</span>
					</div>
				</div>
			{:else}
				<img
					src={thumbnailUrl}
					alt={post.name ?? 'Gallery post'}
					class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
					loading="lazy"
				/>
			{/if}
		{:else}
			<div class="flex h-full w-full items-center justify-center">
				<svg class="h-12 w-12 text-text-muted opacity-30" fill="none" stroke="currentColor" stroke-width="1" viewBox="0 0 24 24">
					<rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
					<circle cx="8.5" cy="8.5" r="1.5" />
					<polyline points="21 15 16 10 5 21" />
				</svg>
			</div>
		{/if}

		<!-- Video play icon overlay -->
		{#if isVideo && !post.thumbnail?.nsfw}
			<div class="absolute inset-0 flex items-center justify-center">
				<div class="flex h-10 w-10 items-center justify-center rounded-full bg-black/60 text-white backdrop-blur-sm">
					<svg class="ml-0.5 h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M8 5v14l11-7z" />
					</svg>
				</div>
			</div>
		{/if}

		<!-- Pin badge -->
		{#if post.pinned}
			<div class="absolute left-2 top-2 rounded-full bg-brand-500/90 px-1.5 py-0.5 text-[10px] font-bold text-white backdrop-blur-sm">
				<svg class="inline-block h-3 w-3" fill="currentColor" viewBox="0 0 24 24">
					<path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
				</svg>
			</div>
		{/if}

		<!-- Lock badge -->
		{#if post.locked}
			<div class="absolute right-2 top-2 rounded-full bg-bg-floating/80 p-1 backdrop-blur-sm">
				<svg class="h-3 w-3 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
					<path d="M7 11V7a5 5 0 0110 0v4" />
				</svg>
			</div>
		{/if}

		<!-- Reply count badge -->
		{#if post.reply_count > 0}
			<div class="absolute bottom-2 right-2 flex items-center gap-1 rounded-full bg-bg-floating/80 px-1.5 py-0.5 text-[10px] font-medium text-text-secondary backdrop-blur-sm">
				<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
				</svg>
				{post.reply_count}
			</div>
		{/if}
	</div>

	<!-- Info below thumbnail -->
	<div class="flex flex-col gap-1 p-2.5">
		<!-- Title -->
		<h3 class="line-clamp-1 text-sm font-semibold text-text-primary group-hover:text-brand-400">
			{post.name ?? 'Untitled'}
		</h3>

		<!-- Tags -->
		{#if post.tags && post.tags.length > 0}
			<div class="flex flex-wrap gap-1">
				{#each post.tags.slice(0, 3) as tag (tag.id)}
					<span
						class="inline-flex items-center gap-0.5 rounded-full px-1.5 py-0 text-[9px] font-medium"
						style="background-color: {tag.color ? tag.color + '20' : 'var(--bg-modifier)'}; color: {tag.color || 'var(--text-secondary)'}"
					>
						{#if tag.emoji}<span>{tag.emoji}</span>{/if}
						{tag.name}
					</span>
				{/each}
				{#if post.tags.length > 3}
					<span class="text-[9px] text-text-muted">+{post.tags.length - 3}</span>
				{/if}
			</div>
		{/if}

		<!-- Author & date -->
		<div class="flex items-center gap-2 text-[11px] text-text-muted">
			{#if post.author?.avatar_id}
				<img
					src={avatarUrl(post.author.avatar_id, post.author.instance_id || undefined)}
					alt=""
					class="h-4 w-4 rounded-full object-cover"
				/>
			{/if}
			<span class="truncate font-medium">
				{post.author?.display_name || post.author?.username || 'Unknown'}
			</span>
			<span class="shrink-0">{formatDate(post.created_at)}</span>
		</div>
	</div>
</button>
