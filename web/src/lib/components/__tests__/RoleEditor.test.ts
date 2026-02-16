import { describe, it, expect } from 'vitest';

/**
 * Tests for RoleEditor permission toggle logic.
 * Since Svelte 5 components can't be rendered in happy-dom (SSR mode),
 * we test the pure bitfield operations as standalone functions.
 */

// --- Permission helpers (replicated from RoleEditor.svelte) ---

function permState(allow: bigint, deny: bigint, bit: bigint): 'allow' | 'deny' | 'neutral' {
	if ((allow & bit) === bit) return 'allow';
	if ((deny & bit) === bit) return 'deny';
	return 'neutral';
}

function toggleAllow(allow: bigint, deny: bigint, bit: bigint): { allow: bigint; deny: bigint } {
	if ((allow & bit) === bit) {
		return { allow: allow & ~bit, deny };
	}
	return { allow: allow | bit, deny: deny & ~bit };
}

function toggleDeny(allow: bigint, deny: bigint, bit: bigint): { allow: bigint; deny: bigint } {
	if ((deny & bit) === bit) {
		return { allow, deny: deny & ~bit };
	}
	return { allow: allow & ~bit, deny: deny | bit };
}

// --- Permission bit definitions (must match types/index.ts) ---

const Permission = {
	ManageChannels:    1n << 0n,
	ManageGuild:       1n << 1n,
	ManagePermissions: 1n << 2n,
	ManageRoles:       1n << 3n,
	ManageEmoji:       1n << 4n,
	ManageWebhooks:    1n << 5n,
	KickMembers:       1n << 6n,
	BanMembers:        1n << 7n,
	TimeoutMembers:    1n << 8n,
	AssignRoles:       1n << 9n,
	ChangeNickname:    1n << 10n,
	ManageNicknames:   1n << 11n,
	ChangeAvatar:      1n << 12n,
	RemoveAvatars:     1n << 13n,
	ViewAuditLog:      1n << 14n,
	ViewGuildInsights: 1n << 15n,
	MentionEveryone:   1n << 16n,
	ViewChannel:       1n << 20n,
	ReadHistory:       1n << 21n,
	SendMessages:      1n << 22n,
	ManageMessages:    1n << 23n,
	EmbedLinks:        1n << 24n,
	UploadFiles:       1n << 25n,
	AddReactions:      1n << 26n,
	UseExternalEmoji:  1n << 27n,
	Connect:           1n << 28n,
	Speak:             1n << 29n,
	MuteMembers:       1n << 30n,
	DeafenMembers:     1n << 31n,
	MoveMembers:       1n << 32n,
	UseVAD:            1n << 33n,
	PrioritySpeaker:   1n << 34n,
	Stream:            1n << 35n,
	Masquerade:        1n << 36n,
	CreateInvites:     1n << 37n,
	ManageThreads:     1n << 38n,
	CreateThreads:     1n << 39n,
	Administrator:     1n << 63n,
} as const;

// --- Permission groups from RoleEditor (for completeness check) ---

const permissionGroupKeys = [
	// Server
	'ManageGuild', 'ManageChannels', 'ManageEmoji', 'ManageWebhooks', 'CreateInvites',
	// Members
	'KickMembers', 'BanMembers', 'TimeoutMembers', 'ManageRoles', 'AssignRoles', 'ManageNicknames', 'RemoveAvatars',
	// Information
	'ViewAuditLog', 'ViewGuildInsights', 'MentionEveryone', 'ManagePermissions',
	// Channel
	'ViewChannel', 'ReadHistory', 'SendMessages', 'ManageMessages', 'EmbedLinks',
	'UploadFiles', 'AddReactions', 'UseExternalEmoji', 'Masquerade', 'ManageThreads', 'CreateThreads',
	// Voice
	'Connect', 'Speak', 'MuteMembers', 'DeafenMembers', 'MoveMembers', 'UseVAD', 'PrioritySpeaker', 'Stream',
	// Special
	'ChangeNickname', 'ChangeAvatar', 'Administrator',
];

// --- Tests ---

describe('permState', () => {
	it('returns neutral when bit is not set in either allow or deny', () => {
		expect(permState(0n, 0n, Permission.ManageGuild)).toBe('neutral');
	});

	it('returns allow when bit is set in allow', () => {
		expect(permState(Permission.ManageGuild, 0n, Permission.ManageGuild)).toBe('allow');
	});

	it('returns deny when bit is set in deny', () => {
		expect(permState(0n, Permission.ManageGuild, Permission.ManageGuild)).toBe('deny');
	});

	it('correctly identifies bits in a multi-bit value', () => {
		const allow = Permission.ManageGuild | Permission.KickMembers;
		const deny = Permission.BanMembers;
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('allow');
		expect(permState(allow, deny, Permission.KickMembers)).toBe('allow');
		expect(permState(allow, deny, Permission.BanMembers)).toBe('deny');
		expect(permState(allow, deny, Permission.ManageEmoji)).toBe('neutral');
	});

	it('works with high bits like Administrator (bit 63)', () => {
		const allow = Permission.Administrator;
		expect(permState(allow, 0n, Permission.Administrator)).toBe('allow');
		expect(permState(0n, allow, Permission.Administrator)).toBe('deny');
	});
});

describe('toggleAllow', () => {
	it('sets allow bit when neutral', () => {
		const result = toggleAllow(0n, 0n, Permission.ManageGuild);
		expect(result.allow & Permission.ManageGuild).toBe(Permission.ManageGuild);
		expect(result.deny & Permission.ManageGuild).toBe(0n);
	});

	it('clears allow bit when already allowed (back to neutral)', () => {
		const result = toggleAllow(Permission.ManageGuild, 0n, Permission.ManageGuild);
		expect(result.allow & Permission.ManageGuild).toBe(0n);
		expect(result.deny & Permission.ManageGuild).toBe(0n);
	});

	it('sets allow and clears deny when denied', () => {
		const result = toggleAllow(0n, Permission.ManageGuild, Permission.ManageGuild);
		expect(result.allow & Permission.ManageGuild).toBe(Permission.ManageGuild);
		expect(result.deny & Permission.ManageGuild).toBe(0n);
	});

	it('does not affect other bits', () => {
		const allow = Permission.KickMembers;
		const deny = Permission.BanMembers;
		const result = toggleAllow(allow, deny, Permission.ManageGuild);
		expect(result.allow & Permission.KickMembers).toBe(Permission.KickMembers);
		expect(result.deny & Permission.BanMembers).toBe(Permission.BanMembers);
	});
});

describe('toggleDeny', () => {
	it('sets deny bit when neutral', () => {
		const result = toggleDeny(0n, 0n, Permission.ManageGuild);
		expect(result.deny & Permission.ManageGuild).toBe(Permission.ManageGuild);
		expect(result.allow & Permission.ManageGuild).toBe(0n);
	});

	it('clears deny bit when already denied (back to neutral)', () => {
		const result = toggleDeny(0n, Permission.ManageGuild, Permission.ManageGuild);
		expect(result.deny & Permission.ManageGuild).toBe(0n);
		expect(result.allow & Permission.ManageGuild).toBe(0n);
	});

	it('sets deny and clears allow when allowed', () => {
		const result = toggleDeny(Permission.ManageGuild, 0n, Permission.ManageGuild);
		expect(result.deny & Permission.ManageGuild).toBe(Permission.ManageGuild);
		expect(result.allow & Permission.ManageGuild).toBe(0n);
	});

	it('does not affect other bits', () => {
		const allow = Permission.KickMembers;
		const deny = Permission.BanMembers;
		const result = toggleDeny(allow, deny, Permission.ManageGuild);
		expect(result.allow & Permission.KickMembers).toBe(Permission.KickMembers);
		expect(result.deny & Permission.BanMembers).toBe(Permission.BanMembers);
	});
});

describe('toggle cycle', () => {
	it('neutral -> allow -> neutral when toggling allow twice', () => {
		let { allow, deny } = toggleAllow(0n, 0n, Permission.ManageGuild);
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('allow');

		({ allow, deny } = toggleAllow(allow, deny, Permission.ManageGuild));
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('neutral');
	});

	it('neutral -> deny -> neutral when toggling deny twice', () => {
		let { allow, deny } = toggleDeny(0n, 0n, Permission.ManageGuild);
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('deny');

		({ allow, deny } = toggleDeny(allow, deny, Permission.ManageGuild));
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('neutral');
	});

	it('allow -> deny when toggling deny after allow', () => {
		let { allow, deny } = toggleAllow(0n, 0n, Permission.ManageGuild);
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('allow');

		({ allow, deny } = toggleDeny(allow, deny, Permission.ManageGuild));
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('deny');
	});

	it('deny -> allow when toggling allow after deny', () => {
		let { allow, deny } = toggleDeny(0n, 0n, Permission.ManageGuild);
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('deny');

		({ allow, deny } = toggleAllow(allow, deny, Permission.ManageGuild));
		expect(permState(allow, deny, Permission.ManageGuild)).toBe('allow');
	});
});

describe('permission group completeness', () => {
	it('every permission in the Permission object is represented in the groups', () => {
		const allKeys = Object.keys(Permission);
		for (const key of allKeys) {
			expect(permissionGroupKeys).toContain(key);
		}
	});

	it('every group key exists in the Permission object', () => {
		for (const key of permissionGroupKeys) {
			expect(Permission).toHaveProperty(key);
		}
	});

	it('group keys have no duplicates', () => {
		const unique = new Set(permissionGroupKeys);
		expect(unique.size).toBe(permissionGroupKeys.length);
	});
});

describe('BigInt to Number conversion safety', () => {
	it('all permission bits except Administrator fit safely in Number', () => {
		for (const [key, bit] of Object.entries(Permission)) {
			if (key === 'Administrator') continue;
			expect(bit).toBeLessThanOrEqual(BigInt(Number.MAX_SAFE_INTEGER));
		}
	});

	it('combined allow/deny bitfield with all non-admin perms fits in Number', () => {
		let combined = 0n;
		for (const [key, bit] of Object.entries(Permission)) {
			if (key === 'Administrator') continue;
			combined |= bit;
		}
		expect(combined).toBeLessThanOrEqual(BigInt(Number.MAX_SAFE_INTEGER));
	});
});
