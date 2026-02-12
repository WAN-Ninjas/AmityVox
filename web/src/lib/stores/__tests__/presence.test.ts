import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { presenceMap, updatePresence, removePresence, getPresence } from '../presence';

describe('presence store', () => {
	beforeEach(() => {
		presenceMap.set(new Map());
	});

	it('starts with empty presence map', () => {
		expect(get(presenceMap).size).toBe(0);
	});

	it('updatePresence sets status for a user', () => {
		updatePresence('user-1', 'online');
		expect(get(presenceMap).get('user-1')).toBe('online');
	});

	it('updatePresence overwrites existing status', () => {
		updatePresence('user-1', 'online');
		updatePresence('user-1', 'idle');
		expect(get(presenceMap).get('user-1')).toBe('idle');
	});

	it('removePresence clears a user', () => {
		updatePresence('user-1', 'online');
		removePresence('user-1');
		expect(get(presenceMap).has('user-1')).toBe(false);
	});

	it('getPresence returns derived status for a user', () => {
		updatePresence('user-1', 'dnd');
		const status = getPresence('user-1');
		expect(get(status)).toBe('dnd');
	});

	it('getPresence returns offline for unknown user', () => {
		const status = getPresence('user-unknown');
		expect(get(status)).toBe('offline');
	});
});
