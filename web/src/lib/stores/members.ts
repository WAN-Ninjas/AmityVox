// Members store -- tracks guild member metadata (timeouts, etc.) for the current guild.

import { derived } from 'svelte/store';
import type { GuildMember, Role, User } from '$lib/types';
import { createMapStore } from '$lib/stores/mapHelpers';

// Map of user_id -> GuildMember for the current guild.
export const guildMembers = createMapStore<string, GuildMember>();

// Map of role_id -> Role for the current guild (set alongside members).
export const guildRolesMap = createMapStore<string, Role>();

/**
 * Set the full member list (called by MemberList when loading members).
 */
export function setGuildMembers(members: GuildMember[]) {
	guildMembers.setAll(members.map(m => [m.user_id, m]));
}

/**
 * Set the guild roles map (called by MemberList when loading roles).
 */
export function setGuildRoles(roles: Role[]) {
	guildRolesMap.setAll(roles.map(r => [r.id, r]));
}

/**
 * Update a single guild member's data in the store (e.g. when roles change via websocket).
 */
export function updateGuildMember(userId: string, updates: Partial<GuildMember>) {
	guildMembers.updateEntry(userId, existing => ({ ...existing, ...updates }));
}

/**
 * Update the embedded user object for a member (e.g. when the user changes their avatar/display name).
 */
export function updateUserInMembers(user: User) {
	guildMembers.updateEntry(user.id, existing => ({ ...existing, user }));
}

/**
 * Derived store: maps user_id -> timeout_until for members that are currently timed out.
 */
export const memberTimeouts = derived(guildMembers, ($members) => {
	const timeouts = new Map<string, string>();
	const now = Date.now();
	for (const [userId, member] of $members) {
		if (member.timeout_until && new Date(member.timeout_until).getTime() > now) {
			timeouts.set(userId, member.timeout_until);
		}
	}
	return timeouts;
});

/**
 * Check if a given user is currently timed out.
 */
export function isTimedOut(timeouts: Map<string, string>, userId: string): boolean {
	const until = timeouts.get(userId);
	if (!until) return false;
	return new Date(until).getTime() > Date.now();
}
