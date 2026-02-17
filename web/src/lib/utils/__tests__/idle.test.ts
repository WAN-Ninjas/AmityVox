import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
	setManualStatus,
	getManualStatus,
	isAutoIdled,
	startIdleDetection,
	stopIdleDetection,
	computeExpiryTime
} from '../idle';

// Mock external dependencies — must mock before idle.ts resolves its imports.
// $app/navigation is a SvelteKit virtual module unavailable in vitest.
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/stores/gateway', () => ({
	getGatewayClient: () => ({
		updatePresence: vi.fn()
	})
}));

vi.mock('$lib/stores/presence', () => ({
	updatePresence: vi.fn()
}));

vi.mock('$lib/stores/auth', () => ({
	currentUser: {
		subscribe: (fn: (val: any) => void) => {
			fn({
				id: 'test-user',
				status_presence: 'online'
			});
			return () => {};
		}
	}
}));

vi.mock('svelte/store', () => ({
	get: (store: any) => {
		let val: any;
		store.subscribe((v: any) => { val = v; })();
		return val;
	},
	writable: (initial: any) => {
		let value = initial;
		const subs: any[] = [];
		return {
			subscribe: (fn: any) => { fn(value); subs.push(fn); return () => {}; },
			set: (v: any) => { value = v; subs.forEach(fn => fn(v)); },
			update: (fn: any) => { value = fn(value); subs.forEach(s => s(value)); }
		};
	}
}));

describe('idle detection', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		stopIdleDetection();
		setManualStatus('online');
	});

	afterEach(() => {
		stopIdleDetection();
		vi.useRealTimers();
	});

	describe('setManualStatus', () => {
		it('should track the manual status', () => {
			setManualStatus('busy');
			expect(getManualStatus()).toBe('busy');
		});

		it('should update when changed', () => {
			setManualStatus('online');
			expect(getManualStatus()).toBe('online');
			setManualStatus('invisible');
			expect(getManualStatus()).toBe('invisible');
		});
	});

	describe('isAutoIdled', () => {
		it('should be false initially', () => {
			expect(isAutoIdled()).toBe(false);
		});

		it('should be false when not running', () => {
			expect(isAutoIdled()).toBe(false);
		});
	});

	describe('startIdleDetection / stopIdleDetection', () => {
		it('should add and remove event listeners', () => {
			const addSpy = vi.spyOn(document, 'addEventListener');
			const removeSpy = vi.spyOn(document, 'removeEventListener');

			startIdleDetection();
			expect(addSpy).toHaveBeenCalledWith('mousemove', expect.any(Function), { passive: true });
			expect(addSpy).toHaveBeenCalledWith('keydown', expect.any(Function), { passive: true });
			expect(addSpy).toHaveBeenCalledWith('mousedown', expect.any(Function), { passive: true });
			expect(addSpy).toHaveBeenCalledWith('touchstart', expect.any(Function), { passive: true });

			stopIdleDetection();
			expect(removeSpy).toHaveBeenCalledWith('mousemove', expect.any(Function));
			expect(removeSpy).toHaveBeenCalledWith('keydown', expect.any(Function));

			addSpy.mockRestore();
			removeSpy.mockRestore();
		});

		it('should not double-register listeners', () => {
			const addSpy = vi.spyOn(document, 'addEventListener');

			startIdleDetection();
			startIdleDetection(); // Second call should be a no-op

			// Should only be called 4 times (once per event type)
			const activityCalls = addSpy.mock.calls.filter(
				(c) => ['mousemove', 'keydown', 'mousedown', 'touchstart'].includes(c[0] as string)
			);
			expect(activityCalls).toHaveLength(4);

			stopIdleDetection();
			addSpy.mockRestore();
		});
	});

	describe('manual status override', () => {
		it('busy status should prevent auto-idle', () => {
			setManualStatus('busy');
			startIdleDetection();

			// Advance past idle timeout
			vi.advanceTimersByTime(6 * 60 * 1000);

			expect(isAutoIdled()).toBe(false);
			stopIdleDetection();
		});

		it('invisible status should prevent auto-idle', () => {
			setManualStatus('invisible');
			startIdleDetection();

			vi.advanceTimersByTime(6 * 60 * 1000);

			expect(isAutoIdled()).toBe(false);
			stopIdleDetection();
		});
	});
});

describe('computeExpiryTime', () => {
	it('returns null for null input', () => {
		expect(computeExpiryTime(null)).toBeNull();
	});

	it('returns end of day for -1', () => {
		const result = computeExpiryTime(-1);
		expect(result).toBeTruthy();
		const date = new Date(result!);
		expect(date.getHours()).toBe(23);
		expect(date.getMinutes()).toBe(59);
		expect(date.getSeconds()).toBe(59);
	});

	it('returns future timestamp for positive ms', () => {
		const before = Date.now();
		const result = computeExpiryTime(3600000); // 1 hour
		const ts = new Date(result!).getTime();
		expect(ts).toBeGreaterThanOrEqual(before + 3600000);
		expect(ts).toBeLessThanOrEqual(before + 3600000 + 50); // small tolerance
	});

	it('returns ISO 8601 format', () => {
		const result = computeExpiryTime(1000);
		expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/);
	});
});
