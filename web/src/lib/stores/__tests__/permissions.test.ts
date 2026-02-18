import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getMyPermissions: vi.fn()
	}
}));

// Mock the guilds store to provide currentGuildId.
vi.mock('../guilds', () => {
	const { writable } = require('svelte/store');
	return {
		currentGuildId: writable<string | null>(null)
	};
});

import {
	guildPermissions,
	loadPermissions,
	invalidatePermissions,
	clearPermissions,
	currentGuildPermissions,
	canManageChannels,
	canManageGuild,
	canManageMessages,
	canManageThreads,
	canCreateThreads,
	canKickMembers,
	canBanMembers,
	canManageRoles,
	canViewAuditLog,
	isAdministrator
} from '../permissions';
import { currentGuildId } from '../guilds';
import { api } from '$lib/api/client';
import { Permission, hasPermission } from '$lib/types';

describe('permissions store', () => {
	beforeEach(() => {
		guildPermissions.set(new Map());
		currentGuildId.set(null);
		vi.clearAllMocks();
	});

	it('starts with empty permissions map', () => {
		expect(get(guildPermissions).size).toBe(0);
	});

	it('loadPermissions fetches and caches correctly', async () => {
		const perms = Permission.ManageChannels | Permission.ManageGuild;
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: perms.toString()
		});

		await loadPermissions('guild-1');

		const map = get(guildPermissions);
		expect(map.size).toBe(1);
		expect(map.get('guild-1')).toBe(perms);
		expect(api.getMyPermissions).toHaveBeenCalledWith('guild-1');
	});

	it('invalidatePermissions removes cached entry', async () => {
		const perms = Permission.ManageChannels;
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: perms.toString()
		});

		await loadPermissions('guild-1');
		expect(get(guildPermissions).has('guild-1')).toBe(true);

		invalidatePermissions('guild-1');
		expect(get(guildPermissions).has('guild-1')).toBe(false);
	});

	it('clearPermissions clears all entries', async () => {
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: '1'
		});

		await loadPermissions('guild-1');
		await loadPermissions('guild-2');
		expect(get(guildPermissions).size).toBe(2);

		clearPermissions();
		expect(get(guildPermissions).size).toBe(0);
	});

	it('currentGuildPermissions derives from current guild', async () => {
		const perms = Permission.ManageMessages | Permission.ManageChannels;
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: perms.toString()
		});

		await loadPermissions('guild-1');
		expect(get(currentGuildPermissions)).toBe(0n);

		currentGuildId.set('guild-1');
		expect(get(currentGuildPermissions)).toBe(perms);
	});

	it('currentGuildPermissions returns 0n for unknown guild', () => {
		currentGuildId.set('nonexistent');
		expect(get(currentGuildPermissions)).toBe(0n);
	});

	it('convenience stores return correct values when permissions are set', async () => {
		const perms = Permission.ManageChannels | Permission.ManageGuild |
			Permission.ManageMessages | Permission.ManageThreads |
			Permission.CreateThreads | Permission.KickMembers |
			Permission.BanMembers | Permission.ManageRoles |
			Permission.ViewAuditLog;
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: perms.toString()
		});

		await loadPermissions('guild-1');
		currentGuildId.set('guild-1');

		expect(get(canManageChannels)).toBe(true);
		expect(get(canManageGuild)).toBe(true);
		expect(get(canManageMessages)).toBe(true);
		expect(get(canManageThreads)).toBe(true);
		expect(get(canCreateThreads)).toBe(true);
		expect(get(canKickMembers)).toBe(true);
		expect(get(canBanMembers)).toBe(true);
		expect(get(canManageRoles)).toBe(true);
		expect(get(canViewAuditLog)).toBe(true);
		expect(get(isAdministrator)).toBe(false);
	});

	it('convenience stores return false when no permissions', () => {
		currentGuildId.set('guild-1');

		expect(get(canManageChannels)).toBe(false);
		expect(get(canManageGuild)).toBe(false);
		expect(get(canManageMessages)).toBe(false);
		expect(get(isAdministrator)).toBe(false);
	});

	it('fail-closed on API error (no cached permissions)', async () => {
		vi.mocked(api.getMyPermissions).mockRejectedValue(new Error('Network error'));

		await loadPermissions('guild-1');

		expect(get(guildPermissions).has('guild-1')).toBe(false);
		currentGuildId.set('guild-1');
		expect(get(currentGuildPermissions)).toBe(0n);
		expect(get(canManageChannels)).toBe(false);
	});

	it('isAdministrator detects bit 63', async () => {
		const perms = Permission.Administrator;
		vi.mocked(api.getMyPermissions).mockResolvedValue({
			permissions: perms.toString()
		});

		await loadPermissions('guild-1');
		currentGuildId.set('guild-1');

		expect(get(isAdministrator)).toBe(true);
	});

	describe('hasPermission helper', () => {
		it('works for low bits (0-16)', () => {
			const perms = Permission.ManageChannels | Permission.ManageGuild | Permission.MentionHere;
			expect(hasPermission(perms, Permission.ManageChannels)).toBe(true);
			expect(hasPermission(perms, Permission.ManageGuild)).toBe(true);
			expect(hasPermission(perms, Permission.MentionHere)).toBe(true);
			expect(hasPermission(perms, Permission.KickMembers)).toBe(false);
		});

		it('works for mid bits (20-39)', () => {
			const perms = Permission.ManageMessages | Permission.ManageThreads | Permission.CreateThreads;
			expect(hasPermission(perms, Permission.ManageMessages)).toBe(true);
			expect(hasPermission(perms, Permission.ManageThreads)).toBe(true);
			expect(hasPermission(perms, Permission.CreateThreads)).toBe(true);
			expect(hasPermission(perms, Permission.SendMessages)).toBe(false);
		});

		it('works for high bit 63 (Administrator)', () => {
			const perms = Permission.Administrator;
			expect(hasPermission(perms, Permission.Administrator)).toBe(true);
			expect(hasPermission(perms, Permission.ManageGuild)).toBe(false);
		});

		it('returns false for 0n permissions', () => {
			expect(hasPermission(0n, Permission.ManageChannels)).toBe(false);
			expect(hasPermission(0n, Permission.Administrator)).toBe(false);
		});
	});
});
