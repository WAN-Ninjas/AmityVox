// Permissions store — caches computed guild-level permissions for the current user.

import { derived } from 'svelte/store';
import { api } from '$lib/api/client';
import { currentGuildId } from './guilds';
import { Permission, hasPermission } from '$lib/types';
import { createMapStore } from '$lib/stores/mapHelpers';

export const guildPermissions = createMapStore<string, bigint>();

export async function loadPermissions(guildId: string) {
	try {
		const result = await api.getMyPermissions(guildId);
		const perms = BigInt(result.permissions);
		guildPermissions.setEntry(guildId, perms);
	} catch {
		// Fail-closed: on error, don't cache anything (defaults to 0n = no permissions).
	}
}

export function invalidatePermissions(guildId: string) {
	guildPermissions.removeEntry(guildId);
}

export function clearPermissions() {
	guildPermissions.clear();
}

export const currentGuildPermissions = derived(
	[guildPermissions, currentGuildId],
	([$perms, $guildId]) => ($guildId ? $perms.get($guildId) ?? 0n : 0n)
);

// Convenience derived stores for common permission checks.
export const canManageChannels = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ManageChannels));
export const canManageGuild = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ManageGuild));
export const canManageMessages = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ManageMessages));
export const canManageThreads = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ManageThreads));
export const canCreateThreads = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.CreateThreads));
export const canKickMembers = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.KickMembers));
export const canBanMembers = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.BanMembers));
export const canManageRoles = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ManageRoles));
export const canTimeoutMembers = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.TimeoutMembers));
export const canAssignRoles = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.AssignRoles));
export const canViewAuditLog = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.ViewAuditLog));
export const isAdministrator = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.Administrator));
export const canMentionHere = derived(currentGuildPermissions, ($p) => hasPermission($p, Permission.MentionHere));
