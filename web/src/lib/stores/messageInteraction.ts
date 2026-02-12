// Tracks active message interaction state (replying to, editing).
// Shared between MessageItem and MessageInput.

import { writable } from 'svelte/store';
import type { Message } from '$lib/types';

// The message being replied to (null = not replying).
export const replyingTo = writable<Message | null>(null);

// The message being edited (null = not editing).
export const editingMessage = writable<Message | null>(null);

export function startReply(message: Message) {
	editingMessage.set(null);
	replyingTo.set(message);
}

export function cancelReply() {
	replyingTo.set(null);
}

export function startEdit(message: Message) {
	replyingTo.set(null);
	editingMessage.set(message);
}

export function cancelEdit() {
	editingMessage.set(null);
}
