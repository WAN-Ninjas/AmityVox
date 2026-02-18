// Client-side nicknames store — personal nicknames visible only to the current user.
// Persisted in localStorage under 'av-client-nicknames'.

import { get } from 'svelte/store';
import { createMapStore } from './mapHelpers';

const STORAGE_KEY = 'av-client-nicknames';
const hasStorage = typeof window !== 'undefined' && typeof localStorage !== 'undefined';

function loadFromStorage(): Map<string, string> {
	if (!hasStorage) return new Map();
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return new Map();
		return new Map(Object.entries(JSON.parse(raw)));
	} catch {
		return new Map();
	}
}

function saveToStorage(map: Map<string, string>) {
	if (!hasStorage) return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(Object.fromEntries(map)));
	} catch { /* ignore storage failures */ }
}

export const clientNicknames = createMapStore<string, string>();
clientNicknames.setAll(loadFromStorage());

/** Get the client-side nickname for a user, or null if none set. */
export function getClientNickname(userId: string): string | null {
	return get(clientNicknames).get(userId) ?? null;
}

/** Set a client-side nickname for a user. Pass empty string to clear. */
export function setClientNickname(userId: string, nickname: string) {
	const trimmed = nickname.trim();
	if (trimmed) {
		clientNicknames.setEntry(userId, trimmed);
	} else {
		clientNicknames.removeEntry(userId);
	}
	saveToStorage(get(clientNicknames));
}
