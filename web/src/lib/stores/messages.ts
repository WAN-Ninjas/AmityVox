// Message store — manages messages for the current channel.

import { writable, derived, get } from 'svelte/store';
import type { Message, Reaction } from '$lib/types';
import { api } from '$lib/api/client';

type MessagePatch = Partial<Message> & Pick<Message, 'id'> & { channel_id?: string };

interface ReactionEvent {
	channel_id?: string;
	message_id: string;
	user_id?: string;
	emoji?: string;
	reactions?: Reaction[];
}

// Messages keyed by channel ID, each containing a sorted array.
export const messagesByChannel = writable<Map<string, Message[]>>(new Map());
export const loadingMessagesByChannel = writable<Map<string, boolean>>(new Map());
export const isLoadingMessages = derived(loadingMessagesByChannel, ($map) =>
	Array.from($map.values()).some(Boolean)
);

let loadRequestSequence = 0;
const latestInitialLoadByChannel = new Map<string, number>();

export function getChannelMessages(channelId: string) {
	return derived(messagesByChannel, ($map) => $map.get(channelId) ?? []);
}

export async function loadMessages(channelId: string, before?: string) {
	const isInitialLoad = before === undefined;
	const requestId = ++loadRequestSequence;
	if (isInitialLoad) {
		latestInitialLoadByChannel.set(channelId, requestId);
	}

	setChannelLoading(channelId, true);
	try {
		const msgs = await api.getMessages(channelId, { before, limit: 50 });
		if (isInitialLoad && latestInitialLoadByChannel.get(channelId) !== requestId) {
			return;
		}
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
		if (!isInitialLoad || latestInitialLoadByChannel.get(channelId) === requestId) {
			setChannelLoading(channelId, false);
		}
	}
}

export async function backfillLoadedChannels(limit = 100) {
	const snapshot = get(messagesByChannel);
	await Promise.all(
		Array.from(snapshot.entries()).map(async ([channelId, messages]) => {
			const latest = messages.at(-1);
			if (!latest) return;
			const missed = await api.getMessages(channelId, { after: latest.id, limit });
			if (missed.length === 0) return;
			messagesByChannel.update((map) => {
				const existing = map.get(channelId) ?? [];
				const merged = new Map<string, Message>();
				for (const message of existing) merged.set(message.id, message);
				for (const message of missed) merged.set(message.id, message);
				map.set(
					channelId,
					Array.from(merged.values()).sort((a, b) => a.id.localeCompare(b.id))
				);
				return new Map(map);
			});
		})
	);
}

function setChannelLoading(channelId: string, loading: boolean) {
	loadingMessagesByChannel.update((map) => {
		const next = new Map(map);
		if (loading) {
			next.set(channelId, true);
		} else {
			next.delete(channelId);
		}
		return next;
	});
}

export function appendMessage(msg: Message) {
	messagesByChannel.update((map) => {
		const existing = map.get(msg.channel_id) ?? [];
		const existingIndex = existing.findIndex(
			(m) => m.id === msg.id || (msg.nonce && m.nonce === msg.nonce)
		);
		if (existingIndex !== -1) {
			const next = [...existing];
			next[existingIndex] =
				next[existingIndex].id === msg.id ? mergeMessage(next[existingIndex], msg) : msg;
			map.set(msg.channel_id, next);
			return new Map(map);
		}
		map.set(msg.channel_id, [...existing, msg]);
		return new Map(map);
	});
}

export function updateMessage(msg: MessagePatch) {
	messagesByChannel.update((map) => {
		let changed = false;
		const channelEntries = msg.channel_id
			? [[msg.channel_id, map.get(msg.channel_id)] as const]
			: Array.from(map.entries());

		for (const [channelId, existing] of channelEntries) {
			if (!existing) continue;
			let channelChanged = false;
			const next = existing.map((m) => {
				if (m.id !== msg.id) return m;
				channelChanged = true;
				return mergeMessage(m, msg);
			});
			if (channelChanged) {
				changed = true;
				map.set(channelId, next);
			}
		}

		return changed ? new Map(map) : map;
	});
}

export function applyReactionEvent(
	event: ReactionEvent,
	action: 'add' | 'remove',
	selfUserId?: string
) {
	if (event.reactions) {
		updateMessage({
			id: event.message_id,
			channel_id: event.channel_id,
			reactions: event.reactions
		});
		return;
	}
	if (!event.emoji) return;

	messagesByChannel.update((map) => {
		let changed = false;
		const channelEntries = event.channel_id
			? [[event.channel_id, map.get(event.channel_id)] as const]
			: Array.from(map.entries());

		for (const [channelId, existing] of channelEntries) {
			if (!existing) continue;
			let channelChanged = false;
			const next = existing.map((message) => {
				if (message.id !== event.message_id) return message;
				channelChanged = true;
				return {
					...message,
					reactions: patchReactions(message.reactions ?? [], event, action, selfUserId)
				};
			});
			if (channelChanged) {
				changed = true;
				map.set(channelId, next);
			}
		}

		return changed ? new Map(map) : map;
	});
}

function mergeMessage(existing: Message, patch: MessagePatch): Message {
	return { ...existing, ...patch, id: existing.id, channel_id: existing.channel_id };
}

function patchReactions(
	reactions: Reaction[],
	event: ReactionEvent,
	action: 'add' | 'remove',
	selfUserId?: string
): Reaction[] {
	const emoji = event.emoji;
	if (!emoji) return reactions;

	const index = reactions.findIndex((reaction) => reaction.emoji === emoji);
	const isSelf = Boolean(selfUserId && event.user_id === selfUserId);

	if (action === 'add') {
		if (index === -1) {
			return [...reactions, { emoji, count: 1, me: isSelf }];
		}
		const next = [...reactions];
		const current = next[index];
		if (isSelf && current.me) return reactions;
		next[index] = {
			...current,
			count: current.count + 1,
			me: current.me || isSelf
		};
		return next;
	}

	if (index === -1) return reactions;

	const next = [...reactions];
	const current = next[index];
	if (isSelf && !current.me) return reactions;
	const count = Math.max(0, current.count - 1);
	if (count === 0) {
		next.splice(index, 1);
	} else {
		next[index] = {
			...current,
			count,
			me: isSelf ? false : current.me
		};
	}
	return next;
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

export function removeMessages(channelId: string, messageIds: string[]) {
	messagesByChannel.update((map) => {
		const existing = map.get(channelId);
		if (!existing) return map;
		const idSet = new Set(messageIds);
		map.set(
			channelId,
			existing.filter((m) => !idSet.has(m.id))
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
