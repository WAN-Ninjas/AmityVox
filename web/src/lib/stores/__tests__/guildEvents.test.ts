import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: {
		getGuildEvents: vi.fn()
	}
}));

import { api } from '$lib/api/client';
import {
	guildEventsByGuild,
	loadGuildEvents,
	upsertGuildEvent,
	removeGuildEvent
} from '../guildEvents';
import type { GuildEvent } from '$lib/types';

function createMockEvent(overrides?: Partial<GuildEvent>): GuildEvent {
	return {
		id: crypto.randomUUID(),
		guild_id: 'guild-1',
		creator_id: 'user-1',
		name: 'Town Hall',
		description: null,
		location: null,
		channel_id: null,
		image_id: null,
		scheduled_start: new Date(Date.now() + 60_000).toISOString(),
		scheduled_end: null,
		status: 'scheduled',
		interested_count: 0,
		created_at: new Date().toISOString(),
		user_rsvp: null,
		...overrides
	};
}

describe('guildEvents store', () => {
	beforeEach(() => {
		guildEventsByGuild.set(new Map());
		vi.mocked(api.getGuildEvents).mockReset();
	});

	it('loads and sorts upcoming guild events', async () => {
		const later = createMockEvent({ id: 'later', scheduled_start: '2026-06-02T12:00:00.000Z' });
		const sooner = createMockEvent({ id: 'sooner', scheduled_start: '2026-06-01T12:00:00.000Z' });
		vi.mocked(api.getGuildEvents).mockResolvedValue([later, sooner]);

		await loadGuildEvents('guild-1');

		expect(vi.mocked(api.getGuildEvents)).toHaveBeenCalledWith('guild-1', {
			status: 'scheduled',
			limit: 5
		});
		expect(get(guildEventsByGuild).get('guild-1')?.map((event) => event.id)).toEqual([
			'sooner',
			'later'
		]);
	});

	it('upserts scheduled events and removes non-scheduled updates', () => {
		const event = createMockEvent({ id: 'event-1' });

		upsertGuildEvent(event);
		expect(get(guildEventsByGuild).get('guild-1')?.[0].id).toBe('event-1');

		upsertGuildEvent({ ...event, status: 'cancelled' });
		expect(get(guildEventsByGuild).get('guild-1')).toEqual([]);
	});

	it('removes deleted events', () => {
		const event = createMockEvent({ id: 'event-1' });
		upsertGuildEvent(event);

		removeGuildEvent('guild-1', 'event-1');

		expect(get(guildEventsByGuild).get('guild-1')).toEqual([]);
	});
});
