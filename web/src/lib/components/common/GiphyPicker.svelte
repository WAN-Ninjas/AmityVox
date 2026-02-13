<script lang="ts">
	import { api } from '$lib/api/client';
	import {
		type FavoriteGif,
		loadFavorites,
		saveFavorites,
		isFavorited,
		addFavorite,
		removeFavorite,
		MAX_FAVORITES
	} from '$lib/utils/gifFavorites';

	interface Props {
		onselect: (gifUrl: string) => void;
		onclose: () => void;
	}

	let { onselect, onclose }: Props = $props();

	// --- State ---
	type View = 'home' | 'browse';
	let view = $state<View>('home');
	let search = $state('');
	let gifs = $state<any[]>([]);
	let categories = $state<any[]>([]);
	let loading = $state(false);
	let error = $state('');
	let browseLabel = $state('');
	let browsingFavorites = $state(false);
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;

	// --- Favorites (localStorage) ---
	let favorites = $state<FavoriteGif[]>(loadFavorites());

	function isFavorite(gifId: string): boolean {
		return isFavorited(favorites, gifId);
	}

	function toggleFavorite(gif: any) {
		const gifId = gif.id;
		if (isFavorite(gifId)) {
			favorites = removeFavorite(favorites, gifId);
			if (browsingFavorites) {
				gifs = gifs.filter((g: any) => g.id !== gifId);
			}
		} else {
			favorites = addFavorite(favorites, {
				id: gifId,
				title: gif.title || '',
				url: gif.images?.fixed_height?.url ?? gif.images?.original?.url ?? '',
				previewUrl: gif.images?.fixed_height_small?.url ?? gif.images?.fixed_height?.url ?? ''
			});
		}
		saveFavorites(favorites);
	}

	// --- Data loading ---
	$effect(() => {
		loadCategories();
	});

	async function loadCategories() {
		try {
			const data = await api.getGiphyCategories(15);
			categories = data?.data ?? [];
		} catch (e) {
			console.debug('Failed to load Giphy categories:', e);
		}
	}

	async function loadTrending() {
		loading = true;
		error = '';
		browsingFavorites = false;
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
		loading = true;
		error = '';
		browsingFavorites = false;
		try {
			const data = await api.searchGiphy(query.trim());
			gifs = data?.data ?? [];
		} catch {
			error = 'Search failed';
		} finally {
			loading = false;
		}
	}

	async function loadCategoryGifs(categoryName: string) {
		loading = true;
		error = '';
		browsingFavorites = false;
		try {
			const data = await api.searchGiphy(categoryName, 25);
			gifs = data?.data ?? [];
		} catch {
			error = 'Failed to load category';
		} finally {
			loading = false;
		}
	}

	// --- Navigation ---
	function goHome() {
		view = 'home';
		search = '';
		gifs = [];
		browseLabel = '';
		browsingFavorites = false;
		error = '';
	}

	function openTrending() {
		view = 'browse';
		browseLabel = 'Trending';
		loadTrending();
	}

	function openFavorites() {
		view = 'browse';
		browseLabel = 'Favorites';
		browsingFavorites = true;
		loading = false;
		error = '';
		// Convert favorites to gif-like objects for rendering
		gifs = favorites.map((f) => ({
			id: f.id,
			title: f.title,
			images: {
				fixed_height: { url: f.url },
				fixed_height_small: { url: f.previewUrl },
				original: { url: f.url }
			}
		}));
	}

	function openCategory(name: string) {
		view = 'browse';
		browseLabel = name;
		loadCategoryGifs(name);
	}

	function handleSearchInput() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			const q = search.trim();
			if (q) {
				view = 'browse';
				browseLabel = '';
				searchGifs(q);
			} else {
				goHome();
			}
		}, 300);
	}

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.giphy-picker')) onclose();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}

	function selectGif(gif: any) {
		onselect(gif.images?.fixed_height?.url ?? gif.images?.original?.url);
	}
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<div class="giphy-picker absolute bottom-full right-0 z-50 mb-2 w-80 rounded-lg bg-bg-floating shadow-xl">
	<!-- Search bar -->
	<div class="flex items-center gap-1 border-b border-bg-modifier p-2">
		{#if view === 'browse'}
			<button
				class="flex-shrink-0 rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
				onclick={goHome}
				title="Back"
			>
				<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M19 12H5M12 19l-7-7 7-7" />
				</svg>
			</button>
		{/if}
		<input
			type="text"
			class="w-full rounded bg-bg-primary px-3 py-1.5 text-sm text-text-primary placeholder:text-text-muted outline-none"
			placeholder="Search GIFs..."
			bind:value={search}
			oninput={handleSearchInput}
		/>
	</div>

	<!-- Content area -->
	<div class="max-h-72 overflow-y-auto p-2">
		{#if view === 'home'}
			<!-- Home: Favorites + Trending + Categories -->
			<div class="grid grid-cols-2 gap-1.5">
				<!-- Favorites card -->
				{#if favorites.length > 0}
					<button
						class="group relative col-span-1 overflow-hidden rounded-lg bg-bg-modifier"
						style="height: 80px;"
						onclick={openFavorites}
					>
						{#if favorites[0]?.previewUrl}
							<img
								src={favorites[0].previewUrl}
								alt=""
								class="absolute inset-0 h-full w-full object-cover opacity-40"
							/>
						{/if}
						<div class="absolute inset-0 bg-black/40 group-hover:bg-black/30 transition-colors"></div>
						<div class="relative flex h-full flex-col items-center justify-center gap-1">
							<svg class="h-5 w-5 text-yellow-400" viewBox="0 0 24 24" fill="currentColor">
								<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
							</svg>
							<span class="text-xs font-semibold text-white">Favorites</span>
							<span class="text-2xs text-white/70">{favorites.length}</span>
						</div>
					</button>
				{/if}

				<!-- Trending card -->
				<button
					class="group relative col-span-1 overflow-hidden rounded-lg bg-bg-modifier"
					style="height: 80px;"
					onclick={openTrending}
				>
					<div class="absolute inset-0 bg-gradient-to-br from-orange-500/30 to-red-500/30 group-hover:from-orange-500/40 group-hover:to-red-500/40 transition-colors"></div>
					<div class="relative flex h-full flex-col items-center justify-center gap-1">
						<svg class="h-5 w-5 text-orange-400" viewBox="0 0 24 24" fill="currentColor">
							<path d="M13.5.67s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.21 7.51 4 10.62 4 14c0 4.42 3.58 8 8 8s8-3.58 8-8C20 8.61 17.41 3.8 13.5.67zM11.71 19c-1.78 0-3.22-1.4-3.22-3.14 0-1.62 1.05-2.76 2.81-3.12 1.77-.36 3.6-1.21 4.62-2.58.39 1.29.59 2.65.59 4.04 0 2.65-2.15 4.8-4.8 4.8z" />
						</svg>
						<span class="text-xs font-semibold text-white">Trending</span>
					</div>
				</button>

				<!-- Category cards -->
				{#each categories as cat (cat.name_encoded)}
					<button
						class="group relative col-span-1 overflow-hidden rounded-lg bg-bg-modifier"
						style="height: 80px;"
						onclick={() => openCategory(cat.name)}
					>
						{#if cat.gif?.images?.fixed_height_small?.url}
							<img
								src={cat.gif.images.fixed_height_small.url}
								alt=""
								class="absolute inset-0 h-full w-full object-cover"
								loading="lazy"
							/>
						{/if}
						<div class="absolute inset-0 bg-black/50 group-hover:bg-black/35 transition-colors"></div>
						<div class="relative flex h-full items-center justify-center">
							<span class="text-xs font-semibold text-white capitalize">{cat.name}</span>
						</div>
					</button>
				{/each}
			</div>

		{:else}
			<!-- Browse: GIF grid -->
			{#if loading}
				<div class="flex items-center justify-center py-8">
					<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				</div>
			{:else if error}
				<p class="py-4 text-center text-xs text-text-muted">{error}</p>
			{:else if gifs.length === 0}
				<p class="py-4 text-center text-xs text-text-muted">
					{browsingFavorites ? 'No favorites yet — star some GIFs!' : 'No GIFs found'}
				</p>
			{:else}
				<div class="columns-2 gap-1">
					{#each gifs as gif (gif.id)}
						<div class="group relative mb-1">
							<button
								class="w-full overflow-hidden rounded hover:ring-2 hover:ring-brand-500"
								onclick={() => selectGif(gif)}
							>
								<img
									src={gif.images?.fixed_height_small?.url ?? gif.images?.fixed_height?.url}
									alt={gif.title}
									class="w-full"
									loading="lazy"
								/>
							</button>
							<!-- Star overlay -->
							<button
								class="absolute right-1 top-1 rounded-full bg-black/50 p-1 opacity-0 transition-opacity group-hover:opacity-100 {isFavorite(gif.id) ? '!opacity-100' : ''}"
								onclick={(e) => { e.stopPropagation(); toggleFavorite(gif); }}
								title={isFavorite(gif.id) ? 'Remove from favorites' : 'Add to favorites'}
							>
								{#if isFavorite(gif.id)}
									<svg class="h-3.5 w-3.5 text-yellow-400" viewBox="0 0 24 24" fill="currentColor">
										<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
									</svg>
								{:else}
									<svg class="h-3.5 w-3.5 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
										<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" />
									</svg>
								{/if}
							</button>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	</div>

	<!-- Footer -->
	<div class="border-t border-bg-modifier p-1.5 text-center">
		{#if view === 'browse' && browseLabel}
			<span class="text-2xs text-text-muted">{browseLabel}</span>
		{:else}
			<span class="text-2xs text-text-muted">Powered by GIPHY</span>
		{/if}
	</div>
</div>
