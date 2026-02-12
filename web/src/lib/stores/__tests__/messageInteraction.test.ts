import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
	replyingTo,
	editingMessage,
	startReply,
	cancelReply,
	startEdit,
	cancelEdit
} from '../messageInteraction';
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
		mention_everyone: false,
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

describe('messageInteraction store', () => {
	beforeEach(() => {
		replyingTo.set(null);
		editingMessage.set(null);
	});

	it('starts with null reply and edit state', () => {
		expect(get(replyingTo)).toBeNull();
		expect(get(editingMessage)).toBeNull();
	});

	it('startReply sets replyingTo and clears editingMessage', () => {
		const msg = createMockMessage({ id: 'reply-target' });
		startReply(msg);

		expect(get(replyingTo)?.id).toBe('reply-target');
		expect(get(editingMessage)).toBeNull();
	});

	it('startEdit sets editingMessage and clears replyingTo', () => {
		const msg = createMockMessage({ id: 'edit-target' });
		startEdit(msg);

		expect(get(editingMessage)?.id).toBe('edit-target');
		expect(get(replyingTo)).toBeNull();
	});

	it('cancelReply clears replyingTo', () => {
		const msg = createMockMessage();
		startReply(msg);
		cancelReply();

		expect(get(replyingTo)).toBeNull();
	});

	it('cancelEdit clears editingMessage', () => {
		const msg = createMockMessage();
		startEdit(msg);
		cancelEdit();

		expect(get(editingMessage)).toBeNull();
	});

	it('starting reply cancels edit (mutual exclusivity)', () => {
		const editMsg = createMockMessage({ id: 'edit-msg' });
		const replyMsg = createMockMessage({ id: 'reply-msg' });

		startEdit(editMsg);
		expect(get(editingMessage)?.id).toBe('edit-msg');

		startReply(replyMsg);
		expect(get(replyingTo)?.id).toBe('reply-msg');
		expect(get(editingMessage)).toBeNull();
	});

	it('starting edit cancels reply (mutual exclusivity)', () => {
		const replyMsg = createMockMessage({ id: 'reply-msg' });
		const editMsg = createMockMessage({ id: 'edit-msg' });

		startReply(replyMsg);
		expect(get(replyingTo)?.id).toBe('reply-msg');

		startEdit(editMsg);
		expect(get(editingMessage)?.id).toBe('edit-msg');
		expect(get(replyingTo)).toBeNull();
	});
});
