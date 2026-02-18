// Blocked users store -- tracks which users the current user has blocked.
// Supports two-tier blocking: 'ignore' (show placeholder) and 'block' (completely hide).

import { derived, get } from 'svelte/store';
import { api } from '$lib/api/client';
import { createMapStore } from './mapHelpers';

export type BlockLevel = 'ignore' | 'block';

/** Map from user ID to their block level. */
const blockedUsers = createMapStore<string, BlockLevel>();

/** Backward-compatible set of all blocked user IDs (any level). */
const blockedUserIds = derived(blockedUsers, $map => new Set($map.keys()));

export async function loadBlockedUsers() {
	try {
		const blocks = await api.getBlockedUsers();
		const map = new Map<string, BlockLevel>();
		for (const b of blocks) {
			map.set(b.target_id, (b.level as BlockLevel) ?? 'block');
		}
		blockedUsers.setAll(map);
	} catch {
		// Silently fail -- blocked list may not be available yet.
	}
}

export function addBlockedUser(userId: string, level: BlockLevel = 'block') {
	blockedUsers.setEntry(userId, level);
}

export function updateBlockedUserLevel(userId: string, level: BlockLevel) {
	blockedUsers.updateEntry(userId, () => level);
}

export function removeBlockedUser(userId: string) {
	blockedUsers.removeEntry(userId);
}

export function getBlockLevel(userId: string): BlockLevel | null {
	return get(blockedUsers).get(userId) ?? null;
}

export function isBlocked(userId: string): boolean {
	return get(blockedUsers).has(userId);
}

export { blockedUsers, blockedUserIds };
