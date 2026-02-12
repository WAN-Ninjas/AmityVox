import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { toasts, addToast, dismissToast } from '../toast';

describe('toast store', () => {
	beforeEach(() => {
		// Clear all toasts.
		const current = get(toasts);
		current.forEach(t => dismissToast(t.id));
	});

	it('adds a toast', () => {
		const id = addToast('Test message', 'info', 0);
		const list = get(toasts);
		expect(list).toHaveLength(1);
		expect(list[0].id).toBe(id);
		expect(list[0].message).toBe('Test message');
		expect(list[0].type).toBe('info');
		dismissToast(id);
	});

	it('adds multiple toasts', () => {
		const id1 = addToast('First', 'info', 0);
		const id2 = addToast('Second', 'error', 0);
		const id3 = addToast('Third', 'success', 0);
		expect(get(toasts)).toHaveLength(3);
		dismissToast(id1);
		dismissToast(id2);
		dismissToast(id3);
	});

	it('dismisses a specific toast', () => {
		const id1 = addToast('Keep', 'info', 0);
		const id2 = addToast('Remove', 'error', 0);
		dismissToast(id2);
		const list = get(toasts);
		expect(list).toHaveLength(1);
		expect(list[0].id).toBe(id1);
		dismissToast(id1);
	});

	it('auto-dismisses after duration', () => {
		vi.useFakeTimers();
		const id = addToast('Auto dismiss', 'info', 3000);
		expect(get(toasts)).toHaveLength(1);
		vi.advanceTimersByTime(3000);
		expect(get(toasts)).toHaveLength(0);
		vi.useRealTimers();
	});

	it('supports different toast types', () => {
		const ids = [
			addToast('Info', 'info', 0),
			addToast('Success', 'success', 0),
			addToast('Error', 'error', 0),
			addToast('Warning', 'warning', 0)
		];
		const list = get(toasts);
		expect(list.map(t => t.type)).toEqual(['info', 'success', 'error', 'warning']);
		ids.forEach(id => dismissToast(id));
	});

	it('handles dismissing non-existent toast gracefully', () => {
		dismissToast('non-existent-id');
		expect(get(toasts)).toHaveLength(0);
	});
});
