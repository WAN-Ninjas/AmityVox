// Presence store — tracks online/idle/dnd status of users.

import { derived } from 'svelte/store';
import { createMapStore } from '$lib/stores/mapHelpers';

export const presenceMap = createMapStore<string, string>();

export function getPresence(userId: string) {
	return derived(presenceMap, ($map) => $map.get(userId) ?? 'offline');
}

export function updatePresence(userId: string, status: string) {
	presenceMap.setEntry(userId, status);
}

export function removePresence(userId: string) {
	presenceMap.removeEntry(userId);
}
