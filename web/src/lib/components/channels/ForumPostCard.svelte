<script lang="ts">
	import type { ForumPost } from '$lib/types';
	import { avatarUrl } from '$lib/utils/avatar';

	interface Props {
		post: ForumPost;
		onclick?: () => void;
	}

	let { post, onclick }: Props = $props();

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
	class="group flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-bg-secondary {post.pinned ? 'border-brand-500/30 bg-brand-500/5' : 'border-transparent hover:border-bg-modifier'}"
	{onclick}
>
	<!-- Author avatar -->
	<div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-bg-modifier">
		{#if post.author?.avatar_id}
			<img
				src={avatarUrl(post.author.avatar_id, post.author.instance_id || undefined)}
				alt=""
				class="h-full w-full object-cover"
			/>
		{:else}
			<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
			</svg>
		{/if}
	</div>

	<div class="min-w-0 flex-1">
		<!-- Title row -->
		<div class="flex items-center gap-2">
			{#if post.pinned}
				<svg class="h-3.5 w-3.5 shrink-0 text-brand-400" fill="currentColor" viewBox="0 0 24 24">
					<path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z" />
				</svg>
			{/if}
			<h3 class="truncate text-sm font-semibold text-text-primary group-hover:text-brand-400">
				{post.name ?? 'Untitled Post'}
			</h3>
			{#if post.locked}
				<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
					<path d="M7 11V7a5 5 0 0110 0v4" />
				</svg>
			{/if}
		</div>

		<!-- Content preview -->
		{#if post.content_preview}
			<p class="mt-0.5 line-clamp-2 text-xs text-text-secondary">{post.content_preview}</p>
		{/if}

		<!-- Tags -->
		{#if post.tags && post.tags.length > 0}
			<div class="mt-1.5 flex flex-wrap gap-1">
				{#each post.tags as tag (tag.id)}
					<span
						class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium"
						style="background-color: {tag.color ? tag.color + '20' : 'var(--bg-modifier)'}; color: {tag.color || 'var(--text-secondary)'}"
					>
						{#if tag.emoji}<span>{tag.emoji}</span>{/if}
						{tag.name}
					</span>
				{/each}
			</div>
		{/if}

		<!-- Meta row -->
		<div class="mt-1.5 flex items-center gap-3 text-xs text-text-muted">
			<span class="truncate font-medium">
				{post.author?.display_name || post.author?.username || 'Unknown'}
			</span>
			<span>{formatDate(post.created_at)}</span>
			<span class="flex items-center gap-1">
				<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
				</svg>
				{post.reply_count}
			</span>
			{#if post.last_activity_at}
				<span class="text-text-muted">Active {formatDate(post.last_activity_at)}</span>
			{/if}
		</div>
	</div>

	<svg
		class="mt-1 h-4 w-4 shrink-0 text-text-muted opacity-0 transition-opacity group-hover:opacity-100"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		viewBox="0 0 24 24"
	>
		<path d="M9 5l7 7-7 7" />
	</svg>
</button>
