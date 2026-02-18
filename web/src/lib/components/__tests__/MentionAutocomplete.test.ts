import { describe, it, expect } from 'vitest';

/**
 * Test the filtering logic from MentionAutocomplete.svelte as pure functions.
 * Since Svelte 5 components can't be rendered in happy-dom, we replicate the
 * filtering and item-building logic here.
 */

interface MemberLike {
	user_id: string;
	nickname?: string | null;
	user?: { username?: string; display_name?: string | null } | null;
	roles?: string[];
}

interface RoleLike {
	id: string;
	name: string;
	color?: string | null;
	mentionable: boolean;
}

interface AutocompleteItem {
	type: 'here' | 'user' | 'role';
	id: string;
	label: string;
	sublabel?: string;
	color?: string;
}

function filterMembers(members: Map<string, MemberLike>, query: string, limit = 10): MemberLike[] {
	const lowerQuery = query.toLowerCase();
	const results: MemberLike[] = [];
	for (const [, member] of members) {
		if (results.length >= limit) break;
		const username = member.user?.username?.toLowerCase() ?? '';
		const displayName = member.user?.display_name?.toLowerCase() ?? '';
		const nickname = member.nickname?.toLowerCase() ?? '';
		if (username.includes(lowerQuery) || displayName.includes(lowerQuery) || nickname.includes(lowerQuery)) {
			results.push(member);
		}
	}
	return results;
}

function filterRoles(roles: Map<string, RoleLike>, query: string, canManageRoles: boolean, limit = 10): RoleLike[] {
	const lowerQuery = query.toLowerCase();
	const results: RoleLike[] = [];
	for (const [, role] of roles) {
		if (results.length >= limit) break;
		if (!role.mentionable && !canManageRoles) continue;
		if (role.name.toLowerCase().includes(lowerQuery)) {
			results.push(role);
		}
	}
	return results;
}

function shouldShowHere(query: string): boolean {
	return 'here'.startsWith(query.toLowerCase()) || query === '';
}

function buildItems(
	showHere: boolean,
	members: MemberLike[],
	roles: RoleLike[]
): AutocompleteItem[] {
	const list: AutocompleteItem[] = [];
	if (showHere) {
		list.push({ type: 'here', id: 'here', label: '@here', sublabel: 'Notify all channel members' });
	}
	for (const member of members) {
		const displayName = member.nickname ?? member.user?.display_name ?? member.user?.username ?? 'Unknown';
		const username = member.user?.username ?? '';
		list.push({
			type: 'user',
			id: member.user_id,
			label: displayName,
			sublabel: username !== displayName ? username : undefined
		});
	}
	for (const role of roles) {
		list.push({
			type: 'role',
			id: role.id,
			label: role.name,
			color: role.color ?? undefined
		});
	}
	return list;
}

const testMembers = new Map<string, MemberLike>([
	['user1', { user_id: 'user1', nickname: 'CoolNick', user: { username: 'alice', display_name: 'Alice W' } }],
	['user2', { user_id: 'user2', nickname: null, user: { username: 'bob', display_name: null } }],
	['user3', { user_id: 'user3', nickname: 'Charlie', user: { username: 'charlie123', display_name: 'Charlie' } }],
]);

const testRoles = new Map<string, RoleLike>([
	['role1', { id: 'role1', name: 'Moderator', color: '#e74c3c', mentionable: true }],
	['role2', { id: 'role2', name: 'Admin', color: '#3498db', mentionable: false }],
	['role3', { id: 'role3', name: 'Member', color: null, mentionable: true }],
]);

describe('MentionAutocomplete filtering', () => {
	describe('filterMembers', () => {
		it('returns all members for empty query', () => {
			const results = filterMembers(testMembers, '');
			expect(results).toHaveLength(3);
		});

		it('filters by username', () => {
			const results = filterMembers(testMembers, 'alice');
			expect(results).toHaveLength(1);
			expect(results[0].user_id).toBe('user1');
		});

		it('filters by nickname', () => {
			const results = filterMembers(testMembers, 'cool');
			expect(results).toHaveLength(1);
			expect(results[0].user_id).toBe('user1');
		});

		it('filters by display name', () => {
			const results = filterMembers(testMembers, 'Alice W');
			expect(results).toHaveLength(1);
			expect(results[0].user_id).toBe('user1');
		});

		it('is case insensitive', () => {
			const results = filterMembers(testMembers, 'BOB');
			expect(results).toHaveLength(1);
			expect(results[0].user_id).toBe('user2');
		});

		it('returns empty for no match', () => {
			const results = filterMembers(testMembers, 'zzz');
			expect(results).toHaveLength(0);
		});

		it('respects limit', () => {
			const results = filterMembers(testMembers, '', 2);
			expect(results).toHaveLength(2);
		});
	});

	describe('filterRoles', () => {
		it('returns only mentionable roles for regular user', () => {
			const results = filterRoles(testRoles, '', false);
			expect(results).toHaveLength(2);
			expect(results.map(r => r.name)).toContain('Moderator');
			expect(results.map(r => r.name)).toContain('Member');
			expect(results.map(r => r.name)).not.toContain('Admin');
		});

		it('returns all roles for users with ManageRoles', () => {
			const results = filterRoles(testRoles, '', true);
			expect(results).toHaveLength(3);
		});

		it('filters by name', () => {
			const results = filterRoles(testRoles, 'mod', false);
			expect(results).toHaveLength(1);
			expect(results[0].name).toBe('Moderator');
		});

		it('is case insensitive', () => {
			const results = filterRoles(testRoles, 'MEMBER', false);
			expect(results).toHaveLength(1);
			expect(results[0].name).toBe('Member');
		});
	});

	describe('shouldShowHere', () => {
		it('shows for empty query', () => {
			expect(shouldShowHere('')).toBe(true);
		});

		it('shows for "h"', () => {
			expect(shouldShowHere('h')).toBe(true);
		});

		it('shows for "her"', () => {
			expect(shouldShowHere('her')).toBe(true);
		});

		it('shows for "here"', () => {
			expect(shouldShowHere('here')).toBe(true);
		});

		it('hides for "herex"', () => {
			expect(shouldShowHere('herex')).toBe(false);
		});

		it('hides for "admin"', () => {
			expect(shouldShowHere('admin')).toBe(false);
		});
	});

	describe('buildItems', () => {
		it('puts @here first', () => {
			const items = buildItems(true, [], []);
			expect(items).toHaveLength(1);
			expect(items[0].type).toBe('here');
			expect(items[0].label).toBe('@here');
		});

		it('orders: @here, then users, then roles', () => {
			const members = [testMembers.get('user1')!];
			const roles = [testRoles.get('role1')!];
			const items = buildItems(true, members, roles);
			expect(items).toHaveLength(3);
			expect(items[0].type).toBe('here');
			expect(items[1].type).toBe('user');
			expect(items[2].type).toBe('role');
		});

		it('uses nickname as label when available', () => {
			const members = [testMembers.get('user1')!];
			const items = buildItems(false, members, []);
			expect(items[0].label).toBe('CoolNick');
			expect(items[0].sublabel).toBe('alice');
		});

		it('uses username as label when no nickname or display_name', () => {
			const members = [testMembers.get('user2')!];
			const items = buildItems(false, members, []);
			expect(items[0].label).toBe('bob');
			expect(items[0].sublabel).toBeUndefined(); // username === label
		});

		it('includes role color', () => {
			const roles = [testRoles.get('role1')!];
			const items = buildItems(false, [], roles);
			expect(items[0].color).toBe('#e74c3c');
		});
	});
});
