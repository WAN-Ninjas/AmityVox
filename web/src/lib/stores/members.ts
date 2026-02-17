// Members store -- tracks guild member metadata (timeouts, etc.) for the current guild.

import { writable, derived } from 'svelte/store';
import type { GuildMember, Role, User } from '$lib/types';

// Map of user_id -> GuildMember for the current guild.
export const guildMembers = writable<Map<string, GuildMember>>(new Map());

// Map of role_id -> Role for the current guild (set alongside members).
export const guildRolesMap = writable<Map<string, Role>>(new Map());

/**
 * Set the full member list (called by MemberList when loading members).
 */
export function setGuildMembers(members: GuildMember[]) {
	const map = new Map<string, GuildMember>();
	for (const m of members) {
		map.set(m.user_id, m);
	}
	guildMembers.set(map);
}

/**
 * Set the guild roles map (called by MemberList when loading roles).
 */
export function setGuildRoles(roles: Role[]) {
	const map = new Map<string, Role>();
	for (const r of roles) {
		map.set(r.id, r);
	}
	guildRolesMap.set(map);
}

/**
 * Update a single guild member's data in the store (e.g. when roles change via websocket).
 */
export function updateGuildMember(userId: string, updates: Partial<GuildMember>) {
	guildMembers.update((map) => {
		const existing = map.get(userId);
		if (!existing) return map;
		map.set(userId, { ...existing, ...updates });
		return new Map(map);
	});
}

/**
 * Update the embedded user object for a member (e.g. when the user changes their avatar/display name).
 */
export function updateUserInMembers(user: User) {
	guildMembers.update((map) => {
		const existing = map.get(user.id);
		if (!existing) return map;
		map.set(user.id, { ...existing, user });
		return new Map(map);
	});
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
