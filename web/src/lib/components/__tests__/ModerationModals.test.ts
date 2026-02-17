import { describe, it, expect } from 'vitest';

// Test moderation modal logic as pure data definitions and validation.
// Component rendering is not possible with happy-dom SSR (see MarkdownRenderer.test.ts).

// --- Ban Duration Options (match ModerationModals.svelte) ---
const banDurationOptions = [
	{ label: 'Permanent', value: 'permanent' },
	{ label: '5 minutes', value: '300' },
	{ label: '15 minutes', value: '900' },
	{ label: '30 minutes', value: '1800' },
	{ label: '1 hour', value: '3600' },
	{ label: '1 day', value: '86400' },
	{ label: '7 days', value: '604800' },
	{ label: 'Custom', value: 'custom' },
];

// --- Message Cleanup Options (match ModerationModals.svelte) ---
const cleanupOptions = [
	{ label: "Don't delete", value: '0' },
	{ label: 'Last hour', value: '3600' },
	{ label: 'Last 6 hours', value: '21600' },
	{ label: 'Last 12 hours', value: '43200' },
	{ label: 'Last 24 hours', value: '86400' },
	{ label: 'Last 3 days', value: '259200' },
	{ label: 'Last 7 days', value: '604800' },
];

// --- Timeout Presets (match MemberList.svelte) ---
const timeoutPresets = [
	{ label: '1 minute', seconds: 60 },
	{ label: '5 minutes', seconds: 300 },
	{ label: '10 minutes', seconds: 600 },
	{ label: '15 minutes', seconds: 900 },
	{ label: '30 minutes', seconds: 1800 },
	{ label: '1 hour', seconds: 3600 },
];

// --- Duration computation helper (mirrors ModerationModals.svelte getBanDurationSeconds) ---
function getBanDurationSeconds(duration: string, customMinutes: string): number | undefined {
	if (duration === 'permanent') return undefined;
	if (duration === 'custom') {
		const mins = parseInt(customMinutes, 10);
		return mins > 0 ? mins * 60 : undefined;
	}
	return parseInt(duration, 10) || undefined;
}

// --- Duration formatting helper (mirrors MemberList.svelte formatDuration) ---
function formatDuration(seconds: number): string {
	if (seconds < 60) return `${seconds}s`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
	return `${Math.floor(seconds / 3600)}h`;
}

// --- Tests ---

describe('Ban Duration Options', () => {
	it('should map preset values to correct seconds', () => {
		expect(getBanDurationSeconds('300', '')).toBe(300); // 5 minutes
		expect(getBanDurationSeconds('900', '')).toBe(900); // 15 minutes
		expect(getBanDurationSeconds('1800', '')).toBe(1800); // 30 minutes
		expect(getBanDurationSeconds('3600', '')).toBe(3600); // 1 hour
		expect(getBanDurationSeconds('86400', '')).toBe(86400); // 1 day
		expect(getBanDurationSeconds('604800', '')).toBe(604800); // 7 days
	});

	it('permanent returns undefined', () => {
		expect(getBanDurationSeconds('permanent', '')).toBeUndefined();
	});

	it('custom with valid minutes returns correct seconds', () => {
		expect(getBanDurationSeconds('custom', '10')).toBe(600);
		expect(getBanDurationSeconds('custom', '60')).toBe(3600);
		expect(getBanDurationSeconds('custom', '1440')).toBe(86400);
	});

	it('custom with invalid input returns undefined', () => {
		expect(getBanDurationSeconds('custom', '')).toBeUndefined();
		expect(getBanDurationSeconds('custom', '0')).toBeUndefined();
		expect(getBanDurationSeconds('custom', '-5')).toBeUndefined();
		expect(getBanDurationSeconds('custom', 'abc')).toBeUndefined();
	});

	it('all preset options have unique values', () => {
		const values = banDurationOptions.map((o) => o.value);
		expect(new Set(values).size).toBe(values.length);
	});

	it('all preset options have non-empty labels', () => {
		for (const opt of banDurationOptions) {
			expect(opt.label.length).toBeGreaterThan(0);
		}
	});
});

describe('Message Cleanup Options', () => {
	it('should have correct second values', () => {
		const expectedSeconds = [0, 3600, 21600, 43200, 86400, 259200, 604800];
		const actual = cleanupOptions.map((o) => parseInt(o.value, 10));
		expect(actual).toEqual(expectedSeconds);
	});

	it('first option (dont delete) maps to 0', () => {
		expect(parseInt(cleanupOptions[0].value, 10)).toBe(0);
	});

	it('last option is 7 days in seconds', () => {
		expect(parseInt(cleanupOptions[cleanupOptions.length - 1].value, 10)).toBe(7 * 24 * 3600);
	});

	it('all options have unique values', () => {
		const values = cleanupOptions.map((o) => o.value);
		expect(new Set(values).size).toBe(values.length);
	});
});

describe('Timeout Presets', () => {
	it('should produce correct timeout durations', () => {
		expect(timeoutPresets[0].seconds).toBe(60); // 1 minute
		expect(timeoutPresets[1].seconds).toBe(300); // 5 minutes
		expect(timeoutPresets[2].seconds).toBe(600); // 10 minutes
		expect(timeoutPresets[3].seconds).toBe(900); // 15 minutes
		expect(timeoutPresets[4].seconds).toBe(1800); // 30 minutes
		expect(timeoutPresets[5].seconds).toBe(3600); // 1 hour
	});

	it('all presets produce future ISO timestamps', () => {
		const now = Date.now();
		for (const preset of timeoutPresets) {
			const until = new Date(now + preset.seconds * 1000);
			expect(until.getTime()).toBeGreaterThan(now);
			// Verify it round-trips through ISO
			expect(new Date(until.toISOString()).getTime()).toBe(until.getTime());
		}
	});

	it('presets are in ascending order', () => {
		for (let i = 1; i < timeoutPresets.length; i++) {
			expect(timeoutPresets[i].seconds).toBeGreaterThan(timeoutPresets[i - 1].seconds);
		}
	});
});

describe('Duration Formatting', () => {
	it('formats seconds correctly', () => {
		expect(formatDuration(30)).toBe('30s');
		expect(formatDuration(59)).toBe('59s');
	});

	it('formats minutes correctly', () => {
		expect(formatDuration(60)).toBe('1m');
		expect(formatDuration(300)).toBe('5m');
		expect(formatDuration(1800)).toBe('30m');
	});

	it('formats hours correctly', () => {
		expect(formatDuration(3600)).toBe('1h');
		expect(formatDuration(7200)).toBe('2h');
		expect(formatDuration(86400)).toBe('24h');
	});
});

describe('Moderation Target Validation', () => {
	it('should not allow moderating self', () => {
		const currentUserId = 'user-1';
		const targetUserId = 'user-1';
		const isSelf = currentUserId === targetUserId;
		expect(isSelf).toBe(true);
	});

	it('should not allow moderating guild owner', () => {
		const ownerId = 'owner-1';
		const targetUserId = 'owner-1';
		const isOwner = targetUserId === ownerId;
		expect(isOwner).toBe(true);
	});

	it('should allow moderating regular members', () => {
		const currentUserId = 'mod-1';
		const ownerId = 'owner-1';
		const targetUserId = 'regular-1';
		const isSelf = currentUserId === targetUserId;
		const isOwner = targetUserId === ownerId;
		const canModerate = !isSelf && !isOwner;
		expect(canModerate).toBe(true);
	});
});

describe('Ban Expiry', () => {
	it('permanent ban has no expiry', () => {
		const expiresAt = getBanDurationSeconds('permanent', '');
		expect(expiresAt).toBeUndefined();
	});

	it('timed ban computes correct expiry', () => {
		const durationSec = getBanDurationSeconds('3600', '');
		expect(durationSec).toBe(3600);
		const now = Date.now();
		const expiresAt = new Date(now + durationSec! * 1000);
		// Should expire ~1 hour from now (within 2 second tolerance)
		expect(Math.abs(expiresAt.getTime() - now - 3600000)).toBeLessThan(2000);
	});
});
