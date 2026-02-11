// Presence store — tracks online/idle/dnd status of users.

import { writable, derived } from 'svelte/store';

export const presenceMap = writable<Map<string, string>>(new Map());

export function getPresence(userId: string) {
	return derived(presenceMap, ($map) => $map.get(userId) ?? 'offline');
}

export function updatePresence(userId: string, status: string) {
	presenceMap.update((map) => {
		map.set(userId, status);
		return new Map(map);
	});
}

export function removePresence(userId: string) {
	presenceMap.update((map) => {
		map.delete(userId);
		return new Map(map);
	});
}
