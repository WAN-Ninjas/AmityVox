// Notifications store — server-backed persistent notifications with cursor pagination.
// Replaces the old ephemeral client-side notification store.

import { writable, derived, get } from 'svelte/store';
import { currentChannelId } from './channels';
import { isDndActive, notificationSoundsEnabled, notificationSoundPreset, notificationVolume } from './settings';
import { playNotificationSound } from '$lib/utils/sounds';
import { createMapStore } from './mapHelpers';
import { api } from '$lib/api/client';
import type { ServerNotification, ServerNotificationType, NotificationCategory } from '$lib/types';

// Re-export types for backwards compatibility.
export type { ServerNotification, ServerNotificationType, NotificationCategory };

// --- Core state ---

const notificationMap = createMapStore<string, ServerNotification>();

// Cursor for pagination — oldest loaded notification ID.
export const oldestCursor = writable<string | null>(null);
export const hasMore = writable(true);
export const loadingMore = writable(false);
export const initialLoaded = writable(false);

// --- Derived stores ---

// All notifications sorted by ID (newest first, since ULIDs sort lexicographically by time).
export const notifications = derived(notificationMap, ($map) =>
	Array.from($map.values()).sort((a, b) => b.id.localeCompare(a.id))
);

// Unread notification count.
export const unreadNotificationCount = derived(notificationMap, ($map) => {
	let count = 0;
	for (const n of $map.values()) {
		if (!n.read) count++;
	}
	return count;
});

// Notifications grouped by guild/channel.
export const groupedNotifications = derived(notifications, ($notifications) => {
	const groups = new Map<string, { label: string; notifications: ServerNotification[] }>();

	for (const n of $notifications) {
		const key = n.guild_id
			? `guild:${n.guild_id}`
			: n.channel_id
				? `dm:${n.channel_id}`
				: 'other';

		const label = n.guild_name
			? `${n.guild_name}${n.channel_name ? ` / #${n.channel_name}` : ''}`
			: n.channel_name
				? n.channel_name
				: 'Other';

		if (!groups.has(key)) {
			groups.set(key, { label, notifications: [] });
		}
		groups.get(key)!.notifications.push(n);
	}

	return Array.from(groups.values());
});

// Whether a notification was suppressed by DND.
export const lastNotificationSuppressed = writable(false);

// --- API-backed functions ---

// Load the first page of notifications from the server. Called on READY.
export async function loadNotifications() {
	if (get(initialLoaded)) return;

	loadingMore.set(true);
	try {
		const result = await api.getNotifications({ limit: 50 });
		const map = new Map<string, ServerNotification>();
		for (const n of result) {
			map.set(n.id, n);
		}
		notificationMap.setAll(map);
		if (result.length > 0) {
			oldestCursor.set(result[result.length - 1].id);
		}
		hasMore.set(result.length === 50);
		initialLoaded.set(true);
	} catch {
		// Silently fail — notifications are non-critical.
	} finally {
		loadingMore.set(false);
	}
}

// Load the next page of notifications (infinite scroll).
export async function loadMoreNotifications() {
	if (get(loadingMore) || !get(hasMore)) return;

	const cursor = get(oldestCursor);
	loadingMore.set(true);
	try {
		const result = await api.getNotifications({ before: cursor ?? undefined, limit: 50 });
		for (const n of result) {
			notificationMap.setEntry(n.id, n);
		}
		if (result.length > 0) {
			oldestCursor.set(result[result.length - 1].id);
		}
		hasMore.set(result.length === 50);
	} catch {
		// Silently fail.
	} finally {
		loadingMore.set(false);
	}
}

// Handle NOTIFICATION_CREATE from gateway. Adds to map and plays sound if enabled.
export function handleNotificationCreate(n: ServerNotification) {
	// Don't alert if we're currently viewing that channel.
	const currentChannel = get(currentChannelId);
	const silentView = n.channel_id && currentChannel === n.channel_id;

	const dndActive = get(isDndActive);

	if (dndActive) {
		// Store but mark as read when DND is active.
		notificationMap.setEntry(n.id, { ...n, read: true });
		lastNotificationSuppressed.set(true);
		setTimeout(() => lastNotificationSuppressed.set(false), 2000);
		return;
	}

	notificationMap.setEntry(n.id, n);

	// Play sound if enabled and not viewing the channel.
	if (!silentView && get(notificationSoundsEnabled)) {
		const preset = get(notificationSoundPreset);
		const volume = get(notificationVolume);
		playNotificationSound(preset, volume);
	}

	// Show browser notification if tab is hidden and service worker isn't handling push.
	if (!silentView && typeof document !== 'undefined' && document.visibilityState === 'hidden') {
		if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
			try {
				new Notification(n.actor_name, {
					body: n.content ?? n.type,
					tag: `amityvox-${n.id}`,
				});
			} catch {
				// Fallback notification may fail in some environments.
			}
		}
	}
}

// Handle NOTIFICATION_UPDATE from gateway (cross-tab sync).
export function handleNotificationUpdate(data: { id: string; read: boolean }) {
	notificationMap.updateEntry(data.id, (entry) => ({ ...entry, read: data.read }));
}

// Handle NOTIFICATION_DELETE from gateway.
export function handleNotificationDelete(data: { id: string }) {
	notificationMap.removeEntry(data.id);
}

// Mark a single notification as read (optimistic + API).
export async function markNotificationRead(id: string) {
	notificationMap.updateEntry(id, (entry) => ({ ...entry, read: true }));
	try {
		await api.markNotificationRead(id);
	} catch {
		// Revert on failure.
		notificationMap.updateEntry(id, (entry) => ({ ...entry, read: false }));
	}
}

// Mark a single notification as unread (optimistic + API).
export async function markNotificationUnread(id: string) {
	notificationMap.updateEntry(id, (entry) => ({ ...entry, read: false }));
	try {
		await api.markNotificationUnread(id);
	} catch {
		notificationMap.updateEntry(id, (entry) => ({ ...entry, read: true }));
	}
}

// Mark all notifications as read (optimistic + API).
export async function markAllNotificationsRead() {
	// Save old state for potential revert.
	const oldEntries: [string, boolean][] = [];
	notificationMap.update((map) => {
		for (const [id, entry] of map) {
			if (!entry.read) {
				oldEntries.push([id, false]);
				map.set(id, { ...entry, read: true });
			}
		}
		return oldEntries.length > 0 ? new Map(map) : map;
	});

	try {
		await api.markAllNotificationsRead();
	} catch {
		// Revert on failure.
		notificationMap.update((map) => {
			for (const [id, wasRead] of oldEntries) {
				const entry = map.get(id);
				if (entry) map.set(id, { ...entry, read: wasRead });
			}
			return new Map(map);
		});
	}
}

// Delete a single notification (optimistic + API).
export async function deleteNotification(id: string) {
	let removed: ServerNotification | undefined;
	notificationMap.update((map) => {
		removed = map.get(id);
		map.delete(id);
		return new Map(map);
	});

	try {
		await api.deleteNotification(id);
	} catch {
		// Revert on failure.
		if (removed) {
			notificationMap.setEntry(id, removed);
		}
	}
}

// Clear all notifications (API + clear map).
export async function clearAllNotifications() {
	try {
		await api.clearAllNotifications();
		notificationMap.clear();
	} catch {
		// Don't clear if API fails.
	}
}

// Search notifications (returns results, doesn't modify store).
export async function searchNotifications(query: string, params?: { limit?: number; before?: string }): Promise<ServerNotification[]> {
	return api.searchNotifications(query, params);
}

// Mark all notifications for a specific channel as read.
export function markChannelNotificationsRead(channelId: string) {
	notificationMap.update((map) => {
		let changed = false;
		for (const [id, entry] of map) {
			if (entry.channel_id === channelId && !entry.read) {
				map.set(id, { ...entry, read: true });
				changed = true;
			}
		}
		return changed ? new Map(map) : map;
	});
}
