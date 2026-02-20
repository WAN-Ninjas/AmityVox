// Channel store — manages channels for the current guild.

import { writable, derived } from 'svelte/store';
import type { Channel } from '$lib/types';
import { api } from '$lib/api/client';
import { createMapStore } from '$lib/stores/mapHelpers';

export const channels = createMapStore<string, Channel>();
export const currentChannelId = writable<string | null>(null);

// When set, the channel page should open this thread in its side panel.
export const pendingThreadOpen = writable<string | null>(null);

// Signal to open the edit channel modal from external components (e.g. TopBar gear icon).
export const editChannelSignal = writable<string | null>(null);

// Tracks which thread is currently open in the side panel.
export const activeThreadId = writable<string | null>(null);

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

export const forumChannels = derived(channelList, ($list) =>
	$list.filter((c) => c.channel_type === 'forum' && !c.parent_channel_id)
);

export const galleryChannels = derived(channelList, ($list) =>
	$list.filter((c) => c.channel_type === 'gallery' && !c.parent_channel_id)
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
	} catch (e) {
		// Rollback on failure, then re-throw so callers can show a toast.
		hiddenThreadIds.update((set) => {
			const next = new Set(set);
			next.delete(threadId);
			return next;
		});
		throw e;
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
	} catch (e) {
		// Rollback on failure, then re-throw so callers can show a toast.
		hiddenThreadIds.update((set) => {
			const next = new Set(set);
			next.add(threadId);
			return next;
		});
		throw e;
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
		let filters: Record<string, number> = {};
		try {
			filters = stored ? JSON.parse(stored) : {};
		} catch {
			// Corrupted JSON — reset to empty so subsequent writes succeed.
		}
		if (minutes === null) {
			delete filters[channelId];
		} else {
			filters[channelId] = minutes;
		}
		localStorage.setItem(THREAD_FILTER_KEY, JSON.stringify(filters));
	} catch {
		// Ignore storage errors (e.g. quota exceeded, private browsing).
	}
}

// --- Core Functions ---

export async function loadChannels(guildId: string) {
	const list = await api.getGuildChannels(guildId);
	channels.setAll(list.map(c => [c.id, c]));
}

/** Load channels from federation cache data (simplified channel objects). */
export function loadFederatedChannels(channelsJson: unknown[]) {
	if (!Array.isArray(channelsJson)) {
		channels.setAll([]);
		return;
	}
	const list = (channelsJson as Array<{ id: string; name: string; topic?: string | null; position?: number; channel_type?: string }>)
		.filter((c): c is { id: string; name: string; topic?: string | null; position?: number; channel_type?: string } =>
			!!c && typeof (c as any).id === 'string' && typeof (c as any).name === 'string'
		)
		.map(c => ({
			id: c.id,
			guild_id: null,
			category_id: null,
			name: c.name,
			topic: c.topic ?? null,
			position: c.position ?? 0,
			channel_type: (c.channel_type ?? 'text') as Channel['channel_type'],
			parent_channel_id: null,
			slowmode_seconds: 0,
			nsfw: false,
			encrypted: false,
			archived: false,
			locked: false,
			locked_by: null,
			locked_at: null,
			last_message_id: null,
			owner_id: null,
			user_limit: 0,
			bitrate: 0,
			last_activity_at: null,
			created_at: new Date(0).toISOString(),
		} as Channel));
	channels.setAll(list.map(c => [c.id, c]));
}

export function setChannel(id: string | null) {
	currentChannelId.set(id);
}

export function updateChannel(channel: Channel) {
	channels.setEntry(channel.id, channel);
}

export function removeChannel(channelId: string) {
	channels.removeEntry(channelId);
}
