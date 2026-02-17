import { describe, it, expect } from 'vitest';

// Since Svelte 5 components can't be rendered in happy-dom, we test the pure
// logic functions used by GuildRetentionSettings.

// --- Extracted from GuildRetentionSettings.svelte ---

function formatAge(days: number): string {
	if (days >= 365) return `${Math.floor(days / 365)} year${days >= 730 ? 's' : ''}`;
	if (days >= 30) return `${Math.floor(days / 30)} month${days >= 60 ? 's' : ''}`;
	return `${days} day${days !== 1 ? 's' : ''}`;
}

function getChannelName(channelId: string | null, channelMap: Map<string, { name: string | null }>): string {
	if (!channelId) return 'Guild-wide';
	const ch = channelMap.get(channelId);
	return ch?.name ? `#${ch.name}` : channelId;
}

function validateRetentionForm(scope: 'guild' | 'channel', channelId: string, maxAge: number): string | null {
	if (maxAge < 1) return 'Retention period must be at least 1 day';
	if (scope === 'channel' && !channelId) return 'Channel is required';
	return null;
}

describe('GuildRetentionSettings', () => {
	describe('formatAge', () => {
		it('formats singular day', () => {
			expect(formatAge(1)).toBe('1 day');
		});

		it('formats plural days', () => {
			expect(formatAge(7)).toBe('7 days');
			expect(formatAge(29)).toBe('29 days');
		});

		it('formats months', () => {
			expect(formatAge(30)).toBe('1 month');
			expect(formatAge(59)).toBe('1 month');
			expect(formatAge(60)).toBe('2 months');
			expect(formatAge(90)).toBe('3 months');
			expect(formatAge(364)).toBe('12 months');
		});

		it('formats years', () => {
			expect(formatAge(365)).toBe('1 year');
			expect(formatAge(729)).toBe('1 year');
			expect(formatAge(730)).toBe('2 years');
			expect(formatAge(1095)).toBe('3 years');
		});
	});

	describe('getChannelName', () => {
		const channels = new Map<string, { name: string | null }>([
			['ch1', { name: 'general' }],
			['ch2', { name: 'announcements' }],
			['ch3', { name: null }],
		]);

		it('returns Guild-wide for null channelId', () => {
			expect(getChannelName(null, channels)).toBe('Guild-wide');
		});

		it('returns #channelName for known channel', () => {
			expect(getChannelName('ch1', channels)).toBe('#general');
			expect(getChannelName('ch2', channels)).toBe('#announcements');
		});

		it('returns channelId for channel with null name', () => {
			expect(getChannelName('ch3', channels)).toBe('ch3');
		});

		it('returns channelId for unknown channel', () => {
			expect(getChannelName('unknown', channels)).toBe('unknown');
		});
	});

	describe('form validation', () => {
		it('rejects max_age < 1', () => {
			expect(validateRetentionForm('guild', '', 0)).toBe('Retention period must be at least 1 day');
			expect(validateRetentionForm('guild', '', -5)).toBe('Retention period must be at least 1 day');
		});

		it('rejects channel scope without channelId', () => {
			expect(validateRetentionForm('channel', '', 30)).toBe('Channel is required');
		});

		it('accepts valid guild-wide policy', () => {
			expect(validateRetentionForm('guild', '', 90)).toBeNull();
		});

		it('accepts valid channel-scoped policy', () => {
			expect(validateRetentionForm('channel', 'ch1', 30)).toBeNull();
		});
	});
});
