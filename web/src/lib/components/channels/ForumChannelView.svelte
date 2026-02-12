<script lang="ts">
	import type { Channel } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		onopenthread?: (channel: Channel) => void;
	}

	let { channelId, onopenthread }: Props = $props();

	let threads = $state<Channel[]>([]);
	let loading = $state(true);
	let error = $state('');

	// New post form state
	let showNewPostForm = $state(false);
	let newPostTitle = $state('');
	let newPostContent = $state('');
	let creating = $state(false);

	// Load threads whenever channelId changes
	$effect(() => {
		const id = channelId;
		loading = true;
		error = '';
		threads = [];
		api.getThreads(id)
			.then((result) => {
				threads = result;
			})
			.catch((err) => {
				error = err.message || 'Failed to load forum posts';
			})
			.finally(() => {
				loading = false;
			});
	});

	async function createPost() {
		const title = newPostTitle.trim();
		const content = newPostContent.trim();
		if (!title) {
			addToast('Post title is required', 'warning');
			return;
		}
		if (!content) {
			addToast('Post content is required', 'warning');
			return;
		}

		creating = true;
		try {
			// For forum channels, create an initial message then open it as a thread.
			// Send the content as the first message, then create a thread on it.
			const message = await api.sendMessage(channelId, content);
			const thread = await api.createThread(channelId, message.id, title);
			threads = [thread, ...threads];
			newPostTitle = '';
			newPostContent = '';
			showNewPostForm = false;
			addToast('Post created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create post', 'error');
		} finally {
			creating = false;
		}
	}

	function openThread(thread: Channel) {
		onopenthread?.(thread);
	}

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

	function cancelNewPost() {
		showNewPostForm = false;
		newPostTitle = '';
		newPostContent = '';
	}

	function handleFormKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			cancelNewPost();
		}
	}
</script>

<div class="flex h-full flex-col bg-bg-primary">
	<!-- Header -->
	<div class="flex h-12 items-center justify-between border-b border-bg-floating px-4">
		<div class="flex items-center gap-2">
			<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
			</svg>
			<h2 class="text-sm font-semibold text-text-primary">Forum</h2>
		</div>
		<button
			class="rounded-md bg-brand-500 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600"
			onclick={() => (showNewPostForm = true)}
			disabled={showNewPostForm}
		>
			New Post
		</button>
	</div>

	<!-- New post form -->
	{#if showNewPostForm}
		<div class="border-b border-bg-floating bg-bg-secondary p-4">
			<div class="mx-auto max-w-2xl space-y-3">
				<input
					bind:value={newPostTitle}
					onkeydown={handleFormKeydown}
					type="text"
					placeholder="Post title"
					class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
					maxlength="100"
					disabled={creating}
				/>
				<textarea
					bind:value={newPostContent}
					onkeydown={handleFormKeydown}
					placeholder="Write the first message for your post..."
					class="min-h-[80px] w-full resize-y rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
					rows="3"
					disabled={creating}
				></textarea>
				<div class="flex items-center justify-end gap-2">
					<button
						class="rounded-md px-3 py-1.5 text-xs font-medium text-text-muted transition-colors hover:text-text-primary"
						onclick={cancelNewPost}
						disabled={creating}
					>
						Cancel
					</button>
					<button
						class="rounded-md bg-brand-500 px-4 py-1.5 text-xs font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
						onclick={createPost}
						disabled={creating}
					>
						{#if creating}
							<span class="flex items-center gap-1.5">
								<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
								Creating...
							</span>
						{:else}
							Create Post
						{/if}
					</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Thread list -->
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
			</div>
		{:else if threads.length === 0}
			<div class="flex flex-col items-center justify-center py-16">
				<svg class="mb-3 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1" viewBox="0 0 24 24">
					<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
				</svg>
				<p class="text-sm font-medium text-text-secondary">No posts yet</p>
				<p class="mt-1 text-xs text-text-muted">Be the first to start a discussion.</p>
			</div>
		{:else}
			<div class="mx-auto max-w-3xl space-y-1 p-4">
				{#each threads as thread (thread.id)}
					<button
						class="group flex w-full items-start gap-3 rounded-lg border border-transparent p-3 text-left transition-colors hover:border-bg-modifier hover:bg-bg-secondary"
						onclick={() => openThread(thread)}
					>
						<div class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-bg-modifier text-text-muted">
							<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
							</svg>
						</div>
						<div class="min-w-0 flex-1">
							<div class="flex items-baseline gap-2">
								<h3 class="truncate text-sm font-semibold text-text-primary group-hover:text-brand-400">
									{thread.name ?? 'Untitled Post'}
								</h3>
							</div>
							<div class="mt-1 flex items-center gap-3 text-xs text-text-muted">
								{#if thread.owner_id}
									<span class="truncate">{thread.owner_id.slice(0, 8)}</span>
								{/if}
								<span>{formatDate(thread.created_at)}</span>
								{#if thread.last_message_id}
									<span class="flex items-center gap-1">
										<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M3 10h11M9 21V3m0 0L3 9m6-6l6 6" />
										</svg>
										has replies
									</span>
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
				{/each}
			</div>
		{/if}
	</div>
</div>
