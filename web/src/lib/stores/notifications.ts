// Notifications store — tracks in-app notifications for mentions, replies, DMs, friend requests.

import { writable, derived, get } from 'svelte/store';
import { currentChannelId } from './channels';
import { isDndActive, notificationSoundsEnabled, notificationSoundPreset, notificationVolume } from './settings';
import { playNotificationSound } from '$lib/utils/sounds';
import { createMapStore } from './mapHelpers';

export type NotificationType = 'mention' | 'reply' | 'dm' | 'friend_request';

export interface AppNotification {
	id: string;
	type: NotificationType;
	guild_id: string | null;
	guild_name: string | null;
	channel_id: string | null;
	channel_name: string | null;
	message_id: string | null;
	sender_id: string;
	sender_name: string;
	content: string | null;
	read: boolean;
	created_at: string;
}

const notificationMap = createMapStore<string, AppNotification>();

let counter = 0;

// All notifications sorted by creation time (newest first).
export const notifications = derived(notificationMap, ($map) =>
	Array.from($map.values()).sort((a, b) => b.created_at.localeCompare(a.created_at))
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
	const groups = new Map<string, { label: string; notifications: AppNotification[] }>();

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

// Whether a notification was suppressed by DND (for visual feedback).
export const lastNotificationSuppressed = writable(false);

// Add a notification from a gateway event.
// When DND is active, the notification is stored but marked as read (silently),
// and no browser notification or sound is triggered by the caller.
export function addNotification(notification: Omit<AppNotification, 'id' | 'read' | 'created_at'>): string {
	// Don't add notification if we're currently viewing that channel.
	if (notification.channel_id && get(currentChannelId) === notification.channel_id) {
		return '';
	}

	const dndActive = get(isDndActive);
	const id = `notif-${++counter}-${Date.now()}`;
	const entry: AppNotification = {
		...notification,
		id,
		// When DND is active, mark as read immediately so it doesn't produce alerts.
		read: dndActive,
		created_at: new Date().toISOString()
	};

	if (dndActive) {
		lastNotificationSuppressed.set(true);
		// Reset suppressed flag after a brief delay.
		setTimeout(() => lastNotificationSuppressed.set(false), 2000);
	} else if (get(notificationSoundsEnabled)) {
		// Play notification sound when not in DND and sounds are enabled.
		const preset = get(notificationSoundPreset);
		const volume = get(notificationVolume);
		playNotificationSound(preset, volume);
	}

	notificationMap.update((map) => {
		map.set(id, entry);
		// Keep a maximum of 100 notifications to avoid unbounded growth.
		if (map.size > 100) {
			const sorted = Array.from(map.entries()).sort(
				(a, b) => b[1].created_at.localeCompare(a[1].created_at)
			);
			const trimmed = new Map(sorted.slice(0, 100));
			return trimmed;
		}
		return new Map(map);
	});

	return dndActive ? '' : id;
}

// Mark a single notification as read.
export function markNotificationRead(id: string) {
	notificationMap.updateEntry(id, (entry) => ({ ...entry, read: true }));
}

// Mark all notifications as read.
export function markAllNotificationsRead() {
	notificationMap.update((map) => {
		let changed = false;
		for (const [id, entry] of map) {
			if (!entry.read) {
				map.set(id, { ...entry, read: true });
				changed = true;
			}
		}
		return changed ? new Map(map) : map;
	});
}

// Remove a single notification.
export function removeNotification(id: string) {
	notificationMap.removeEntry(id);
}

// Clear all notifications.
export function clearAllNotifications() {
	notificationMap.clear();
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
