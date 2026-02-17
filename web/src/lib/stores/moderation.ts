// Shared moderation modal state — allows MemberList and MessageItem to trigger
// kick/ban modals without duplicating logic.

import { writable } from 'svelte/store';

export interface ModerationTarget {
	userId: string;
	guildId: string;
	displayName: string;
}

export const kickModalTarget = writable<ModerationTarget | null>(null);
export const banModalTarget = writable<ModerationTarget | null>(null);
