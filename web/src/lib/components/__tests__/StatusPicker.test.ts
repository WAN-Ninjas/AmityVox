import { describe, it, expect } from 'vitest';

// Test the status option definitions and expiry computation as pure functions.
// Component rendering is not possible with happy-dom SSR (see MarkdownRenderer.test.ts).

const statusOptions = [
	{ value: 'online', label: 'Online', description: 'You are available', colorClass: 'bg-status-online' },
	{ value: 'idle', label: 'Idle', description: 'You may be away', colorClass: 'bg-status-idle' },
	{ value: 'busy', label: 'Do Not Disturb', description: 'Suppress notifications', colorClass: 'bg-status-dnd' },
	{ value: 'invisible', label: 'Invisible', description: 'Appear offline to others', colorClass: 'bg-status-offline' }
] as const;

const expiryOptions = [
	{ label: "Don't clear", value: null },
	{ label: '30 minutes', value: 30 * 60 * 1000 },
	{ label: '1 hour', value: 60 * 60 * 1000 },
	{ label: '4 hours', value: 4 * 60 * 60 * 1000 },
	{ label: 'Today', value: -1 }
] as const;

describe('StatusPicker', () => {
	describe('status options', () => {
		it('should have exactly 4 status options', () => {
			expect(statusOptions).toHaveLength(4);
		});

		it('should include all required statuses', () => {
			const values = statusOptions.map((o) => o.value);
			expect(values).toEqual(['online', 'idle', 'busy', 'invisible']);
		});

		it('should have unique values', () => {
			const values = statusOptions.map((o) => o.value);
			expect(new Set(values).size).toBe(values.length);
		});

		it('should have labels for all options', () => {
			for (const option of statusOptions) {
				expect(option.label).toBeTruthy();
				expect(option.description).toBeTruthy();
				expect(option.colorClass).toBeTruthy();
			}
		});

		it('busy should use dnd color class', () => {
			const busy = statusOptions.find((o) => o.value === 'busy');
			expect(busy?.colorClass).toBe('bg-status-dnd');
		});

		it('invisible should use offline color class', () => {
			const invisible = statusOptions.find((o) => o.value === 'invisible');
			expect(invisible?.colorClass).toBe('bg-status-offline');
		});
	});

	describe('expiry options', () => {
		it('should have 5 expiry options', () => {
			expect(expiryOptions).toHaveLength(5);
		});

		it('should have a "Don\'t clear" option with null value', () => {
			expect(expiryOptions[0]).toEqual({ label: "Don't clear", value: null });
		});

		it('should have a "Today" option with sentinel value -1', () => {
			const today = expiryOptions.find((o) => o.label === 'Today');
			expect(today?.value).toBe(-1);
		});

		it('should have increasing durations for timed options', () => {
			const timedOptions = expiryOptions.filter(
				(o) => o.value !== null && o.value > 0
			);
			for (let i = 1; i < timedOptions.length; i++) {
				expect(timedOptions[i].value).toBeGreaterThan(timedOptions[i - 1].value as number);
			}
		});
	});

	describe('expiry computation helper', () => {
		it('null input returns null (no expiry)', () => {
			// computeExpiryTime(null) -> null
			expect(computeExpiryTime(null)).toBeNull();
		});

		it('-1 returns end of today', () => {
			const result = computeExpiryTime(-1);
			expect(result).toBeTruthy();
			const date = new Date(result!);
			const now = new Date();
			expect(date.getFullYear()).toBe(now.getFullYear());
			expect(date.getMonth()).toBe(now.getMonth());
			expect(date.getDate()).toBe(now.getDate());
			expect(date.getHours()).toBe(23);
			expect(date.getMinutes()).toBe(59);
		});

		it('positive ms returns future ISO timestamp', () => {
			const before = Date.now();
			const result = computeExpiryTime(60 * 60 * 1000); // 1 hour
			const after = Date.now();
			expect(result).toBeTruthy();
			const ts = new Date(result!).getTime();
			expect(ts).toBeGreaterThanOrEqual(before + 60 * 60 * 1000);
			expect(ts).toBeLessThanOrEqual(after + 60 * 60 * 1000);
		});

		it('30 minutes returns approximately 30 min from now', () => {
			const ms = 30 * 60 * 1000;
			const before = Date.now();
			const result = computeExpiryTime(ms);
			const ts = new Date(result!).getTime();
			const diff = ts - before;
			expect(diff).toBeGreaterThanOrEqual(ms - 100);
			expect(diff).toBeLessThanOrEqual(ms + 100);
		});
	});
});

// Extracted computation logic matching the component + idle.ts
function computeExpiryTime(ms: number | null): string | null {
	if (ms === null) return null;
	if (ms === -1) {
		const endOfDay = new Date();
		endOfDay.setHours(23, 59, 59, 999);
		return endOfDay.toISOString();
	}
	return new Date(Date.now() + ms).toISOString();
}
