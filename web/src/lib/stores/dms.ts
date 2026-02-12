// DM store — manages direct message channels.

import { writable, derived } from 'svelte/store';
import type { Channel } from '$lib/types';
import { api } from '$lib/api/client';

export const dmChannels = writable<Map<string, Channel>>(new Map());
export const dmLoaded = writable(false);

export const dmList = derived(dmChannels, ($dms) =>
	Array.from($dms.values()).sort((a, b) => {
		// Sort by last_message_id descending (most recent first).
		// ULIDs sort lexicographically by time.
		const aId = a.last_message_id ?? a.id;
		const bId = b.last_message_id ?? b.id;
		return bId.localeCompare(aId);
	})
);

export async function loadDMs() {
	try {
		const list = await api.getMyDMs();
		const map = new Map<string, Channel>();
		for (const ch of list) {
			map.set(ch.id, ch);
		}
		dmChannels.set(map);
		dmLoaded.set(true);
	} catch {
		// Silently fail — DMs may not be available yet.
	}
}

export function addDMChannel(channel: Channel) {
	dmChannels.update((map) => {
		map.set(channel.id, channel);
		return new Map(map);
	});
}

export function removeDMChannel(channelId: string) {
	dmChannels.update((map) => {
		map.delete(channelId);
		return new Map(map);
	});
}

export function updateDMChannel(channel: Channel) {
	dmChannels.update((map) => {
		if (map.has(channel.id)) {
			map.set(channel.id, channel);
			return new Map(map);
		}
		return map;
	});
}
