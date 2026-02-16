import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getChannelNotificationPreferences: vi.fn().mockResolvedValue([]),
		updateChannelNotificationPreference: vi.fn().mockImplementation((data: any) =>
			Promise.resolve({ user_id: 'me', channel_id: data.channel_id, level: data.level, muted_until: data.muted_until ?? null })
		),
		deleteChannelNotificationPreference: vi.fn().mockResolvedValue(undefined),
		updateNotificationPreferences: vi.fn().mockImplementation((data: any) =>
			Promise.resolve({ user_id: 'me', guild_id: data.guild_id, level: data.level, suppress_everyone: false, suppress_roles: false, muted_until: data.muted_until ?? null })
		)
	}
}));

import {
	channelMutePrefs,
	guildMutePrefs,
	isChannelMuted,
	isGuildMuted,
	muteChannel,
	unmuteChannel,
	muteGuild,
	unmuteGuild,
	getMutedChannels,
	getMutedGuilds,
	loadChannelMutePrefs,
	_resetForTests
} from '../muting';
import { api } from '$lib/api/client';

describe('muting store', () => {
	beforeEach(() => {
		_resetForTests();
		vi.clearAllMocks();
	});

	describe('isChannelMuted', () => {
		it('returns false when channel has no preference', () => {
			expect(isChannelMuted('ch-1')).toBe(false);
		});

		it('returns true after muting a channel indefinitely', async () => {
			await muteChannel('ch-1');
			expect(isChannelMuted('ch-1')).toBe(true);
		});

		it('returns true after muting a channel with duration', async () => {
			await muteChannel('ch-1', 60 * 60 * 1000); // 1 hour
			expect(isChannelMuted('ch-1')).toBe(true);
		});

		it('returns false after unmuting a channel', async () => {
			await muteChannel('ch-1');
			expect(isChannelMuted('ch-1')).toBe(true);
			await unmuteChannel('ch-1');
			expect(isChannelMuted('ch-1')).toBe(false);
		});
	});

	describe('isGuildMuted', () => {
		it('returns false when guild has no preference', () => {
			expect(isGuildMuted('guild-1')).toBe(false);
		});

		it('returns true after muting a guild indefinitely', async () => {
			await muteGuild('guild-1');
			expect(isGuildMuted('guild-1')).toBe(true);
		});

		it('returns false after unmuting a guild', async () => {
			await muteGuild('guild-1');
			expect(isGuildMuted('guild-1')).toBe(true);
			await unmuteGuild('guild-1');
			expect(isGuildMuted('guild-1')).toBe(false);
		});
	});

	describe('muteChannel', () => {
		it('calls API with correct params for indefinite mute', async () => {
			await muteChannel('ch-1');
			expect(api.updateChannelNotificationPreference).toHaveBeenCalledWith({
				channel_id: 'ch-1',
				level: 'none',
				muted_until: null
			});
		});

		it('calls API with correct params for timed mute', async () => {
			const before = Date.now();
			await muteChannel('ch-1', 15 * 60 * 1000);
			const call = (api.updateChannelNotificationPreference as any).mock.calls[0][0];
			expect(call.channel_id).toBe('ch-1');
			expect(call.level).toBe('none');
			// Muted_until should be ~15 minutes from now.
			const mutedUntil = new Date(call.muted_until).getTime();
			expect(mutedUntil).toBeGreaterThanOrEqual(before + 15 * 60 * 1000 - 1000);
			expect(mutedUntil).toBeLessThanOrEqual(before + 15 * 60 * 1000 + 5000);
		});
	});

	describe('unmuteChannel', () => {
		it('calls delete API and removes from map', async () => {
			await muteChannel('ch-1');
			expect(isChannelMuted('ch-1')).toBe(true);
			await unmuteChannel('ch-1');
			expect(api.deleteChannelNotificationPreference).toHaveBeenCalledWith('ch-1');
			expect(isChannelMuted('ch-1')).toBe(false);
		});
	});

	describe('muteGuild', () => {
		it('calls API with correct params for indefinite mute', async () => {
			await muteGuild('guild-1');
			expect(api.updateNotificationPreferences).toHaveBeenCalledWith({
				guild_id: 'guild-1',
				level: 'none',
				muted_until: null
			});
		});
	});

	describe('getMutedChannels', () => {
		it('returns empty array when nothing is muted', () => {
			expect(getMutedChannels()).toEqual([]);
		});

		it('returns muted channels', async () => {
			await muteChannel('ch-1');
			await muteChannel('ch-2');
			const muted = getMutedChannels();
			expect(muted.length).toBe(2);
			expect(muted.map(m => m.channel_id).sort()).toEqual(['ch-1', 'ch-2']);
		});
	});

	describe('getMutedGuilds', () => {
		it('returns empty array when nothing is muted', () => {
			expect(getMutedGuilds()).toEqual([]);
		});

		it('returns muted guilds', async () => {
			await muteGuild('guild-1');
			const muted = getMutedGuilds();
			expect(muted.length).toBe(1);
			expect(muted[0].guild_id).toBe('guild-1');
		});
	});

	describe('loadChannelMutePrefs', () => {
		it('loads preferences from API', async () => {
			(api.getChannelNotificationPreferences as any).mockResolvedValueOnce([
				{ user_id: 'me', channel_id: 'ch-a', level: 'none', muted_until: null },
				{ user_id: 'me', channel_id: 'ch-b', level: 'mentions', muted_until: null }
			]);
			await loadChannelMutePrefs();
			const prefs = get(channelMutePrefs);
			expect(prefs.size).toBe(2);
			expect(prefs.get('ch-a')?.level).toBe('none');
			expect(prefs.get('ch-b')?.level).toBe('mentions');
		});
	});
});
