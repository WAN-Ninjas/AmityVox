<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { fileUrl } from '$lib/utils/avatar';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { Sticker, StickerPack } from '$lib/types';

	interface Props {
		guildId: string;
		instanceId?: string | null;
	}

	let { guildId, instanceId = null }: Props = $props();

	let stickerPacks = $state<StickerPack[]>([]);
	let stickersByPack = $state<Map<string, Sticker[]>>(new Map());
	let expandedPackId = $state<string | null>(null);
	let newPackName = $state('');
	let newPackDescription = $state('');
	let newStickerName = $state('');
	let newStickerFile = $state<File | null>(null);
	let loadedGuildId = $state<string | null>(null);
	let loadOp = $state(createAsyncOp());
	let loadPackOp = $state(createAsyncOp());
	let createOp = $state(createAsyncOp());
	let uploadOp = $state(createAsyncOp());

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadStickerPacks();
		}
	});

	async function loadStickerPacks() {
		const result = await loadOp.run(() => api.getGuildStickerPacks(guildId));
		if (result) {
			stickerPacks = result;
			loadedGuildId = guildId;
		} else {
			stickerPacks = [];
		}
	}

	async function loadPackStickersData(packId: string) {
		const stickers = await loadPackOp.run(() => api.getPackStickers(guildId, packId));
		if (stickers) {
			stickersByPack = new Map([...stickersByPack, [packId, stickers]]);
		} else {
			stickersByPack = new Map([...stickersByPack, [packId, []]]);
		}
	}

	async function handleCreateStickerPack() {
		if (!newPackName.trim()) return;
		await createOp.run(async () => {
			const pack = await api.createGuildStickerPack(guildId, newPackName.trim(), newPackDescription.trim() || undefined);
			stickerPacks = [...stickerPacks, pack];
			newPackName = '';
			newPackDescription = '';
			addToast('Sticker pack created', 'success');
		}, msg => addToast(msg, 'error'), 'Failed to create sticker pack');
	}

	async function handleDeleteStickerPack(packId: string) {
		if (!(await confirmAction({ title: 'Delete Sticker Pack', message: 'Delete this sticker pack and all its stickers?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteGuildStickerPack(guildId, packId);
			stickerPacks = stickerPacks.filter((pack) => pack.id !== packId);
			stickersByPack = new Map([...stickersByPack].filter(([key]) => key !== packId));
			if (expandedPackId === packId) expandedPackId = null;
			addToast('Sticker pack deleted', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete sticker pack'), 'error');
		}
	}

	function toggleExpandPack(packId: string) {
		if (expandedPackId === packId) {
			expandedPackId = null;
		} else {
			expandedPackId = packId;
			if (!stickersByPack.has(packId)) {
				loadPackStickersData(packId);
			}
		}
	}

	function handleStickerFileSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file?.type.startsWith('image/')) return;
		newStickerFile = file;
		if (!newStickerName) newStickerName = file.name.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9_]/g, '_').slice(0, 32);
	}

	async function handleUploadSticker(packId: string) {
		if (!newStickerFile || !newStickerName.trim()) return;
		const stickerFile = newStickerFile;
		await uploadOp.run(async () => {
			const uploaded = await api.uploadFile(stickerFile);
			let format = 'png';
			if (stickerFile.type === 'image/gif') format = 'gif';
			else if (stickerFile.type === 'image/apng') format = 'apng';
			else if (stickerFile.type === 'image/png') format = 'png';
			const sticker = await api.addStickerToGuildPack(guildId, packId, {
				name: newStickerName.trim(),
				file_id: uploaded.id,
				format
			});
			const existing = stickersByPack.get(packId) ?? [];
			stickersByPack = new Map([...stickersByPack, [packId, [...existing, sticker]]]);
			stickerPacks = stickerPacks.map((pack) => pack.id === packId ? { ...pack, sticker_count: (pack.sticker_count ?? 0) + 1 } : pack);
			newStickerName = '';
			newStickerFile = null;
			addToast('Sticker uploaded', 'success');
		}, msg => addToast(msg, 'error'), 'Failed to upload sticker');
	}

	async function handleDeleteSticker(packId: string, stickerId: string) {
		if (!(await confirmAction({ title: 'Delete Sticker', message: 'Delete this sticker?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteStickerFromGuildPack(guildId, packId, stickerId);
			const existing = stickersByPack.get(packId) ?? [];
			stickersByPack = new Map([...stickersByPack, [packId, existing.filter((sticker) => sticker.id !== stickerId)]]);
			stickerPacks = stickerPacks.map((pack) => pack.id === packId ? { ...pack, sticker_count: Math.max(0, (pack.sticker_count ?? 1) - 1) } : pack);
			addToast('Sticker deleted', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete sticker'), 'error');
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Sticker Packs</h1>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Sticker Pack</h3>
	<div class="mb-3">
		<label for="newPackName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Pack Name</label>
		<input id="newPackName" type="text" class="input w-full" bind:value={newPackName} placeholder="My Stickers" maxlength="50" />
	</div>
	<div class="mb-3">
		<label for="newPackDescription" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Description (optional)</label>
		<input id="newPackDescription" type="text" class="input w-full" bind:value={newPackDescription} placeholder="A collection of custom stickers" maxlength="200" />
	</div>
	<button class="btn-primary" onclick={handleCreateStickerPack} disabled={createOp.loading || !newPackName.trim()}>
		{createOp.loading ? 'Creating...' : 'Create Pack'}
	</button>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading sticker packs...</p>
{:else if stickerPacks.length === 0}
	<p class="text-sm text-text-muted">No sticker packs yet. Create one above!</p>
{:else}
	<div class="space-y-3">
		{#each stickerPacks as pack (pack.id)}
			<div class="rounded-lg bg-bg-secondary">
				<div class="flex items-center gap-3 p-4">
					<button
						class="flex flex-1 items-center gap-3 text-left"
						onclick={() => toggleExpandPack(pack.id)}
					>
						<svg
							class="h-4 w-4 shrink-0 text-text-muted transition-transform {expandedPackId === pack.id ? 'rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M9 5l7 7-7 7" />
						</svg>
						<div>
							<span class="text-sm font-medium text-text-primary">{pack.name}</span>
							{#if pack.description}
								<span class="ml-2 text-xs text-text-muted">{pack.description}</span>
							{/if}
						</div>
						<span class="ml-auto text-xs text-text-muted">{pack.sticker_count ?? 0} sticker{(pack.sticker_count ?? 0) !== 1 ? 's' : ''}</span>
					</button>
					<button
						class="text-xs text-red-400 hover:text-red-300"
						onclick={() => handleDeleteStickerPack(pack.id)}
					>
						Delete
					</button>
				</div>

				{#if expandedPackId === pack.id}
					<div class="border-t border-bg-modifier p-4">
						<div class="mb-4 rounded bg-bg-primary p-3">
							<h4 class="mb-2 text-xs font-semibold text-text-muted">Add Sticker</h4>
							<div class="mb-2 flex gap-3">
								<div>
									<label for={`stickerImage-${pack.id}`} class="mb-1 block text-2xs font-bold uppercase tracking-wide text-text-muted">Image</label>
									<input id={`stickerImage-${pack.id}`} type="file" accept="image/png,image/gif,image/apng,image/webp" onchange={handleStickerFileSelect} class="text-xs text-text-muted" />
								</div>
								<div class="flex-1">
									<label for={`stickerName-${pack.id}`} class="mb-1 block text-2xs font-bold uppercase tracking-wide text-text-muted">Name</label>
									<input id={`stickerName-${pack.id}`} type="text" class="input w-full text-sm" bind:value={newStickerName} placeholder="sticker_name" maxlength="32" />
								</div>
							</div>
							<button
								class="btn-primary text-xs"
								onclick={() => handleUploadSticker(pack.id)}
								disabled={uploadOp.loading || !newStickerFile || !newStickerName.trim()}
							>
								{uploadOp.loading ? 'Uploading...' : 'Add Sticker'}
							</button>
						</div>

						{#if loadPackOp.loading}
							<p class="text-xs text-text-muted">Loading stickers...</p>
						{:else}
							{@const packStickers = stickersByPack.get(pack.id) ?? []}
							{#if packStickers.length === 0}
								<p class="text-xs text-text-muted">No stickers in this pack yet.</p>
							{:else}
								<div class="grid grid-cols-4 gap-3">
									{#each packStickers as sticker (sticker.id)}
										<div class="flex flex-col items-center gap-1 rounded-lg bg-bg-primary p-3">
											<img
												src={fileUrl(sticker.file_id, instanceId || undefined)}
												alt={sticker.name}
												class="h-12 w-12 object-contain"
												loading="lazy"
											/>
											<span class="max-w-full truncate text-xs text-text-muted">{sticker.name}</span>
											<button
												class="text-2xs text-red-400 hover:text-red-300"
												onclick={() => handleDeleteSticker(pack.id, sticker.id)}
											>
												Delete
											</button>
										</div>
									{/each}
								</div>
							{/if}
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
