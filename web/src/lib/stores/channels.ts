// Channel store — manages channels for the current guild.

import { writable, derived } from 'svelte/store';
import type { Channel } from '$lib/types';
import { api } from '$lib/api/client';

export const channels = writable<Map<string, Channel>>(new Map());
export const currentChannelId = writable<string | null>(null);

// When set, the channel page should open this thread in its side panel.
export const pendingThreadOpen = writable<string | null>(null);

export const channelList = derived(channels, ($channels) =>
	Array.from($channels.values()).sort((a, b) => a.position - b.position)
);

// Text channels excluding threads (threads have parent_channel_id set).
export const textChannels = derived(channelList, ($list) =>
	$list.filter((c) => (c.channel_type === 'text' || c.channel_type === 'announcement') && !c.parent_channel_id)
);

export const voiceChannels = derived(channelList, ($list) =>
	$list.filter((c) => c.channel_type === 'voice' || c.channel_type === 'stage')
);

export const currentChannel = derived(
	[channels, currentChannelId],
	([$channels, $id]) => ($id ? $channels.get($id) ?? null : null)
);

// --- Hidden Threads ---

export const hiddenThreadIds = writable<Set<string>>(new Set());

export async function loadHiddenThreads() {
	try {
		const ids = await api.getHiddenThreads();
		hiddenThreadIds.set(new Set(ids));
	} catch {
		// Silently fail — hidden threads are a preference, not critical.
	}
}

export async function hideThread(channelId: string, threadId: string) {
	// Optimistic update.
	hiddenThreadIds.update((set) => {
		const next = new Set(set);
		next.add(threadId);
		return next;
	});
	try {
		await api.hideThread(channelId, threadId);
	} catch {
		// Rollback on failure.
		hiddenThreadIds.update((set) => {
			const next = new Set(set);
			next.delete(threadId);
			return next;
		});
	}
}

export async function unhideThread(channelId: string, threadId: string) {
	// Optimistic update.
	hiddenThreadIds.update((set) => {
		const next = new Set(set);
		next.delete(threadId);
		return next;
	});
	try {
		await api.unhideThread(channelId, threadId);
	} catch {
		// Rollback on failure.
		hiddenThreadIds.update((set) => {
			const next = new Set(set);
			next.add(threadId);
			return next;
		});
	}
}

// --- Threads By Parent ---

// Derived store: Map<parentChannelId, Channel[]> grouped, sorted by last_activity_at DESC,
// excluding hidden threads.
export const threadsByParent = derived(
	[channels, hiddenThreadIds],
	([$channels, $hidden]) => {
		const map = new Map<string, Channel[]>();
		for (const ch of $channels.values()) {
			if (!ch.parent_channel_id) continue;
			if ($hidden.has(ch.id)) continue;
			const list = map.get(ch.parent_channel_id) ?? [];
			list.push(ch);
			map.set(ch.parent_channel_id, list);
		}
		// Sort each group by last_activity_at DESC.
		for (const [key, list] of map) {
			list.sort((a, b) => {
				const aTime = a.last_activity_at ? new Date(a.last_activity_at).getTime() : 0;
				const bTime = b.last_activity_at ? new Date(b.last_activity_at).getTime() : 0;
				return bTime - aTime;
			});
			map.set(key, list);
		}
		return map;
	}
);

// --- Thread Activity Filter (localStorage) ---

const THREAD_FILTER_KEY = 'amityvox_thread_activity_filter';

export function getThreadActivityFilter(channelId: string): number | null {
	try {
		const stored = localStorage.getItem(THREAD_FILTER_KEY);
		if (!stored) return null;
		const filters = JSON.parse(stored) as Record<string, number>;
		return filters[channelId] ?? null;
	} catch {
		return null;
	}
}

export function setThreadActivityFilter(channelId: string, minutes: number | null) {
	try {
		const stored = localStorage.getItem(THREAD_FILTER_KEY);
		const filters: Record<string, number> = stored ? JSON.parse(stored) : {};
		if (minutes === null) {
			delete filters[channelId];
		} else {
			filters[channelId] = minutes;
		}
		localStorage.setItem(THREAD_FILTER_KEY, JSON.stringify(filters));
	} catch {
		// Ignore storage errors.
	}
}

// --- Core Functions ---

export async function loadChannels(guildId: string) {
	const list = await api.getGuildChannels(guildId);
	const map = new Map<string, Channel>();
	for (const c of list) {
		map.set(c.id, c);
	}
	channels.set(map);
}

export function setChannel(id: string | null) {
	currentChannelId.set(id);
}

export function updateChannel(channel: Channel) {
	channels.update((map) => {
		map.set(channel.id, channel);
		return new Map(map);
	});
}

export function removeChannel(channelId: string) {
	channels.update((map) => {
		map.delete(channelId);
		return new Map(map);
	});
}
