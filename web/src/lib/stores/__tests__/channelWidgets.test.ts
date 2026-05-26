import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	api: {
		getChannelWidgets: vi.fn()
	}
}));

import { api, type ChannelWidget } from '$lib/api/client';
import { clientConfig } from '$lib/stores/clientConfig';
import {
	channelWidgetsByChannel,
	loadChannelWidgets,
	upsertChannelWidget,
	removeChannelWidget
} from '../channelWidgets';

function createWidget(overrides?: Partial<ChannelWidget>): ChannelWidget {
	return {
		id: crypto.randomUUID(),
		channel_id: 'ch-1',
		guild_id: 'guild-1',
		widget_type: 'notes',
		title: 'Notes',
		config: {},
		creator_id: 'user-1',
		position: 0,
		active: true,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		...overrides
	};
}

describe('channelWidgets store', () => {
	beforeEach(() => {
		channelWidgetsByChannel.set(new Map());
		clientConfig.set({ feature_flags: { widgets: { enabled: true } } } as any);
		vi.mocked(api.getChannelWidgets).mockReset();
	});

	it('loads and sorts channel widgets', async () => {
		const later = createWidget({ id: 'later', position: 2 });
		const sooner = createWidget({ id: 'sooner', position: 1 });
		vi.mocked(api.getChannelWidgets).mockResolvedValue([later, sooner]);

		await loadChannelWidgets('ch-1');

		expect(vi.mocked(api.getChannelWidgets)).toHaveBeenCalledWith('ch-1');
		expect(get(channelWidgetsByChannel).get('ch-1')?.map((widget) => widget.id)).toEqual([
			'sooner',
			'later'
		]);
	});

	it('upserts and removes widgets', () => {
		const widget = createWidget({ id: 'widget-1', title: 'Original' });
		upsertChannelWidget(widget);
		upsertChannelWidget({ ...widget, title: 'Updated' });

		expect(get(channelWidgetsByChannel).get('ch-1')).toHaveLength(1);
		expect(get(channelWidgetsByChannel).get('ch-1')?.[0].title).toBe('Updated');

		removeChannelWidget('ch-1', 'widget-1');
		expect(get(channelWidgetsByChannel).get('ch-1')).toEqual([]);
	});
});
