// Multi-instance store -- manages connections to multiple AmityVox instances.
// Allows users to connect to several instances simultaneously and switch between them.
// Instance profiles are persisted server-side and cached in localStorage.

import { writable, derived, get } from 'svelte/store';
import { createMapStore } from './mapHelpers';

export interface InstanceProfile {
	id: string;
	instance_url: string;
	instance_name: string | null;
	instance_icon: string | null;
	is_primary: boolean;
	last_connected: string | null;
	created_at: string;
}

export interface InstanceConnection {
	profile: InstanceProfile;
	token: string | null;
	connected: boolean;
	unreadCount: number;
	mentionCount: number;
	error: string | null;
}

// All known instance profiles from the server.
const instanceProfilesStore = createMapStore<string, InstanceProfile>();

// Active connections keyed by instance URL.
const connectionStore = createMapStore<string, InstanceConnection>();

// Currently selected instance URL.
export const activeInstanceUrl = writable<string | null>(null);

// Exported read-only derived stores.
export const instanceProfiles = derived(instanceProfilesStore, ($map) =>
	Array.from($map.values()).sort((a, b) => {
		if (a.is_primary && !b.is_primary) return -1;
		if (!a.is_primary && b.is_primary) return 1;
		return (b.last_connected || '').localeCompare(a.last_connected || '');
	})
);

export const instanceConnections = derived(connectionStore, ($map) =>
	Array.from($map.values())
);

export const activeInstance = derived(
	[activeInstanceUrl, connectionStore],
	([$url, $connections]) => {
		if (!$url) return null;
		return $connections.get($url) || null;
	}
);

// Total unread count across all non-active instances.
export const crossInstanceUnreadCount = derived(
	[connectionStore, activeInstanceUrl],
	([$connections, $activeUrl]) => {
		let total = 0;
		for (const [url, conn] of $connections) {
			if (url !== $activeUrl) {
				total += conn.unreadCount;
			}
		}
		return total;
	}
);

// Total mention count across all non-active instances.
export const crossInstanceMentionCount = derived(
	[connectionStore, activeInstanceUrl],
	([$connections, $activeUrl]) => {
		let total = 0;
		for (const [url, conn] of $connections) {
			if (url !== $activeUrl) {
				total += conn.mentionCount;
			}
		}
		return total;
	}
);

// ----- Actions -----

// Load instance profiles from localStorage cache.
export function loadInstanceProfilesFromCache(): void {
	try {
		const cached = localStorage.getItem('instance_profiles');
		if (cached) {
			const profiles: InstanceProfile[] = JSON.parse(cached);
			const map = new Map<string, InstanceProfile>();
			for (const p of profiles) {
				map.set(p.instance_url, p);
			}
			instanceProfilesStore.setAll(map);
		}
	} catch {
		// Ignore parse errors.
	}
}

// Set instance profiles from API response.
export function setInstanceProfiles(profiles: InstanceProfile[]): void {
	const map = new Map<string, InstanceProfile>();
	for (const p of profiles) {
		map.set(p.instance_url, p);
	}
	instanceProfilesStore.setAll(map);

	// Cache to localStorage.
	try {
		localStorage.setItem('instance_profiles', JSON.stringify(profiles));
	} catch {
		// Ignore storage errors.
	}
}

// Add or update a single instance profile.
export function upsertInstanceProfile(profile: InstanceProfile): void {
	instanceProfilesStore.setEntry(profile.instance_url, profile);
}

// Remove an instance profile.
export function removeInstanceProfile(instanceUrl: string): void {
	instanceProfilesStore.removeEntry(instanceUrl);
	connectionStore.removeEntry(instanceUrl);
}

// Set the active instance by URL.
export function setActiveInstance(instanceUrl: string): void {
	activeInstanceUrl.set(instanceUrl);
	try {
		localStorage.setItem('active_instance', instanceUrl);
	} catch {
		// Ignore storage errors.
	}
}

// Restore active instance from localStorage.
export function restoreActiveInstance(): void {
	try {
		const url = localStorage.getItem('active_instance');
		if (url) {
			activeInstanceUrl.set(url);
		}
	} catch {
		// Ignore.
	}
}

// Register a connection for an instance.
export function connectInstance(instanceUrl: string, token: string): void {
	const profiles = get(instanceProfilesStore);
	const profile = profiles.get(instanceUrl);

	connectionStore.setEntry(instanceUrl, {
		profile: profile || {
			id: '',
			instance_url: instanceUrl,
			instance_name: null,
			instance_icon: null,
			is_primary: false,
			last_connected: new Date().toISOString(),
			created_at: new Date().toISOString(),
		},
		token,
		connected: true,
		unreadCount: 0,
		mentionCount: 0,
		error: null,
	});
}

// Disconnect an instance.
export function disconnectInstance(instanceUrl: string): void {
	connectionStore.updateEntry(instanceUrl, (conn) => ({ ...conn, connected: false, token: null }));
}

// Update unread counts for an instance.
export function updateInstanceUnreads(instanceUrl: string, unreadCount: number, mentionCount: number): void {
	connectionStore.updateEntry(instanceUrl, (conn) => ({ ...conn, unreadCount, mentionCount }));
}

// Set an error for an instance connection.
export function setInstanceError(instanceUrl: string, error: string): void {
	connectionStore.updateEntry(instanceUrl, (conn) => ({ ...conn, error, connected: false }));
}

// Clear all instance data (for logout).
export function clearInstanceData(): void {
	instanceProfilesStore.clear();
	connectionStore.clear();
	activeInstanceUrl.set(null);
	try {
		localStorage.removeItem('instance_profiles');
		localStorage.removeItem('active_instance');
	} catch {
		// Ignore.
	}
}
