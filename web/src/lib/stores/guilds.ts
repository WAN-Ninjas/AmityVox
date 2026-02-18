// Guild store — manages guild list and current guild selection.
// Supports both local guilds and federated (remote) guilds.

import { writable, derived, get } from 'svelte/store';
import type { Guild, FederatedGuild } from '$lib/types';
import { api } from '$lib/api/client';

export const guilds = writable<Map<string, Guild>>(new Map());
export const currentGuildId = writable<string | null>(null);
export const guildList = derived(guilds, ($guilds) => Array.from($guilds.values()));
export const currentGuild = derived(
	[guilds, currentGuildId],
	([$guilds, $id]) => ($id ? $guilds.get($id) ?? null : null)
);

/** Federated guild metadata from the READY payload, keyed by guild_id. */
export const federatedGuilds = writable<Map<string, FederatedGuild>>(new Map());

/** Set of guild IDs that are federated (remote). */
export const federatedGuildIds = derived(federatedGuilds, ($fg) => new Set($fg.keys()));

/** Returns true if the given guild ID is a federated (remote) guild. */
export function isFederatedGuild(guildId: string): boolean {
	return get(federatedGuilds).has(guildId);
}

export async function loadGuilds() {
	const list = await api.getMyGuilds();
	const map = new Map<string, Guild>();
	for (const g of list) {
		map.set(g.id, g);
	}
	guilds.set(map);
}

/** Load federated guilds from the READY payload. */
export function loadFederatedGuilds(fgs: FederatedGuild[]) {
	const map = new Map<string, FederatedGuild>();
	for (const fg of fgs) {
		map.set(fg.guild_id, fg);
	}
	federatedGuilds.set(map);
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

export function removeFederatedGuild(guildId: string) {
	federatedGuilds.update((map) => {
		map.delete(guildId);
		return new Map(map);
	});
}
