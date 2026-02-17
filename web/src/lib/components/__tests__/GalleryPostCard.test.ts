import { describe, it, expect } from 'vitest';

// Test pure logic extracted from GalleryPostCard.svelte

interface GalleryTag {
	id: string;
	name: string;
	emoji: string | null;
	color: string | null;
}

interface GalleryPost {
	id: string;
	name: string | null;
	pinned: boolean;
	locked: boolean;
	reply_count: number;
	tags: GalleryTag[];
	author?: { display_name: string | null; username: string } | null;
	thumbnail?: { content_type: string; nsfw: boolean } | null;
	created_at: string;
}

function getDisplayTitle(post: GalleryPost): string {
	return post.name ?? 'Untitled';
}

function getAuthorName(post: GalleryPost): string {
	return post.author?.display_name || post.author?.username || 'Unknown';
}

function isVideoPost(post: GalleryPost): boolean {
	return post.thumbnail?.content_type?.startsWith('video/') ?? false;
}

function isNsfwPost(post: GalleryPost): boolean {
	return post.thumbnail?.nsfw ?? false;
}

function getVisibleTags(tags: GalleryTag[], max: number = 3): { visible: GalleryTag[]; overflow: number } {
	return {
		visible: tags.slice(0, max),
		overflow: Math.max(0, tags.length - max)
	};
}

function formatTagStyle(tag: GalleryTag): { bg: string; color: string } {
	return {
		bg: tag.color ? tag.color + '20' : 'var(--bg-modifier)',
		color: tag.color || 'var(--text-secondary)'
	};
}

describe('GalleryPostCard', () => {
	describe('getDisplayTitle', () => {
		it('returns post name when available', () => {
			const post: GalleryPost = {
				id: '1', name: 'My Photo', pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString()
			};
			expect(getDisplayTitle(post)).toBe('My Photo');
		});

		it('returns "Untitled" when name is null', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString()
			};
			expect(getDisplayTitle(post)).toBe('Untitled');
		});
	});

	describe('getAuthorName', () => {
		it('prefers display_name over username', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				author: { display_name: 'John Doe', username: 'john' }
			};
			expect(getAuthorName(post)).toBe('John Doe');
		});

		it('falls back to username', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				author: { display_name: null, username: 'john' }
			};
			expect(getAuthorName(post)).toBe('john');
		});

		it('returns "Unknown" when no author', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString()
			};
			expect(getAuthorName(post)).toBe('Unknown');
		});
	});

	describe('isVideoPost', () => {
		it('returns true for video thumbnails', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				thumbnail: { content_type: 'video/mp4', nsfw: false }
			};
			expect(isVideoPost(post)).toBe(true);
		});

		it('returns false for image thumbnails', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				thumbnail: { content_type: 'image/jpeg', nsfw: false }
			};
			expect(isVideoPost(post)).toBe(false);
		});

		it('returns false when no thumbnail', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString()
			};
			expect(isVideoPost(post)).toBe(false);
		});
	});

	describe('isNsfwPost', () => {
		it('returns true for nsfw thumbnails', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				thumbnail: { content_type: 'image/jpeg', nsfw: true }
			};
			expect(isNsfwPost(post)).toBe(true);
		});

		it('returns false for safe thumbnails', () => {
			const post: GalleryPost = {
				id: '1', name: null, pinned: false, locked: false,
				reply_count: 0, tags: [], created_at: new Date().toISOString(),
				thumbnail: { content_type: 'image/jpeg', nsfw: false }
			};
			expect(isNsfwPost(post)).toBe(false);
		});
	});

	describe('getVisibleTags', () => {
		const tags: GalleryTag[] = [
			{ id: '1', name: 'Art', emoji: null, color: null },
			{ id: '2', name: 'Photo', emoji: null, color: '#ff0000' },
			{ id: '3', name: 'Meme', emoji: '😂', color: null },
			{ id: '4', name: 'Nature', emoji: null, color: null },
			{ id: '5', name: 'Travel', emoji: null, color: null },
		];

		it('returns first 3 tags by default', () => {
			const { visible, overflow } = getVisibleTags(tags);
			expect(visible).toHaveLength(3);
			expect(overflow).toBe(2);
		});

		it('returns all tags when fewer than max', () => {
			const { visible, overflow } = getVisibleTags(tags.slice(0, 2));
			expect(visible).toHaveLength(2);
			expect(overflow).toBe(0);
		});

		it('handles empty tags', () => {
			const { visible, overflow } = getVisibleTags([]);
			expect(visible).toHaveLength(0);
			expect(overflow).toBe(0);
		});

		it('supports custom max', () => {
			const { visible, overflow } = getVisibleTags(tags, 2);
			expect(visible).toHaveLength(2);
			expect(overflow).toBe(3);
		});
	});

	describe('formatTagStyle', () => {
		it('uses tag color when available', () => {
			const tag: GalleryTag = { id: '1', name: 'Art', emoji: null, color: '#ff5500' };
			const style = formatTagStyle(tag);
			expect(style.bg).toBe('#ff550020');
			expect(style.color).toBe('#ff5500');
		});

		it('uses defaults when no color', () => {
			const tag: GalleryTag = { id: '1', name: 'Art', emoji: null, color: null };
			const style = formatTagStyle(tag);
			expect(style.bg).toBe('var(--bg-modifier)');
			expect(style.color).toBe('var(--text-secondary)');
		});
	});
});
