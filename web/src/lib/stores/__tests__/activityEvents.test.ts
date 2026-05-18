import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { activityInvalidationsByChannel, invalidateChannelActivity } from '../activityEvents';

describe('activityEvents store', () => {
	beforeEach(() => {
		activityInvalidationsByChannel.set(new Map());
	});

	it('increments invalidation counters per channel', () => {
		invalidateChannelActivity('channel-1');
		invalidateChannelActivity('channel-1');
		invalidateChannelActivity('channel-2');

		expect(get(activityInvalidationsByChannel).get('channel-1')).toBe(2);
		expect(get(activityInvalidationsByChannel).get('channel-2')).toBe(1);
	});

	it('ignores missing channel IDs', () => {
		invalidateChannelActivity();

		expect(get(activityInvalidationsByChannel).size).toBe(0);
	});
});
