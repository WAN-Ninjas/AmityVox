// Blocked users store -- tracks which users the current user has blocked.
// Supports two-tier blocking: 'ignore' (show placeholder) and 'block' (completely hide).

import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api/client';

export type BlockLevel = 'ignore' | 'block';

/** Map from user ID to their block level. */
const blockedUsers = writable<Map<string, BlockLevel>>(new Map());

/** Backward-compatible set of all blocked user IDs (any level). */
const blockedUserIds = derived(blockedUsers, $map => new Set($map.keys()));

export async function loadBlockedUsers() {
	try {
		const blocks = await api.getBlockedUsers();
		const map = new Map<string, BlockLevel>();
		for (const b of blocks) {
			map.set(b.target_id, (b.level as BlockLevel) ?? 'block');
		}
		blockedUsers.set(map);
	} catch {
		// Silently fail -- blocked list may not be available yet.
	}
}

export function addBlockedUser(userId: string, level: BlockLevel = 'block') {
	blockedUsers.update(map => {
		const next = new Map(map);
		next.set(userId, level);
		return next;
	});
}

export function updateBlockedUserLevel(userId: string, level: BlockLevel) {
	blockedUsers.update(map => {
		if (!map.has(userId)) return map;
		const next = new Map(map);
		next.set(userId, level);
		return next;
	});
}

export function removeBlockedUser(userId: string) {
	blockedUsers.update(map => {
		const next = new Map(map);
		next.delete(userId);
		return next;
	});
}

export function getBlockLevel(userId: string): BlockLevel | null {
	return get(blockedUsers).get(userId) ?? null;
}

export function isBlocked(userId: string): boolean {
	return get(blockedUsers).has(userId);
}

export { blockedUsers, blockedUserIds };
