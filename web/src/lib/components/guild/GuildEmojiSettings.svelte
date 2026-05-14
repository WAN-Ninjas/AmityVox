<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { fileUrl } from '$lib/utils/avatar';
	import type { CustomEmoji } from '$lib/types';

	interface Props {
		guildId: string;
		instanceId?: string | null;
	}

	let { guildId, instanceId = null }: Props = $props();

	let emoji = $state<CustomEmoji[]>([]);
	let loadingEmoji = $state(false);
	let emojiFile = $state<File | null>(null);
	let emojiName = $state('');
	let uploadingEmoji = $state(false);
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingEmoji) {
			loadEmoji();
		}
	});

	async function loadEmoji() {
		loadingEmoji = true;
		try {
			emoji = await api.getGuildEmoji(guildId);
			loadedGuildId = guildId;
		} catch {
			emoji = [];
		} finally {
			loadingEmoji = false;
		}
	}

	async function handleDeleteEmoji(emojiId: string) {
		if (!(await confirmAction({ title: 'Delete Emoji', message: 'Delete this emoji?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteGuildEmoji(guildId, emojiId);
			emoji = emoji.filter((value) => value.id !== emojiId);
			addToast('Emoji deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete emoji', 'error');
		}
	}

	function handleEmojiFileSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file?.type.startsWith('image/')) return;
		emojiFile = file;
		if (!emojiName) emojiName = file.name.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9_]/g, '_').slice(0, 32);
	}

	async function handleUploadEmoji() {
		if (!emojiFile || !emojiName.trim()) return;
		uploadingEmoji = true;
		try {
			const newEmoji = await api.uploadEmoji(guildId, emojiName.trim(), emojiFile);
			emoji = [...emoji, newEmoji];
			emojiFile = null;
			emojiName = '';
			addToast('Emoji uploaded', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to upload emoji', 'error');
		} finally {
			uploadingEmoji = false;
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Custom Emoji</h1>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Upload Emoji</h3>
	<div class="mb-3 flex gap-3">
		<div>
			<label for="emojiImage" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Image</label>
			<input id="emojiImage" type="file" accept="image/png,image/gif,image/jpeg,image/webp" onchange={handleEmojiFileSelect} class="text-sm text-text-muted" />
		</div>
		<div class="flex-1">
			<label for="emojiName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Name</label>
			<input id="emojiName" type="text" class="input w-full" bind:value={emojiName} placeholder="emoji_name" maxlength="32" pattern="[a-zA-Z0-9_]+" />
		</div>
	</div>
	<button class="btn-primary" onclick={handleUploadEmoji} disabled={uploadingEmoji || !emojiFile || !emojiName.trim()}>
		{uploadingEmoji ? 'Uploading...' : 'Upload Emoji'}
	</button>
</div>

{#if loadingEmoji}
	<p class="text-sm text-text-muted">Loading emoji...</p>
{:else if emoji.length === 0}
	<p class="text-sm text-text-muted">No custom emoji yet. Upload one above!</p>
{:else}
	<div class="grid grid-cols-4 gap-3">
		{#each emoji as entry (entry.id)}
			<div class="flex flex-col items-center gap-1 rounded-lg bg-bg-secondary p-3">
				<img src={fileUrl(entry.id, instanceId || undefined)} alt={entry.name} class="h-8 w-8" />
				<span class="text-xs text-text-muted">:{entry.name}:</span>
				<button
					class="text-2xs text-red-400 hover:text-red-300"
					onclick={() => handleDeleteEmoji(entry.id)}
				>
					Delete
				</button>
			</div>
		{/each}
	</div>
{/if}
