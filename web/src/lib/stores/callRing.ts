// Incoming call ring store — tracks active incoming calls for DM/Group channels.

import { writable, derived } from 'svelte/store';

export interface IncomingCall {
	channelId: string;
	callerId: string;
	callerName: string;
	callerDisplayName?: string | null;
	callerAvatarId?: string | null;
	channelType: 'dm' | 'group';
	timestamp: number;
}

const RING_TIMEOUT_MS = 30_000; // Stop ringing after 30 seconds.

// Map of channelId -> IncomingCall
const _incomingCalls = writable<Map<string, IncomingCall>>(new Map());
export const incomingCalls = { subscribe: _incomingCalls.subscribe };

// Derived: the most recent incoming call (for the modal).
export const activeIncomingCall = derived(_incomingCalls, ($calls) => {
	if ($calls.size === 0) return null;
	// Return the most recent call.
	let latest: IncomingCall | null = null;
	for (const call of $calls.values()) {
		if (!latest || call.timestamp > latest.timestamp) {
			latest = call;
		}
	}
	return latest;
});

// Derived: count of active incoming calls (for badge).
export const incomingCallCount = derived(_incomingCalls, ($calls) => $calls.size);

const timeouts = new Map<string, ReturnType<typeof setTimeout>>();

/** Add an incoming call ring. Auto-expires after 30 seconds. */
export function addIncomingCall(call: IncomingCall) {
	_incomingCalls.update((map) => {
		const next = new Map(map);
		next.set(call.channelId, call);
		return next;
	});

	// Clear any existing timeout for this channel.
	const existing = timeouts.get(call.channelId);
	if (existing) clearTimeout(existing);

	// Auto-dismiss after timeout.
	const timeout = setTimeout(() => {
		dismissIncomingCall(call.channelId);
	}, RING_TIMEOUT_MS);
	timeouts.set(call.channelId, timeout);
}

/** Dismiss an incoming call (declined, answered, or timed out). */
export function dismissIncomingCall(channelId: string) {
	_incomingCalls.update((map) => {
		const next = new Map(map);
		next.delete(channelId);
		return next;
	});
	const timeout = timeouts.get(channelId);
	if (timeout) {
		clearTimeout(timeout);
		timeouts.delete(channelId);
	}
}

/** Clear all incoming calls (e.g., on disconnect). */
export function clearIncomingCalls() {
	_incomingCalls.set(new Map());
	for (const [, timeout] of timeouts) {
		clearTimeout(timeout);
	}
	timeouts.clear();
}
