import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getGuildChannels: vi.fn()
	}
}));

import {
	channels,
	currentChannelId,
	channelList,
	textChannels,
	voiceChannels,
	currentChannel,
	setChannel,
	updateChannel,
	removeChannel,
	loadChannels
} from '../channels';
import { api } from '$lib/api/client';
import type { Channel } from '$lib/types';

function createMockChannel(overrides?: Partial<Channel>): Channel {
	return {
		id: crypto.randomUUID(),
		guild_id: 'guild-1',
		category_id: null,
		channel_type: 'text',
		name: 'general',
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

describe('channels store', () => {
	beforeEach(() => {
		channels.set(new Map());
		currentChannelId.set(null);
	});

	it('starts with empty channel map', () => {
		expect(get(channels).size).toBe(0);
		expect(get(channelList)).toEqual([]);
	});

	it('setChannel sets the current channel id', () => {
		setChannel('ch-1');
		expect(get(currentChannelId)).toBe('ch-1');
	});

	it('setChannel with null clears the current channel', () => {
		setChannel('ch-1');
		setChannel(null);
		expect(get(currentChannelId)).toBeNull();
	});

	it('updateChannel adds a channel', () => {
		const ch = createMockChannel({ id: 'ch-1', name: 'general' });
		updateChannel(ch);

		const map = get(channels);
		expect(map.size).toBe(1);
		expect(map.get('ch-1')?.name).toBe('general');
	});

	it('updateChannel modifies an existing channel', () => {
		const ch = createMockChannel({ id: 'ch-1', name: 'general' });
		updateChannel(ch);

		const updated = { ...ch, name: 'announcements' };
		updateChannel(updated);

		const map = get(channels);
		expect(map.size).toBe(1);
		expect(map.get('ch-1')?.name).toBe('announcements');
	});

	it('removeChannel removes a channel', () => {
		const ch1 = createMockChannel({ id: 'ch-1', name: 'general' });
		const ch2 = createMockChannel({ id: 'ch-2', name: 'random' });
		updateChannel(ch1);
		updateChannel(ch2);

		removeChannel('ch-1');

		const map = get(channels);
		expect(map.size).toBe(1);
		expect(map.has('ch-1')).toBe(false);
		expect(map.has('ch-2')).toBe(true);
	});

	it('removeChannel on non-existent channel does nothing', () => {
		const ch = createMockChannel({ id: 'ch-1' });
		updateChannel(ch);

		removeChannel('ch-nonexistent');

		expect(get(channels).size).toBe(1);
	});

	it('textChannels derived store filters text and announcement channels', () => {
		const text = createMockChannel({ id: 'ch-1', channel_type: 'text', position: 0 });
		const voice = createMockChannel({ id: 'ch-2', channel_type: 'voice', position: 1 });
		const announcement = createMockChannel({ id: 'ch-3', channel_type: 'announcement', position: 2 });
		updateChannel(text);
		updateChannel(voice);
		updateChannel(announcement);

		const filtered = get(textChannels);
		expect(filtered).toHaveLength(2);
		expect(filtered.map((c) => c.id)).toContain('ch-1');
		expect(filtered.map((c) => c.id)).toContain('ch-3');
		expect(filtered.map((c) => c.id)).not.toContain('ch-2');
	});

	it('voiceChannels derived store filters voice and stage channels', () => {
		const text = createMockChannel({ id: 'ch-1', channel_type: 'text', position: 0 });
		const voice = createMockChannel({ id: 'ch-2', channel_type: 'voice', position: 1 });
		const stage = createMockChannel({ id: 'ch-3', channel_type: 'stage', position: 2 });
		updateChannel(text);
		updateChannel(voice);
		updateChannel(stage);

		const filtered = get(voiceChannels);
		expect(filtered).toHaveLength(2);
		expect(filtered.map((c) => c.id)).toContain('ch-2');
		expect(filtered.map((c) => c.id)).toContain('ch-3');
		expect(filtered.map((c) => c.id)).not.toContain('ch-1');
	});

	it('channelList sorts channels by position', () => {
		const ch1 = createMockChannel({ id: 'ch-a', position: 2, name: 'second' });
		const ch2 = createMockChannel({ id: 'ch-b', position: 0, name: 'first' });
		const ch3 = createMockChannel({ id: 'ch-c', position: 1, name: 'middle' });
		updateChannel(ch1);
		updateChannel(ch2);
		updateChannel(ch3);

		const list = get(channelList);
		expect(list[0].name).toBe('first');
		expect(list[1].name).toBe('middle');
		expect(list[2].name).toBe('second');
	});

	it('currentChannel derived store returns the selected channel', () => {
		const ch = createMockChannel({ id: 'ch-1', name: 'general' });
		updateChannel(ch);
		setChannel('ch-1');

		expect(get(currentChannel)?.name).toBe('general');
	});

	it('currentChannel returns null when no channel is selected', () => {
		expect(get(currentChannel)).toBeNull();
	});

	it('loadChannels replaces all channels for a guild', async () => {
		// Pre-populate with an existing channel.
		const existing = createMockChannel({ id: 'ch-old', name: 'old' });
		updateChannel(existing);

		// Mock API to return new channels.
		const apiChannels = [
			createMockChannel({ id: 'ch-new-1', name: 'new-general', position: 0 }),
			createMockChannel({ id: 'ch-new-2', name: 'new-random', position: 1 })
		];
		vi.mocked(api.getGuildChannels).mockResolvedValue(apiChannels);

		await loadChannels('guild-1');

		const map = get(channels);
		expect(map.size).toBe(2);
		expect(map.has('ch-old')).toBe(false);
		expect(map.has('ch-new-1')).toBe(true);
		expect(map.has('ch-new-2')).toBe(true);
		expect(map.get('ch-new-1')?.name).toBe('new-general');
	});
});
