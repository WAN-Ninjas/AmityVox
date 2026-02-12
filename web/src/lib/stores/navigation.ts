// Navigation store -- tracks recent channels and back/forward navigation history.
// Persists recent channels to localStorage.

import { writable, derived, get } from 'svelte/store';
import { goto } from '$app/navigation';

const STORAGE_KEY = 'amityvox_recent_channels';
const MAX_RECENT = 10;

export interface RecentChannel {
	guildId: string | null;
	channelId: string;
	visitedAt: number; // timestamp
}

interface NavigationState {
	recentChannels: RecentChannel[];
	backStack: string[]; // URLs
	forwardStack: string[]; // URLs
	currentUrl: string | null;
}

function loadFromStorage(): RecentChannel[] {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) {
			const parsed = JSON.parse(raw);
			if (Array.isArray(parsed)) {
				return parsed.slice(0, MAX_RECENT);
			}
		}
	} catch {
		// localStorage may be unavailable
	}
	return [];
}

function saveToStorage(channels: RecentChannel[]) {
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(channels));
	} catch {
		// localStorage may be unavailable
	}
}

const initialState: NavigationState = {
	recentChannels: loadFromStorage(),
	backStack: [],
	forwardStack: [],
	currentUrl: null
};

export const navigationState = writable<NavigationState>(initialState);

export const recentChannels = derived(navigationState, ($state) => $state.recentChannels);
export const canGoBack = derived(navigationState, ($state) => $state.backStack.length > 0);
export const canGoForward = derived(navigationState, ($state) => $state.forwardStack.length > 0);

// Guard flag to prevent the route-change $effect from re-pushing when goBack/goForward
// triggers a navigation. Set before calling goto(), cleared after the navigation completes.
let navigatingFromHistory = false;

/**
 * Record a channel visit. Adds to recent channels (FIFO, no duplicates)
 * and updates the navigation history stacks.
 */
export function pushChannel(guildId: string | null, channelId: string) {
	// If this navigation was triggered by goBack/goForward, only update recent channels
	// but do not modify the back/forward stacks (they were already adjusted).
	const fromHistory = navigatingFromHistory;
	navigatingFromHistory = false;

	navigationState.update((state) => {
		// Build the URL for this channel
		const url = guildId
			? `/app/guilds/${guildId}/channels/${channelId}`
			: `/app/dms/${channelId}`;

		// Update recent channels: remove duplicate, add to front
		const filtered = state.recentChannels.filter((r) => r.channelId !== channelId);
		const newRecent: RecentChannel[] = [
			{ guildId, channelId, visitedAt: Date.now() },
			...filtered
		].slice(0, MAX_RECENT);

		saveToStorage(newRecent);

		if (fromHistory) {
			// goBack/goForward already set the stacks correctly; just update recents.
			return {
				...state,
				recentChannels: newRecent
			};
		}

		// Normal navigation: push current URL onto back stack, clear forward stack.
		let newBackStack = [...state.backStack];
		const newForwardStack: string[] = [];

		if (state.currentUrl && state.currentUrl !== url) {
			newBackStack.push(state.currentUrl);
		}

		return {
			recentChannels: newRecent,
			backStack: newBackStack,
			forwardStack: newForwardStack,
			currentUrl: url
		};
	});
}

/**
 * Navigate back in history.
 */
export function goBack() {
	const state = get(navigationState);
	if (state.backStack.length === 0) return;

	const prevUrl = state.backStack[state.backStack.length - 1];

	navigationState.update((s) => {
		const newBackStack = s.backStack.slice(0, -1);
		const newForwardStack = [...s.forwardStack];
		if (s.currentUrl) {
			newForwardStack.push(s.currentUrl);
		}
		return {
			...s,
			backStack: newBackStack,
			forwardStack: newForwardStack,
			currentUrl: prevUrl
		};
	});

	navigatingFromHistory = true;
	goto(prevUrl);
}

/**
 * Navigate forward in history.
 */
export function goForward() {
	const state = get(navigationState);
	if (state.forwardStack.length === 0) return;

	const nextUrl = state.forwardStack[state.forwardStack.length - 1];

	navigationState.update((s) => {
		const newForwardStack = s.forwardStack.slice(0, -1);
		const newBackStack = [...s.backStack];
		if (s.currentUrl) {
			newBackStack.push(s.currentUrl);
		}
		return {
			...s,
			backStack: newBackStack,
			forwardStack: newForwardStack,
			currentUrl: nextUrl
		};
	});

	navigatingFromHistory = true;
	goto(nextUrl);
}

/**
 * Set the current URL without pushing to history (for initial load or non-channel pages).
 * If called after goBack/goForward, clears the history guard without modifying stacks.
 */
export function setCurrentUrl(url: string) {
	const fromHistory = navigatingFromHistory;
	navigatingFromHistory = false;

	if (fromHistory) {
		// Stacks were already adjusted by goBack/goForward, skip.
		return;
	}

	navigationState.update((s) => ({
		...s,
		currentUrl: url
	}));
}
