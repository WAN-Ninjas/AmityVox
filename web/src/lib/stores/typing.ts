// Typing store — tracks who is typing in each channel.
// Typing indicators auto-expire after 8 seconds.

import { derived, get } from 'svelte/store';
import { currentChannelId } from './channels';
import { currentUser } from './auth';
import { createMapStore } from '$lib/stores/mapHelpers';

// Map of channel ID -> Map of user ID -> timeout handle
const typingState = createMapStore<string, Map<string, ReturnType<typeof setTimeout>>>();

// Derived: user IDs typing in the current channel (excluding self)
export const currentTypingUsers = derived(
	[typingState, currentChannelId, currentUser],
	([$state, $channelId, $user]) => {
		if (!$channelId) return [];
		const channelTyping = $state.get($channelId);
		if (!channelTyping) return [];
		return Array.from(channelTyping.keys()).filter((uid) => uid !== $user?.id);
	}
);

export function addTypingUser(channelId: string, userId: string) {
	// Don't track own typing
	const self = get(currentUser);
	if (self && userId === self.id) return;

	typingState.update((state) => {
		let channelMap = state.get(channelId);
		if (!channelMap) {
			channelMap = new Map();
			state.set(channelId, channelMap);
		}

		// Clear existing timeout for this user
		const existing = channelMap.get(userId);
		if (existing) clearTimeout(existing);

		// Set new timeout (8 seconds)
		const timeout = setTimeout(() => {
			clearTypingUser(channelId, userId);
		}, 8000);

		channelMap.set(userId, timeout);
		return new Map(state);
	});
}

export function clearTypingUser(channelId: string, userId: string) {
	typingState.update((state) => {
		const channelMap = state.get(channelId);
		if (!channelMap) return state;

		const timeout = channelMap.get(userId);
		if (timeout) clearTimeout(timeout);
		channelMap.delete(userId);

		if (channelMap.size === 0) {
			state.delete(channelId);
		}
		return new Map(state);
	});
}
