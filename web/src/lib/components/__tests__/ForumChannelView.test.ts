import { describe, it, expect } from 'vitest';

// Since Svelte 5 components can't be rendered in happy-dom, we test the pure
// logic functions used by ForumChannelView and ForumPostCard.

// --- Extracted from ForumPostCard.svelte ---

function formatDate(iso: string): string {
	const date = new Date(iso);
	const now = new Date();
	const diffMs = now.getTime() - date.getTime();
	const diffHours = diffMs / (1000 * 60 * 60);

	if (diffHours < 1) {
		const mins = Math.floor(diffMs / (1000 * 60));
		return mins <= 1 ? 'just now' : `${mins}m ago`;
	}
	if (diffHours < 24) {
		return `${Math.floor(diffHours)}h ago`;
	}
	if (diffHours < 24 * 7) {
		return `${Math.floor(diffHours / 24)}d ago`;
	}
	return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

// --- Extracted from ForumChannelView.svelte ---

interface ForumPost {
	id: string;
	name: string | null;
	pinned: boolean;
	locked: boolean;
	reply_count: number;
}

function filterPinned(posts: ForumPost[]): { pinned: ForumPost[]; regular: ForumPost[] } {
	return {
		pinned: posts.filter((p) => p.pinned),
		regular: posts.filter((p) => !p.pinned)
	};
}

function parseSortBy(value: string): 'latest_activity' | 'creation_date' {
	if (value === 'creation_date') return 'creation_date';
	return 'latest_activity';
}

function parseLimit(value: string): number {
	const limit = parseInt(value, 10);
	if (isNaN(limit) || limit <= 0 || limit > 100) return 25;
	return limit;
}

describe('ForumChannelView', () => {
	describe('formatDate', () => {
		it('returns "just now" for recent timestamps', () => {
			const now = new Date().toISOString();
			expect(formatDate(now)).toBe('just now');
		});

		it('returns minutes ago for < 1 hour', () => {
			const thirtyMinsAgo = new Date(Date.now() - 30 * 60 * 1000).toISOString();
			expect(formatDate(thirtyMinsAgo)).toBe('30m ago');
		});

		it('returns hours ago for < 24 hours', () => {
			const fiveHoursAgo = new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString();
			expect(formatDate(fiveHoursAgo)).toBe('5h ago');
		});

		it('returns days ago for < 7 days', () => {
			const threeDaysAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString();
			expect(formatDate(threeDaysAgo)).toBe('3d ago');
		});

		it('returns formatted date for >= 7 days', () => {
			const twoWeeksAgo = new Date(Date.now() - 14 * 24 * 60 * 60 * 1000).toISOString();
			const result = formatDate(twoWeeksAgo);
			// Should contain a month abbreviation and year
			expect(result).toMatch(/\w+ \d+, \d{4}/);
		});
	});

	describe('filterPinned', () => {
		it('separates pinned and regular posts', () => {
			const posts: ForumPost[] = [
				{ id: '1', name: 'Pinned Post', pinned: true, locked: false, reply_count: 10 },
				{ id: '2', name: 'Regular Post', pinned: false, locked: false, reply_count: 5 },
				{ id: '3', name: 'Another Pinned', pinned: true, locked: true, reply_count: 0 },
				{ id: '4', name: 'Another Regular', pinned: false, locked: false, reply_count: 3 },
			];

			const { pinned, regular } = filterPinned(posts);

			expect(pinned).toHaveLength(2);
			expect(regular).toHaveLength(2);
			expect(pinned[0].id).toBe('1');
			expect(pinned[1].id).toBe('3');
			expect(regular[0].id).toBe('2');
			expect(regular[1].id).toBe('4');
		});

		it('handles all pinned', () => {
			const posts: ForumPost[] = [
				{ id: '1', name: 'A', pinned: true, locked: false, reply_count: 0 },
			];
			const { pinned, regular } = filterPinned(posts);
			expect(pinned).toHaveLength(1);
			expect(regular).toHaveLength(0);
		});

		it('handles no pinned', () => {
			const posts: ForumPost[] = [
				{ id: '1', name: 'A', pinned: false, locked: false, reply_count: 0 },
			];
			const { pinned, regular } = filterPinned(posts);
			expect(pinned).toHaveLength(0);
			expect(regular).toHaveLength(1);
		});

		it('handles empty array', () => {
			const { pinned, regular } = filterPinned([]);
			expect(pinned).toHaveLength(0);
			expect(regular).toHaveLength(0);
		});
	});

	describe('parseSortBy', () => {
		it('returns latest_activity for empty string', () => {
			expect(parseSortBy('')).toBe('latest_activity');
		});

		it('returns latest_activity for unknown value', () => {
			expect(parseSortBy('invalid')).toBe('latest_activity');
		});

		it('returns creation_date when specified', () => {
			expect(parseSortBy('creation_date')).toBe('creation_date');
		});

		it('returns latest_activity when specified', () => {
			expect(parseSortBy('latest_activity')).toBe('latest_activity');
		});
	});

	describe('parseLimit', () => {
		it('defaults to 25 for empty string', () => {
			expect(parseLimit('')).toBe(25);
		});

		it('defaults to 25 for non-numeric', () => {
			expect(parseLimit('abc')).toBe(25);
		});

		it('defaults to 25 for zero', () => {
			expect(parseLimit('0')).toBe(25);
		});

		it('defaults to 25 for negative', () => {
			expect(parseLimit('-5')).toBe(25);
		});

		it('defaults to 25 for over 100', () => {
			expect(parseLimit('200')).toBe(25);
		});

		it('returns valid limits', () => {
			expect(parseLimit('10')).toBe(10);
			expect(parseLimit('50')).toBe(50);
			expect(parseLimit('100')).toBe(100);
		});
	});
});
