<script lang="ts">
	import { api } from '$lib/api/client';

	interface Props {
		onselect: (gifUrl: string) => void;
		onclose: () => void;
	}

	let { onselect, onclose }: Props = $props();

	let search = $state('');
	let gifs = $state<any[]>([]);
	let loading = $state(true);
	let error = $state('');
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;

	// Load trending on mount.
	$effect(() => {
		loadTrending();
	});

	async function loadTrending() {
		loading = true;
		error = '';
		try {
			const data = await api.getTrendingGiphy(25);
			gifs = data?.data ?? [];
		} catch (e: any) {
			if (e.status === 503) {
				error = 'GIF search is not enabled on this instance';
			} else {
				error = 'Failed to load GIFs';
			}
		} finally {
			loading = false;
		}
	}

	async function searchGifs(query: string) {
		if (!query.trim()) {
			loadTrending();
			return;
		}
		loading = true;
		error = '';
		try {
			const data = await api.searchGiphy(query.trim());
			gifs = data?.data ?? [];
		} catch (e: any) {
			error = 'Search failed';
		} finally {
			loading = false;
		}
	}

	function handleSearchInput() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			searchGifs(search);
		}, 300);
	}

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.giphy-picker')) onclose();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<div class="giphy-picker absolute bottom-full right-0 z-50 mb-2 w-80 rounded-lg bg-bg-floating shadow-xl">
	<!-- Search -->
	<div class="border-b border-bg-modifier p-2">
		<input
			type="text"
			class="w-full rounded bg-bg-primary px-3 py-1.5 text-sm text-text-primary placeholder:text-text-muted outline-none"
			placeholder="Search GIFs..."
			bind:value={search}
			oninput={handleSearchInput}
		/>
	</div>

	<!-- GIF grid -->
	<div class="max-h-64 overflow-y-auto p-2">
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if error}
			<p class="py-4 text-center text-xs text-text-muted">{error}</p>
		{:else if gifs.length === 0}
			<p class="py-4 text-center text-xs text-text-muted">No GIFs found</p>
		{:else}
			<div class="columns-2 gap-1">
				{#each gifs as gif (gif.id)}
					<button
						class="mb-1 w-full overflow-hidden rounded hover:ring-2 hover:ring-brand-500"
						onclick={() => onselect(gif.images?.fixed_height?.url ?? gif.images?.original?.url)}
					>
						<img
							src={gif.images?.fixed_height_small?.url ?? gif.images?.fixed_height?.url}
							alt={gif.title}
							class="w-full"
							loading="lazy"
						/>
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Powered by Giphy attribution -->
	<div class="border-t border-bg-modifier p-1.5 text-center">
		<span class="text-2xs text-text-muted">Powered by GIPHY</span>
	</div>
</div>
