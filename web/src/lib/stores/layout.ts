// Layout store — persisted sidebar/panel widths for resizable panes.

import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const STORAGE_KEY = 'av-layout';

interface LayoutState {
	channelSidebarWidth: number;
	memberListWidth: number;
}

const DEFAULTS: LayoutState = {
	channelSidebarWidth: 224, // w-56 = 14rem = 224px
	memberListWidth: 240,     // w-60 = 15rem = 240px
};

const MIN_SIDEBAR = 180;
const MAX_SIDEBAR = 400;
const MIN_MEMBER_LIST = 200;
const MAX_MEMBER_LIST = 400;

function loadState(): LayoutState {
	if (!browser) return { ...DEFAULTS };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) {
			const parsed = JSON.parse(raw);
			return {
				channelSidebarWidth: clamp(parsed.channelSidebarWidth ?? DEFAULTS.channelSidebarWidth, MIN_SIDEBAR, MAX_SIDEBAR),
				memberListWidth: clamp(parsed.memberListWidth ?? DEFAULTS.memberListWidth, MIN_MEMBER_LIST, MAX_MEMBER_LIST),
			};
		}
	} catch { /* ignore */ }
	return { ...DEFAULTS };
}

function clamp(v: number, min: number, max: number): number {
	return Math.max(min, Math.min(max, v));
}

const state = writable<LayoutState>(loadState());

// Persist on change.
if (browser) {
	state.subscribe((s) => {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
	});
}

export const channelSidebarWidth = {
	subscribe: (fn: (v: number) => void) => {
		return state.subscribe((s) => fn(s.channelSidebarWidth));
	},
	set(v: number) {
		state.update((s) => ({ ...s, channelSidebarWidth: clamp(v, MIN_SIDEBAR, MAX_SIDEBAR) }));
	},
	reset() {
		state.update((s) => ({ ...s, channelSidebarWidth: DEFAULTS.channelSidebarWidth }));
	}
};

export const memberListWidth = {
	subscribe: (fn: (v: number) => void) => {
		return state.subscribe((s) => fn(s.memberListWidth));
	},
	set(v: number) {
		state.update((s) => ({ ...s, memberListWidth: clamp(v, MIN_MEMBER_LIST, MAX_MEMBER_LIST) }));
	},
	reset() {
		state.update((s) => ({ ...s, memberListWidth: DEFAULTS.memberListWidth }));
	}
};
