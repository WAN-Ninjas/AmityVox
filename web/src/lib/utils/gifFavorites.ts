export const FAVORITES_KEY = 'amityvox_gif_favorites';
export const MAX_FAVORITES = 50;

export interface FavoriteGif {
	id: string;
	title: string;
	url: string;
	previewUrl: string;
}

export function loadFavorites(): FavoriteGif[] {
	try {
		const raw = localStorage.getItem(FAVORITES_KEY);
		if (!raw) return [];
		const parsed = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed;
	} catch {
		return [];
	}
}

export function saveFavorites(favs: FavoriteGif[]) {
	localStorage.setItem(FAVORITES_KEY, JSON.stringify(favs));
}

export function isFavorited(favorites: FavoriteGif[], gifId: string): boolean {
	return favorites.some((f) => f.id === gifId);
}

export function addFavorite(
	favorites: FavoriteGif[],
	gif: { id: string; title?: string; url: string; previewUrl: string }
): FavoriteGif[] {
	const entry: FavoriteGif = {
		id: gif.id,
		title: gif.title || '',
		url: gif.url,
		previewUrl: gif.previewUrl
	};
	return [entry, ...favorites].slice(0, MAX_FAVORITES);
}

export function removeFavorite(favorites: FavoriteGif[], gifId: string): FavoriteGif[] {
	return favorites.filter((f) => f.id !== gifId);
}

/** Filter favorites by title substring match (case-insensitive). */
export function filterFavoritesByQuery(favorites: FavoriteGif[], query: string): FavoriteGif[] {
	const q = query.toLowerCase();
	return favorites.filter(f => f.title.toLowerCase().includes(q));
}
