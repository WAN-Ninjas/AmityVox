<script lang="ts">
	import type { Attachment, MediaTag } from '$lib/types';
	import { fileUrl as buildFileUrl } from '$lib/utils/avatar';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import Modal from '$lib/components/common/Modal.svelte';

	interface Props {
		attachment: Attachment | null;
		open: boolean;
		onclose: () => void;
		ondelete?: () => void;
		onupdate?: (attachment: Attachment) => void;
		canManage?: boolean;
		guildId?: string;
	}

	let { attachment, open = $bindable(), onclose, ondelete, onupdate, canManage = false, guildId }: Props = $props();

	let editingMeta = $state(false);
	let editNsfw = $state(false);
	let editAltText = $state('');
	let editDescription = $state('');
	let saving = $state(false);
	let mediaTags = $state<MediaTag[]>([]);
	let appliedTagIds = $state<Set<string>>(new Set());
	let tagBusyId = $state<string | null>(null);

	$effect(() => {
		if (attachment) {
			editNsfw = attachment.nsfw;
			editAltText = attachment.alt_text ?? '';
			editDescription = attachment.description ?? '';
			appliedTagIds = new Set(attachment.tags?.map((tag) => tag.id) ?? []);
		}
	});

	$effect(() => {
		if (open && guildId && canManage) {
			api.getMediaTags(guildId).then((tags) => (mediaTags = tags)).catch(() => (mediaTags = []));
		}
	});

	const isVideo = $derived(attachment?.content_type.startsWith('video/'));
	const isImage = $derived(attachment?.content_type.startsWith('image/'));
	const fileUrl = $derived(attachment ? buildFileUrl(attachment.id, attachment.instance_id || undefined) : '');

	async function saveMeta() {
		if (!attachment || saving) return;
		saving = true;
		try {
			await api.updateAttachment(attachment.id, {
				nsfw: editNsfw,
				alt_text: editAltText.trim() || undefined,
				description: editDescription.trim() || undefined
			});
			addToast('Metadata updated', 'success');
			editingMeta = false;
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update'), 'error');
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!attachment) return;
		try {
			await api.deleteAttachment(attachment.id);
			addToast('File deleted', 'success');
			ondelete?.();
			onclose();
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete'), 'error');
		}
	}

	async function toggleTag(tag: MediaTag) {
		if (!attachment || tagBusyId) return;
		tagBusyId = tag.id;
		const wasApplied = appliedTagIds.has(tag.id);
		try {
			if (wasApplied) {
				await api.untagAttachment(attachment.id, tag.id);
				appliedTagIds = new Set([...appliedTagIds].filter((id) => id !== tag.id));
			} else {
				await api.tagAttachment(attachment.id, tag.id);
				appliedTagIds = new Set([...appliedTagIds, tag.id]);
			}
			const nextTags = wasApplied
				? (attachment.tags ?? []).filter((item) => item.id !== tag.id)
				: [...(attachment.tags ?? []), tag];
			onupdate?.({ ...attachment, tags: nextTags });
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to update tags'), 'error');
		} finally {
			tagBusyId = null;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
	}
</script>

<Modal {open} title={attachment?.filename ?? 'Media Preview'} {onclose}>
	{#if attachment}
		<div class="space-y-4">
			<!-- Media preview -->
			<div class="flex justify-center rounded-lg bg-bg-primary p-2">
				{#if isImage}
					<img
						class="max-h-96 rounded object-contain"
						src={fileUrl}
						alt={attachment.alt_text ?? attachment.filename}
					/>
				{:else if isVideo}
					<!-- svelte-ignore a11y_media_has_caption -->
					<video
						class="max-h-96 rounded"
						src={fileUrl}
						controls
					>
					</video>
				{/if}
			</div>

			<!-- Metadata -->
			<div class="grid grid-cols-2 gap-2 text-xs">
				<div>
					<span class="text-text-muted">Filename:</span>
					<span class="ml-1 text-text-secondary">{attachment.filename}</span>
				</div>
				<div>
					<span class="text-text-muted">Size:</span>
					<span class="ml-1 text-text-secondary">{formatBytes(attachment.size_bytes)}</span>
				</div>
				<div>
					<span class="text-text-muted">Type:</span>
					<span class="ml-1 text-text-secondary">{attachment.content_type}</span>
				</div>
				{#if attachment.width && attachment.height}
					<div>
						<span class="text-text-muted">Dimensions:</span>
						<span class="ml-1 text-text-secondary">{attachment.width}x{attachment.height}</span>
					</div>
				{/if}
				{#if attachment.duration_seconds}
					<div>
						<span class="text-text-muted">Duration:</span>
						<span class="ml-1 text-text-secondary">{Math.floor(attachment.duration_seconds / 60)}:{String(Math.floor(attachment.duration_seconds % 60)).padStart(2, '0')}</span>
					</div>
				{/if}
				<div>
					<span class="text-text-muted">Uploaded:</span>
					<span class="ml-1 text-text-secondary">{new Date(attachment.created_at).toLocaleDateString()}</span>
				</div>
			</div>

			{#if attachment.alt_text}
				<div class="text-xs">
					<span class="text-text-muted">Alt text:</span>
					<span class="ml-1 text-text-secondary">{attachment.alt_text}</span>
				</div>
			{/if}

			{#if attachment.description}
				<div class="text-xs">
					<span class="text-text-muted">Description:</span>
					<span class="ml-1 text-text-secondary">{attachment.description}</span>
				</div>
			{/if}

			{#if attachment.nsfw}
				<span class="inline-block rounded bg-red-500/20 px-2 py-0.5 text-xs font-medium text-red-400">NSFW</span>
			{/if}

			{#if attachment.tags && attachment.tags.length > 0}
				<div class="flex flex-wrap gap-1">
					{#each attachment.tags as tag (tag.id)}
						<span class="rounded-full bg-bg-modifier px-2 py-0.5 text-xs text-text-secondary">{tag.name}</span>
					{/each}
				</div>
			{/if}

			<!-- Edit metadata -->
			{#if canManage}
				{#if editingMeta}
					<div class="space-y-2 rounded-md border border-bg-modifier p-3">
						<label class="flex items-center gap-2 text-xs text-text-secondary">
							<input type="checkbox" bind:checked={editNsfw} class="rounded" />
							NSFW
						</label>
						<input
							type="text"
							class="input w-full text-xs"
							placeholder="Alt text"
							bind:value={editAltText}
							maxlength="500"
						/>
						<textarea
							class="input w-full resize-none text-xs"
							placeholder="Description"
							bind:value={editDescription}
							rows="2"
							maxlength="2000"
						></textarea>
						<div class="flex gap-2">
							<button class="btn-primary text-xs" onclick={saveMeta} disabled={saving}>
								{saving ? 'Saving...' : 'Save'}
							</button>
							<button class="btn-secondary text-xs" onclick={() => (editingMeta = false)}>Cancel</button>
						</div>
					</div>
				{:else}
					<div class="flex gap-2">
						<button class="btn-secondary text-xs" onclick={() => (editingMeta = true)}>Edit Metadata</button>
						<button class="rounded px-3 py-1.5 text-xs font-medium text-red-400 transition-colors hover:bg-red-500/10" onclick={handleDelete}>
							Delete
						</button>
					</div>
				{/if}
				{#if guildId}
					<div class="rounded-md border border-bg-modifier p-3">
						<h4 class="mb-2 text-xs font-medium text-text-muted">Tags</h4>
						{#if mediaTags.length === 0}
							<p class="text-xs text-text-muted">No media tags configured.</p>
						{:else}
							<div class="flex flex-wrap gap-1.5">
								{#each mediaTags as tag (tag.id)}
									<button
										class="rounded-full border px-2 py-0.5 text-xs transition-colors {appliedTagIds.has(tag.id) ? 'border-brand-500 bg-brand-500/15 text-brand-300' : 'border-bg-modifier text-text-muted hover:text-text-primary'}"
										onclick={() => toggleTag(tag)}
										disabled={tagBusyId === tag.id}
									>
										{tagBusyId === tag.id ? '...' : tag.name}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			{/if}

			<!-- Download -->
			<div class="flex justify-end">
				<a
					href={fileUrl}
					download={attachment.filename}
					class="btn-secondary text-xs"
				>
					Download
				</a>
			</div>
		</div>
	{/if}
</Modal>
