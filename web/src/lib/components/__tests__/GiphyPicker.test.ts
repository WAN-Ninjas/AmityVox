import { describe, it, expect, beforeEach } from 'vitest';

/**
 * Test the favorites logic from GiphyPicker.svelte as pure functions.
 * Since Svelte 5 components can't render in happy-dom, we extract
 * and test the localStorage-backed favorites management.
 */

const FAVORITES_KEY = 'amityvox_gif_favorites';
const MAX_FAVORITES = 50;

interface FavoriteGif {
	id: string;
	title: string;
	url: string;
	previewUrl: string;
}

// --- Pure functions mirroring GiphyPicker.svelte logic ---

function loadFavorites(): FavoriteGif[] {
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

function saveFavorites(favs: FavoriteGif[]) {
	localStorage.setItem(FAVORITES_KEY, JSON.stringify(favs));
}

function isFavorite(favorites: FavoriteGif[], gifId: string): boolean {
	return favorites.some((f) => f.id === gifId);
}

function addFavorite(favorites: FavoriteGif[], gif: { id: string; title?: string; url: string; previewUrl: string }): FavoriteGif[] {
	const entry: FavoriteGif = {
		id: gif.id,
		title: gif.title || '',
		url: gif.url,
		previewUrl: gif.previewUrl
	};
	return [entry, ...favorites].slice(0, MAX_FAVORITES);
}

function removeFavorite(favorites: FavoriteGif[], gifId: string): FavoriteGif[] {
	return favorites.filter((f) => f.id !== gifId);
}

// --- Helpers ---

function makeFav(id: string): FavoriteGif {
	return { id, title: `GIF ${id}`, url: `https://giphy.com/${id}.gif`, previewUrl: `https://giphy.com/${id}_s.gif` };
}

// --- Tests ---

describe('GiphyPicker favorites', () => {
	beforeEach(() => {
		localStorage.clear();
	});

	describe('loadFavorites', () => {
		it('returns empty array when nothing stored', () => {
			expect(loadFavorites()).toEqual([]);
		});

		it('returns stored favorites', () => {
			const favs = [makeFav('a'), makeFav('b')];
			localStorage.setItem(FAVORITES_KEY, JSON.stringify(favs));
			expect(loadFavorites()).toEqual(favs);
		});

		it('returns empty array for invalid JSON', () => {
			localStorage.setItem(FAVORITES_KEY, 'not-json{{{');
			expect(loadFavorites()).toEqual([]);
		});

		it('returns empty array for non-array JSON', () => {
			localStorage.setItem(FAVORITES_KEY, '{"hello": "world"}');
			expect(loadFavorites()).toEqual([]);
		});

		it('returns empty array for null stored value', () => {
			localStorage.setItem(FAVORITES_KEY, 'null');
			expect(loadFavorites()).toEqual([]);
		});
	});

	describe('saveFavorites', () => {
		it('persists to localStorage', () => {
			const favs = [makeFav('x')];
			saveFavorites(favs);
			expect(JSON.parse(localStorage.getItem(FAVORITES_KEY)!)).toEqual(favs);
		});

		it('round-trips through load', () => {
			const favs = [makeFav('1'), makeFav('2'), makeFav('3')];
			saveFavorites(favs);
			expect(loadFavorites()).toEqual(favs);
		});
	});

	describe('isFavorite', () => {
		it('returns true when GIF is in favorites', () => {
			const favs = [makeFav('a'), makeFav('b')];
			expect(isFavorite(favs, 'a')).toBe(true);
			expect(isFavorite(favs, 'b')).toBe(true);
		});

		it('returns false when GIF is not in favorites', () => {
			const favs = [makeFav('a')];
			expect(isFavorite(favs, 'z')).toBe(false);
		});

		it('returns false for empty favorites', () => {
			expect(isFavorite([], 'x')).toBe(false);
		});
	});

	describe('addFavorite', () => {
		it('adds to the front (newest first)', () => {
			const favs = [makeFav('old')];
			const result = addFavorite(favs, { id: 'new', title: 'New', url: 'u', previewUrl: 'p' });
			expect(result[0].id).toBe('new');
			expect(result[1].id).toBe('old');
		});

		it('caps at MAX_FAVORITES (50)', () => {
			const favs: FavoriteGif[] = [];
			for (let i = 0; i < 50; i++) {
				favs.push(makeFav(`gif-${i}`));
			}
			expect(favs).toHaveLength(50);

			const result = addFavorite(favs, { id: 'overflow', url: 'u', previewUrl: 'p' });
			expect(result).toHaveLength(50);
			expect(result[0].id).toBe('overflow');
			expect(result[49].id).toBe('gif-48'); // last item is gif-48, gif-49 was dropped
		});

		it('uses empty string for missing title', () => {
			const result = addFavorite([], { id: 'no-title', url: 'u', previewUrl: 'p' });
			expect(result[0].title).toBe('');
		});
	});

	describe('removeFavorite', () => {
		it('removes by ID', () => {
			const favs = [makeFav('a'), makeFav('b'), makeFav('c')];
			const result = removeFavorite(favs, 'b');
			expect(result).toHaveLength(2);
			expect(result.map((f) => f.id)).toEqual(['a', 'c']);
		});

		it('does nothing if ID not found', () => {
			const favs = [makeFav('a')];
			const result = removeFavorite(favs, 'z');
			expect(result).toEqual(favs);
		});

		it('handles empty list', () => {
			expect(removeFavorite([], 'x')).toEqual([]);
		});
	});

	describe('full workflow', () => {
		it('add → save → load → check → remove → save → load', () => {
			// Start empty
			let favs = loadFavorites();
			expect(favs).toEqual([]);

			// Add two
			favs = addFavorite(favs, { id: 'g1', title: 'First', url: 'u1', previewUrl: 'p1' });
			favs = addFavorite(favs, { id: 'g2', title: 'Second', url: 'u2', previewUrl: 'p2' });
			saveFavorites(favs);

			// Reload and verify order (newest first)
			favs = loadFavorites();
			expect(favs).toHaveLength(2);
			expect(favs[0].id).toBe('g2');
			expect(favs[1].id).toBe('g1');

			// Check membership
			expect(isFavorite(favs, 'g1')).toBe(true);
			expect(isFavorite(favs, 'g2')).toBe(true);
			expect(isFavorite(favs, 'g3')).toBe(false);

			// Remove one
			favs = removeFavorite(favs, 'g1');
			saveFavorites(favs);

			// Reload and verify
			favs = loadFavorites();
			expect(favs).toHaveLength(1);
			expect(favs[0].id).toBe('g2');
			expect(isFavorite(favs, 'g1')).toBe(false);
		});
	});
});
