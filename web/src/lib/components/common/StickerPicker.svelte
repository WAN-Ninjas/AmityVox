<script lang="ts">
	import type { StickerPack, Sticker } from '$lib/types';
	import { api } from '$lib/api/client';
	import { currentGuildId } from '$lib/stores/guilds';

	interface Props {
		onselect: (sticker: Sticker) => void;
		onclose: () => void;
	}

	let { onselect, onclose }: Props = $props();

	let search = $state('');
	let guildPacks = $state<StickerPack[]>([]);
	let userPacks = $state<StickerPack[]>([]);
	let stickersByPack = $state<Map<string, Sticker[]>>(new Map());
	let activePackId = $state<string | null>(null);
	let loading = $state(true);
	let error = $state('');

	// Load packs on mount.
	$effect(() => {
		loadPacks();
	});

	// Load stickers when active pack changes.
	$effect(() => {
		if (activePackId && !stickersByPack.has(activePackId)) {
			loadStickersForPack(activePackId);
		}
	});

	async function loadPacks() {
		loading = true;
		error = '';
		try {
			const guildId = $currentGuildId;
			const [userP, guildP] = await Promise.all([
				api.getUserStickerPacks(),
				guildId ? api.getGuildStickerPacks(guildId) : Promise.resolve([])
			]);
			userPacks = userP;
			guildPacks = guildP;

			// Select the first pack with stickers, or just the first pack.
			const allPacks = [...guildP, ...userP];
			if (allPacks.length > 0) {
				activePackId = allPacks[0].id;
			}
		} catch (e: any) {
			error = 'Failed to load sticker packs';
		} finally {
			loading = false;
		}
	}

	async function loadStickersForPack(packId: string) {
		const guildId = $currentGuildId;
		// Determine if this is a guild pack or user pack.
		const gPack = guildPacks.find(p => p.id === packId);
		try {
			let stickers: Sticker[];
			if (gPack && guildId) {
				stickers = await api.getPackStickers(guildId, packId);
			} else {
				// For user packs, the backend uses the same guild pack endpoint structure
				// but user packs don't have a guild context. We need to handle this.
				// The backend only has guild-scoped sticker fetching, so user packs
				// would need a different endpoint. For now, we skip if no guild context.
				// NOTE: The current backend only supports fetching stickers via guild routes.
				// User packs would need to be fetched through a guild route where the user has access.
				stickers = [];
			}
			stickersByPack = new Map([...stickersByPack, [packId, stickers]]);
		} catch {
			stickersByPack = new Map([...stickersByPack, [packId, []]]);
		}
	}

	const allPacks = $derived([...guildPacks, ...userPacks]);

	const activeStickers = $derived.by(() => {
		if (!activePackId) return [];
		const stickers = stickersByPack.get(activePackId) ?? [];
		if (!search.trim()) return stickers;
		const q = search.toLowerCase();
		return stickers.filter(s =>
			s.name.toLowerCase().includes(q) ||
			(s.tags && s.tags.toLowerCase().includes(q))
		);
	});

	const activePackName = $derived(
		allPacks.find(p => p.id === activePackId)?.name ?? 'Stickers'
	);

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.sticker-picker')) onclose();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<div class="sticker-picker absolute bottom-full right-0 z-50 mb-2 w-80 rounded-lg bg-bg-floating shadow-xl">
	<!-- Search -->
	<div class="border-b border-bg-modifier p-2">
		<input
			type="text"
			class="w-full rounded bg-bg-primary px-3 py-1.5 text-sm text-text-primary placeholder:text-text-muted outline-none"
			placeholder="Search stickers..."
			bind:value={search}
		/>
	</div>

	<!-- Pack tabs -->
	{#if allPacks.length > 1}
		<div class="flex gap-0.5 overflow-x-auto border-b border-bg-modifier px-1 py-1">
			{#each allPacks as pack (pack.id)}
				<button
					class="shrink-0 rounded px-2 py-1 text-xs transition-colors {activePackId === pack.id ? 'bg-brand-500/20 text-brand-400 font-medium' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
					onclick={() => (activePackId = pack.id)}
					title={pack.name}
				>
					{pack.name}
				</button>
			{/each}
		</div>
	{/if}

	<!-- Sticker grid -->
	<div class="max-h-64 overflow-y-auto p-2">
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if error}
			<p class="py-4 text-center text-xs text-text-muted">{error}</p>
		{:else if allPacks.length === 0}
			<p class="py-4 text-center text-xs text-text-muted">No sticker packs available</p>
		{:else if activeStickers.length === 0}
			<p class="py-4 text-center text-xs text-text-muted">
				{search.trim() ? 'No stickers match your search' : 'This pack has no stickers'}
			</p>
		{:else}
			<div class="grid grid-cols-4 gap-1.5">
				{#each activeStickers as sticker (sticker.id)}
					<button
						class="flex flex-col items-center gap-1 rounded-lg p-1.5 transition-colors hover:bg-bg-modifier"
						onclick={() => onselect(sticker)}
						title={sticker.name}
					>
						<img
							src="/api/v1/files/{sticker.file_id}"
							alt={sticker.name}
							class="h-14 w-14 object-contain"
							loading="lazy"
						/>
						<span class="max-w-full truncate text-2xs text-text-muted">{sticker.name}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Pack name footer -->
	{#if activePackId && !loading}
		<div class="border-t border-bg-modifier px-2 py-1 text-center">
			<span class="text-2xs text-text-muted">{activePackName}</span>
		</div>
	{/if}
</div>
