// Muting store — tracks channel and guild mute preferences for notification suppression.

import { derived, get } from 'svelte/store';
import { api } from '$lib/api/client';
import type { ChannelNotificationPreference, NotificationPreference } from '$lib/types';
import { createMapStore } from './mapHelpers';

// Per-channel mute preferences (keyed by channel_id).
const channelMuteMap = createMapStore<string, ChannelNotificationPreference>();

// Per-guild mute preferences (keyed by guild_id).
const guildMuteMap = createMapStore<string, NotificationPreference>();

// Expose derived readonly stores.
export const channelMutePrefs = derived(channelMuteMap, ($m) => $m);
export const guildMutePrefs = derived(guildMuteMap, ($m) => $m);

// Load channel mute preferences from the server.
export async function loadChannelMutePrefs() {
	try {
		const prefs = await api.getChannelNotificationPreferences();
		const map = new Map<string, ChannelNotificationPreference>();
		for (const p of prefs) {
			map.set(p.channel_id, p);
		}
		channelMuteMap.setAll(map);
	} catch {
		// Silently fail — muting is non-critical.
	}
}

// Check if a channel is currently muted.
export function isChannelMuted(channelId: string): boolean {
	const pref = get(channelMuteMap).get(channelId);
	if (!pref) return false;
	if (pref.level === 'none') {
		// If muted_until is set, check if it's expired.
		if (pref.muted_until) {
			return new Date(pref.muted_until) > new Date();
		}
		return true; // Indefinitely muted.
	}
	return false;
}

// Check if a guild is currently muted.
export function isGuildMuted(guildId: string): boolean {
	const pref = get(guildMuteMap).get(guildId);
	if (!pref) return false;
	if (pref.muted_until) {
		return new Date(pref.muted_until) > new Date();
	}
	return pref.level === 'none';
}

// Mute a channel. If durationMs is provided, sets a timed mute; otherwise mutes indefinitely.
export async function muteChannel(channelId: string, durationMs?: number) {
	const mutedUntil = durationMs
		? new Date(Date.now() + durationMs).toISOString()
		: null;

	try {
		const pref = await api.updateChannelNotificationPreference({
			channel_id: channelId,
			level: 'none',
			muted_until: mutedUntil
		});
		channelMuteMap.setEntry(channelId, pref);
	} catch {
		// Silently fail.
	}
}

// Unmute a channel (reset to inherit from guild/global).
export async function unmuteChannel(channelId: string) {
	try {
		await api.deleteChannelNotificationPreference(channelId);
		channelMuteMap.removeEntry(channelId);
	} catch {
		// Silently fail.
	}
}

// Mute a guild. If durationMs is provided, sets a timed mute; otherwise mutes indefinitely.
export async function muteGuild(guildId: string, durationMs?: number) {
	const mutedUntil = durationMs
		? new Date(Date.now() + durationMs).toISOString()
		: null;

	try {
		const pref = await api.updateNotificationPreferences({
			guild_id: guildId,
			level: 'none',
			muted_until: mutedUntil
		});
		guildMuteMap.setEntry(guildId, pref);
	} catch {
		// Silently fail.
	}
}

// Unmute a guild (reset to default).
export async function unmuteGuild(guildId: string) {
	try {
		const pref = await api.updateNotificationPreferences({
			guild_id: guildId,
			level: 'mentions',
			muted_until: null
		});
		guildMuteMap.removeEntry(guildId);
	} catch {
		// Silently fail.
	}
}

// Update the guild mute map from server data (called when loading guild prefs).
export function setGuildMutePref(guildId: string, pref: NotificationPreference) {
	guildMuteMap.setEntry(guildId, pref);
}

// Get all currently muted channels (for settings page display).
export function getMutedChannels(): ChannelNotificationPreference[] {
	const map = get(channelMuteMap);
	const now = new Date();
	const result: ChannelNotificationPreference[] = [];
	for (const pref of map.values()) {
		if (pref.level === 'none') {
			// Include if indefinitely muted or muted_until is in the future.
			if (!pref.muted_until || new Date(pref.muted_until) > now) {
				result.push(pref);
			}
		}
	}
	return result;
}

// Reset stores (for testing only).
export function _resetForTests() {
	channelMuteMap.clear();
	guildMuteMap.clear();
}

// Get all currently muted guilds (for settings page display).
export function getMutedGuilds(): NotificationPreference[] {
	const map = get(guildMuteMap);
	const now = new Date();
	const result: NotificationPreference[] = [];
	for (const pref of map.values()) {
		if (pref.level === 'none') {
			if (!pref.muted_until || new Date(pref.muted_until) > now) {
				result.push(pref);
			}
		}
	}
	return result;
}
