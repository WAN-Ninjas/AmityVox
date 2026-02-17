import { describe, it, expect } from 'vitest';
import {
	isFavorited,
	addFavorite,
	removeFavorite,
	filterFavoritesByQuery,
	MAX_FAVORITES,
	type FavoriteGif
} from '../gifFavorites';

const makeFav = (id: string, title: string): FavoriteGif => ({
	id,
	title,
	url: `https://media.giphy.com/${id}.gif`,
	previewUrl: `https://media.giphy.com/${id}_s.gif`
});

describe('gifFavorites', () => {
	describe('isFavorited', () => {
		it('returns true when gif is in favorites', () => {
			const favs = [makeFav('a', 'Cat'), makeFav('b', 'Dog')];
			expect(isFavorited(favs, 'a')).toBe(true);
		});

		it('returns false when gif is not in favorites', () => {
			const favs = [makeFav('a', 'Cat')];
			expect(isFavorited(favs, 'z')).toBe(false);
		});

		it('returns false for empty list', () => {
			expect(isFavorited([], 'a')).toBe(false);
		});
	});

	describe('addFavorite', () => {
		it('adds a favorite to the beginning', () => {
			const favs = [makeFav('a', 'Cat')];
			const result = addFavorite(favs, { id: 'b', title: 'Dog', url: 'u', previewUrl: 'p' });
			expect(result[0].id).toBe('b');
			expect(result).toHaveLength(2);
		});

		it('caps at MAX_FAVORITES', () => {
			const favs = Array.from({ length: MAX_FAVORITES }, (_, i) => makeFav(`${i}`, `Gif ${i}`));
			const result = addFavorite(favs, { id: 'new', title: 'New', url: 'u', previewUrl: 'p' });
			expect(result).toHaveLength(MAX_FAVORITES);
			expect(result[0].id).toBe('new');
		});
	});

	describe('removeFavorite', () => {
		it('removes a favorite by id', () => {
			const favs = [makeFav('a', 'Cat'), makeFav('b', 'Dog')];
			const result = removeFavorite(favs, 'a');
			expect(result).toHaveLength(1);
			expect(result[0].id).toBe('b');
		});

		it('returns same list if id not found', () => {
			const favs = [makeFav('a', 'Cat')];
			const result = removeFavorite(favs, 'z');
			expect(result).toHaveLength(1);
		});
	});

	describe('filterFavoritesByQuery', () => {
		const favs = [
			makeFav('1', 'Funny Cat Dancing'),
			makeFav('2', 'Dog Running'),
			makeFav('3', 'Cat Sleeping'),
			makeFav('4', 'Bird Flying'),
			makeFav('5', '')
		];

		it('filters by case-insensitive title match', () => {
			const result = filterFavoritesByQuery(favs, 'cat');
			expect(result).toHaveLength(2);
			expect(result.map(f => f.id)).toEqual(['1', '3']);
		});

		it('returns all when query matches everything', () => {
			const result = filterFavoritesByQuery(favs, '');
			expect(result).toHaveLength(5);
		});

		it('returns empty when nothing matches', () => {
			const result = filterFavoritesByQuery(favs, 'elephant');
			expect(result).toHaveLength(0);
		});

		it('is case-insensitive', () => {
			const result = filterFavoritesByQuery(favs, 'DOG');
			expect(result).toHaveLength(1);
			expect(result[0].id).toBe('2');
		});

		it('matches partial strings', () => {
			const result = filterFavoritesByQuery(favs, 'fly');
			expect(result).toHaveLength(1);
			expect(result[0].id).toBe('4');
		});
	});
});
