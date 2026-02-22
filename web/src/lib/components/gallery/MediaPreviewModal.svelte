<script lang="ts">
	import type { Attachment } from '$lib/types';
	import { fileUrl as buildFileUrl } from '$lib/utils/avatar';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import Modal from '$lib/components/common/Modal.svelte';

	interface Props {
		attachment: Attachment | null;
		open: boolean;
		onclose: () => void;
		ondelete?: () => void;
		canManage?: boolean;
	}

	let { attachment, open = $bindable(), onclose, ondelete, canManage = false }: Props = $props();

	let editingMeta = $state(false);
	let editNsfw = $state(false);
	let editAltText = $state('');
	let editDescription = $state('');
	let saving = $state(false);

	$effect(() => {
		if (attachment) {
			editNsfw = attachment.nsfw;
			editAltText = attachment.alt_text ?? '';
			editDescription = attachment.description ?? '';
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
		} catch (err: any) {
			addToast(err.message || 'Failed to update', 'error');
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
		} catch (err: any) {
			addToast(err.message || 'Failed to delete', 'error');
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
	}
</script>

<Modal bind:open title={attachment?.filename ?? 'Media Preview'} {onclose}>
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
