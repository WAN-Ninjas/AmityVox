// Presence store — tracks online/idle/dnd status of users.

import { derived } from 'svelte/store';
import { createMapStore } from '$lib/stores/mapHelpers';

export const presenceMap = createMapStore<string, string>();
export const activityMap = createMapStore<string, { activity_type: string | null; activity_name: string | null }>();

export function getPresence(userId: string) {
	return derived(presenceMap, ($map) => $map.get(userId) ?? 'offline');
}

export function updatePresence(userId: string, status: string) {
	presenceMap.setEntry(userId, status);
}

export function updateActivity(userId: string, activityType: string | null | undefined, activityName: string | null | undefined) {
	if (!activityType || !activityName) {
		activityMap.removeEntry(userId);
		return;
	}
	activityMap.setEntry(userId, { activity_type: activityType, activity_name: activityName });
}

export function removePresence(userId: string) {
	presenceMap.removeEntry(userId);
	activityMap.removeEntry(userId);
}
