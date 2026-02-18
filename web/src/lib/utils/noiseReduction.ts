/**
 * Client-side noise reduction for remote audio using Web Audio API BiquadFilters.
 *
 * Applies a high-pass filter (~85Hz cutoff) and a low-shelf reduction to remove
 * background noise (fans, AC, traffic rumble). This is a pragmatic biquad approach,
 * not ML-based, but effective for common low-frequency noise sources.
 *
 * Per-user enable/disable persists in localStorage.
 */

import { createLocalStorageCache } from './localStorageCache';

const nrCache = createLocalStorageCache<boolean>('av-voice-noise-reduction');
const nodes = new Map<string, { highpass: BiquadFilterNode; lowshelf: BiquadFilterNode; source: MediaStreamAudioSourceNode; ctx: AudioContext }>();

/** Check if noise reduction is enabled for a user. */
export function isNoiseReductionEnabled(userId: string): boolean {
	return nrCache.get(userId) ?? false;
}

/** Enable or disable noise reduction for a user. Persists to localStorage. */
export function setNoiseReduction(userId: string, enabled: boolean) {
	if (enabled) {
		nrCache.set(userId, true);
	} else {
		nrCache.remove(userId);
	}

	// Live-update: toggle the filter bypass if nodes exist.
	const node = nodes.get(userId);
	if (node) {
		// When disabled, set highpass frequency to 0 and lowshelf gain to 0 (bypass).
		if (enabled) {
			node.highpass.frequency.value = 85;
			node.lowshelf.gain.value = -6;
		} else {
			node.highpass.frequency.value = 0;
			node.lowshelf.gain.value = 0;
		}
	}
}

/**
 * Route an audio element through noise reduction filters.
 * Call after track.attach() to apply high-pass + low-shelf filtering.
 *
 * Returns the output audio element to be added to the DOM.
 */
export function routeAudioThroughNoiseFilter(userId: string, audioElement: HTMLAudioElement): HTMLAudioElement {
	try {
		const ctx = new AudioContext();
		const source = ctx.createMediaStreamSource(audioElement.srcObject as MediaStream);

		// High-pass filter: removes rumble below ~85Hz (fans, AC, traffic).
		const highpass = ctx.createBiquadFilter();
		highpass.type = 'highpass';
		highpass.frequency.value = isNoiseReductionEnabled(userId) ? 85 : 0;
		highpass.Q.value = 0.7;

		// Low-shelf: reduces low-frequency content by -6dB for cleaner audio.
		const lowshelf = ctx.createBiquadFilter();
		lowshelf.type = 'lowshelf';
		lowshelf.frequency.value = 200;
		lowshelf.gain.value = isNoiseReductionEnabled(userId) ? -6 : 0;

		source.connect(highpass);
		highpass.connect(lowshelf);

		const dest = ctx.createMediaStreamDestination();
		lowshelf.connect(dest);

		const outputEl = new Audio();
		outputEl.srcObject = dest.stream;
		outputEl.autoplay = true;
		outputEl.id = audioElement.id;

		nodes.set(userId, { highpass, lowshelf, source, ctx });

		// Mute original to prevent double audio.
		audioElement.muted = true;
		audioElement.pause();

		return outputEl;
	} catch {
		return audioElement;
	}
}

/** Clean up noise filter nodes for a user. */
export function cleanupNoiseFilter(userId: string) {
	const node = nodes.get(userId);
	if (node) {
		try {
			node.source.disconnect();
			node.highpass.disconnect();
			node.lowshelf.disconnect();
			node.ctx.close();
		} catch { /* ignore */ }
		nodes.delete(userId);
	}
}

/** Clean up all noise filter nodes. */
export function cleanupAllNoiseFilters() {
	for (const [userId] of nodes) {
		cleanupNoiseFilter(userId);
	}
}
