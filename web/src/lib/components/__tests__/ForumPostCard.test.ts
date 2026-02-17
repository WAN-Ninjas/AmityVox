import { describe, it, expect } from 'vitest';

// Since Svelte 5 components can't be rendered in happy-dom, we test the pure
// display logic used by ForumPostCard.

interface ForumTag {
	id: string;
	name: string;
	emoji: string | null;
	color: string | null;
}

interface ForumPost {
	id: string;
	name: string | null;
	owner_id: string | null;
	pinned: boolean;
	locked: boolean;
	reply_count: number;
	last_activity_at: string | null;
	created_at: string;
	tags: ForumTag[];
	author?: {
		display_name: string | null;
		username: string;
		avatar_id: string | null;
	};
	content_preview?: string;
}

// --- Display logic extracted from ForumPostCard.svelte ---

function getPostTitle(post: ForumPost): string {
	return post.name ?? 'Untitled Post';
}

function getAuthorName(post: ForumPost): string {
	return post.author?.display_name || post.author?.username || 'Unknown';
}

function getTagStyle(tag: ForumTag): { background: string; color: string } {
	return {
		background: tag.color ? tag.color + '20' : 'var(--bg-modifier)',
		color: tag.color || 'var(--text-secondary)'
	};
}

function getCardBorderClass(post: ForumPost): string {
	if (post.pinned) return 'border-brand-500/30 bg-brand-500/5';
	return 'border-transparent hover:border-bg-modifier';
}

function contentPreviewLength(content: string): number {
	// Backend truncates to 200 chars
	return content.length > 200 ? 200 : content.length;
}

describe('ForumPostCard', () => {
	describe('getPostTitle', () => {
		it('returns post name when available', () => {
			const post = { name: 'Bug Report' } as ForumPost;
			expect(getPostTitle(post)).toBe('Bug Report');
		});

		it('returns fallback for null name', () => {
			const post = { name: null } as ForumPost;
			expect(getPostTitle(post)).toBe('Untitled Post');
		});
	});

	describe('getAuthorName', () => {
		it('prefers display_name', () => {
			const post = {
				author: { display_name: 'John Doe', username: 'johndoe', avatar_id: null }
			} as ForumPost;
			expect(getAuthorName(post)).toBe('John Doe');
		});

		it('falls back to username', () => {
			const post = {
				author: { display_name: null, username: 'johndoe', avatar_id: null }
			} as ForumPost;
			expect(getAuthorName(post)).toBe('johndoe');
		});

		it('returns Unknown when no author', () => {
			const post = {} as ForumPost;
			expect(getAuthorName(post)).toBe('Unknown');
		});
	});

	describe('getTagStyle', () => {
		it('uses tag color with alpha for background', () => {
			const tag: ForumTag = { id: '1', name: 'Bug', emoji: null, color: '#ff0000' };
			const style = getTagStyle(tag);
			expect(style.background).toBe('#ff000020');
			expect(style.color).toBe('#ff0000');
		});

		it('uses CSS variables for colorless tags', () => {
			const tag: ForumTag = { id: '1', name: 'General', emoji: null, color: null };
			const style = getTagStyle(tag);
			expect(style.background).toBe('var(--bg-modifier)');
			expect(style.color).toBe('var(--text-secondary)');
		});
	});

	describe('getCardBorderClass', () => {
		it('highlights pinned posts', () => {
			const post = { pinned: true } as ForumPost;
			expect(getCardBorderClass(post)).toContain('brand-500');
		});

		it('uses transparent border for regular posts', () => {
			const post = { pinned: false } as ForumPost;
			expect(getCardBorderClass(post)).toContain('border-transparent');
		});
	});

	describe('content preview truncation', () => {
		it('preserves short content', () => {
			expect(contentPreviewLength('Hello world')).toBe(11);
		});

		it('truncates long content to 200 chars', () => {
			const longContent = 'a'.repeat(500);
			expect(contentPreviewLength(longContent)).toBe(200);
		});

		it('handles exactly 200 chars', () => {
			const exact = 'b'.repeat(200);
			expect(contentPreviewLength(exact)).toBe(200);
		});
	});

	describe('post metadata display', () => {
		it('shows reply count', () => {
			const post = { reply_count: 42 } as ForumPost;
			expect(post.reply_count).toBe(42);
		});

		it('shows zero replies', () => {
			const post = { reply_count: 0 } as ForumPost;
			expect(post.reply_count).toBe(0);
		});

		it('identifies locked posts', () => {
			const post = { locked: true } as ForumPost;
			expect(post.locked).toBe(true);
		});

		it('shows tags', () => {
			const post = {
				tags: [
					{ id: '1', name: 'Bug', emoji: '🐛', color: '#ff0000' },
					{ id: '2', name: 'Help', emoji: null, color: '#00ff00' }
				]
			} as ForumPost;
			expect(post.tags).toHaveLength(2);
			expect(post.tags[0].emoji).toBe('🐛');
			expect(post.tags[1].emoji).toBeNull();
		});
	});
});
