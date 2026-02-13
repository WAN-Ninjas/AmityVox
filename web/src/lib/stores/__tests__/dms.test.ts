import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock API client.
vi.mock('$lib/api/client', () => ({
	api: {
		getMyDMs: vi.fn().mockResolvedValue([])
	}
}));

import {
	dmChannels,
	dmList,
	addDMChannel,
	removeDMChannel,
	updateDMChannel
} from '../dms';
import type { Channel } from '$lib/types';

function mockDMChannel(overrides?: Partial<Channel>): Channel {
	return {
		id: crypto.randomUUID(),
		guild_id: null,
		category_id: null,
		channel_type: 'dm',
		name: 'Test DM',
		topic: null,
		position: 0,
		slowmode_seconds: 0,
		nsfw: false,
		encrypted: false,
		last_message_id: null,
		owner_id: null,
		user_limit: 0,
		bitrate: 0,
		locked: false,
		locked_by: null,
		locked_at: null,
		archived: false,
		created_at: new Date().toISOString(),
		...overrides
	};
}

describe('dms store', () => {
	beforeEach(() => {
		dmChannels.set(new Map());
	});

	it('starts with empty DM list', () => {
		expect(get(dmList)).toEqual([]);
	});

	it('addDMChannel adds a channel', () => {
		const dm = mockDMChannel({ id: 'dm-1', name: 'Alice' });
		addDMChannel(dm);

		expect(get(dmList)).toHaveLength(1);
		expect(get(dmList)[0].name).toBe('Alice');
	});

	it('removeDMChannel removes a channel', () => {
		const dm = mockDMChannel({ id: 'dm-1' });
		addDMChannel(dm);
		removeDMChannel('dm-1');

		expect(get(dmList)).toHaveLength(0);
	});

	it('updateDMChannel updates an existing channel', () => {
		const dm = mockDMChannel({ id: 'dm-1', name: 'Alice' });
		addDMChannel(dm);

		const updated = { ...dm, name: 'Alice (updated)' };
		updateDMChannel(updated);

		expect(get(dmList)[0].name).toBe('Alice (updated)');
	});

	it('updateDMChannel does nothing for non-existent channel', () => {
		const dm = mockDMChannel({ id: 'dm-nonexistent', name: 'Ghost' });
		updateDMChannel(dm);

		expect(get(dmList)).toHaveLength(0);
	});

	it('sorts DMs by last_message_id descending', () => {
		const dm1 = mockDMChannel({ id: 'dm-1', name: 'Old', last_message_id: '01AAA' });
		const dm2 = mockDMChannel({ id: 'dm-2', name: 'New', last_message_id: '01ZZZ' });
		addDMChannel(dm1);
		addDMChannel(dm2);

		const list = get(dmList);
		expect(list[0].name).toBe('New');
		expect(list[1].name).toBe('Old');
	});
});
