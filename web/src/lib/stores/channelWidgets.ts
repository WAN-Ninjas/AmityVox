import { writable } from 'svelte/store';
import { get } from 'svelte/store';
import { api, type ChannelWidget } from '$lib/api/client';
import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';

export const channelWidgetsByChannel = writable<Map<string, ChannelWidget[]>>(new Map());

let loadRequestSequence = 0;
const latestLoadByChannel = new Map<string, number>();

export async function loadChannelWidgets(channelId: string) {
	if (!isFeatureEnabled(get(clientConfig), 'widgets')) {
		channelWidgetsByChannel.update((map) => {
			const next = new Map(map);
			next.delete(channelId);
			return next;
		});
		return;
	}
	const requestId = ++loadRequestSequence;
	latestLoadByChannel.set(channelId, requestId);

	const widgets = await api.getChannelWidgets(channelId);
	if (latestLoadByChannel.get(channelId) !== requestId) return;

	channelWidgetsByChannel.update((map) => {
		const next = new Map(map);
		next.set(channelId, sortWidgets(widgets));
		return next;
	});
}

export function upsertChannelWidget(widget: ChannelWidget) {
	channelWidgetsByChannel.update((map) => {
		const next = new Map(map);
		const existing = next.get(widget.channel_id) ?? [];
		next.set(
			widget.channel_id,
			sortWidgets([...existing.filter((item) => item.id !== widget.id), widget])
		);
		return next;
	});
}

export function removeChannelWidget(channelId: string, widgetId: string) {
	channelWidgetsByChannel.update((map) => {
		const existing = map.get(channelId);
		if (!existing) return map;

		const next = new Map(map);
		next.set(channelId, existing.filter((widget) => widget.id !== widgetId));
		return next;
	});
}

function sortWidgets(widgets: ChannelWidget[]) {
	return [...widgets].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id));
}
