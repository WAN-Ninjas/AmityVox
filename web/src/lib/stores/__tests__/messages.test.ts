import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: {
		getMessages: vi.fn()
	}
}));

import {
	messagesByChannel,
	appendMessage,
	applyReactionEvent,
	backfillLoadedChannels,
	clearChannelMessages,
	getChannelMessages,
	loadMessages,
	reconcileLoadedChannels,
	updateMessage
} from '../messages';
import { api } from '$lib/api/client';
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
		encryption_session_id: null,
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
		vi.mocked(api.getMessages).mockReset();
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

	it('appendMessage replaces a matching nonce to reduce sender duplicates', () => {
		const pending = createMockMessage({ id: 'pending-1', nonce: 'nonce-1', content: 'Sending' });
		const confirmed = createMockMessage({ id: 'msg-1', nonce: 'nonce-1', content: 'Sent' });

		appendMessage(pending);
		appendMessage(confirmed);

		const channelMsgs = get(messagesByChannel).get('ch-1');
		expect(channelMsgs).toHaveLength(1);
		expect(channelMsgs![0].id).toBe('msg-1');
		expect(channelMsgs![0].content).toBe('Sent');
	});

	it('updateMessage merges partial patches into existing messages', () => {
		const msg = createMockMessage({
			id: 'msg-1',
			content: 'Original',
			reactions: [{ emoji: '👍', count: 1, me: false }]
		});
		appendMessage(msg);

		updateMessage({
			id: 'msg-1',
			channel_id: 'ch-1',
			embeds: [
				{
					type: 'link',
					url: null,
					title: 'Example',
					description: null,
					color: null,
					thumbnail_url: null,
					thumbnail_width: null,
					thumbnail_height: null,
					image_url: null,
					image_width: null,
					image_height: null,
					video_url: null,
					author_name: null,
					author_url: null,
					provider_name: null,
					provider_url: null
				}
			]
		});

		const updated = get(messagesByChannel).get('ch-1')![0];
		expect(updated.content).toBe('Original');
		expect(updated.reactions).toEqual([{ emoji: '👍', count: 1, me: false }]);
		expect(updated.embeds[0].title).toBe('Example');
	});

	it('applyReactionEvent patches sparse add events', () => {
		appendMessage(createMockMessage({ id: 'msg-1' }));

		applyReactionEvent(
			{ channel_id: 'ch-1', message_id: 'msg-1', user_id: 'self', emoji: '🔥' },
			'add',
			'self'
		);

		expect(get(messagesByChannel).get('ch-1')![0].reactions).toEqual([
			{ emoji: '🔥', count: 1, me: true }
		]);
	});

	it('applyReactionEvent patches sparse remove events without channel_id', () => {
		appendMessage(
			createMockMessage({
				id: 'msg-1',
				reactions: [{ emoji: '🔥', count: 2, me: true }]
			})
		);

		applyReactionEvent({ message_id: 'msg-1', user_id: 'self', emoji: '🔥' }, 'remove', 'self');

		expect(get(messagesByChannel).get('ch-1')![0].reactions).toEqual([
			{ emoji: '🔥', count: 1, me: false }
		]);
	});

	it('applyReactionEvent ignores duplicate sparse self events', () => {
		appendMessage(
			createMockMessage({
				id: 'msg-1',
				reactions: [{ emoji: '🔥', count: 1, me: true }]
			})
		);

		applyReactionEvent(
			{ channel_id: 'ch-1', message_id: 'msg-1', user_id: 'self', emoji: '🔥' },
			'add',
			'self'
		);
		expect(get(messagesByChannel).get('ch-1')![0].reactions).toEqual([
			{ emoji: '🔥', count: 1, me: true }
		]);

		applyReactionEvent(
			{ channel_id: 'ch-1', message_id: 'msg-1', user_id: 'self', emoji: '🔥' },
			'remove',
			'self'
		);
		applyReactionEvent(
			{ channel_id: 'ch-1', message_id: 'msg-1', user_id: 'self', emoji: '🔥' },
			'remove',
			'self'
		);
		expect(get(messagesByChannel).get('ch-1')![0].reactions).toEqual([]);
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

	it('loadMessages ignores stale initial responses for the same channel', async () => {
		let resolveFirst: (messages: Message[]) => void = () => {};
		let resolveSecond: (messages: Message[]) => void = () => {};
		vi.mocked(api.getMessages)
			.mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve; }))
			.mockReturnValueOnce(new Promise((resolve) => { resolveSecond = resolve; }));

		const firstLoad = loadMessages('ch-1');
		const secondLoad = loadMessages('ch-1');

		resolveSecond([createMockMessage({ id: 'newer', channel_id: 'ch-1' })]);
		await secondLoad;

		resolveFirst([createMockMessage({ id: 'older', channel_id: 'ch-1' })]);
		await firstLoad;

		const channelMessages = get(messagesByChannel).get('ch-1') ?? [];
		expect(channelMessages.map((message) => message.id)).toEqual(['newer']);
	});

	it('backfillLoadedChannels reconciles latest windows and loads after each channel latest id', async () => {
		appendMessage(createMockMessage({ id: 'msg-1', channel_id: 'ch-1' }));
		appendMessage(createMockMessage({ id: 'msg-2', channel_id: 'ch-2' }));
		vi.mocked(api.getMessages)
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-1', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-3', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-2', channel_id: 'ch-2' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-4', channel_id: 'ch-2' })]);

		await backfillLoadedChannels();

		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { limit: 100 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { after: 'msg-1', limit: 100 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-2', { limit: 100 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-2', { after: 'msg-2', limit: 100 });
		expect(get(messagesByChannel).get('ch-1')?.map((message) => message.id)).toEqual(['msg-1', 'msg-3']);
		expect(get(messagesByChannel).get('ch-2')?.map((message) => message.id)).toEqual(['msg-2', 'msg-4']);
	});

	it('reconcileLoadedChannels refreshes edits and removes deleted messages in the latest window', async () => {
		appendMessage(createMockMessage({ id: 'msg-1', channel_id: 'ch-1', content: 'Older' }));
		appendMessage(createMockMessage({ id: 'msg-2', channel_id: 'ch-1', content: 'Deleted' }));
		appendMessage(createMockMessage({ id: 'msg-3', channel_id: 'ch-1', content: 'Original' }));
		vi.mocked(api.getMessages)
			.mockResolvedValueOnce([
				createMockMessage({ id: 'msg-1', channel_id: 'ch-1', content: 'Older' }),
				createMockMessage({ id: 'msg-3', channel_id: 'ch-1', content: 'Edited' })
			])
			.mockResolvedValueOnce([]);

		await reconcileLoadedChannels();

		const ids = get(messagesByChannel).get('ch-1')?.map((message) => message.id);
		expect(ids).toEqual(['msg-1', 'msg-3']);
		expect(get(messagesByChannel).get('ch-1')?.at(-1)?.content).toBe('Edited');
	});

	it('reconcileLoadedChannels pages forward after reconnect when more than one page was missed', async () => {
		appendMessage(createMockMessage({ id: 'msg-1', channel_id: 'ch-1' }));
		vi.mocked(api.getMessages)
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-3', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-1', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-2', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-3', channel_id: 'ch-1' })])
			.mockResolvedValueOnce([]);

		await reconcileLoadedChannels({ limit: 1, maxAfterPages: 3 });

		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { before: 'msg-3', limit: 1 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { after: 'msg-1', limit: 1 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { after: 'msg-2', limit: 1 });
		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { after: 'msg-3', limit: 1 });
		expect(get(messagesByChannel).get('ch-1')?.map((message) => message.id)).toEqual(['msg-1', 'msg-2', 'msg-3']);
	});

	it('reconcileLoadedChannels pages backward to refresh older visible edits and deletes', async () => {
		appendMessage(createMockMessage({ id: 'msg-1', channel_id: 'ch-1', content: 'Old original' }));
		appendMessage(createMockMessage({ id: 'msg-2', channel_id: 'ch-1', content: 'Deleted stale' }));
		appendMessage(createMockMessage({ id: 'msg-3', channel_id: 'ch-1', content: 'Latest original' }));
		vi.mocked(api.getMessages)
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-3', channel_id: 'ch-1', content: 'Latest edited' })])
			.mockResolvedValueOnce([createMockMessage({ id: 'msg-1', channel_id: 'ch-1', content: 'Old edited' })])
			.mockResolvedValueOnce([]);

		await reconcileLoadedChannels({ limit: 1, maxBeforePages: 3 });

		expect(vi.mocked(api.getMessages)).toHaveBeenCalledWith('ch-1', { before: 'msg-3', limit: 1 });
		expect(get(messagesByChannel).get('ch-1')?.map((message) => message.id)).toEqual(['msg-1', 'msg-3']);
		expect(get(messagesByChannel).get('ch-1')?.[0].content).toBe('Old edited');
		expect(get(messagesByChannel).get('ch-1')?.[1].content).toBe('Latest edited');
	});
});
