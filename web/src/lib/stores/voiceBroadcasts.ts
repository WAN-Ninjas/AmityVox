import { api, type VoiceBroadcast } from '$lib/api/client';
import { get } from 'svelte/store';
import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';
import { createMapStore } from '$lib/stores/mapHelpers';

export const voiceBroadcastsByChannel = createMapStore<string, VoiceBroadcast>();

export async function loadVoiceBroadcast(channelId: string): Promise<VoiceBroadcast | null> {
	if (!isFeatureEnabled(get(clientConfig), 'voice_broadcasts')) {
		voiceBroadcastsByChannel.removeEntry(channelId);
		return null;
	}
	const broadcast = await api.getVoiceBroadcast(channelId);
	if (broadcast) {
		voiceBroadcastsByChannel.setEntry(channelId, broadcast);
	} else {
		voiceBroadcastsByChannel.removeEntry(channelId);
	}
	return broadcast;
}

export function upsertVoiceBroadcast(broadcast: VoiceBroadcast) {
	voiceBroadcastsByChannel.setEntry(broadcast.channel_id, broadcast);
}

export function handleVoiceBroadcastStart(data: {
	broadcast_id?: string;
	id?: string;
	guild_id: string;
	channel_id: string;
	broadcaster_id: string;
	title?: string;
	started_at?: string;
	listener_count?: number;
}) {
	upsertVoiceBroadcast({
		id: data.broadcast_id ?? data.id ?? `${data.channel_id}:${data.broadcaster_id}`,
		guild_id: data.guild_id,
		channel_id: data.channel_id,
		broadcaster_id: data.broadcaster_id,
		title: data.title || 'Live Broadcast',
		started_at: data.started_at ?? new Date().toISOString(),
		ended_at: null,
		listener_count: data.listener_count ?? 0
	});
}

export function handleVoiceBroadcastEnd(data: { channel_id: string }) {
	voiceBroadcastsByChannel.removeEntry(data.channel_id);
}

export function handleVoiceBroadcastUpdate(data: {
	broadcast_id?: string;
	id?: string;
	guild_id?: string;
	channel_id: string;
	broadcaster_id?: string;
	title?: string;
	started_at?: string;
	listener_count?: number;
}) {
	voiceBroadcastsByChannel.update((broadcasts) => {
		const current = broadcasts.get(data.channel_id);
		if (!current) return broadcasts;
		const next = new Map(broadcasts);
		next.set(data.channel_id, {
			...current,
			id: data.broadcast_id ?? data.id ?? current.id,
			guild_id: data.guild_id ?? current.guild_id,
			broadcaster_id: data.broadcaster_id ?? current.broadcaster_id,
			title: data.title ?? current.title,
			started_at: data.started_at ?? current.started_at,
			listener_count: data.listener_count ?? current.listener_count
		});
		return next;
	});
}
