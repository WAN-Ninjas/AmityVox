// Toast notification store — manages in-app toast messages.

import { writable, derived } from 'svelte/store';

export interface Toast {
	id: string;
	message: string;
	type: 'info' | 'success' | 'error' | 'warning';
	duration: number;
}

const toastMap = writable<Map<string, Toast>>(new Map());

export const toasts = derived(toastMap, ($map) => Array.from($map.values()));

let counter = 0;

export function addToast(message: string, type: Toast['type'] = 'info', duration = 5000): string {
	const id = `toast-${++counter}`;
	const toast: Toast = { id, message, type, duration };
	toastMap.update((map) => {
		map.set(id, toast);
		return new Map(map);
	});

	if (duration > 0) {
		setTimeout(() => dismissToast(id), duration);
	}

	return id;
}

export function dismissToast(id: string) {
	toastMap.update((map) => {
		map.delete(id);
		return new Map(map);
	});
}
