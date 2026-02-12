// Unreads store — tracks unread message counts and mention counts per channel.

import { writable, derived, get } from 'svelte/store';
import type { ReadState } from '$lib/types';
import { api } from '$lib/api/client';
import { currentChannelId } from './channels';

// Map of channel_id -> { lastReadId, unreadCount, mentionCount }
interface UnreadEntry {
	lastReadId: string | null;
	mentionCount: number;
}

const unreadState = writable<Map<string, UnreadEntry>>(new Map());

// Channels that have any unread messages.
// We track by comparing last_message_id on the channel with lastReadId.
// For simplicity, we track an explicit unread count.
const unreadCounts = writable<Map<string, number>>(new Map());

export { unreadCounts };

// Get unread count for a specific channel.
export function getUnreadCount(channelId: string) {
	return derived(unreadCounts, ($counts) => $counts.get(channelId) ?? 0);
}

// Get mention count for a specific channel.
export function getMentionCount(channelId: string) {
	return derived(unreadState, ($state) => $state.get(channelId)?.mentionCount ?? 0);
}

// Check if any channel in a guild has unreads.
export function hasGuildUnreads(guildId: string, channelGuildMap: Map<string, string>) {
	return derived(unreadCounts, ($counts) => {
		for (const [channelId, count] of $counts) {
			if (count > 0 && channelGuildMap.get(channelId) === guildId) return true;
		}
		return false;
	});
}

// Total unreads across all channels.
export const totalUnreads = derived(unreadCounts, ($counts) => {
	let total = 0;
	for (const count of $counts.values()) total += count;
	return total;
});

// Load read state from server.
export async function loadReadState() {
	try {
		const states = await api.getReadState();
		const map = new Map<string, UnreadEntry>();
		for (const rs of states) {
			map.set(rs.channel_id, {
				lastReadId: rs.last_message_id,
				mentionCount: rs.mention_count
			});
		}
		unreadState.set(map);
	} catch {
		// Read state may not be available yet.
	}
}

// Mark a channel as read (acknowledge).
export async function ackChannel(channelId: string) {
	// Immediately clear unread count locally.
	unreadCounts.update((map) => {
		map.delete(channelId);
		return new Map(map);
	});
	unreadState.update((map) => {
		const entry = map.get(channelId);
		if (entry) {
			map.set(channelId, { ...entry, mentionCount: 0 });
			return new Map(map);
		}
		return map;
	});

	try {
		await api.ackChannel(channelId);
	} catch {
		// Best-effort.
	}
}

// Increment unread count for a channel (called when a new message arrives).
export function incrementUnread(channelId: string, isMention: boolean = false) {
	// Don't increment if we're currently viewing this channel.
	if (get(currentChannelId) === channelId) return;

	unreadCounts.update((map) => {
		map.set(channelId, (map.get(channelId) ?? 0) + 1);
		return new Map(map);
	});

	if (isMention) {
		unreadState.update((map) => {
			const entry = map.get(channelId) ?? { lastReadId: null, mentionCount: 0 };
			map.set(channelId, { ...entry, mentionCount: entry.mentionCount + 1 });
			return new Map(map);
		});
	}
}

// Clear unreads when viewing a channel.
export function clearChannelUnreads(channelId: string) {
	unreadCounts.update((map) => {
		if (map.has(channelId)) {
			map.delete(channelId);
			return new Map(map);
		}
		return map;
	});
}
