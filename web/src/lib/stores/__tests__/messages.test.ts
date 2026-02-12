import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { messagesByChannel, appendMessage, getChannelMessages } from '../messages';

function createMockMessage(overrides?: Record<string, unknown>) {
	return {
		id: crypto.randomUUID(),
		channel_id: 'ch-1',
		author_id: 'user-1',
		content: 'Test message',
		created_at: new Date().toISOString(),
		updated_at: null,
		reply_to: null,
		attachments: [],
		embeds: [],
		reactions: [],
		pinned: false,
		author: null,
		...overrides
	};
}

describe('messages store', () => {
	beforeEach(() => {
		messagesByChannel.set(new Map());
	});

	it('starts with empty map', () => {
		const map = get(messagesByChannel);
		expect(map.size).toBe(0);
	});

	it('appendMessage adds message to correct channel', () => {
		const msg = createMockMessage({ channel_id: 'ch-1' });
		appendMessage(msg as any);

		const map = get(messagesByChannel);
		const channelMsgs = map.get('ch-1');
		expect(channelMsgs).toHaveLength(1);
		expect(channelMsgs![0].id).toBe(msg.id);
	});

	it('appendMessage deduplicates by id', () => {
		const msg = createMockMessage({ id: 'msg-1', channel_id: 'ch-1' });
		appendMessage(msg as any);
		appendMessage(msg as any);

		const map = get(messagesByChannel);
		expect(map.get('ch-1')).toHaveLength(1);
	});

	it('getChannelMessages returns derived store for channel', () => {
		const msg = createMockMessage({ channel_id: 'ch-2' });
		appendMessage(msg as any);

		const store = getChannelMessages('ch-2');
		const msgs = get(store);
		expect(msgs).toHaveLength(1);
	});

	it('getChannelMessages returns empty array for unknown channel', () => {
		const store = getChannelMessages('nonexistent');
		expect(get(store)).toEqual([]);
	});
});
