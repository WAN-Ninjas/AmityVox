import { describe, it, expect } from 'vitest';
import { getMemberRoleColor } from '../roleColor';
import type { Role } from '$lib/types';

function makeRole(id: string, position: number, color: string | null): Role {
	return {
		id,
		guild_id: 'g1',
		name: `Role ${id}`,
		color,
		hoist: false,
		mentionable: false,
		position,
		permissions_allow: '0',
		permissions_deny: '0',
		created_at: new Date().toISOString(),
	};
}

describe('getMemberRoleColor', () => {
	it('returns null when member has no roles', () => {
		const roleMap = new Map<string, Role>();
		expect(getMemberRoleColor([], roleMap)).toBeNull();
		expect(getMemberRoleColor(undefined, roleMap)).toBeNull();
	});

	it('returns null when no roles have colors', () => {
		const roleMap = new Map<string, Role>();
		roleMap.set('r1', makeRole('r1', 1, null));
		roleMap.set('r2', makeRole('r2', 2, null));
		expect(getMemberRoleColor(['r1', 'r2'], roleMap)).toBeNull();
	});

	it('returns the color of a single colored role', () => {
		const roleMap = new Map<string, Role>();
		roleMap.set('r1', makeRole('r1', 1, '#ff0000'));
		expect(getMemberRoleColor(['r1'], roleMap)).toBe('#ff0000');
	});

	it('returns the color of the highest-position role', () => {
		const roleMap = new Map<string, Role>();
		roleMap.set('r1', makeRole('r1', 1, '#ff0000'));
		roleMap.set('r2', makeRole('r2', 5, '#00ff00'));
		roleMap.set('r3', makeRole('r3', 3, '#0000ff'));
		expect(getMemberRoleColor(['r1', 'r2', 'r3'], roleMap)).toBe('#00ff00');
	});

	it('skips colorless roles when finding highest', () => {
		const roleMap = new Map<string, Role>();
		roleMap.set('r1', makeRole('r1', 1, '#ff0000'));
		roleMap.set('r2', makeRole('r2', 10, null)); // highest but no color
		roleMap.set('r3', makeRole('r3', 5, '#0000ff'));
		expect(getMemberRoleColor(['r1', 'r2', 'r3'], roleMap)).toBe('#0000ff');
	});

	it('ignores roles not in the roleMap', () => {
		const roleMap = new Map<string, Role>();
		roleMap.set('r1', makeRole('r1', 1, '#ff0000'));
		expect(getMemberRoleColor(['r1', 'r_missing'], roleMap)).toBe('#ff0000');
	});
});
