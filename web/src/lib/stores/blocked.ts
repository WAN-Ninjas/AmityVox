// Blocked users store -- tracks which users the current user has blocked.
// Used by MessageItem to show "Blocked message" placeholders.

import { writable, get } from 'svelte/store';
import { api } from '$lib/api/client';

const blockedUserIds = writable<Set<string>>(new Set());

export async function loadBlockedUsers() {
	try {
		const blocks = await api.getBlockedUsers();
		blockedUserIds.set(new Set(blocks.map(b => b.target_id)));
	} catch {
		// Silently fail -- blocked list may not be available yet.
	}
}

export function addBlockedUser(userId: string) {
	blockedUserIds.update(s => {
		const next = new Set(s);
		next.add(userId);
		return next;
	});
}

export function removeBlockedUser(userId: string) {
	blockedUserIds.update(s => {
		const next = new Set(s);
		next.delete(userId);
		return next;
	});
}

export function isBlocked(userId: string): boolean {
	return get(blockedUserIds).has(userId);
}

export { blockedUserIds };
