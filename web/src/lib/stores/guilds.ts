// Guild store — manages guild list and current guild selection.

import { writable, derived } from 'svelte/store';
import type { Guild } from '$lib/types';
import { api } from '$lib/api/client';

export const guilds = writable<Map<string, Guild>>(new Map());
export const currentGuildId = writable<string | null>(null);
export const guildList = derived(guilds, ($guilds) => Array.from($guilds.values()));
export const currentGuild = derived(
	[guilds, currentGuildId],
	([$guilds, $id]) => ($id ? $guilds.get($id) ?? null : null)
);

export async function loadGuilds() {
	const list = await api.getMyGuilds();
	const map = new Map<string, Guild>();
	for (const g of list) {
		map.set(g.id, g);
	}
	guilds.set(map);
}

export function setGuild(id: string | null) {
	currentGuildId.set(id);
	// Dynamic import to avoid circular dependency (permissions.ts imports currentGuildId from this module).
	if (id) import('./permissions').then(({ loadPermissions }) => loadPermissions(id));
}

export function updateGuild(guild: Guild) {
	guilds.update((map) => {
		map.set(guild.id, guild);
		return new Map(map);
	});
}

export function removeGuild(guildId: string) {
	guilds.update((map) => {
		map.delete(guildId);
		return new Map(map);
	});
}
