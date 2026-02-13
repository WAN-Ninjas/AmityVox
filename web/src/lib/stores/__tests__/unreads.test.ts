import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getReadState: vi.fn().mockResolvedValue([]),
		ackChannel: vi.fn().mockResolvedValue(undefined)
	}
}));

// Mock the channels store.
vi.mock('../channels', () => {
	const { writable } = require('svelte/store');
	return {
		currentChannelId: writable(null)
	};
});

import {
	unreadCounts,
	unreadState,
	getUnreadCount,
	getMentionCount,
	mentionCounts,
	getLastReadId,
	incrementUnread,
	clearChannelUnreads,
	totalUnreads
} from '../unreads';

describe('unreads store', () => {
	beforeEach(() => {
		// Reset unread counts and mention state.
		unreadCounts.set(new Map());
		unreadState.set(new Map());
	});

	it('starts with zero unreads', () => {
		expect(get(totalUnreads)).toBe(0);
	});

	it('increments unread count for a channel', () => {
		incrementUnread('ch-1');
		incrementUnread('ch-1');
		incrementUnread('ch-2');

		expect(get(getUnreadCount('ch-1'))).toBe(2);
		expect(get(getUnreadCount('ch-2'))).toBe(1);
		expect(get(totalUnreads)).toBe(3);
	});

	it('clears unreads for a channel', () => {
		incrementUnread('ch-1');
		incrementUnread('ch-1');
		clearChannelUnreads('ch-1');

		expect(get(getUnreadCount('ch-1'))).toBe(0);
		expect(get(totalUnreads)).toBe(0);
	});

	it('clearing unreads for non-existent channel does nothing', () => {
		incrementUnread('ch-1');
		clearChannelUnreads('ch-non-existent');

		expect(get(getUnreadCount('ch-1'))).toBe(1);
		expect(get(totalUnreads)).toBe(1);
	});

	it('returns 0 for channels with no unreads', () => {
		expect(get(getUnreadCount('ch-unknown'))).toBe(0);
	});

	it('tracks mention counts when incrementing with isMention=true', () => {
		incrementUnread('ch-1', true);
		incrementUnread('ch-1', true);
		incrementUnread('ch-1'); // not a mention
		incrementUnread('ch-2', true);

		expect(get(getMentionCount('ch-1'))).toBe(2);
		expect(get(getMentionCount('ch-2'))).toBe(1);
		expect(get(getMentionCount('ch-unknown'))).toBe(0);
	});

	it('exposes mentionCounts derived store as a Map', () => {
		incrementUnread('ch-1', true);
		incrementUnread('ch-2', true);
		incrementUnread('ch-3'); // no mention

		const counts = get(mentionCounts);
		expect(counts.get('ch-1')).toBe(1);
		expect(counts.get('ch-2')).toBe(1);
		expect(counts.has('ch-3')).toBe(false);
	});

	it('returns null lastReadId for unknown channels', () => {
		expect(get(getLastReadId('ch-unknown'))).toBeNull();
	});
});
