import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
	messagesByChannel,
	appendMessage,
	clearChannelMessages,
	getChannelMessages
} from '../messages';
import type { Message } from '$lib/types';

function createMockMessage(overrides?: Partial<Message>): Message {
	return {
		id: crypto.randomUUID(),
		channel_id: 'ch-1',
		author_id: 'user-1',
		content: 'Test message',
		nonce: null,
		message_type: 'default',
		edited_at: null,
		flags: 0,
		reply_to_ids: [],
		mention_user_ids: [],
		mention_role_ids: [],
		mention_here: false,
		thread_id: null,
		masquerade_name: null,
		masquerade_avatar: null,
		masquerade_color: null,
		encrypted: false,
		attachments: [],
		embeds: [],
		reactions: [],
		pinned: false,
		created_at: new Date().toISOString(),
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

	it('appendMessage adds message to the correct channel', () => {
		const msg = createMockMessage({ channel_id: 'ch-1' });
		appendMessage(msg);

		const map = get(messagesByChannel);
		const channelMsgs = map.get('ch-1');
		expect(channelMsgs).toHaveLength(1);
		expect(channelMsgs![0].id).toBe(msg.id);
	});

	it('appendMessage adds messages to separate channels independently', () => {
		const msg1 = createMockMessage({ channel_id: 'ch-1' });
		const msg2 = createMockMessage({ channel_id: 'ch-2' });
		appendMessage(msg1);
		appendMessage(msg2);

		const map = get(messagesByChannel);
		expect(map.get('ch-1')).toHaveLength(1);
		expect(map.get('ch-2')).toHaveLength(1);
		expect(map.get('ch-1')![0].id).toBe(msg1.id);
		expect(map.get('ch-2')![0].id).toBe(msg2.id);
	});

	it('appendMessage does not add duplicates', () => {
		const msg = createMockMessage({ id: 'msg-1', channel_id: 'ch-1' });
		appendMessage(msg);
		appendMessage(msg);

		const map = get(messagesByChannel);
		expect(map.get('ch-1')).toHaveLength(1);
	});

	it('clearChannelMessages clears messages for a channel', () => {
		const msg1 = createMockMessage({ channel_id: 'ch-1' });
		const msg2 = createMockMessage({ channel_id: 'ch-2' });
		appendMessage(msg1);
		appendMessage(msg2);

		clearChannelMessages('ch-1');

		const map = get(messagesByChannel);
		expect(map.has('ch-1')).toBe(false);
		expect(map.get('ch-2')).toHaveLength(1);
	});

	it('clearChannelMessages on empty channel does not affect others', () => {
		const msg = createMockMessage({ channel_id: 'ch-1' });
		appendMessage(msg);

		clearChannelMessages('ch-nonexistent');

		const map = get(messagesByChannel);
		expect(map.get('ch-1')).toHaveLength(1);
	});

	it('getChannelMessages returns derived store for channel', () => {
		const msg = createMockMessage({ channel_id: 'ch-2' });
		appendMessage(msg);

		const store = getChannelMessages('ch-2');
		const msgs = get(store);
		expect(msgs).toHaveLength(1);
	});

	it('getChannelMessages returns empty array for unknown channel', () => {
		const store = getChannelMessages('nonexistent');
		expect(get(store)).toEqual([]);
	});
});
