import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

// Mock the API client before importing the store.
vi.mock('$lib/api/client', () => ({
	api: {
		getMyGuilds: vi.fn()
	}
}));

import {
	guilds,
	currentGuildId,
	guildList,
	currentGuild,
	setGuild,
	updateGuild,
	removeGuild,
	loadGuilds,
	federatedGuilds,
	federatedGuildIds,
	isFederatedGuild,
	loadFederatedGuilds,
	removeFederatedGuild
} from '../guilds';
import { api } from '$lib/api/client';
import type { Guild, FederatedGuild } from '$lib/types';

function createMockGuild(overrides?: Partial<Guild>): Guild {
	return {
		id: crypto.randomUUID(),
		instance_id: 'instance-1',
		owner_id: 'user-1',
		name: 'Test Guild',
		description: null,
		icon_id: null,
		banner_id: null,
		default_permissions: 0,
		flags: 0,
		nsfw: false,
		discoverable: false,
		preferred_locale: 'en-US',
		max_members: 1000,
		vanity_url: null,
		verification_level: 0,
		tags: [],
		member_count: 1,
		created_at: new Date().toISOString(),
		...overrides
	};
}

describe('guilds store', () => {
	beforeEach(() => {
		guilds.set(new Map());
		currentGuildId.set(null);
	});

	it('starts with empty guild map', () => {
		expect(get(guilds).size).toBe(0);
		expect(get(guildList)).toEqual([]);
	});

	it('updateGuild adds a guild', () => {
		const guild = createMockGuild({ id: 'guild-1', name: 'My Server' });
		updateGuild(guild);

		const map = get(guilds);
		expect(map.size).toBe(1);
		expect(map.get('guild-1')?.name).toBe('My Server');
	});

	it('updateGuild modifies an existing guild', () => {
		const guild = createMockGuild({ id: 'guild-1', name: 'My Server' });
		updateGuild(guild);

		const updated = { ...guild, name: 'Renamed Server' };
		updateGuild(updated);

		const map = get(guilds);
		expect(map.size).toBe(1);
		expect(map.get('guild-1')?.name).toBe('Renamed Server');
	});

	it('removeGuild removes a guild', () => {
		const g1 = createMockGuild({ id: 'guild-1', name: 'First' });
		const g2 = createMockGuild({ id: 'guild-2', name: 'Second' });
		updateGuild(g1);
		updateGuild(g2);

		removeGuild('guild-1');

		const map = get(guilds);
		expect(map.size).toBe(1);
		expect(map.has('guild-1')).toBe(false);
		expect(map.has('guild-2')).toBe(true);
	});

	it('removeGuild on non-existent guild does nothing', () => {
		const guild = createMockGuild({ id: 'guild-1' });
		updateGuild(guild);

		removeGuild('guild-nonexistent');

		expect(get(guilds).size).toBe(1);
	});

	it('guildList derived store returns array of all guilds', () => {
		const g1 = createMockGuild({ id: 'guild-1', name: 'Alpha' });
		const g2 = createMockGuild({ id: 'guild-2', name: 'Beta' });
		updateGuild(g1);
		updateGuild(g2);

		const list = get(guildList);
		expect(list).toHaveLength(2);
		expect(list.map((g) => g.name)).toContain('Alpha');
		expect(list.map((g) => g.name)).toContain('Beta');
	});

	it('setGuild sets the current guild id', () => {
		setGuild('guild-1');
		expect(get(currentGuildId)).toBe('guild-1');
	});

	it('setGuild with null clears the current guild', () => {
		setGuild('guild-1');
		setGuild(null);
		expect(get(currentGuildId)).toBeNull();
	});

	it('currentGuild derived store returns the selected guild', () => {
		const guild = createMockGuild({ id: 'guild-1', name: 'My Server' });
		updateGuild(guild);
		setGuild('guild-1');

		expect(get(currentGuild)?.name).toBe('My Server');
	});

	it('currentGuild returns null when no guild is selected', () => {
		expect(get(currentGuild)).toBeNull();
	});

	it('currentGuild returns null when selected guild does not exist', () => {
		setGuild('guild-nonexistent');
		expect(get(currentGuild)).toBeNull();
	});

	it('loadGuilds replaces all guilds from the API', async () => {
		// Pre-populate with an existing guild.
		const existing = createMockGuild({ id: 'guild-old', name: 'Old Guild' });
		updateGuild(existing);

		// Mock API to return new guilds.
		const apiGuilds = [
			createMockGuild({ id: 'guild-new-1', name: 'New Alpha' }),
			createMockGuild({ id: 'guild-new-2', name: 'New Beta' })
		];
		vi.mocked(api.getMyGuilds).mockResolvedValue(apiGuilds);

		await loadGuilds();

		const map = get(guilds);
		expect(map.size).toBe(2);
		expect(map.has('guild-old')).toBe(false);
		expect(map.has('guild-new-1')).toBe(true);
		expect(map.has('guild-new-2')).toBe(true);
		expect(map.get('guild-new-1')?.name).toBe('New Alpha');
	});
});

function createMockFederatedGuild(overrides?: Partial<FederatedGuild>): FederatedGuild {
	return {
		guild_id: 'fed-guild-1',
		name: 'Remote Guild',
		icon_id: null,
		description: null,
		member_count: 50,
		channels_json: [],
		roles_json: [],
		instance_domain: 'remote.example.com',
		...overrides
	};
}

describe('federated guilds store', () => {
	beforeEach(() => {
		federatedGuilds.set(new Map());
	});

	it('starts with empty federated guild map', () => {
		expect(get(federatedGuilds).size).toBe(0);
	});

	it('loadFederatedGuilds populates the store', () => {
		const fg1 = createMockFederatedGuild({ guild_id: 'fg-1', name: 'Remote Alpha' });
		const fg2 = createMockFederatedGuild({ guild_id: 'fg-2', name: 'Remote Beta', instance_domain: 'other.net' });

		loadFederatedGuilds([fg1, fg2]);

		const map = get(federatedGuilds);
		expect(map.size).toBe(2);
		expect(map.get('fg-1')?.name).toBe('Remote Alpha');
		expect(map.get('fg-2')?.instance_domain).toBe('other.net');
	});

	it('loadFederatedGuilds replaces previous data', () => {
		loadFederatedGuilds([createMockFederatedGuild({ guild_id: 'fg-old' })]);
		expect(get(federatedGuilds).has('fg-old')).toBe(true);

		loadFederatedGuilds([createMockFederatedGuild({ guild_id: 'fg-new' })]);
		expect(get(federatedGuilds).has('fg-old')).toBe(false);
		expect(get(federatedGuilds).has('fg-new')).toBe(true);
	});

	it('federatedGuildIds derived store returns Set of IDs', () => {
		loadFederatedGuilds([
			createMockFederatedGuild({ guild_id: 'fg-1' }),
			createMockFederatedGuild({ guild_id: 'fg-2' })
		]);

		const ids = get(federatedGuildIds);
		expect(ids.size).toBe(2);
		expect(ids.has('fg-1')).toBe(true);
		expect(ids.has('fg-2')).toBe(true);
		expect(ids.has('fg-3')).toBe(false);
	});

	it('isFederatedGuild returns true for federated guilds', () => {
		loadFederatedGuilds([createMockFederatedGuild({ guild_id: 'fg-1' })]);

		expect(isFederatedGuild('fg-1')).toBe(true);
		expect(isFederatedGuild('local-guild')).toBe(false);
	});

	it('removeFederatedGuild removes a federated guild', () => {
		loadFederatedGuilds([
			createMockFederatedGuild({ guild_id: 'fg-1' }),
			createMockFederatedGuild({ guild_id: 'fg-2' })
		]);

		removeFederatedGuild('fg-1');

		const map = get(federatedGuilds);
		expect(map.size).toBe(1);
		expect(map.has('fg-1')).toBe(false);
		expect(map.has('fg-2')).toBe(true);
	});

	it('removeFederatedGuild on non-existent guild does nothing', () => {
		loadFederatedGuilds([createMockFederatedGuild({ guild_id: 'fg-1' })]);

		removeFederatedGuild('nonexistent');

		expect(get(federatedGuilds).size).toBe(1);
	});

	it('loadFederatedGuilds with empty array clears the store', () => {
		loadFederatedGuilds([createMockFederatedGuild({ guild_id: 'fg-1' })]);
		expect(get(federatedGuilds).size).toBe(1);

		loadFederatedGuilds([]);
		expect(get(federatedGuilds).size).toBe(0);
	});
});
