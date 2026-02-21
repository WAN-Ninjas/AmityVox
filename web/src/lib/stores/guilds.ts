// Guild store — manages guild list and current guild selection.

import { writable, derived } from 'svelte/store';
import type { Guild } from '$lib/types';
import { api } from '$lib/api/client';
import { createMapStore } from '$lib/stores/mapHelpers';

export const guilds = createMapStore<string, Guild>();
export const currentGuildId = writable<string | null>(null);
export const guildList = derived(guilds, ($guilds) => Array.from($guilds.values()));
export const currentGuild = derived(
	[guilds, currentGuildId],
	([$guilds, $id]) => ($id ? $guilds.get($id) ?? null : null)
);

/** Returns true if the given guild is federated (belongs to a remote instance).
 *  Guilds with null/undefined instance_id are always local. */
export function isGuildFederated(guild: Guild, localInstanceId: string): boolean {
	if (!guild.instance_id) return false;
	return guild.instance_id !== localInstanceId;
}

export async function loadGuilds() {
	const list = await api.getMyGuilds();
	guilds.setAll(list.map(g => [g.id, g]));
}

export function setGuild(id: string | null) {
	currentGuildId.set(id);
	// Dynamic import to avoid circular dependency (permissions.ts imports currentGuildId from this module).
	if (id) import('./permissions').then(({ loadPermissions }) => loadPermissions(id));
}

export function updateGuild(guild: Guild) {
	guilds.setEntry(guild.id, guild);
}

export function removeGuild(guildId: string) {
	guilds.removeEntry(guildId);
}
