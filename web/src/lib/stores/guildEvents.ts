import { writable } from 'svelte/store';
import type { GuildEvent } from '$lib/types';
import { api } from '$lib/api/client';

export const guildEventsByGuild = writable<Map<string, GuildEvent[]>>(new Map());

let loadRequestSequence = 0;
const latestLoadByGuild = new Map<string, number>();

export async function loadGuildEvents(guildId: string) {
	const requestId = ++loadRequestSequence;
	latestLoadByGuild.set(guildId, requestId);

	const events = await api.getGuildEvents(guildId, { status: 'scheduled', limit: 5 });
	if (latestLoadByGuild.get(guildId) !== requestId) return;

	guildEventsByGuild.update((map) => {
		const next = new Map(map);
		next.set(guildId, sortUpcomingEvents(events));
		return next;
	});
}

export function upsertGuildEvent(event: GuildEvent) {
	guildEventsByGuild.update((map) => {
		const next = new Map(map);
		const existing = next.get(event.guild_id) ?? [];
		const withoutEvent = existing.filter((item) => item.id !== event.id);

		if (event.status === 'scheduled') {
			next.set(event.guild_id, sortUpcomingEvents([...withoutEvent, event]).slice(0, 5));
		} else {
			next.set(event.guild_id, withoutEvent);
		}

		return next;
	});
}

export function removeGuildEvent(guildId: string, eventId: string) {
	guildEventsByGuild.update((map) => {
		const existing = map.get(guildId);
		if (!existing) return map;

		const next = new Map(map);
		next.set(guildId, existing.filter((event) => event.id !== eventId));
		return next;
	});
}

function sortUpcomingEvents(events: GuildEvent[]) {
	return [...events].sort((a, b) =>
		new Date(a.scheduled_start).getTime() - new Date(b.scheduled_start).getTime()
	);
}
