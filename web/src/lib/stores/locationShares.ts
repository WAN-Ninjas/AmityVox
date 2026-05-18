import { api, type LocationShare } from '$lib/api/client';
import { createMapStore } from '$lib/stores/mapHelpers';

export const locationSharesByChannel = createMapStore<string, LocationShare[]>();

export async function loadLocationShares(channelId: string): Promise<LocationShare[]> {
	const locations = await api.getLocations(channelId);
	locationSharesByChannel.setEntry(channelId, locations ?? []);
	return locations ?? [];
}

export function setLocationShares(channelId: string, locations: LocationShare[]) {
	locationSharesByChannel.setEntry(channelId, locations);
}

export function handleLocationShareChanged(data: { id?: string; channel_id?: string }) {
	if (!data.channel_id) return;
	loadLocationShares(data.channel_id).catch((err) => {
		console.warn('Failed to refresh location shares after gateway event', err);
	});
}

export function removeLocationShare(data: { id?: string; channel_id?: string }) {
	if (!data.channel_id || !data.id) return;
	locationSharesByChannel.updateEntry(data.channel_id, (locations) =>
		locations.map((location) =>
			location.id === data.id
				? { ...location, live: false, expires_at: new Date().toISOString(), updated_at: new Date().toISOString() }
				: location
		)
	);
}
