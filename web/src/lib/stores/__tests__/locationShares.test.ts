import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: {
		getLocations: vi.fn()
	}
}));

import { api, type LocationShare } from '$lib/api/client';
import {
	loadLocationShares,
	locationSharesByChannel,
	removeLocationShare,
	setLocationShares
} from '../locationShares';

function createLocation(overrides?: Partial<LocationShare>): LocationShare {
	return {
		id: 'loc-1',
		user_id: 'user-1',
		channel_id: 'channel-1',
		latitude: 40,
		longitude: -74,
		live: true,
		created_at: '2026-05-15T08:00:00.000Z',
		updated_at: '2026-05-15T08:00:00.000Z',
		username: 'alice',
		...overrides
	};
}

describe('locationShares store', () => {
	beforeEach(() => {
		locationSharesByChannel.set(new Map());
		vi.mocked(api.getLocations).mockReset();
	});

	it('loads locations for a channel', async () => {
		vi.mocked(api.getLocations).mockResolvedValue([createLocation()]);

		await loadLocationShares('channel-1');

		expect(vi.mocked(api.getLocations)).toHaveBeenCalledWith('channel-1');
		expect(get(locationSharesByChannel).get('channel-1')?.[0].id).toBe('loc-1');
	});

	it('marks a stopped live location inactive', () => {
		setLocationShares('channel-1', [createLocation()]);

		removeLocationShare({ id: 'loc-1', channel_id: 'channel-1' });

		const location = get(locationSharesByChannel).get('channel-1')?.[0];
		expect(location?.live).toBe(false);
		expect(location?.expires_at).toBeTruthy();
	});
});
