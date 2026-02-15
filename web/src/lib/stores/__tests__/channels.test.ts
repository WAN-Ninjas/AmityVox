import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getGuildChannels: vi.fn(),
		getHiddenThreads: vi.fn().mockResolvedValue([]),
		hideThread: vi.fn().mockResolvedValue(undefined),
		unhideThread: vi.fn().mockResolvedValue(undefined)
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
	loadChannels,
	threadsByParent,
	hiddenThreadIds,
	getThreadActivityFilter,
	setThreadActivityFilter
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
		parent_channel_id: null,
		last_activity_at: null,
		created_at: new Date().toISOString(),
		...overrides
	};
}

describe('channels store', () => {
	beforeEach(() => {
		channels.set(new Map());
		currentChannelId.set(null);
		hiddenThreadIds.set(new Set());
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

	it('textChannels derived store filters text and announcement channels, excludes threads', () => {
		const text = createMockChannel({ id: 'ch-1', channel_type: 'text', position: 0 });
		const voice = createMockChannel({ id: 'ch-2', channel_type: 'voice', position: 1 });
		const announcement = createMockChannel({ id: 'ch-3', channel_type: 'announcement', position: 2 });
		const thread = createMockChannel({ id: 'ch-4', channel_type: 'text', position: 0, parent_channel_id: 'ch-1' });
		updateChannel(text);
		updateChannel(voice);
		updateChannel(announcement);
		updateChannel(thread);

		const filtered = get(textChannels);
		expect(filtered).toHaveLength(2);
		expect(filtered.map((c) => c.id)).toContain('ch-1');
		expect(filtered.map((c) => c.id)).toContain('ch-3');
		expect(filtered.map((c) => c.id)).not.toContain('ch-2');
		expect(filtered.map((c) => c.id)).not.toContain('ch-4');
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

describe('threadsByParent derived store', () => {
	beforeEach(() => {
		channels.set(new Map());
		hiddenThreadIds.set(new Set());
	});

	it('groups threads by parent_channel_id', () => {
		const parent = createMockChannel({ id: 'parent-1', channel_type: 'text' });
		const thread1 = createMockChannel({ id: 'thread-1', parent_channel_id: 'parent-1', name: 'Thread 1', last_activity_at: '2026-01-01T10:00:00Z' });
		const thread2 = createMockChannel({ id: 'thread-2', parent_channel_id: 'parent-1', name: 'Thread 2', last_activity_at: '2026-01-01T12:00:00Z' });
		const orphan = createMockChannel({ id: 'ch-3', channel_type: 'text' });
		updateChannel(parent);
		updateChannel(thread1);
		updateChannel(thread2);
		updateChannel(orphan);

		const map = get(threadsByParent);
		expect(map.size).toBe(1);
		const threads = map.get('parent-1')!;
		expect(threads).toHaveLength(2);
		// Sorted by last_activity_at DESC — thread2 (12:00) before thread1 (10:00).
		expect(threads[0].id).toBe('thread-2');
		expect(threads[1].id).toBe('thread-1');
	});

	it('excludes hidden threads', () => {
		const parent = createMockChannel({ id: 'parent-1', channel_type: 'text' });
		const thread1 = createMockChannel({ id: 'thread-1', parent_channel_id: 'parent-1', name: 'Visible' });
		const thread2 = createMockChannel({ id: 'thread-2', parent_channel_id: 'parent-1', name: 'Hidden' });
		updateChannel(parent);
		updateChannel(thread1);
		updateChannel(thread2);

		// Hide thread-2.
		hiddenThreadIds.set(new Set(['thread-2']));

		const map = get(threadsByParent);
		const threads = map.get('parent-1')!;
		expect(threads).toHaveLength(1);
		expect(threads[0].id).toBe('thread-1');
	});

	it('groups threads from multiple parents correctly', () => {
		const parent1 = createMockChannel({ id: 'p1', channel_type: 'text' });
		const parent2 = createMockChannel({ id: 'p2', channel_type: 'text' });
		const t1 = createMockChannel({ id: 't1', parent_channel_id: 'p1' });
		const t2 = createMockChannel({ id: 't2', parent_channel_id: 'p2' });
		const t3 = createMockChannel({ id: 't3', parent_channel_id: 'p1' });
		updateChannel(parent1);
		updateChannel(parent2);
		updateChannel(t1);
		updateChannel(t2);
		updateChannel(t3);

		const map = get(threadsByParent);
		expect(map.get('p1')).toHaveLength(2);
		expect(map.get('p2')).toHaveLength(1);
	});

	it('returns empty map when no threads exist', () => {
		const ch = createMockChannel({ id: 'ch-1', channel_type: 'text' });
		updateChannel(ch);

		const map = get(threadsByParent);
		expect(map.size).toBe(0);
	});
});

describe('thread activity filter', () => {
	beforeEach(() => {
		// Clear localStorage mock.
		try { localStorage.removeItem('amityvox_thread_activity_filter'); } catch { /* ok */ }
	});

	it('getThreadActivityFilter returns null when no filter is set', () => {
		expect(getThreadActivityFilter('ch-1')).toBeNull();
	});

	it('setThreadActivityFilter and getThreadActivityFilter round-trip', () => {
		setThreadActivityFilter('ch-1', 60);
		expect(getThreadActivityFilter('ch-1')).toBe(60);
	});

	it('setThreadActivityFilter with null removes the filter', () => {
		setThreadActivityFilter('ch-1', 360);
		expect(getThreadActivityFilter('ch-1')).toBe(360);

		setThreadActivityFilter('ch-1', null);
		expect(getThreadActivityFilter('ch-1')).toBeNull();
	});

	it('filters are independent per channel', () => {
		setThreadActivityFilter('ch-1', 60);
		setThreadActivityFilter('ch-2', 1440);

		expect(getThreadActivityFilter('ch-1')).toBe(60);
		expect(getThreadActivityFilter('ch-2')).toBe(1440);
		expect(getThreadActivityFilter('ch-3')).toBeNull();
	});
});
