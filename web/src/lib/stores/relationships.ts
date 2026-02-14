// Relationships store — manages friend requests, friends, and blocks.

import { writable, derived } from 'svelte/store';
import type { Relationship } from '$lib/types';
import { api } from '$lib/api/client';

export const relationships = writable<Map<string, Relationship>>(new Map());

/** Count of pending incoming friend requests (for badge display). */
export const pendingIncomingCount = derived(relationships, ($rels) => {
	let count = 0;
	for (const rel of $rels.values()) {
		if (rel.type === 'pending_incoming') count++;
	}
	return count;
});

export async function loadRelationships() {
	try {
		const list = await api.getFriends();
		const map = new Map<string, Relationship>();
		for (const rel of list) {
			map.set(rel.target_id, rel);
		}
		relationships.set(map);
	} catch {
		// Silently fail — relationships may not be available yet.
	}
}

export function addOrUpdateRelationship(rel: Relationship) {
	relationships.update((map) => {
		map.set(rel.target_id, rel);
		return new Map(map);
	});
}

export function removeRelationship(targetId: string) {
	relationships.update((map) => {
		map.delete(targetId);
		return new Map(map);
	});
}
