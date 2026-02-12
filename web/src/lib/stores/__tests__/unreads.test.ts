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
		currentChannelId: writable<string | null>(null)
	};
});

import {
	unreadCounts,
	getUnreadCount,
	incrementUnread,
	clearChannelUnreads,
	totalUnreads
} from '../unreads';

describe('unreads store', () => {
	beforeEach(() => {
		// Reset unread counts.
		unreadCounts.set(new Map());
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
});
