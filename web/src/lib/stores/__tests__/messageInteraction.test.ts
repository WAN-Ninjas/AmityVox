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

function createMockMessage(overrides?: Record<string, unknown>) {
	return {
		id: crypto.randomUUID(),
		channel_id: 'ch-1',
		author_id: 'user-1',
		content: 'Test message',
		message_type: 'default' as const,
		nonce: null,
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

	it('startReply sets replyingTo and clears editing', () => {
		const msg = createMockMessage();
		editingMessage.set(createMockMessage()); // set editing first
		startReply(msg as any);

		expect(get(replyingTo)?.id).toBe(msg.id);
		expect(get(editingMessage)).toBeNull();
	});

	it('cancelReply clears replyingTo', () => {
		const msg = createMockMessage();
		startReply(msg as any);
		cancelReply();

		expect(get(replyingTo)).toBeNull();
	});

	it('startEdit sets editingMessage and clears reply', () => {
		const msg = createMockMessage();
		replyingTo.set(createMockMessage() as any); // set reply first
		startEdit(msg as any);

		expect(get(editingMessage)?.id).toBe(msg.id);
		expect(get(replyingTo)).toBeNull();
	});

	it('cancelEdit clears editingMessage', () => {
		const msg = createMockMessage();
		startEdit(msg as any);
		cancelEdit();

		expect(get(editingMessage)).toBeNull();
	});

	it('startReply and startEdit are mutually exclusive', () => {
		const msg1 = createMockMessage({ id: 'reply-msg' });
		const msg2 = createMockMessage({ id: 'edit-msg' });

		startReply(msg1 as any);
		expect(get(replyingTo)?.id).toBe('reply-msg');
		expect(get(editingMessage)).toBeNull();

		startEdit(msg2 as any);
		expect(get(editingMessage)?.id).toBe('edit-msg');
		expect(get(replyingTo)).toBeNull();

		startReply(msg1 as any);
		expect(get(replyingTo)?.id).toBe('reply-msg');
		expect(get(editingMessage)).toBeNull();
	});
});
