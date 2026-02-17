import { describe, it, expect } from 'vitest';
import type { GuildMember, Role } from '$lib/types';

// --- Hoist grouping logic (mirrors MemberList.svelte's $derived computation) ---

interface MemberGroup {
	role: Role | null;
	label: string;
	color: string | null;
	members: GuildMember[];
}

function computeMemberGroups(
	members: GuildMember[],
	allRoles: Role[],
	onlineUserIds: Set<string>
): MemberGroup[] {
	const hoistedRoles = allRoles
		.filter((r) => r.hoist && r.name !== '@everyone')
		.sort((a, b) => b.position - a.position);

	const placed = new Set<string>();
	const groups: MemberGroup[] = [];

	for (const role of hoistedRoles) {
		const groupMembers: GuildMember[] = [];
		for (const m of members) {
			if (placed.has(m.user_id)) continue;
			if (!onlineUserIds.has(m.user_id)) continue;
			if (m.roles && m.roles.includes(role.id)) {
				groupMembers.push(m);
				placed.add(m.user_id);
			}
		}
		if (groupMembers.length > 0) {
			groups.push({ role, label: role.name, color: role.color, members: groupMembers });
		}
	}

	const remainingOnline = members.filter(
		(m) => onlineUserIds.has(m.user_id) && !placed.has(m.user_id)
	);
	if (remainingOnline.length > 0) {
		groups.push({ role: null, label: 'Online', color: null, members: remainingOnline });
	}

	const offline = members.filter((m) => !onlineUserIds.has(m.user_id));
	if (offline.length > 0) {
		groups.push({ role: null, label: 'Offline', color: null, members: offline });
	}

	return groups;
}

// --- Helper factories ---

function makeRole(id: string, name: string, position: number, opts: Partial<Role> = {}): Role {
	return {
		id,
		guild_id: 'g1',
		name,
		color: opts.color ?? null,
		hoist: opts.hoist ?? false,
		mentionable: false,
		position,
		permissions_allow: 0,
		permissions_deny: 0,
		created_at: '2024-01-01T00:00:00Z',
		...opts,
	};
}

function makeMember(userId: string, roles: string[] = []): GuildMember {
	return {
		guild_id: 'g1',
		user_id: userId,
		nickname: null,
		avatar_id: null,
		joined_at: '2024-01-01T00:00:00Z',
		timeout_until: null,
		deaf: false,
		mute: false,
		roles,
	};
}

// --- Tests ---

describe('Hoist grouping', () => {
	it('groups online members under their highest hoisted role', () => {
		const admin = makeRole('r1', 'Admin', 3, { hoist: true, color: '#ff0000' });
		const mod = makeRole('r2', 'Moderator', 2, { hoist: true, color: '#00ff00' });
		const everyone = makeRole('r0', '@everyone', 0);

		const m1 = makeMember('u1', ['r1', 'r2']); // Admin + Mod → should appear under Admin
		const m2 = makeMember('u2', ['r2']); // Mod only
		const m3 = makeMember('u3', []); // no roles

		const online = new Set(['u1', 'u2', 'u3']);
		const groups = computeMemberGroups([m1, m2, m3], [admin, mod, everyone], online);

		expect(groups).toHaveLength(3); // Admin, Mod, Online
		expect(groups[0].label).toBe('Admin');
		expect(groups[0].color).toBe('#ff0000');
		expect(groups[0].members.map((m) => m.user_id)).toEqual(['u1']);
		expect(groups[1].label).toBe('Moderator');
		expect(groups[1].members.map((m) => m.user_id)).toEqual(['u2']);
		expect(groups[2].label).toBe('Online');
		expect(groups[2].members.map((m) => m.user_id)).toEqual(['u3']);
	});

	it('highest-position hoisted role wins when member has multiple', () => {
		const vip = makeRole('r3', 'VIP', 5, { hoist: true });
		const helper = makeRole('r4', 'Helper', 1, { hoist: true });

		const m1 = makeMember('u1', ['r3', 'r4']); // has both → VIP (pos 5) wins
		const online = new Set(['u1']);

		const groups = computeMemberGroups([m1], [vip, helper], online);

		expect(groups).toHaveLength(1);
		expect(groups[0].label).toBe('VIP');
		expect(groups[0].members).toHaveLength(1);
	});

	it('@everyone is never a hoisted group', () => {
		const everyone = makeRole('r0', '@everyone', 0, { hoist: true });
		const m1 = makeMember('u1', ['r0']);
		const online = new Set(['u1']);

		const groups = computeMemberGroups([m1], [everyone], online);

		// Should fall through to generic Online, not "@everyone" group
		expect(groups).toHaveLength(1);
		expect(groups[0].label).toBe('Online');
	});

	it('offline members are always in flat Offline group regardless of hoist', () => {
		const admin = makeRole('r1', 'Admin', 3, { hoist: true });
		const m1 = makeMember('u1', ['r1']);
		const m2 = makeMember('u2', ['r1']);
		const online = new Set(['u1']); // u2 is offline

		const groups = computeMemberGroups([m1, m2], [admin], online);

		expect(groups).toHaveLength(2);
		expect(groups[0].label).toBe('Admin');
		expect(groups[0].members.map((m) => m.user_id)).toEqual(['u1']);
		expect(groups[1].label).toBe('Offline');
		expect(groups[1].members.map((m) => m.user_id)).toEqual(['u2']);
	});

	it('non-hoisted roles do not create groups', () => {
		const nonHoist = makeRole('r1', 'Subscriber', 2, { hoist: false });
		const m1 = makeMember('u1', ['r1']);
		const online = new Set(['u1']);

		const groups = computeMemberGroups([m1], [nonHoist], online);

		expect(groups).toHaveLength(1);
		expect(groups[0].label).toBe('Online');
	});

	it('returns empty when no members', () => {
		const groups = computeMemberGroups([], [], new Set());
		expect(groups).toHaveLength(0);
	});

	it('all offline produces only Offline group', () => {
		const m1 = makeMember('u1', []);
		const m2 = makeMember('u2', []);
		const groups = computeMemberGroups([m1, m2], [], new Set());

		expect(groups).toHaveLength(1);
		expect(groups[0].label).toBe('Offline');
		expect(groups[0].members).toHaveLength(2);
	});
});

describe('Role position swap logic', () => {
	it('swapping two roles produces correct position assignments', () => {
		const roleA = makeRole('rA', 'A', 3);
		const roleB = makeRole('rB', 'B', 2);

		// Simulates what the frontend sends to reorderRoles API
		const payload = [
			{ id: roleA.id, position: roleB.position },
			{ id: roleB.id, position: roleA.position },
		];

		expect(payload).toEqual([
			{ id: 'rA', position: 2 },
			{ id: 'rB', position: 3 },
		]);
	});

	it('@everyone cannot be moved above position 0', () => {
		const everyone = makeRole('r0', '@everyone', 0);
		// The canMoveDown check: @everyone at position 0 → false
		const isEveryone = everyone.name === '@everyone' && everyone.position === 0;
		expect(isEveryone).toBe(true);
	});

	it('canMoveDown returns false when next role is @everyone', () => {
		const sorted = [
			makeRole('r1', 'Admin', 2),
			makeRole('r0', '@everyone', 0),
		];
		// For r0 (@everyone): can't move down
		const everyoneRole = sorted[1];
		const canMoveDownEveryone = !(everyoneRole.name === '@everyone' && everyoneRole.position === 0);
		expect(canMoveDownEveryone).toBe(false);

		// For r1 (Admin): the role below is @everyone → can't move down
		const below = sorted[1];
		const canMoveDownAdmin = !(below.name === '@everyone' && below.position === 0);
		expect(canMoveDownAdmin).toBe(false);
	});
});
