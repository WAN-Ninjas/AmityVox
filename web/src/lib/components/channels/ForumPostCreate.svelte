<script lang="ts">
	import type { ForumTag } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		tags: ForumTag[];
		requireTags: boolean;
		guidelines: string | null;
		oncreated?: () => void;
		oncancel?: () => void;
	}

	let { channelId, tags, requireTags, guidelines, oncreated, oncancel }: Props = $props();

	let title = $state('');
	let content = $state('');
	let selectedTagIds = $state<Set<string>>(new Set());
	let pendingFiles = $state<File[]>([]);
	let creating = $state(false);
	let showGuidelines = $state(!!guidelines);
	let fileInput: HTMLInputElement;

	function toggleTag(tagId: string) {
		selectedTagIds = new Set(selectedTagIds);
		if (selectedTagIds.has(tagId)) {
			selectedTagIds.delete(tagId);
		} else {
			selectedTagIds.add(tagId);
		}
	}

	function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		if (!target.files) return;
		const newFiles = Array.from(target.files);
		pendingFiles = [...pendingFiles, ...newFiles];
		target.value = '';
	}

	function removeFile(index: number) {
		pendingFiles = pendingFiles.filter((_, i) => i !== index);
	}

	function formatFileSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	async function createPost() {
		const trimTitle = title.trim();
		const trimContent = content.trim();

		if (!trimTitle) {
			addToast('Post title is required', 'warning');
			return;
		}
		if (!trimContent) {
			addToast('Post content is required', 'warning');
			return;
		}
		if (requireTags && selectedTagIds.size === 0) {
			addToast('At least one tag is required', 'warning');
			return;
		}

		creating = true;
		try {
			// Upload files first.
			const attachmentIds: string[] = [];
			for (const file of pendingFiles) {
				const uploaded = await api.uploadFile(file);
				attachmentIds.push(uploaded.id);
			}

			await api.createForumPost(channelId, {
				title: trimTitle,
				content: trimContent,
				tag_ids: selectedTagIds.size > 0 ? Array.from(selectedTagIds) : undefined,
				attachment_ids: attachmentIds.length > 0 ? attachmentIds : undefined
			});
			title = '';
			content = '';
			selectedTagIds = new Set();
			pendingFiles = [];
			addToast('Post created', 'success');
			oncreated?.();
		} catch (err: any) {
			addToast(err.message || 'Failed to create post', 'error');
		} finally {
			creating = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			oncancel?.();
		}
	}
</script>

<div class="border-b border-bg-floating bg-bg-secondary p-4">
	<div class="mx-auto max-w-2xl space-y-3">
		<!-- Guidelines -->
		{#if guidelines}
			<div class="rounded-md border border-bg-modifier bg-bg-tertiary p-3">
				<button
					class="flex w-full items-center justify-between text-left text-xs font-medium text-text-secondary"
					onclick={() => (showGuidelines = !showGuidelines)}
				>
					<span>Post Guidelines</span>
					<svg
						class="h-4 w-4 transition-transform {showGuidelines ? 'rotate-180' : ''}"
						fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
					>
						<path d="M19 9l-7 7-7-7" />
					</svg>
				</button>
				{#if showGuidelines}
					<p class="mt-2 text-xs text-text-muted">{guidelines}</p>
				{/if}
			</div>
		{/if}

		<!-- Title -->
		<input
			bind:value={title}
			onkeydown={handleKeydown}
			type="text"
			placeholder="Post title"
			class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
			maxlength="100"
			disabled={creating}
		/>

		<!-- Content -->
		<textarea
			bind:value={content}
			onkeydown={handleKeydown}
			placeholder="Write the first message for your post..."
			class="min-h-[80px] w-full resize-y rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
			rows="3"
			disabled={creating}
		></textarea>

		<!-- Tag picker -->
		{#if tags.length > 0}
			<div>
				<label class="mb-1.5 block text-xs font-medium text-text-secondary">
					Tags {requireTags ? '(required)' : '(optional)'}
				</label>
				<div class="flex flex-wrap gap-1.5">
					{#each tags as tag (tag.id)}
						<button
							class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors {selectedTagIds.has(tag.id) ? 'border-brand-500 bg-brand-500/15 text-brand-400' : 'border-bg-modifier bg-bg-tertiary text-text-secondary hover:border-text-muted'}"
							onclick={() => toggleTag(tag.id)}
							disabled={creating}
						>
							{#if tag.emoji}<span>{tag.emoji}</span>{/if}
							{tag.name}
							{#if selectedTagIds.has(tag.id)}
								<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M5 13l4 4L19 7" />
								</svg>
							{/if}
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- File attachments -->
		<div>
			<input
				bind:this={fileInput}
				type="file"
				multiple
				class="hidden"
				onchange={handleFileSelect}
				disabled={creating}
			/>
			<button
				class="inline-flex items-center gap-1.5 rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:border-text-muted hover:text-text-primary"
				onclick={() => fileInput.click()}
				disabled={creating}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48" />
				</svg>
				Attach Files
			</button>

			{#if pendingFiles.length > 0}
				<div class="mt-2 space-y-1">
					{#each pendingFiles as file, i (file.name + i)}
						<div class="flex items-center gap-2 rounded-md bg-bg-tertiary px-2.5 py-1.5 text-xs">
							{#if file.type.startsWith('image/')}
								<img
									src={URL.createObjectURL(file)}
									alt=""
									class="h-8 w-8 rounded object-cover"
								/>
							{:else}
								<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
									<polyline points="14 2 14 8 20 8" />
								</svg>
							{/if}
							<span class="min-w-0 flex-1 truncate text-text-primary">{file.name}</span>
							<span class="shrink-0 text-text-muted">{formatFileSize(file.size)}</span>
							<button
								class="shrink-0 text-text-muted hover:text-red-400"
								onclick={() => removeFile(i)}
								disabled={creating}
								title="Remove"
							>
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Actions -->
		<div class="flex items-center justify-end gap-2">
			<button
				class="rounded-md px-3 py-1.5 text-xs font-medium text-text-muted transition-colors hover:text-text-primary"
				onclick={() => oncancel?.()}
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
