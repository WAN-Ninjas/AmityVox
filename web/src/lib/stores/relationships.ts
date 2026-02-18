// Relationships store — manages friend requests, friends, and blocks.

import { derived } from 'svelte/store';
import type { Relationship } from '$lib/types';
import { api } from '$lib/api/client';
import { createMapStore } from '$lib/stores/mapHelpers';

export const relationships = createMapStore<string, Relationship>();

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
		relationships.setAll(list.map(rel => [rel.target_id, rel]));
	} catch {
		// Silently fail — relationships may not be available yet.
	}
}

export function addOrUpdateRelationship(rel: Relationship) {
	relationships.setEntry(rel.target_id, rel);
}

export function removeRelationship(targetId: string) {
	relationships.removeEntry(targetId);
}
