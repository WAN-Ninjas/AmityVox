import { createMapStore } from '$lib/stores/mapHelpers';

export const activityInvalidationsByChannel = createMapStore<string, number>();

export function invalidateChannelActivity(channelId?: string) {
	if (!channelId) return;
	activityInvalidationsByChannel.update((map) => {
		const next = new Map(map);
		next.set(channelId, (next.get(channelId) ?? 0) + 1);
		return next;
	});
}
