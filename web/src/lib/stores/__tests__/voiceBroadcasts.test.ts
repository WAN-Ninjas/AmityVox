import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: {
		getVoiceBroadcast: vi.fn()
	}
}));

import { api, type VoiceBroadcast } from '$lib/api/client';
import { clientConfig } from '$lib/stores/clientConfig';
import {
	handleVoiceBroadcastEnd,
	handleVoiceBroadcastStart,
	handleVoiceBroadcastUpdate,
	loadVoiceBroadcast,
	upsertVoiceBroadcast,
	voiceBroadcastsByChannel
} from '../voiceBroadcasts';

function createBroadcast(overrides?: Partial<VoiceBroadcast>): VoiceBroadcast {
	return {
		id: 'broadcast-1',
		guild_id: 'guild-1',
		channel_id: 'voice-1',
		broadcaster_id: 'user-1',
		title: 'Live Broadcast',
		started_at: '2026-05-15T08:00:00.000Z',
		ended_at: null,
		listener_count: 0,
		...overrides
	};
}

describe('voiceBroadcasts store', () => {
	beforeEach(() => {
		voiceBroadcastsByChannel.set(new Map());
		clientConfig.set({ feature_flags: { voice_broadcasts: { enabled: true } } } as any);
		vi.mocked(api.getVoiceBroadcast).mockReset();
	});

	it('loads and clears active broadcast state', async () => {
		vi.mocked(api.getVoiceBroadcast).mockResolvedValue(createBroadcast());
		await loadVoiceBroadcast('voice-1');
		expect(get(voiceBroadcastsByChannel).get('voice-1')?.id).toBe('broadcast-1');

		vi.mocked(api.getVoiceBroadcast).mockResolvedValue(null);
		await loadVoiceBroadcast('voice-1');
		expect(get(voiceBroadcastsByChannel).has('voice-1')).toBe(false);
	});

	it('handles gateway start and end events', () => {
		handleVoiceBroadcastStart({
			broadcast_id: 'broadcast-2',
			guild_id: 'guild-1',
			channel_id: 'voice-1',
			broadcaster_id: 'user-2',
			title: 'Town Hall'
		});

		expect(get(voiceBroadcastsByChannel).get('voice-1')?.title).toBe('Town Hall');

		handleVoiceBroadcastEnd({ channel_id: 'voice-1' });
		expect(get(voiceBroadcastsByChannel).has('voice-1')).toBe(false);
	});

	it('upserts broadcasts by channel', () => {
		upsertVoiceBroadcast(createBroadcast({ title: 'Original' }));
		upsertVoiceBroadcast(createBroadcast({ title: 'Updated' }));

		expect(get(voiceBroadcastsByChannel).get('voice-1')?.title).toBe('Updated');
	});

	it('applies listener count updates without replacing existing broadcast details', () => {
		upsertVoiceBroadcast(createBroadcast({ title: 'Town Hall', listener_count: 0 }));

		handleVoiceBroadcastUpdate({
			channel_id: 'voice-1',
			listener_count: 4
		});

		const broadcast = get(voiceBroadcastsByChannel).get('voice-1');
		expect(broadcast?.title).toBe('Town Hall');
		expect(broadcast?.listener_count).toBe(4);
	});
});
