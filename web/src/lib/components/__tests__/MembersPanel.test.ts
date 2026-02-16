import { describe, it, expect } from 'vitest';

/**
 * Tests for MembersPanel filtering logic.
 * Since Svelte 5 components can't be rendered in happy-dom (SSR mode),
 * we test the pure filtering function as a standalone function.
 */

// --- Type (minimal subset for testing) ---

interface User {
	id: string;
	username: string;
	display_name: string | null;
}

interface GuildMember {
	guild_id: string;
	user_id: string;
	nickname: string | null;
	avatar_id: string | null;
	joined_at: string;
	timeout_until: string | null;
	deaf: boolean;
	mute: boolean;
	user?: User;
	roles?: string[];
}

// --- Filter function (replicated from MembersPanel.svelte) ---

function filterMembers(members: GuildMember[], query: string): GuildMember[] {
	if (!query.trim()) return members;
	const q = query.toLowerCase();
	return members.filter((m) => {
		const username = m.user?.username?.toLowerCase() ?? '';
		const displayName = m.user?.display_name?.toLowerCase() ?? '';
		const nickname = m.nickname?.toLowerCase() ?? '';
		return username.includes(q) || displayName.includes(q) || nickname.includes(q);
	});
}

// --- Test data ---

function makeMember(overrides: {
	user_id?: string;
	username?: string;
	display_name?: string | null;
	nickname?: string | null;
}): GuildMember {
	const id = overrides.user_id ?? '01ABCDEF';
	return {
		guild_id: 'guild1',
		user_id: id,
		nickname: overrides.nickname ?? null,
		avatar_id: null,
		joined_at: '2024-01-01T00:00:00Z',
		timeout_until: null,
		deaf: false,
		mute: false,
		user: {
			id,
			username: overrides.username ?? 'testuser',
			display_name: overrides.display_name ?? null,
		},
	};
}

const members: GuildMember[] = [
	makeMember({ user_id: '1', username: 'alice', display_name: 'Alice Wonderland', nickname: 'ally' }),
	makeMember({ user_id: '2', username: 'bob', display_name: 'Bob Builder', nickname: null }),
	makeMember({ user_id: '3', username: 'charlie', display_name: null, nickname: 'chuck' }),
	makeMember({ user_id: '4', username: 'diana', display_name: 'Diana Prince', nickname: 'wonder' }),
	makeMember({ user_id: '5', username: 'eve', display_name: null, nickname: null }),
];

// --- Tests ---

describe('filterMembers', () => {
	it('returns all members when query is empty', () => {
		expect(filterMembers(members, '')).toHaveLength(5);
	});

	it('returns all members when query is only whitespace', () => {
		expect(filterMembers(members, '   ')).toHaveLength(5);
	});

	it('filters by username', () => {
		const result = filterMembers(members, 'alice');
		expect(result).toHaveLength(1);
		expect(result[0].user_id).toBe('1');
	});

	it('filters by display name', () => {
		const result = filterMembers(members, 'builder');
		expect(result).toHaveLength(1);
		expect(result[0].user_id).toBe('2');
	});

	it('filters by nickname', () => {
		const result = filterMembers(members, 'chuck');
		expect(result).toHaveLength(1);
		expect(result[0].user_id).toBe('3');
	});

	it('is case-insensitive', () => {
		expect(filterMembers(members, 'ALICE')).toHaveLength(1);
		expect(filterMembers(members, 'Alice')).toHaveLength(1);
		expect(filterMembers(members, 'aLiCe')).toHaveLength(1);
	});

	it('matches partial strings', () => {
		const result = filterMembers(members, 'li');
		// matches: alice (username), Alice (display), charlie (username)
		expect(result.length).toBeGreaterThanOrEqual(2);
		const ids = result.map((m) => m.user_id);
		expect(ids).toContain('1'); // alice
		expect(ids).toContain('3'); // charlie
	});

	it('matches across different fields (username OR display_name OR nickname)', () => {
		const result = filterMembers(members, 'wonder');
		// matches: Alice Wonderland (display_name) and diana's nickname "wonder"
		const ids = result.map((m) => m.user_id);
		expect(ids).toContain('1'); // Alice Wonderland
		expect(ids).toContain('4'); // nickname: wonder
	});

	it('returns empty when no matches', () => {
		expect(filterMembers(members, 'zzzzz')).toHaveLength(0);
	});

	it('handles members without user data', () => {
		const sparse: GuildMember[] = [
			{
				guild_id: 'guild1',
				user_id: '99',
				nickname: 'ghost',
				avatar_id: null,
				joined_at: '2024-01-01T00:00:00Z',
				timeout_until: null,
				deaf: false,
				mute: false,
				// no user property
			},
		];
		const result = filterMembers(sparse, 'ghost');
		expect(result).toHaveLength(1);
	});

	it('handles members where user has null fields', () => {
		const sparse: GuildMember[] = [
			makeMember({ user_id: '100', username: 'minimal', display_name: null, nickname: null }),
		];
		const result = filterMembers(sparse, 'minimal');
		expect(result).toHaveLength(1);
	});

	it('handles empty member list', () => {
		expect(filterMembers([], 'test')).toHaveLength(0);
	});
});
