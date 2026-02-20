// Unreads store — tracks unread message counts and mention counts per channel.

import { derived, get } from 'svelte/store';
import type { ReadState } from '$lib/types';
import { api } from '$lib/api/client';
import { currentChannelId } from './channels';
import { createMapStore } from './mapHelpers';

// Map of channel_id -> { lastReadId, mentionCount }
interface UnreadEntry {
	lastReadId: string | null;
	mentionCount: number;
}

const unreadState = createMapStore<string, UnreadEntry>();

// Channels that have any unread messages.
// We track by comparing last_message_id on the channel with lastReadId.
// For simplicity, we track an explicit unread count.
const unreadCounts = createMapStore<string, number>();

// Map of channel_id -> guild_id for unread tracking across all guilds.
// Populated on READY and from CHANNEL_CREATE/UPDATE events so guild sidebar
// can show unread indicators even for guilds the user hasn't navigated to yet.
const channelGuildMap = createMapStore<string, string>();

export { unreadCounts, unreadState, channelGuildMap };

// Get unread count for a specific channel.
export function getUnreadCount(channelId: string) {
	return derived(unreadCounts, ($counts) => $counts.get(channelId) ?? 0);
}

// Get mention count for a specific channel.
export function getMentionCount(channelId: string) {
	return derived(unreadState, ($state) => $state.get(channelId)?.mentionCount ?? 0);
}

// Mention counts map derived from unreadState (channel_id -> mentionCount).
export const mentionCounts = derived(unreadState, ($state) => {
	const map = new Map<string, number>();
	for (const [channelId, entry] of $state) {
		if (entry.mentionCount > 0) {
			map.set(channelId, entry.mentionCount);
		}
	}
	return map;
});

// Get the last read message ID for a channel.
export function getLastReadId(channelId: string) {
	return derived(unreadState, ($state) => $state.get(channelId)?.lastReadId ?? null);
}

// Check if any channel in a guild has unreads (uses channelGuildMap).
export const guildUnreadSet = derived(
	[unreadCounts, channelGuildMap],
	([$counts, $map]) => {
		const set = new Set<string>();
		for (const [channelId, count] of $counts) {
			if (count > 0) {
				const guildId = $map.get(channelId);
				if (guildId) set.add(guildId);
			}
		}
		return set;
	}
);

// Per-guild mention counts (guild_id -> total mention count across all channels).
export const guildMentionCounts = derived(
	[unreadState, channelGuildMap],
	([$state, $map]) => {
		const counts = new Map<string, number>();
		for (const [channelId, entry] of $state) {
			if (entry.mentionCount > 0) {
				const guildId = $map.get(channelId);
				if (guildId) {
					counts.set(guildId, (counts.get(guildId) ?? 0) + entry.mentionCount);
				}
			}
		}
		return counts;
	}
);

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
		unreadState.setAll(map);
	} catch {
		// Read state may not be available yet.
	}
}

// Mark a channel as read (acknowledge).
export async function ackChannel(channelId: string) {
	// Immediately clear unread count locally.
	unreadCounts.removeEntry(channelId);
	unreadState.updateEntry(channelId, (entry) => ({ ...entry, mentionCount: 0 }));

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

	unreadCounts.setEntry(channelId, (get(unreadCounts).get(channelId) ?? 0) + 1);

	if (isMention) {
		const current = get(unreadState).get(channelId) ?? { lastReadId: null, mentionCount: 0 };
		unreadState.setEntry(channelId, { ...current, mentionCount: current.mentionCount + 1 });
	}
}

// Clear unreads when viewing a channel.
export function clearChannelUnreads(channelId: string) {
	unreadCounts.removeEntry(channelId);
}

// Register a channel's guild mapping (called from channel events and loading).
export function registerChannelGuild(channelId: string, guildId: string) {
	channelGuildMap.setEntry(channelId, guildId);
}

// Load channel→guild mapping for all guilds the user is in.
// Called on READY to ensure guild sidebar can show unread indicators.
export async function loadChannelGuildMap() {
	try {
		const guildList = await api.getMyGuilds();
		const results = await Promise.all(
			guildList.map((g) =>
				api
					.getGuildChannels(g.id)
					.then((channels) => channels.map((ch) => [ch.id, g.id] as [string, string]))
					.catch(() => [] as [string, string][])
			)
		);
		const map = new Map<string, string>();
		for (const pairs of results) {
			for (const [channelId, guildId] of pairs) {
				map.set(channelId, guildId);
			}
		}
		channelGuildMap.setAll(map);
	} catch {
		// Non-critical — will be populated from channel events as they arrive.
	}
}

// Mark all channels as read.
export async function markAllRead() {
	const counts = get(unreadCounts);
	const channelIds = [...counts.keys()].filter((id) => (counts.get(id) ?? 0) > 0);

	// Clear all locally first.
	unreadCounts.clear();
	unreadState.update((map) => {
		for (const id of channelIds) {
			const entry = map.get(id);
			if (entry) map.set(id, { ...entry, mentionCount: 0 });
		}
		return new Map(map);
	});

	// Ack each channel on the server (best-effort).
	for (const id of channelIds) {
		try {
			await api.ackChannel(id);
		} catch {
			// Best-effort.
		}
	}
}
