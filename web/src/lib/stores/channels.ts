// Channel store — manages channels for the current guild.

import { writable, derived } from 'svelte/store';
import type { Channel } from '$lib/types';
import { api } from '$lib/api/client';

export const channels = writable<Map<string, Channel>>(new Map());
export const currentChannelId = writable<string | null>(null);

export const channelList = derived(channels, ($channels) =>
	Array.from($channels.values()).sort((a, b) => a.position - b.position)
);

export const textChannels = derived(channelList, ($list) =>
	$list.filter((c) => c.channel_type === 'text' || c.channel_type === 'announcement')
);

export const voiceChannels = derived(channelList, ($list) =>
	$list.filter((c) => c.channel_type === 'voice' || c.channel_type === 'stage')
);

export const currentChannel = derived(
	[channels, currentChannelId],
	([$channels, $id]) => ($id ? $channels.get($id) ?? null : null)
);

export async function loadChannels(guildId: string) {
	const list = await api.getGuildChannels(guildId);
	const map = new Map<string, Channel>();
	for (const c of list) {
		map.set(c.id, c);
	}
	channels.set(map);
}

export function setChannel(id: string | null) {
	currentChannelId.set(id);
}

export function updateChannel(channel: Channel) {
	channels.update((map) => {
		map.set(channel.id, channel);
		return new Map(map);
	});
}

export function removeChannel(channelId: string) {
	channels.update((map) => {
		map.delete(channelId);
		return new Map(map);
	});
}
