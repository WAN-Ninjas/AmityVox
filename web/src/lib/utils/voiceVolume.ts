/**
 * Per-user voice volume control using Web Audio API GainNodes.
 *
 * Each participant's audio is routed through a GainNode, allowing independent
 * volume adjustment (0–200%). Volumes persist in localStorage.
 */

const STORAGE_KEY = 'av-voice-user-volumes';
const nodes = new Map<string, { gain: GainNode; source: MediaStreamAudioSourceNode; ctx: AudioContext }>();

let cachedVolumes: Record<string, number> | null = null;

function loadVolumes(): Record<string, number> {
	if (cachedVolumes) return cachedVolumes;
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		cachedVolumes = raw ? JSON.parse(raw) : {};
	} catch {
		cachedVolumes = {};
	}
	return cachedVolumes!;
}

function saveVolumes() {
	if (cachedVolumes) {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(cachedVolumes));
	}
}

/** Get the stored volume for a user (default 100 = 100%). */
export function getUserVolume(userId: string): number {
	return loadVolumes()[userId] ?? 100;
}

/** Set and persist the volume for a user (0–200). */
export function setUserVolume(userId: string, volume: number) {
	const vols = loadVolumes();
	const clamped = Math.max(0, Math.min(200, Math.round(volume)));
	if (clamped === 100) {
		delete vols[userId];
	} else {
		vols[userId] = clamped;
	}
	cachedVolumes = vols;
	saveVolumes();

	// Live-update the gain node if one exists.
	const node = nodes.get(userId);
	if (node) {
		node.gain.gain.value = clamped / 100;
	}
}

/**
 * Route an audio element through a GainNode for the given user.
 * Call this right after creating the <audio> element from track.attach().
 * Returns the destination element that should be added to the DOM (or null if
 * Web Audio API is unavailable).
 */
export function routeAudioThroughGain(userId: string, audioElement: HTMLAudioElement): HTMLAudioElement {
	try {
		const ctx = new AudioContext();
		const source = ctx.createMediaStreamSource(audioElement.srcObject as MediaStream);
		const gain = ctx.createGain();
		gain.gain.value = getUserVolume(userId) / 100;
		source.connect(gain);

		// Connect gain to a MediaStreamDestination so we can use a regular <audio> element.
		const dest = ctx.createMediaStreamDestination();
		gain.connect(dest);

		const outputEl = new Audio();
		outputEl.srcObject = dest.stream;
		outputEl.autoplay = true;
		outputEl.id = audioElement.id;

		// Store nodes for live adjustment and cleanup.
		nodes.set(userId, { gain, source, ctx });

		// Mute the original element so we don't hear double audio.
		audioElement.muted = true;
		audioElement.pause();

		return outputEl;
	} catch {
		// Fallback: return original element if Web Audio fails.
		return audioElement;
	}
}

/** Clean up gain nodes for a participant (call on track unsubscribe / disconnect). */
export function cleanupUserAudio(userId: string) {
	const node = nodes.get(userId);
	if (node) {
		try {
			node.source.disconnect();
			node.gain.disconnect();
			node.ctx.close();
		} catch { /* ignore */ }
		nodes.delete(userId);
	}
}

/** Clean up all gain nodes (call on room disconnect). */
export function cleanupAllAudio() {
	for (const [userId] of nodes) {
		cleanupUserAudio(userId);
	}
}
