// Voice state store — manages LiveKit connection, participants, and self mute/deafen state.

import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api/client';
import {
	Room,
	RoomEvent,
	Track,
	ConnectionState,
	type RemoteParticipant,
	type LocalParticipant,
	type RemoteTrackPublication,
	type Participant
} from 'livekit-client';

export interface VoiceParticipant {
	userId: string;
	username: string;
	displayName: string | null;
	avatarId: string | null;
	muted: boolean;
	deafened: boolean;
	speaking: boolean;
}

export type VoiceConnectionState = 'disconnected' | 'connecting' | 'connected';

// Stores
export const voiceChannelId = writable<string | null>(null);
export const voiceGuildId = writable<string | null>(null);
export const voiceChannelName = writable<string | null>(null);
export const voiceState = writable<VoiceConnectionState>('disconnected');
export const selfMute = writable(false);
export const selfDeaf = writable(false);
export const voiceParticipants = writable<Map<string, VoiceParticipant>>(new Map());

// Derived
export const isVoiceConnected = derived(voiceState, ($s) => $s === 'connected');
export const isVoiceConnecting = derived(voiceState, ($s) => $s === 'connecting');
export const participantList = derived(voiceParticipants, ($p) => Array.from($p.values()));
export const participantCount = derived(voiceParticipants, ($p) => $p.size);

// LiveKit Room instance (not reactive — only accessed via functions)
let room: Room | null = null;

export function getRoom(): Room | null {
	return room;
}

export async function joinVoice(channelId: string, guildId: string, channelName: string) {
	// Already connected to this channel
	if (get(voiceChannelId) === channelId && get(voiceState) === 'connected') return;

	// Disconnect from any existing channel first
	if (get(voiceState) !== 'disconnected') {
		await leaveVoice();
	}

	voiceState.set('connecting');
	voiceChannelId.set(channelId);
	voiceGuildId.set(guildId);
	voiceChannelName.set(channelName);
	selfMute.set(false);
	selfDeaf.set(false);
	voiceParticipants.set(new Map());

	try {
		// Get LiveKit token from backend
		const { token, url } = await api.joinVoice(channelId);

		// Create and connect LiveKit room
		room = new Room({
			adaptiveStream: true,
			dynacast: true
		});

		// Wire up room events
		room.on(RoomEvent.ParticipantConnected, handleParticipantConnected);
		room.on(RoomEvent.ParticipantDisconnected, handleParticipantDisconnected);
		room.on(RoomEvent.TrackSubscribed, handleTrackSubscribed);
		room.on(RoomEvent.ActiveSpeakersChanged, handleActiveSpeakersChanged);
		room.on(RoomEvent.Disconnected, handleDisconnected);
		room.on(RoomEvent.ConnectionStateChanged, handleConnectionStateChanged);

		await room.connect(url, token);

		// Enable microphone
		await room.localParticipant.setMicrophoneEnabled(true);

		// Add self to participants
		addLocalParticipant(room.localParticipant);

		// Add existing remote participants
		for (const participant of room.remoteParticipants.values()) {
			addRemoteParticipant(participant);
		}

		voiceState.set('connected');
	} catch (err) {
		console.error('[Voice] Failed to join:', err);
		cleanup();
		throw err;
	}
}

export async function leaveVoice() {
	const channelId = get(voiceChannelId);
	if (channelId) {
		try {
			await api.leaveVoice(channelId);
		} catch {
			// Best-effort — server will clean up eventually
		}
	}
	cleanup();
}

export function toggleMute() {
	const muted = !get(selfMute);
	selfMute.set(muted);

	if (room?.localParticipant) {
		room.localParticipant.setMicrophoneEnabled(!muted);
	}

	// Update self in participants map
	updateSelfInParticipants();
}

export function toggleDeafen() {
	const deafened = !get(selfDeaf);
	selfDeaf.set(deafened);

	if (room) {
		// Mute all incoming audio when deafened
		for (const participant of room.remoteParticipants.values()) {
			for (const pub of participant.trackPublications.values()) {
				if (pub.track && pub.track.kind === Track.Kind.Audio) {
					if (deafened) {
						(pub.track as any).mediaStreamTrack?.enabled && ((pub.track as any).mediaStreamTrack.enabled = false);
					} else {
						(pub.track as any).mediaStreamTrack && ((pub.track as any).mediaStreamTrack.enabled = true);
					}
				}
			}
		}

		// Also mute mic when deafening
		if (deafened && !get(selfMute)) {
			selfMute.set(true);
			room.localParticipant.setMicrophoneEnabled(false);
		}
	}

	updateSelfInParticipants();
}

// Update the voice participants map when we receive a VOICE_STATE_UPDATE from the gateway.
export function handleVoiceStateUpdate(data: {
	channel_id: string;
	user_id: string;
	username?: string;
	display_name?: string | null;
	avatar_id?: string | null;
	muted?: boolean;
	deafened?: boolean;
	action?: 'join' | 'leave' | 'update';
}) {
	const currentChannelId = get(voiceChannelId);

	if (data.action === 'leave') {
		// Remove participant from this channel's state
		if (data.channel_id === currentChannelId) {
			voiceParticipants.update((map) => {
				const next = new Map(map);
				next.delete(data.user_id);
				return next;
			});
		}
		// Also update the channel participants map for sidebar display
		updateChannelVoiceParticipants(data.channel_id, data.user_id, 'remove');
		return;
	}

	// Join or update
	if (data.action === 'join' || data.action === 'update') {
		const participant: VoiceParticipant = {
			userId: data.user_id,
			username: data.username ?? 'Unknown',
			displayName: data.display_name ?? null,
			avatarId: data.avatar_id ?? null,
			muted: data.muted ?? false,
			deafened: data.deafened ?? false,
			speaking: false
		};

		if (data.channel_id === currentChannelId) {
			voiceParticipants.update((map) => {
				const next = new Map(map);
				next.set(data.user_id, participant);
				return next;
			});
		}

		if (data.action === 'join') {
			updateChannelVoiceParticipants(data.channel_id, data.user_id, 'add', participant);
		}
	}
}

// --- Channel-level voice participants (for sidebar display) ---

// Map of channelId → Map<userId, VoiceParticipant>
export const channelVoiceUsers = writable<Map<string, Map<string, VoiceParticipant>>>(new Map());

function updateChannelVoiceParticipants(
	channelId: string,
	userId: string,
	action: 'add' | 'remove',
	participant?: VoiceParticipant
) {
	channelVoiceUsers.update((outer) => {
		const next = new Map(outer);
		const inner = new Map(next.get(channelId) ?? new Map());

		if (action === 'add' && participant) {
			inner.set(userId, participant);
		} else if (action === 'remove') {
			inner.delete(userId);
		}

		if (inner.size === 0) {
			next.delete(channelId);
		} else {
			next.set(channelId, inner);
		}

		return next;
	});
}

// --- Internal helpers ---

function cleanup() {
	if (room) {
		room.removeAllListeners();
		room.disconnect();
		room = null;
	}
	voiceState.set('disconnected');
	voiceChannelId.set(null);
	voiceGuildId.set(null);
	voiceChannelName.set(null);
	selfMute.set(false);
	selfDeaf.set(false);
	voiceParticipants.set(new Map());
}

function addLocalParticipant(participant: LocalParticipant) {
	const metadata = parseMetadata(participant.metadata);
	voiceParticipants.update((map) => {
		const next = new Map(map);
		next.set(metadata.userId ?? participant.identity, {
			userId: metadata.userId ?? participant.identity,
			username: metadata.username ?? participant.identity,
			displayName: metadata.displayName ?? null,
			avatarId: metadata.avatarId ?? null,
			muted: !participant.isMicrophoneEnabled,
			deafened: false,
			speaking: participant.isSpeaking
		});
		return next;
	});
}

function addRemoteParticipant(participant: RemoteParticipant) {
	const metadata = parseMetadata(participant.metadata);
	voiceParticipants.update((map) => {
		const next = new Map(map);
		next.set(metadata.userId ?? participant.identity, {
			userId: metadata.userId ?? participant.identity,
			username: metadata.username ?? participant.identity,
			displayName: metadata.displayName ?? null,
			avatarId: metadata.avatarId ?? null,
			muted: !participant.isMicrophoneEnabled,
			deafened: false,
			speaking: participant.isSpeaking
		});
		return next;
	});
}

function parseMetadata(metadata: string | undefined): {
	userId?: string;
	username?: string;
	displayName?: string;
	avatarId?: string;
} {
	if (!metadata) return {};
	try {
		return JSON.parse(metadata);
	} catch {
		return {};
	}
}

function updateSelfInParticipants() {
	if (!room) return;
	const identity = room.localParticipant.identity;
	const metadata = parseMetadata(room.localParticipant.metadata);
	const userId = metadata.userId ?? identity;

	voiceParticipants.update((map) => {
		const existing = map.get(userId);
		if (!existing) return map;
		const next = new Map(map);
		next.set(userId, {
			...existing,
			muted: get(selfMute),
			deafened: get(selfDeaf)
		});
		return next;
	});
}

// --- LiveKit event handlers ---

function handleParticipantConnected(participant: RemoteParticipant) {
	addRemoteParticipant(participant);
}

function handleParticipantDisconnected(participant: RemoteParticipant) {
	const metadata = parseMetadata(participant.metadata);
	const userId = metadata.userId ?? participant.identity;
	voiceParticipants.update((map) => {
		const next = new Map(map);
		next.delete(userId);
		return next;
	});
}

function handleTrackSubscribed(
	_track: any,
	_publication: RemoteTrackPublication,
	participant: RemoteParticipant
) {
	// When we subscribe to a track, update the participant's mute state
	const metadata = parseMetadata(participant.metadata);
	const userId = metadata.userId ?? participant.identity;
	voiceParticipants.update((map) => {
		const existing = map.get(userId);
		if (!existing) return map;
		const next = new Map(map);
		next.set(userId, {
			...existing,
			muted: !participant.isMicrophoneEnabled
		});
		return next;
	});
}

function handleActiveSpeakersChanged(speakers: Participant[]) {
	const speakerIds = new Set(
		speakers.map((s) => {
			const metadata = parseMetadata(s.metadata);
			return metadata.userId ?? s.identity;
		})
	);

	voiceParticipants.update((map) => {
		const next = new Map(map);
		for (const [id, p] of next) {
			const isSpeaking = speakerIds.has(id);
			if (p.speaking !== isSpeaking) {
				next.set(id, { ...p, speaking: isSpeaking });
			}
		}
		return next;
	});
}

function handleDisconnected() {
	cleanup();
}

function handleConnectionStateChanged(state: ConnectionState) {
	if (state === ConnectionState.Disconnected) {
		cleanup();
	}
}
