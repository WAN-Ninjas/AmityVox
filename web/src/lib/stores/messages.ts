// Message store — manages messages for the current channel.

import { writable, derived } from 'svelte/store';
import type { Message } from '$lib/types';
import { api } from '$lib/api/client';

// Messages keyed by channel ID, each containing a sorted array.
export const messagesByChannel = writable<Map<string, Message[]>>(new Map());
export const isLoadingMessages = writable(false);

export function getChannelMessages(channelId: string) {
	return derived(messagesByChannel, ($map) => $map.get(channelId) ?? []);
}

export async function loadMessages(channelId: string, before?: string) {
	isLoadingMessages.set(true);
	try {
		const msgs = await api.getMessages(channelId, { before, limit: 50 });
		messagesByChannel.update((map) => {
			const existing = map.get(channelId) ?? [];
			// Merge, deduplicate by ID, sort by created_at.
			const merged = new Map<string, Message>();
			for (const m of existing) merged.set(m.id, m);
			for (const m of msgs) merged.set(m.id, m);
			map.set(
				channelId,
				Array.from(merged.values()).sort((a, b) => a.id.localeCompare(b.id))
			);
			return new Map(map);
		});
	} finally {
		isLoadingMessages.set(false);
	}
}

export function appendMessage(msg: Message) {
	messagesByChannel.update((map) => {
		const existing = map.get(msg.channel_id) ?? [];
		// Avoid duplicates (by nonce or ID).
		if (existing.some((m) => m.id === msg.id)) return map;
		map.set(msg.channel_id, [...existing, msg]);
		return new Map(map);
	});
}

export function updateMessage(msg: Message) {
	messagesByChannel.update((map) => {
		const existing = map.get(msg.channel_id);
		if (!existing) return map;
		map.set(
			msg.channel_id,
			existing.map((m) => (m.id === msg.id ? msg : m))
		);
		return new Map(map);
	});
}

export function removeMessage(channelId: string, messageId: string) {
	messagesByChannel.update((map) => {
		const existing = map.get(channelId);
		if (!existing) return map;
		map.set(
			channelId,
			existing.filter((m) => m.id !== messageId)
		);
		return new Map(map);
	});
}

export function clearChannelMessages(channelId: string) {
	messagesByChannel.update((map) => {
		map.delete(channelId);
		return new Map(map);
	});
}
