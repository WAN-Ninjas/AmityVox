// DM store — manages direct message channels.

import { writable, derived } from 'svelte/store';
import type { Channel, User } from '$lib/types';
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
		dmChannels.update((existing) => {
			const merged = new Map(existing);
			for (const ch of list) {
				merged.set(ch.id, ch);
			}
			return merged;
		});
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

/**
 * Update a user's data across all DM channel recipients (e.g. when they change avatar/name).
 */
export function updateUserInDMs(user: User) {
	dmChannels.update((map) => {
		let changed = false;
		for (const [id, ch] of map) {
			if (!ch.recipients) continue;
			const idx = ch.recipients.findIndex((r) => r.id === user.id);
			if (idx >= 0) {
				const updated = { ...ch, recipients: [...ch.recipients] };
				updated.recipients[idx] = user;
				map.set(id, updated);
				changed = true;
			}
		}
		return changed ? new Map(map) : map;
	});
}
