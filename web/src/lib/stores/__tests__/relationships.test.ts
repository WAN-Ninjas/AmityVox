import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
	relationships,
	pendingIncomingCount,
	addOrUpdateRelationship,
	removeRelationship,
} from '../relationships';
import type { Relationship } from '$lib/types';

function makeRelationship(overrides: Partial<Relationship> = {}): Relationship {
	return {
		id: 'rel1',
		user_id: 'me',
		target_id: 'other',
		type: 'friend',
		created_at: '2024-01-01T00:00:00Z',
		...overrides,
	};
}

describe('relationships store', () => {
	beforeEach(() => {
		relationships.set(new Map());
	});

	it('starts empty', () => {
		expect(get(relationships).size).toBe(0);
		expect(get(pendingIncomingCount)).toBe(0);
	});

	it('addOrUpdateRelationship adds a new relationship', () => {
		const rel = makeRelationship({ target_id: 'alice', type: 'friend' });
		addOrUpdateRelationship(rel);

		expect(get(relationships).size).toBe(1);
		expect(get(relationships).get('alice')?.type).toBe('friend');
	});

	it('addOrUpdateRelationship updates an existing relationship', () => {
		addOrUpdateRelationship(makeRelationship({ target_id: 'alice', type: 'pending_incoming' }));
		expect(get(pendingIncomingCount)).toBe(1);

		addOrUpdateRelationship(makeRelationship({ target_id: 'alice', type: 'friend' }));
		expect(get(pendingIncomingCount)).toBe(0);
		expect(get(relationships).get('alice')?.type).toBe('friend');
	});

	it('removeRelationship removes a relationship', () => {
		addOrUpdateRelationship(makeRelationship({ target_id: 'alice' }));
		expect(get(relationships).size).toBe(1);

		removeRelationship('alice');
		expect(get(relationships).size).toBe(0);
	});

	it('removeRelationship is a no-op for unknown target', () => {
		addOrUpdateRelationship(makeRelationship({ target_id: 'alice' }));
		removeRelationship('nonexistent');
		expect(get(relationships).size).toBe(1);
	});

	it('pendingIncomingCount counts only pending_incoming', () => {
		addOrUpdateRelationship(makeRelationship({ target_id: 'alice', type: 'friend' }));
		addOrUpdateRelationship(makeRelationship({ target_id: 'bob', type: 'pending_incoming' }));
		addOrUpdateRelationship(makeRelationship({ target_id: 'charlie', type: 'pending_outgoing' }));
		addOrUpdateRelationship(makeRelationship({ target_id: 'dave', type: 'pending_incoming' }));
		addOrUpdateRelationship(makeRelationship({ target_id: 'eve', type: 'blocked' }));

		expect(get(pendingIncomingCount)).toBe(2);
	});

	it('pendingIncomingCount updates when relationship is removed', () => {
		addOrUpdateRelationship(makeRelationship({ target_id: 'bob', type: 'pending_incoming' }));
		expect(get(pendingIncomingCount)).toBe(1);

		removeRelationship('bob');
		expect(get(pendingIncomingCount)).toBe(0);
	});
});
