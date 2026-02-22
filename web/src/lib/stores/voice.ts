// Voice state store — manages LiveKit connection, participants, and self mute/deafen state.

import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api/client';
import { notificationSoundsEnabled, notificationVolume, isDndActive } from './settings';
import { playNotificationSound } from '$lib/utils/sounds';
import {
	Room,
	RoomEvent,
	Track,
	ConnectionState,
	VideoPresets,
	ScreenSharePresets,
	type RemoteParticipant,
	type RemoteTrack,
	type LocalParticipant,
	type LocalTrackPublication,
	type RemoteTrackPublication,
	type Participant
} from 'livekit-client';
import { routeAudioThroughNoiseFilter, cleanupNoiseFilter, cleanupAllNoiseFilters } from '$lib/utils/noiseReduction';
import { routeAudioThroughGain, cleanupUserAudio, cleanupAllAudio, getAudioLevel, computeRmsLevel } from '$lib/utils/voiceVolume';

export interface VoiceParticipant {
	userId: string;
	username: string;
	displayName: string | null;
	avatarId: string | null;
	instanceId: string | null;
	muted: boolean;
	deafened: boolean;
	speaking: boolean;
}

export interface VideoTrackInfo {
	trackSid: string;
	userId: string;
	source: 'camera' | 'screenshare';
	videoElement: HTMLVideoElement;
	participantIdentity: string;
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

// Multi-track video: holds all active video tracks (cameras + screen shares).
export const videoTracks = writable<Map<string, VideoTrackInfo>>(new Map());
export const selfCamera = writable(false);

// Derived
export const videoTrackList = derived(videoTracks, ($t) => Array.from($t.values()));
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
		// Get LiveKit token from backend.
		// If the channel belongs to a federated guild, the backend returns a redirect
		// with { federated: true, guild_id, channel_id } and we use the guild-join proxy.
		let token: string;
		let url: string;
		const joinResp = await api.joinVoice(channelId) as Record<string, unknown>;
		if (joinResp.federated) {
			const fedResp = await api.joinFederatedVoiceByGuild(
				joinResp.guild_id as string,
				joinResp.channel_id as string
			);
			token = fedResp.token;
			url = fedResp.url;
		} else {
			token = joinResp.token as string;
			url = joinResp.url as string;
		}

		// Load saved device preferences from localStorage.
		const savedInputDevice = localStorage.getItem('av-voice-input-device') || undefined;
		const savedOutputDevice = localStorage.getItem('av-voice-output-device') || undefined;

		// Load voice preferences for audio processing settings.
		let noiseSuppression = true;
		let echoCancellation = true;
		let autoGainControl = true;
		try {
			const prefs = await api.getVoicePreferences();
			noiseSuppression = prefs.noise_suppression ?? true;
			echoCancellation = prefs.echo_cancellation ?? true;
			autoGainControl = prefs.auto_gain_control ?? true;
		} catch {
			// Use defaults on error.
		}

		// Create and connect LiveKit room
		room = new Room({
			adaptiveStream: true,
			dynacast: true,
			audioCaptureDefaults: {
				deviceId: savedInputDevice,
				noiseSuppression,
				echoCancellation,
				autoGainControl
			},
			audioOutput: {
				deviceId: savedOutputDevice
			},
			videoCaptureDefaults: {
				resolution: VideoPresets.h720.resolution
			},
			publishDefaults: {
				videoEncoding: { maxBitrate: 2_000_000, maxFramerate: 30 },
				screenShareEncoding: ScreenSharePresets.h1080fps30.encoding,
				simulcast: true,
				videoSimulcastLayers: [VideoPresets.h360, VideoPresets.h180]
			}
		});

		// Wire up room events
		room.on(RoomEvent.ParticipantConnected, handleParticipantConnected);
		room.on(RoomEvent.ParticipantDisconnected, handleParticipantDisconnected);
		room.on(RoomEvent.TrackSubscribed, handleTrackSubscribed);
		room.on(RoomEvent.TrackUnsubscribed, handleTrackUnsubscribed);
		room.on(RoomEvent.ActiveSpeakersChanged, handleActiveSpeakersChanged);
		room.on(RoomEvent.LocalTrackPublished, handleLocalTrackPublished);
		room.on(RoomEvent.LocalTrackUnpublished, handleLocalTrackUnpublished);
		room.on(RoomEvent.Disconnected, handleDisconnected);
		room.on(RoomEvent.ConnectionStateChanged, handleConnectionStateChanged);

		await room.connect(url, token);

		// Enable microphone
		await room.localParticipant.setMicrophoneEnabled(true);

		// Set up local audio level monitoring for instant speaking detection.
		setupLocalAudioMonitor();

		// Add self to participants
		addLocalParticipant(room.localParticipant);

		// Add existing remote participants
		for (const participant of room.remoteParticipants.values()) {
			addRemoteParticipant(participant);
		}

		voiceState.set('connected');

		// Start audio level polling for responsive speaking indicators.
		startAudioLevelPolling();
		playVoiceSound('voice-join');
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
	playVoiceSound('voice-leave');
	cleanup();
}

export function toggleMute() {
	const muted = !get(selfMute);
	selfMute.set(muted);

	if (room?.localParticipant) {
		room.localParticipant.setMicrophoneEnabled(!muted);
		// Refresh local audio monitor when unmuting (track may change).
		if (!muted) {
			setTimeout(() => setupLocalAudioMonitor(), 100);
		}
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

export async function toggleCamera() {
	if (!room) return;
	const enabled = !get(selfCamera);
	selfCamera.set(enabled);
	try {
		await room.localParticipant.setCameraEnabled(enabled, {
			resolution: { width: 1280, height: 720, frameRate: 30 },
			facingMode: 'user'
		});
	} catch (err) {
		console.error('[Voice] Failed to toggle camera:', err);
		selfCamera.set(!enabled);
	}
}

// Update the voice participants map when we receive a VOICE_STATE_UPDATE from the gateway.
export function handleVoiceStateUpdate(data: {
	channel_id: string;
	user_id: string;
	username?: string;
	display_name?: string | null;
	avatar_id?: string | null;
	instance_id?: string | null;
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
			instanceId: data.instance_id ?? null,
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

/** Clear all channel voice users — call on gateway READY to flush stale state. */
export function clearChannelVoiceUsers() {
	channelVoiceUsers.set(new Map());
}

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

// --- Audio level monitoring for instant speaking detection ---

const SPEAKING_THRESHOLD = 0.015;
const SPEAKING_TIMEOUT_MS = 200;
let audioLevelFrameId: number | null = null;
let localAnalyserCtx: AudioContext | null = null;
let localAnalyserSource: MediaStreamAudioSourceNode | null = null;
let localAnalyser: AnalyserNode | null = null;
const lastSpeakingTime = new Map<string, number>();

function getLocalAudioLevel(): number {
	if (!localAnalyser) return 0;
	return computeRmsLevel(localAnalyser);
}

function setupLocalAudioMonitor() {
	if (!room) return;
	cleanupLocalAudioMonitor();

	const localPub = room.localParticipant.getTrackPublication(Track.Source.Microphone);
	const mediaStreamTrack = localPub?.track?.mediaStreamTrack;
	if (!mediaStreamTrack) return;

	try {
		localAnalyserCtx = new AudioContext();
		const stream = new MediaStream([mediaStreamTrack]);
		localAnalyserSource = localAnalyserCtx.createMediaStreamSource(stream);
		localAnalyser = localAnalyserCtx.createAnalyser();
		localAnalyser.fftSize = 256;
		localAnalyserSource.connect(localAnalyser);
	} catch {
		cleanupLocalAudioMonitor();
	}
}

function cleanupLocalAudioMonitor() {
	try {
		localAnalyserSource?.disconnect();
		localAnalyser?.disconnect();
		localAnalyserCtx?.close();
	} catch { /* ignore */ }
	localAnalyserSource = null;
	localAnalyser = null;
	localAnalyserCtx = null;
}

function startAudioLevelPolling() {
	if (audioLevelFrameId !== null) return;
	lastSpeakingTime.clear();
	const POLL_INTERVAL_MS = 50; // ~20fps — sufficient for speaking indicators
	let lastPollTime = 0;

	function poll() {
		if (!room) {
			audioLevelFrameId = null;
			return;
		}

		const now = Date.now();

		// Throttle processing to avoid per-frame overhead with many participants.
		if (now - lastPollTime < POLL_INTERVAL_MS) {
			audioLevelFrameId = requestAnimationFrame(poll);
			return;
		}
		lastPollTime = now;
		const updates: { userId: string; speaking: boolean }[] = [];
		const participants = get(voiceParticipants);

		// Check local participant audio level.
		const localMeta = parseMetadata(room.localParticipant.metadata);
		const localUserId = localMeta.userId ?? room.localParticipant.identity;
		const localLevel = getLocalAudioLevel();
		if (localLevel > SPEAKING_THRESHOLD) {
			lastSpeakingTime.set(localUserId, now);
			const p = participants.get(localUserId);
			if (p && !p.speaking) updates.push({ userId: localUserId, speaking: true });
		} else {
			const lastTime = lastSpeakingTime.get(localUserId);
			if (lastTime && now - lastTime > SPEAKING_TIMEOUT_MS) {
				const p = participants.get(localUserId);
				if (p?.speaking) updates.push({ userId: localUserId, speaking: false });
			}
		}

		// Check remote participant audio levels.
		for (const [userId, p] of participants) {
			if (userId === localUserId) continue;
			const level = getAudioLevel(userId);
			if (level > SPEAKING_THRESHOLD) {
				lastSpeakingTime.set(userId, now);
				if (!p.speaking) updates.push({ userId, speaking: true });
			} else {
				const lastTime = lastSpeakingTime.get(userId);
				if (lastTime && now - lastTime > SPEAKING_TIMEOUT_MS) {
					if (p.speaking) updates.push({ userId, speaking: false });
				}
			}
		}

		// Batch-update the store.
		if (updates.length > 0) {
			voiceParticipants.update((map) => {
				const next = new Map(map);
				for (const { userId, speaking } of updates) {
					const existing = next.get(userId);
					if (existing && existing.speaking !== speaking) {
						next.set(userId, { ...existing, speaking });
					}
				}
				return next;
			});

			// Also sync to sidebar channelVoiceUsers store.
			const currentChannel = get(voiceChannelId);
			if (currentChannel) {
				channelVoiceUsers.update((outer) => {
					const channelMap = outer.get(currentChannel);
					if (!channelMap) return outer;
					let changed = false;
					const nextInner = new Map(channelMap);
					for (const { userId, speaking } of updates) {
						const p = nextInner.get(userId);
						if (p && p.speaking !== speaking) {
							nextInner.set(userId, { ...p, speaking });
							changed = true;
						}
					}
					if (!changed) return outer;
					const nextOuter = new Map(outer);
					nextOuter.set(currentChannel, nextInner);
					return nextOuter;
				});
			}
		}

		audioLevelFrameId = requestAnimationFrame(poll);
	}

	audioLevelFrameId = requestAnimationFrame(poll);
}

function stopAudioLevelPolling() {
	if (audioLevelFrameId !== null) {
		cancelAnimationFrame(audioLevelFrameId);
		audioLevelFrameId = null;
	}
	lastSpeakingTime.clear();
	cleanupLocalAudioMonitor();
}

// --- Internal helpers ---

function playVoiceSound(preset: 'voice-join' | 'voice-leave') {
	if (!get(isDndActive) && get(notificationSoundsEnabled)) {
		playNotificationSound(preset, get(notificationVolume));
	}
}

function cleanup() {
	// Stop audio level polling before disconnecting.
	stopAudioLevelPolling();
	if (room) {
		room.removeAllListeners();
		room.disconnect();
		room = null;
	}
	// Clean up noise reduction filter nodes and per-user audio gain nodes.
	cleanupAllNoiseFilters();
	cleanupAllAudio();
	// Remove all attached audio elements.
	if (audioContainer) {
		audioContainer.remove();
		audioContainer = null;
	}
	// Clean up all video track elements.
	const tracks = get(videoTracks);
	for (const info of tracks.values()) {
		info.videoElement.remove();
	}
	videoTracks.set(new Map());
	selfCamera.set(false);

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
			instanceId: metadata.instanceId ?? null,
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
			instanceId: metadata.instanceId ?? null,
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
	instanceId?: string;
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
	playVoiceSound('voice-join');
}

function handleParticipantDisconnected(participant: RemoteParticipant) {
	const metadata = parseMetadata(participant.metadata);
	const userId = metadata.userId ?? participant.identity;
	voiceParticipants.update((map) => {
		const next = new Map(map);
		next.delete(userId);
		return next;
	});
	playVoiceSound('voice-leave');
}

// Container for remote audio elements so they play through speakers.
let audioContainer: HTMLDivElement | null = null;

function getAudioContainer(): HTMLDivElement {
	if (!audioContainer) {
		audioContainer = document.createElement('div');
		audioContainer.id = 'livekit-audio';
		audioContainer.style.display = 'none';
		document.body.appendChild(audioContainer);
	}
	return audioContainer;
}

function handleTrackSubscribed(
	track: RemoteTrack,
	publication: RemoteTrackPublication,
	participant: RemoteParticipant
) {
	const metadata = parseMetadata(participant.metadata);
	const userId = metadata.userId ?? participant.identity;

	// Attach audio tracks to the DOM, routed through noise filter then per-user volume gain.
	if (track.kind === Track.Kind.Audio) {
		const rawEl = track.attach();
		rawEl.id = `audio-${participant.identity}`;
		const filteredEl = routeAudioThroughNoiseFilter(userId, rawEl);
		const outputEl = routeAudioThroughGain(userId, filteredEl);
		getAudioContainer().appendChild(outputEl);
	}

	// Attach video tracks (camera + screen share) to the videoTracks store.
	if (track.kind === Track.Kind.Video) {
		const isScreenShare = publication.source === Track.Source.ScreenShare;
		const videoEl = track.attach() as HTMLVideoElement;
		videoEl.autoplay = true;
		videoEl.playsInline = true;
		videoEl.muted = true; // Audio comes via separate audio track
		const info: VideoTrackInfo = {
			trackSid: track.sid,
			userId,
			source: isScreenShare ? 'screenshare' : 'camera',
			videoElement: videoEl,
			participantIdentity: participant.identity
		};
		videoTracks.update((map) => {
			const next = new Map(map);
			next.set(track.sid, info);
			return next;
		});
	}

	// Update the participant's mute state.
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

function handleTrackUnsubscribed(
	track: RemoteTrack,
	_publication: RemoteTrackPublication,
	participant: RemoteParticipant
) {
	// Remove video track from store.
	if (track.kind === Track.Kind.Video) {
		videoTracks.update((map) => {
			const next = new Map(map);
			next.delete(track.sid);
			return next;
		});
	}
	// Clean up noise reduction and volume gain nodes for audio tracks.
	if (track.kind === Track.Kind.Audio) {
		const metadata = parseMetadata(participant.metadata);
		const userId = metadata.userId ?? participant.identity;
		cleanupNoiseFilter(userId);
		cleanupUserAudio(userId);
	}
	// Detach all media elements for this track.
	track.detach().forEach((el) => el.remove());
}

function handleLocalTrackPublished(publication: LocalTrackPublication, participant: LocalParticipant) {
	const track = publication.track;
	if (!track || track.kind !== Track.Kind.Video) return;

	const metadata = parseMetadata(participant.metadata);
	const userId = metadata.userId ?? participant.identity;
	const isScreenShare = publication.source === Track.Source.ScreenShare;

	const videoEl = track.attach() as HTMLVideoElement;
	videoEl.autoplay = true;
	videoEl.playsInline = true;
	videoEl.muted = true; // Audio comes via separate audio track

	const info: VideoTrackInfo = {
		trackSid: track.sid,
		userId,
		source: isScreenShare ? 'screenshare' : 'camera',
		videoElement: videoEl,
		participantIdentity: participant.identity
	};
	videoTracks.update((map) => {
		const next = new Map(map);
		next.set(track.sid, info);
		return next;
	});
}

function handleLocalTrackUnpublished(publication: LocalTrackPublication, _participant: LocalParticipant) {
	const track = publication.track;
	if (!track || track.kind !== Track.Kind.Video) return;

	videoTracks.update((map) => {
		const next = new Map(map);
		next.delete(track.sid);
		return next;
	});
	track.detach().forEach((el) => el.remove());
}

// Speaking detection is handled by the audio level polling loop (startAudioLevelPolling)
// which monitors actual audio send/receive via Web Audio API AnalyserNodes.
// This is a no-op kept for the LiveKit event subscription.
function handleActiveSpeakersChanged(_speakers: Participant[]) {
	// Intentionally empty — audio level polling handles speaking state.
}

function handleDisconnected() {
	cleanup();
}

function handleConnectionStateChanged(state: ConnectionState) {
	if (state === ConnectionState.Disconnected) {
		cleanup();
	}
}
