<script lang="ts">
	import type { GalleryTag } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		tags: GalleryTag[];
		requireTags: boolean;
		guidelines: string | null;
		oncreated?: () => void;
		oncancel?: () => void;
	}

	let { channelId, tags, requireTags, guidelines, oncreated, oncancel }: Props = $props();

	let title = $state('');
	let description = $state('');
	let selectedTagIds = $state<Set<string>>(new Set());
	let pendingFiles = $state<File[]>([]);
	let creating = $state(false);
	let showGuidelines = $state(!!guidelines);
	let fileInput: HTMLInputElement;

	const ACCEPTED_TYPES = 'image/*,video/*';

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
		const newFiles = Array.from(target.files).filter(
			(f) => f.type.startsWith('image/') || f.type.startsWith('video/')
		);
		if (newFiles.length === 0) {
			addToast('Only image and video files are allowed', 'warning');
			return;
		}
		pendingFiles = [...pendingFiles, ...newFiles];
		target.value = '';
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		if (!e.dataTransfer?.files) return;
		const newFiles = Array.from(e.dataTransfer.files).filter(
			(f) => f.type.startsWith('image/') || f.type.startsWith('video/')
		);
		if (newFiles.length === 0) {
			addToast('Only image and video files are allowed', 'warning');
			return;
		}
		pendingFiles = [...pendingFiles, ...newFiles];
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
		if (pendingFiles.length === 0) {
			addToast('At least one image or video is required', 'warning');
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

			await api.createGalleryPost(channelId, {
				title: title.trim() || undefined,
				description: description.trim() || undefined,
				tag_ids: selectedTagIds.size > 0 ? Array.from(selectedTagIds) : undefined,
				attachment_ids: attachmentIds
			});
			title = '';
			description = '';
			selectedTagIds = new Set();
			pendingFiles = [];
			addToast('Gallery post created', 'success');
			oncreated?.();
		} catch (err: any) {
			addToast(err.message || 'Failed to create gallery post', 'error');
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

		<!-- File drop zone -->
		<div
			class="flex min-h-[120px] cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed border-bg-modifier bg-bg-tertiary p-4 transition-colors hover:border-brand-500/50"
			role="button"
			tabindex="0"
			ondrop={handleDrop}
			ondragover={(e) => e.preventDefault()}
			onclick={() => fileInput.click()}
			onkeydown={(e) => { if (e.key === 'Enter') fileInput.click(); }}
		>
			<input
				bind:this={fileInput}
				type="file"
				multiple
				accept={ACCEPTED_TYPES}
				class="hidden"
				onchange={handleFileSelect}
				disabled={creating}
			/>
			<svg class="mb-2 h-8 w-8 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
				<path d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
			</svg>
			<p class="text-sm font-medium text-text-secondary">Drop images or videos here</p>
			<p class="text-xs text-text-muted">or click to browse</p>
		</div>

		<!-- File previews -->
		{#if pendingFiles.length > 0}
			<div class="grid grid-cols-4 gap-2">
				{#each pendingFiles as file, i (file.name + i)}
					<div class="group relative aspect-square overflow-hidden rounded-md bg-bg-tertiary">
						{#if file.type.startsWith('image/')}
							<img
								src={URL.createObjectURL(file)}
								alt=""
								class="h-full w-full object-cover"
							/>
						{:else}
							<div class="flex h-full w-full flex-col items-center justify-center">
								<svg class="h-8 w-8 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
									<path d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
								</svg>
								<span class="mt-1 truncate px-1 text-[10px] text-text-muted">{file.name}</span>
							</div>
						{/if}
						<div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/60 to-transparent px-1.5 pb-1 pt-4 text-[10px] text-white">
							{formatFileSize(file.size)}
						</div>
						<button
							class="absolute right-1 top-1 rounded-full bg-black/60 p-0.5 text-white opacity-0 transition-opacity hover:bg-red-500 group-hover:opacity-100"
							onclick={(e) => { e.stopPropagation(); removeFile(i); }}
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

		<!-- Title (optional) -->
		<input
			bind:value={title}
			onkeydown={handleKeydown}
			type="text"
			placeholder="Title (optional)"
			class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
			maxlength="100"
			disabled={creating}
		/>

		<!-- Description (optional) -->
		<textarea
			bind:value={description}
			onkeydown={handleKeydown}
			placeholder="Description (optional)"
			class="min-h-[60px] w-full resize-y rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-brand-500"
			rows="2"
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
				disabled={creating || pendingFiles.length === 0}
			>
				{#if creating}
					<span class="flex items-center gap-1.5">
						<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
						Uploading...
					</span>
				{:else}
					Post to Gallery
				{/if}
			</button>
		</div>
	</div>
</div>
